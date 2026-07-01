package cache

import (
	"slices"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Store is the L2 backend the controller talks to. Get returns the value and
// its remaining TTL; ok=false means miss or expired. Implementations must be
// safe for concurrent use (the controller calls them from parallel fetch
// hooks, under the request's DataBuffer.Lock).
type Store interface {
	Get(key string) (value []byte, remainingTTL time.Duration, ok bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Controller is the long-lived cache lifecycle port (implements
// resolve.CacheController): one per integrator/cache instance, holding only
// immutable collaborators. All per-request mutable state lives on the
// requestCache BeginRequest hands out.
//
// Task 07 scope: the L2 entity controller for the single-candidate case.
// Multi-key freshness/reorder/backfill (08), normalization (09), batch (10),
// negative (11), shadow (12), root fields (13), L1 (17) and partial (19)
// extend it.
type Controller struct {
	store Store
	obs   resolve.CacheObserver
}

// NewController builds a controller over an L2 store; obs may be nil (no
// observability).
func NewController(store Store, obs resolve.CacheObserver) *Controller {
	return &Controller{store: store, obs: obs}
}

// BeginRequest hands out the request-lifetime working surface. Called lazily,
// once per request, under DataBuffer.Lock (the loader's cacheRequest).
func (c *Controller) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	if c.obs != nil {
		c.obs.BeginRequest(ctx)
	}
	return &requestCache{
		store:         c.store,
		obs:           c.obs,
		ctx:           ctx,
		configs:       make(map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig),
		prefixes:      make(map[*resolve.FetchCacheHandle]string),
		missedKeys:    make(map[*resolve.FetchCacheHandle][][]string),
		reuseProvides: make(map[*resolve.FetchCacheHandle]*resolve.Object),
	}
}

// requestCache is the per-request working surface (implements
// resolve.RequestCache).
//
// Concurrency invariant (external lock, no internal mutex): PrepareFetch /
// OnFetchSkipped / OnFetchResult run from parallel per-fetch (and
// per-defer-group) goroutines, but each opens exactly ONE CacheTransaction via
// in.Arena.Begin(), which holds the request's single DataBuffer.Lock for the
// whole hook body. Every mutable field below (the deferred-write set and the
// per-handle config map; later the shared L1 map) is read and written only
// while that lock is held. EndRequest runs once, single-threaded, after the
// whole tree resolves and touches only `deferred` (bytes), so it needs no lock
// either. Verified under -race by the transaction/e2e rows.
type requestCache struct {
	store Store
	obs   resolve.CacheObserver
	ctx   *resolve.Context

	// deferred is the request-end L2 write set: BYTES only, so the flush needs
	// neither lock nor arena.
	deferred []deferredSet
	// configs threads each handle's config from PrepareFetch to the merge hook
	// (the handle itself is opaque to the loader and carries no config).
	configs map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig
	// prefixes keeps each handle's key prefix so the merge hooks can re-render
	// pending candidates with the same templates the lookup used.
	prefixes map[*resolve.FetchCacheHandle]string
	// missedKeys records, per handle and item, the rendered keys whose lookup
	// MISSED, so a hit served from another key can backfill them.
	missedKeys map[*resolve.FetchCacheHandle][][]string
	// reuseProvides overrides the coverage/transform tree for by-key
	// root-field handles: the cached value is the ENTITY, so the walks use the
	// root field's value subtree instead of the whole-response tree.
	reuseProvides map[*resolve.FetchCacheHandle]*resolve.Object
	// l1 is the request-lifetime entity store: NORMALIZED *astjson.Value
	// (never bytes, never marshaled) under the SAME derived keys as L2.
	// EXTERNAL-LOCK INVARIANT: guarded by the caller's CacheTransaction (the
	// DataBuffer lock) like everything else on requestCache — no internal
	// mutex. Values are isolated by tx.StructuralCopy at BOTH boundaries
	// (write and read), so merges can never corrupt a stored value. The map
	// is allocated lazily on the first write. This is NOT the removed
	// @requestScoped feature (D11).
	l1 map[string]*astjson.Value
}

// deferredSet is one pending L2 write, held as bytes until EndRequest; reason
// is metadata only (refresh vs backfill) and never gates the write.
type deferredSet struct {
	key    string
	value  []byte
	ttl    time.Duration
	reason resolve.CacheWriteReason
}

// negativeCacheSentinel is the stored form of a negative entry: a whole-value
// JSON null. It is distinguishable from "no entry" (a store miss) and from a
// positive null FIELD value (which always lives inside an object) — the read
// path routes on a TOP-LEVEL TypeNull cached value only.
const negativeCacheSentinel = "null"

// useL2 reports whether this fetch participates in L2 through this controller.
func (r *requestCache) useL2(cfg *resolve.FetchCacheConfig) bool {
	return cfg != nil && cfg.L2 && r.store != nil
}

// l1Put stores one L1 value; the caller passes a transaction-owned value
// (ParseBytes/StructuralCopy product — never a heap value smuggled into
// arena-noscan memory) and holds the transaction.
func (r *requestCache) l1Put(key string, value *astjson.Value) {
	if r.l1 == nil {
		r.l1 = make(map[string]*astjson.Value)
	}
	r.l1[key] = value
}

// PrepareFetch renders the candidate key from the item data, looks L2 up,
// runs the always-on coverage walk, and AND-reduces per-item hits into the
// decision. It opens exactly one CacheTransaction for all arena work.
func (r *requestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	cfg := in.Config
	if cfg == nil || (!cfg.L1 && !r.useL2(cfg)) {
		return resolve.DecisionFetch, nil
	}
	if cfg.KeySpec.Scope == resolve.CacheScopeRootField {
		if !r.useL2(cfg) {
			// Root fields are L2 providers only (task 13); L1 never applies.
			return resolve.DecisionFetch, nil
		}
		return r.prepareRootFieldFetch(in, cfg)
	}
	if cfg.KeySpec.Scope != resolve.CacheScopeEntity {
		return resolve.DecisionFetch, nil
	}
	if in.BatchStats != nil {
		return r.prepareBatchFetch(in, cfg)
	}
	if len(in.Items) == 0 {
		return resolve.DecisionFetch, nil
	}
	templates := newCacheKeyTemplates(cfg, in.HeaderHash)
	if len(templates) == 0 {
		return resolve.DecisionFetch, nil
	}

	tx := in.Arena.Begin()
	defer tx.Commit()

	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	missedByItem := make([][]string, 0, len(in.Items))
	var shadowStash map[int]resolve.ShadowCacheEntry
	allCovered := true
	mustWriteBack := false
	for i, item := range in.Items {
		state, missed, itemMustWriteBack := r.prepareItemState(tx, cfg, templates, item)
		if entry := shadowStashEntry(tx, cfg, &state); entry != nil {
			if shadowStash == nil {
				shadowStash = make(map[int]resolve.ShadowCacheEntry)
			}
			shadowStash[i] = *entry
		}
		if state.FromCache == nil {
			allCovered = false
		}
		if itemMustWriteBack {
			mustWriteBack = true
		}
		items = append(items, state)
		missedByItem = append(missedByItem, missed)
	}

	decision := resolve.DecisionFetch
	switch {
	case shadowStash != nil:
		// Shadow reads never serve: the loader treats FetchShadow exactly like
		// Fetch (full network, full merge); the stash drives the compare.
		decision = resolve.DecisionFetchShadow
	case allCovered:
		decision = resolve.DecisionSkipFullHit
	}
	handle := &resolve.FetchCacheHandle{
		Decision: decision,
		WasHit:   allCovered,
		// MustWriteBack matters only on a full hit: OnFetchSkipped then still
		// owes best-effort backfill/refresh writes for the keys that missed,
		// the candidates that could not render, or a merged/older selection.
		MustWriteBack: allCovered && mustWriteBack,
		Shadow:        shadowStash != nil,
		ShadowStash:   shadowStash,
		Items:         items,
	}
	r.configs[handle] = cfg
	r.prefixes[handle] = cacheKeyPrefix(cfg, in.HeaderHash)
	r.missedKeys[handle] = missedByItem
	return decision, handle
}

// shadowStashEntry moves a shadow-configured item's would-be-served value into
// a stash entry and CLEARS the serving fields, so nothing can be served while
// the compare still sees the exact selection (value, key, freshness, TTL).
// Returns nil when the config is not in shadow mode or nothing was selected.
func shadowStashEntry(tx *resolve.CacheTransaction, cfg *resolve.FetchCacheConfig, state *resolve.ItemCacheState) *resolve.ShadowCacheEntry {
	if !cfg.ShadowMode || state.FromCache == nil {
		return nil
	}
	cacheTTL := cfg.TTL
	if state.NegativeHit {
		cacheTTL = cfg.NegativeCacheTTL
	}
	shadowKey := ""
	if len(state.RenderedKeys) > 0 {
		shadowKey = state.RenderedKeys[0]
	}
	entry := &resolve.ShadowCacheEntry{
		CachedValue:  tx.StructuralCopy(state.FromCache),
		CacheKey:     shadowKey,
		RemainingTTL: state.SelectedRemainingTTL,
		CacheTTL:     cacheTTL,
	}
	state.FromCache = nil
	state.SelectedRemainingTTL = 0
	state.NegativeHit = false
	state.NeedsWriteback = false
	return entry
}

// prepareRootFieldFetch is the root-field arm: ONE whole-response-scoped key
// per fetch (field coordinate + canonical request variables), one lookup, one
// coverage walk, with the served value shared across the fetch's merge
// targets. Root-field shadow is the historical ASYMMETRY: a hit force-refetches
// and overwrites L2, but never stashes and never compares.
func (r *requestCache) prepareRootFieldFetch(in resolve.PrepareFetchInput, cfg *resolve.FetchCacheConfig) (resolve.Decision, *resolve.FetchCacheHandle) {
	if len(in.Items) == 0 {
		return resolve.DecisionFetch, nil
	}
	if len(cfg.KeySpec.EntityKeyMappings) > 0 {
		if decision, handle, ok := r.prepareRootFieldEntityReuse(in, cfg); ok {
			return decision, handle
		}
		// The reuse preconditions did not hold (e.g. a non-object subtree);
		// fall back to the plain root-field path.
	}
	key := rootFieldCacheKey(cfg, in.HeaderHash, r.ctx)

	tx := in.Arena.Begin()
	defer tx.Commit()

	value, remaining, hit := r.store.Get(key)
	var fromCache *astjson.Value
	var candidate *resolve.CacheCandidate
	if hit {
		if cached, err := tx.ParseBytes(value); err == nil {
			candidate = &resolve.CacheCandidate{
				Value:        append([]byte(nil), value...),
				RemainingTTL: remaining,
			}
			if cfg.ProvidesData != nil && covers(r.ctx, cached, cfg.ProvidesData) {
				fromCache = cached
			}
		}
	}
	if cfg.ShadowMode {
		// The root-field shadow asymmetry: read, then force-refetch WITHOUT a
		// stash or compare — the plain DecisionFetch below makes a compare
		// structurally impossible, and the normal write path overwrites L2.
		fromCache = nil
	}

	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	for _, item := range in.Items {
		state := resolve.ItemCacheState{
			Item:         item,
			RenderedKeys: []string{key},
			FromCache:    fromCache,
		}
		if candidate != nil {
			state.FromCacheCandidates = []resolve.CacheCandidate{*candidate}
		}
		if fromCache != nil {
			state.SelectedRemainingTTL = remaining
		}
		items = append(items, state)
	}

	decision := resolve.DecisionFetch
	if fromCache != nil {
		decision = resolve.DecisionSkipFullHit
	}
	handle := &resolve.FetchCacheHandle{
		Decision: decision,
		WasHit:   fromCache != nil,
		Items:    items,
	}
	r.configs[handle] = cfg
	r.prefixes[handle] = cacheKeyPrefix(cfg, in.HeaderHash)
	return decision, handle
}

// prepareRootFieldEntityReuse serves a BY-KEY root field from the ENTITY key
// space: the lookup item derives from the field's arguments (via the frozen
// EntityKeyMappings), the entity candidates flow through the SAME best-effort
// multi-key machinery as entity fetches (arg-derived renders at lookup,
// data-derived candidates backfill at write), and the served entity value
// splices AT the field's response key. Reuse works exactly when the policy
// shares its CacheName with the entity policy — the prefix makes read key ==
// write key by construction.
func (r *requestCache) prepareRootFieldEntityReuse(in resolve.PrepareFetchInput, cfg *resolve.FetchCacheConfig) (resolve.Decision, *resolve.FetchCacheHandle, bool) {
	responseKey, subtree, ok := rootFieldSubtree(cfg.ProvidesData, cfg.KeySpec.FieldName)
	if !ok {
		return resolve.DecisionFetch, nil, false
	}
	templates := newCacheKeyTemplates(cfg, in.HeaderHash)
	if len(templates) == 0 {
		return resolve.DecisionFetch, nil, false
	}
	lookupItem := entityLookupItem(r.ctx, cfg.KeySpec.EntityKeyMappings)

	tx := in.Arena.Begin()
	defer tx.Commit()

	// The coverage/selection walks run against the FIELD subtree — the cached
	// value is the entity, not the whole response.
	reuseCfg := *cfg
	reuseCfg.ProvidesData = subtree

	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	missedByItem := make([][]string, 0, len(in.Items))
	allCovered := true
	mustWriteBack := false
	for _, item := range in.Items {
		state, missed, itemMustWriteBack := r.prepareItemState(tx, &reuseCfg, templates, lookupItem)
		state.Item = item
		// The entity value splices (and extracts, on the write side) at the
		// field's RESPONSE key.
		state.EntityMergePath = []string{responseKey}
		if cfg.ShadowMode {
			// The root-field shadow asymmetry applies here too: read, then
			// force-refetch without a stash or compare.
			state.FromCache = nil
			state.SelectedRemainingTTL = 0
			state.NegativeHit = false
			state.NeedsWriteback = false
		}
		if state.FromCache == nil {
			allCovered = false
		}
		if itemMustWriteBack {
			mustWriteBack = true
		}
		items = append(items, state)
		missedByItem = append(missedByItem, missed)
	}

	decision := resolve.DecisionFetch
	if allCovered {
		decision = resolve.DecisionSkipFullHit
	}
	handle := &resolve.FetchCacheHandle{
		Decision:      decision,
		WasHit:        allCovered,
		MustWriteBack: allCovered && mustWriteBack,
		Items:         items,
	}
	r.configs[handle] = cfg
	r.prefixes[handle] = cacheKeyPrefix(cfg, in.HeaderHash)
	r.missedKeys[handle] = missedByItem
	r.reuseProvides[handle] = subtree
	return decision, handle, true
}

// rootFieldSubtree finds the root field's value node in the ProvidesData tree
// (matched by SCHEMA name, alias-aware) and returns its response key and
// object subtree; a non-object subtree (lists etc.) declines reuse.
func rootFieldSubtree(tree *resolve.Object, fieldName string) (string, *resolve.Object, bool) {
	if tree == nil {
		return "", nil, false
	}
	for _, field := range tree.Fields {
		schemaName := string(field.Name)
		if len(field.OriginalName) > 0 {
			schemaName = string(field.OriginalName)
		}
		if schemaName != fieldName {
			continue
		}
		subtree, ok := field.Value.(*resolve.Object)
		if !ok {
			return "", nil, false
		}
		return string(field.Name), subtree, true
	}
	return "", nil, false
}

// entityLookupItem builds the arg-derived entity item the templates render
// from: one field per mapping, read from the request variables by the key
// field's name (the documented v1 constraint). Missing variables simply leave
// the candidate unrenderable — best-effort, never an error.
func entityLookupItem(ctx *resolve.Context, mappings []resolve.EntityKeyMapping) *astjson.Value {
	item := astjson.ObjectValue(nil)
	if ctx == nil {
		return item
	}
	variables := ctx.VariablesView()
	for _, mapping := range mappings {
		for _, fieldMapping := range mapping.FieldMappings {
			value := variables.Get(fieldMapping.ArgumentPath...)
			if value == nil {
				continue
			}
			item.Set(nil, fieldMapping.EntityKeyField, value)
		}
	}
	return item
}

// prepareBatchFetch is the batch arm: one ItemCacheState per UNIQUE
// representation (BatchStats bucket), keyed and looked up individually, with
// the original batch position recorded for the splice and (task 19) the
// partial realign. Full-batch semantics: ALL covered serves, ANY uncovered
// refetches everything.
func (r *requestCache) prepareBatchFetch(in resolve.PrepareFetchInput, cfg *resolve.FetchCacheConfig) (resolve.Decision, *resolve.FetchCacheHandle) {
	if len(in.BatchStats) == 0 {
		// The loader's empty-batch skip normally prevents this call entirely;
		// an empty batch has nothing to serve or write.
		return resolve.DecisionFetch, nil
	}
	templates := newCacheKeyTemplates(cfg, in.HeaderHash)
	if len(templates) == 0 {
		return resolve.DecisionFetch, nil
	}

	tx := in.Arena.Begin()
	defer tx.Commit()

	items := make([]resolve.ItemCacheState, 0, len(in.BatchStats))
	missedByItem := make([][]string, 0, len(in.BatchStats))
	var shadowStash map[int]resolve.ShadowCacheEntry
	allCovered := true
	mustWriteBack := false
	for i, bucket := range in.BatchStats {
		var representative *astjson.Value
		if len(bucket) > 0 {
			representative = bucket[0]
		}
		state, missed, itemMustWriteBack := r.prepareItemState(tx, cfg, templates, representative)
		state.BatchIndex = i
		if entry := shadowStashEntry(tx, cfg, &state); entry != nil {
			if shadowStash == nil {
				shadowStash = make(map[int]resolve.ShadowCacheEntry)
			}
			shadowStash[i] = *entry
		}
		if state.FromCache == nil {
			allCovered = false
		}
		if itemMustWriteBack {
			mustWriteBack = true
		}
		items = append(items, state)
		missedByItem = append(missedByItem, missed)
	}

	decision := resolve.DecisionFetch
	switch {
	case shadowStash != nil:
		decision = resolve.DecisionFetchShadow
	case allCovered:
		decision = resolve.DecisionSkipFullHit
	}
	handle := &resolve.FetchCacheHandle{
		Decision:       decision,
		WasHit:         allCovered,
		MustWriteBack:  allCovered && mustWriteBack,
		BatchEntityKey: true,
		Shadow:         shadowStash != nil,
		ShadowStash:    shadowStash,
		Items:          items,
	}
	r.configs[handle] = cfg
	r.prefixes[handle] = cacheKeyPrefix(cfg, in.HeaderHash)
	r.missedKeys[handle] = missedByItem
	return decision, handle
}

// prepareItemState runs the per-item multi-key ladder: best-effort render of
// EVERY candidate (renderable → RenderedKeys, not renderable →
// PendingCandidates), lookup under all rendered keys, freshest-first candidate
// collection, multi-candidate selection, and reorder of the chosen value to
// selection order. It returns the item state, the rendered keys whose lookup
// missed, and whether a full hit on this item still owes write-backs.
func (r *requestCache) prepareItemState(tx *resolve.CacheTransaction, cfg *resolve.FetchCacheConfig, templates []cacheKeyTemplate, item *astjson.Value) (resolve.ItemCacheState, []string, bool) {
	state := resolve.ItemCacheState{Item: item}
	mustWriteBack := false
	var missedKeys []string
	for i, template := range templates {
		key, ok := template.render(item)
		if !ok {
			// An unrenderable candidate is skipped at lookup and retried at
			// write time from the fresh data — never an error (no candidate is
			// required in the best-effort multi-key model).
			state.PendingCandidates = append(state.PendingCandidates, cfg.KeySpec.Candidates[i])
			mustWriteBack = true
			continue
		}
		state.RenderedKeys = append(state.RenderedKeys, key)
	}

	// L1 first: keys are derived ONCE and shared with L2; a covering L1 hit
	// serves with zero parsing and zero marshaling and SHORT-CIRCUITS every
	// L2 read (and therefore every L2 write-back — coverage never required
	// L2 here).
	if cfg.L1 {
		for _, key := range state.RenderedKeys {
			stored := r.l1[key]
			if stored == nil {
				continue
			}
			if stored.Type() == astjson.TypeNull {
				// The L1 negative sentinel: the entity is KNOWN missing within
				// this request.
				state.FromCache = tx.StructuralCopy(stored)
				state.NegativeHit = true
				return state, nil, false
			}
			if covers(r.ctx, stored, cfg.ProvidesData) {
				state.FromCache = tx.StructuralCopy(stored)
				return state, nil, false
			}
		}
	}
	if !r.useL2(cfg) {
		// L1-only: a miss is a plain fetch; there is no L2 to read or back
		// fill.
		return state, nil, false
	}

	type lookupHit struct {
		candidate resolve.CacheCandidate
		cached    *astjson.Value
	}
	hits := make([]lookupHit, 0, len(templates))
	for _, key := range state.RenderedKeys {
		value, remaining, hit := r.store.Get(key)
		if !hit {
			missedKeys = append(missedKeys, key)
			mustWriteBack = true
			continue
		}
		cached, err := tx.ParseBytes(value)
		if err != nil {
			// Malformed cached bytes are treated as a miss for this key; the
			// write path will refresh it.
			missedKeys = append(missedKeys, key)
			mustWriteBack = true
			continue
		}
		hits = append(hits, lookupHit{
			candidate: resolve.CacheCandidate{
				Value:        append([]byte(nil), value...),
				RemainingTTL: remaining,
			},
			cached: cached,
		})
	}
	if len(hits) == 0 {
		return state, missedKeys, mustWriteBack
	}

	// Freshest first: a known remaining TTL beats an unknown one, larger beats
	// smaller; the stable sort keeps candidate order for ties.
	slices.SortStableFunc(hits, func(a, b lookupHit) int {
		return compareCacheCandidateFreshness(a.candidate.RemainingTTL, b.candidate.RemainingTTL)
	})
	state.FromCacheCandidates = make([]resolve.CacheCandidate, 0, len(hits))
	parsed := make([]*astjson.Value, 0, len(hits))
	for _, hit := range hits {
		state.FromCacheCandidates = append(state.FromCacheCandidates, hit.candidate)
		parsed = append(parsed, hit.cached)
		// A TOP-LEVEL null cached value is the negative sentinel: the entity is
		// KNOWN to not exist, so the item is served as null without a coverage
		// walk (there is nothing to cover). The freshest sentinel wins.
		if hit.cached != nil && hit.cached.Type() == astjson.TypeNull && state.FromCache == nil {
			state.FromCache = hit.cached
			state.SelectedRemainingTTL = hit.candidate.RemainingTTL
			state.NegativeHit = true
		}
	}
	if state.NegativeHit {
		if cfg.L1 {
			r.populateL1(tx, &state)
		}
		return state, missedKeys, mustWriteBack
	}
	// The selected value stays in NORMALIZED (stored) form on the handle;
	// OnFetchSkipped denormalizes it to the requesting aliases at splice time.
	selectMultiCandidateCacheValue(tx, r.ctx, &state, parsed, cfg.ProvidesData)
	if cfg.L1 && state.FromCache != nil {
		r.populateL1(tx, &state)
	}
	return state, missedKeys, mustWriteBack || state.NeedsWriteback
}

// populateL1 stores an L2-served value in L1 under every rendered key, so a
// later fetch of the same entity in this request skips L2 entirely. One
// structural copy is shared across the keys; the read boundary copies again.
func (r *requestCache) populateL1(tx *resolve.CacheTransaction, state *resolve.ItemCacheState) {
	if state.FromCache == nil || len(state.RenderedKeys) == 0 {
		return
	}
	copied := tx.StructuralCopy(state.FromCache)
	for _, key := range state.RenderedKeys {
		r.l1Put(key, copied)
	}
}

// OnFetchSkipped splices the chosen cached values into the merge targets at
// the surfaced merge path, inside one CacheTransaction; StructuralCopy guards
// against aliasing when one cached value serves multiple targets. A hit that
// left other candidate keys missed, unrenderable, or shape-stale still owes
// best-effort write-backs (no network): refresh the canonical keys after a
// merged/older selection, backfill the missed keys, and re-render pending
// candidates from the served value.
func (r *requestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil {
		return nil
	}
	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	tx := in.Arena.Begin()
	defer tx.Commit()

	prefix := r.prefixes[h]
	missedByItem := r.missedKeys[h]
	for i, item := range h.Items {
		if item.FromCache == nil || item.Item == nil {
			continue
		}
		// A batch item splices into EVERY merge target of its unique
		// representation (the BatchStats bucket at its original position).
		targets := []*astjson.Value{item.Item}
		if h.BatchEntityKey {
			targets = nil
			if item.BatchIndex >= 0 && item.BatchIndex < len(in.BatchStats) {
				targets = in.BatchStats[item.BatchIndex]
			}
		}
		if item.FromCache.Type() == astjson.TypeNull {
			// A negative hit splices NOTHING: a real successful-but-empty
			// entity fetch leaves the merge targets untouched (mergeResult
			// early-returns), and the resolvable then renders the null bubble
			// and its non-null error exactly as it would uncached. Replacing
			// the target with null here would make the cached response DIFFER
			// from the uncached one — caching must never change the response.
			continue
		}
		provides := resolve.Node(cfg.ProvidesData)
		if subtree := r.reuseProvides[h]; subtree != nil {
			provides = subtree
		}
		// A by-key root-field item carries its own merge path (the field's
		// response key); everything else uses the fetch-level merge path.
		mergePath := in.MergePath
		if len(item.EntityMergePath) > 0 {
			mergePath = item.EntityMergePath
		}
		for _, target := range targets {
			if target == nil {
				continue
			}
			// Denormalize the stored value to the requesting operation's
			// aliases in selection order; the walk builds a fresh
			// transaction-owned value per target, so it is also the
			// aliasing-safe copy for the splice.
			cached := denormalizeToSelection(tx, r.ctx, item.FromCache, provides)
			if len(mergePath) > 0 {
				if _, err := tx.MergeValuesWithPath(target, cached, mergePath...); err != nil {
					return err
				}
			} else if _, err := tx.MergeValues(target, cached); err != nil {
				return err
			}
		}

		value := item.FromCache.MarshalTo(nil)
		if item.NeedsWriteback {
			// The served value was synthesized or older-but-covering: rewrite
			// the canonical entries so the next lookup hits on the first rung.
			for _, key := range item.RenderedKeys {
				r.deferSet(key, value, cfg.TTL, resolve.CacheWriteReasonRefresh)
			}
		} else if i < len(missedByItem) {
			for _, key := range missedByItem[i] {
				r.deferSet(key, value, cfg.TTL, resolve.CacheWriteReasonBackfill)
			}
		}
		for _, candidate := range item.PendingCandidates {
			// A candidate unrenderable from the request item may render from
			// the SERVED value (it can carry more fields); skip silently when
			// it still cannot render — best-effort, never required.
			template := cacheKeyTemplate{prefix: prefix, representation: candidate.Representation}
			key, ok := template.render(item.FromCache)
			if !ok {
				continue
			}
			r.deferSet(key, value, cfg.TTL, resolve.CacheWriteReasonBackfill)
		}
	}
	return nil
}

// OnFetchResult applies the write gate and defers the L2 writes (bytes) to the
// request-end flush. The gate is !FetchFailed && !HasErrors && ResponseData !=
// nil && Type() != Null — it can never reduce to !HasErrors alone, because
// transport/empty-body/parse failures reach this hook with HasErrors == false.
func (r *requestCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil {
		return nil
	}
	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	if in.FetchFailed || in.HasErrors {
		// All failure signals block ALL writes — including negative ones: a
		// transport/parse failure or errored response is a transient error,
		// never a proof of nonexistence (FetchFailed wins over EmptyEntity).
		return nil
	}
	if in.EmptyEntity && in.ResponseData != nil && in.ResponseData.Type() == astjson.TypeNull {
		// The ONE non-failure that still writes: a SUCCESSFUL fetch that
		// legitimately returned no entity caches the null sentinel so repeated
		// lookups for a nonexistent entity skip the network.
		if !cfg.L1 && cfg.NegativeCacheTTL <= 0 {
			return nil
		}
		tx := in.Arena.Begin()
		defer tx.Commit()
		for i := range h.Items {
			h.Items[i].FromCache = tx.Null()
			h.Items[i].NegativeHit = true
			for _, key := range h.Items[i].RenderedKeys {
				if cfg.L1 {
					// Within the request the nonexistence is a fact — the L1
					// sentinel needs no TTL knob.
					r.l1Put(key, tx.Null())
				}
				if r.useL2(cfg) && cfg.NegativeCacheTTL > 0 {
					r.deferSet(key, []byte(negativeCacheSentinel), cfg.NegativeCacheTTL, resolve.CacheWriteReasonRefresh)
				}
			}
		}
		return nil
	}
	if in.ResponseData == nil || in.ResponseData.Type() == astjson.TypeNull {
		return nil
	}
	tx := in.Arena.Begin()
	defer tx.Commit()

	if h.Shadow && r.obs != nil {
		// The staleness probe: compare the stashed cached values against the
		// fresh response BEFORE any write, inside this hook's transaction
		// (compare -> write-L1 -> write-L2 order; no second lock acquisition).
		r.obs.CompareShadow(h, in.ResponseData, tx)
	}

	if cfg.KeySpec.Scope == resolve.CacheScopeRootField && len(cfg.KeySpec.EntityKeyMappings) == 0 {
		// One whole-response value under the fetch's single key, written once
		// (the items share it).
		toStore := in.ResponseData
		if cfg.ProvidesData != nil && cfg.ProvidesData.HasAliases {
			toStore = normalizeToSchema(tx, r.ctx, toStore, cfg.ProvidesData)
		}
		value := toStore.MarshalTo(nil)
		for _, key := range h.Items[0].RenderedKeys {
			r.deferSet(key, value, cfg.TTL, resolve.CacheWriteReasonRefresh)
		}
		return nil
	}

	// A batch response is the _entities array: each unique representation's
	// value sits at its original batch position.
	var batch []*astjson.Value
	if h.BatchEntityKey {
		batch = in.ResponseData.GetArray()
		if batch == nil {
			return nil
		}
	}
	provides := r.reuseProvides[h]
	if provides == nil {
		provides = cfg.ProvidesData
	}
	for _, item := range h.Items {
		itemToStore := in.ResponseData
		if h.BatchEntityKey {
			if item.BatchIndex < 0 || item.BatchIndex >= len(batch) {
				continue
			}
			itemToStore = batch[item.BatchIndex]
		}
		if len(in.MergePath) > 0 {
			// The response merges into the item at the merge path; the value to
			// cache is the entity BELOW that path (D4), never the wrapper.
			entity := itemToStore.Get(in.MergePath...)
			if entity == nil {
				continue
			}
			itemToStore = entity
		}
		if len(item.EntityMergePath) > 0 {
			// A by-key root field caches the ENTITY below its response key,
			// never the whole-response wrapper.
			entity := itemToStore.Get(item.EntityMergePath...)
			if entity == nil {
				continue
			}
			itemToStore = entity
		}
		// Normalize to the stored form (schema names + argument suffixes)
		// before caching; trees without aliases or args skip the transform
		// (HasAliases is the fast-path gate) and store the raw value.
		toStore := itemToStore
		if provides != nil && provides.HasAliases {
			toStore = normalizeToSchema(tx, r.ctx, itemToStore, provides)
		}
		// One key derivation feeds BOTH layers: the rendered keys plus the
		// pending candidates re-rendered from the FRESH normalized value (a
		// candidate the response still cannot render is skipped silently —
		// best-effort, never required).
		keys := item.RenderedKeys
		backfillFrom := len(keys)
		for _, candidate := range item.PendingCandidates {
			template := cacheKeyTemplate{prefix: r.prefixes[h], representation: candidate.Representation}
			key, ok := template.render(toStore)
			if !ok {
				continue
			}
			keys = append(keys, key)
		}
		if cfg.L1 {
			// write-L1 before write-L2; POINTER store, zero marshaling.
			copied := tx.StructuralCopy(toStore)
			for _, key := range keys {
				r.l1Put(key, copied)
			}
		}
		if r.useL2(cfg) {
			// Marshal ONCE per item — only the L2 path holds bytes.
			value := toStore.MarshalTo(nil)
			for i, key := range keys {
				reason := resolve.CacheWriteReasonRefresh
				if i >= backfillFrom {
					reason = resolve.CacheWriteReasonBackfill
				}
				r.deferSet(key, value, cfg.TTL, reason)
			}
		}
	}
	return nil
}

// EndRequest flushes the deferred L2 writes — bytes only, no lock, no arena,
// no transaction — and finalizes observability. It runs once, single-threaded,
// after the root tree and every defer group have resolved.
func (r *requestCache) EndRequest() {
	recorder, _ := r.store.(WriteReasonRecorder)
	for _, set := range r.deferred {
		if recorder != nil {
			recorder.RecordWriteReason(set.key, set.reason)
		}
		r.store.Set(set.key, set.value, set.ttl)
	}
	r.deferred = nil
	if r.obs != nil {
		r.obs.EndRequest(r.ctx)
	}
}

// WriteReasonRecorder is an optional Store extension: a store implementing it
// receives each write's reason (refresh vs backfill) right before the Set.
// Reasons are metadata only — they never gate a write — and exist so tests and
// observability can distinguish refresh from backfill traffic.
type WriteReasonRecorder interface {
	RecordWriteReason(key string, reason resolve.CacheWriteReason)
}

func (r *requestCache) deferSet(key string, value []byte, ttl time.Duration, reason resolve.CacheWriteReason) {
	r.deferred = append(r.deferred, deferredSet{
		key:    key,
		value:  append([]byte(nil), value...),
		ttl:    ttl,
		reason: reason,
	})
}
