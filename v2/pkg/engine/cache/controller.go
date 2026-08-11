package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Item is one entry to write: the key, the encoded value envelope, the TTL the
// entry is written with, and the tags the store indexes it under for
// invalidation.
type Item struct {
	Key   string
	Value []byte
	TTL   time.Duration
	// Tags are the entry's invalidation addresses, coarsest first: every write
	// carries "subgraph:{subgraph}" and "type:{subgraph}:{TypeName}", an entity
	// entry additionally "entity:{subgraph}:{TypeName}:{digest of its @key
	// fields}", shared by every variant of that entity. The three forms are
	// stable API for an invalidation endpoint and a tag-purging CDN; the slice
	// is open for user-defined tags. Tags live on the Item alone and never
	// inside the stored value: index maintenance and delete-by-tag are the
	// store's business.
	Tags []string
}

// Entry is one store read result, positionally aligned with the key it was
// requested under. OK reports whether the key was present; RemainingTTL is the
// entry's remaining freshness, 0 when the store cannot report one.
type Entry struct {
	Value        []byte
	RemainingTTL time.Duration
	OK           bool
}

// Store is the batched L2 backend the controller talks to: ONE GetMany per
// fetch and ONE SetMany per request. GetMany must return exactly one Entry per
// requested key, in key order. Implementations must be safe for concurrent use
// (the controller calls them from parallel fetch hooks, under the request's
// DataBuffer.Lock) and never need to fail a request — the controller turns a
// GetMany error into misses and drops the writes of a failed SetMany.
type Store interface {
	GetMany(ctx context.Context, keys []string) ([]Entry, error)
	SetMany(ctx context.Context, items []Item) error
}

// Controller is the long-lived cache lifecycle port (implements
// resolve.CacheController): one per integrator/cache instance, holding only
// immutable collaborators. All per-request mutable state lives on the
// requestCache BeginRequest hands out.
type Controller struct {
	store Store
	obs   resolve.CacheObserver
	// global carries the runtime knobs of the configuration cascade's global
	// level — the two client-header emission switches. Unset means the
	// defaults: the client cache answer is computed, the CDN tag union is not.
	global cacheconfig.GlobalCacheConfig
}

// ControllerOption configures the long-lived controller.
type ControllerOption func(*Controller)

// WithGlobalConfig hands the controller the caching configuration's GLOBAL
// level, the same value the plan side is configured with. The controller reads
// its runtime knobs from it: whether to compute the client cache answer and
// whether to accumulate the CDN tag union.
func WithGlobalConfig(global cacheconfig.GlobalCacheConfig) ControllerOption {
	return func(c *Controller) {
		c.global = global
	}
}

// NewController builds a controller over an L2 store; obs may be nil (no
// observability).
func NewController(store Store, obs resolve.CacheObserver, options ...ControllerOption) *Controller {
	controller := &Controller{store: store, obs: obs}
	for _, option := range options {
		option(controller)
	}
	return controller
}

// BeginRequest hands out the request-lifetime working surface. Called lazily,
// once per request, under DataBuffer.Lock (the loader's cacheRequest).
func (c *Controller) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	if c.obs != nil {
		c.obs.BeginRequest(ctx)
	}
	request := &requestCache{
		store: c.store,
		obs:   c.obs,
		ctx:   ctx,
		// states allocates lazily on the first cached fetch.
	}
	request.aggregate = c.newResponseAggregate(ctx)
	if request.aggregate != nil {
		ctx.SetCacheResponseInfoSource(request)
	}
	return request
}

// newResponseAggregate builds the request's client-answer accumulator, or nil
// when nothing would read it: both emission knobs off — then no fold, no lock
// and no allocation happen all request long — a @defer plan, whose response
// headers ship with the initial frame while the deferred parts are still being
// fetched, or a caller with no Context to publish the answer through.
func (c *Controller) newResponseAggregate(ctx *resolve.Context) *responseAggregate {
	emitCacheControl := c.global.EmitsClientCacheControl()
	if !emitCacheControl && !c.global.EmitCdnTags {
		return nil
	}
	if ctx == nil || ctx.HasDeferredResponse() {
		return nil
	}
	return &responseAggregate{
		emitCacheControl: emitCacheControl,
		emitTags:         c.global.EmitCdnTags,
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
// either.
type requestCache struct {
	store Store
	obs   resolve.CacheObserver
	ctx   *resolve.Context

	// deferred is the request-end L2 write set: BYTES only, so the flush needs
	// neither lock nor arena.
	deferred []Item
	// states threads each handle's controller-side state from PrepareFetch to
	// the merge hooks (the handle itself is opaque to the loader). ONE lazily
	// allocated map instead of one per field — profiled: per-handle side-map
	// inserts were a measurable share of the hit path's allocations.
	states map[*resolve.FetchCacheHandle]*handleState
	// l1 is the request-lifetime entity store: NORMALIZED *astjson.Value
	// (never bytes, never marshaled, never enveloped) under the RAW PREIMAGE
	// keys the L2 keys derive from.
	// EXTERNAL-LOCK INVARIANT: guarded by the caller's CacheTransaction (the
	// DataBuffer lock) like everything else on requestCache — no internal
	// mutex. Values are isolated by tx.StructuralCopy at BOTH boundaries
	// (write and read), so merges can never corrupt a stored value. The map
	// is allocated lazily on the first write.
	//
	// The L1 key carries NO partition segment: one request has exactly one
	// requester, so every value in this map already belongs to them.
	l1 map[string]*astjson.Value
	// aggregate accumulates the request's client cache answer; nil when none is
	// computed (see newResponseAggregate). It carries its OWN lock, because the
	// folds run where a fetch's cacheability is decided rather than inside the
	// hooks' transactions.
	aggregate *responseAggregate
	// partitionHooks caches the PrivatePartitionProvider's answer per subgraph.
	// The hook is an integrator callback (JWT parse, tenant lookup), so it runs
	// at most once per subgraph per request; "" records that this request has no
	// hook identity there. Same external-lock invariant as the fields above.
	partitionHooks map[string]string
}

// handleState is the controller-side per-fetch state: built once when the
// fetch's lookup starts and read by every per-item step that follows, in the
// lookup and in the merge hooks alike.
type handleState struct {
	// cfg is the handle's fetch config (the loader never carries it).
	cfg *resolve.FetchCacheConfig
	// observed marks the handle as already passed to the observer, so a
	// pre-render FlushTraces and EndRequest's own pass observe it ONCE.
	observed bool
}

// useL2 reports whether this fetch participates in L2 through this controller.
func (r *requestCache) useL2(cfg *resolve.FetchCacheConfig) bool {
	return cfg != nil && cfg.L2 && r.store != nil
}

// resolveL2Access answers, for ONE fetch of this request, whether it may touch
// the store and under which partition segment: a public fetch simply may; a
// statically-private one needs a requester identity — the provider's when the
// request has one, else the forwarded subgraph header hash when the fetch keys
// by those headers. Both sources are TAGGED before hashing, so one source's
// literal text can never forge the other's partition. With NO identity the
// fetch skips L2 entirely — no read, no write, values and sentinel alike —
// and the no-identity hint is emitted exactly once per fetch.
func (r *requestCache) resolveL2Access(cfg *resolve.FetchCacheConfig, headerHash uint64) l2Access {
	if !r.useL2(cfg) {
		return l2Access{}
	}
	if !cfg.Private {
		return l2Access{enabled: true}
	}
	if identity := r.hookIdentity(cfg.SubgraphName); identity != "" {
		return l2Access{enabled: true, partition: sha256Hex("i:" + identity)}
	}
	if cfg.IncludeSubgraphHeaders {
		return l2Access{enabled: true, partition: sha256Hex("h:" + hex64(headerHash))}
	}
	r.observeUncacheablePrivate(cfg.SubgraphName, UncacheablePrivateNoIdentity)
	return l2Access{}
}

// hookIdentity returns this request's provider identity at a subgraph, calling
// the provider at most once per subgraph and caching even its empty answer. An
// EMPTY identity counts as no identity: it would otherwise collapse every
// requester the provider cannot name into one shared partition.
func (r *requestCache) hookIdentity(subgraph string) string {
	if identity, ok := r.partitionHooks[subgraph]; ok {
		return identity
	}
	var identity string
	if r.ctx != nil {
		if value, ok := r.ctx.PrivatePartition(subgraph); ok {
			identity = value
		}
	}
	if r.partitionHooks == nil {
		r.partitionHooks = make(map[string]string)
	}
	r.partitionHooks[subgraph] = identity
	return identity
}

// servableScope reports whether a stored entry may be served to this fetch: an
// entry whose recorded scope is not the one the fetch derives its keys under
// was written by a differently-configured deployment, and is discarded as a
// miss. A private entry reaching a public read is the dangerous direction (it
// would serve one requester's data to everyone); the public-entry-on-a-private
// -read direction is checked symmetrically, for the same price.
func (r *requestCache) servableScope(cfg *resolve.FetchCacheConfig, envelope storedEnvelope) bool {
	if envelope.Scope == entryScope(cfg.Private) {
		return true
	}
	if r.obs != nil {
		r.obs.OnScopeMismatch(cfg.SubgraphName, envelope.Scope)
	}
	return false
}

// observeUncacheablePrivate surfaces one fetch's private data that the store
// never received; a nil observer records nothing.
func (r *requestCache) observeUncacheablePrivate(subgraph, reason string) {
	if r.obs == nil {
		return
	}
	r.obs.OnUncacheablePrivate(subgraph, reason)
}

// storeContext is the context every store call runs under: the request's own,
// so a store implementation inherits its cancellation and deadline.
func (r *requestCache) storeContext() context.Context {
	if r.ctx != nil {
		if ctx := r.ctx.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// storeGetMany runs the fetch's ONE store read and returns entries positionally
// aligned with keys. A store error — or an answer that does not align with the
// keys, which would serve values under the wrong keys — is reported to the
// observer and degrades to an all-miss, so the fetch falls back to the origin
// and the request never fails.
func (r *requestCache) storeGetMany(subgraph string, keys []string) []Entry {
	entries, err := r.store.GetMany(r.storeContext(), keys)
	if err == nil && len(entries) == len(keys) {
		return entries
	}
	if err == nil {
		err = fmt.Errorf("store returned %d entries for %d keys", len(entries), len(keys))
	}
	r.observeStoreError("GetMany", subgraph, len(keys), err)
	return make([]Entry, len(keys))
}

// observeStoreError surfaces one failed store round trip to the observer; a nil
// observer records nothing.
func (r *requestCache) observeStoreError(op, subgraph string, keyCount int, err error) {
	if r.obs == nil {
		return
	}
	r.obs.OnStoreError(op, subgraph, keyCount, err)
}

// subgraphOf names the fetch's datasource for store-error reporting; "" when
// the fetch carries no info (fetch info is off).
func subgraphOf(item *resolve.FetchItem) string {
	if item == nil || item.Fetch == nil || item.Fetch.FetchInfo() == nil {
		return ""
	}
	return item.Fetch.FetchInfo().DataSourceName
}

// registerHandle threads the fetch's state to the merge hooks (the handle
// itself is opaque to the loader) and stamps the handle's observability
// fields: the fetch's ART trace destination (nil when tracing is off) and the
// key-hashing knob. Observability never adds calls outside the controller —
// the observer reads the handle at EndRequest.
func (r *requestCache) registerHandle(h *resolve.FetchCacheHandle, state *handleState, in resolve.PrepareFetchInput) {
	if r.states == nil {
		r.states = make(map[*resolve.FetchCacheHandle]*handleState)
	}
	r.states[h] = state
	h.HashAnalyticsKeys = state.cfg.HashAnalyticsKeys
	if in.Item != nil && in.Item.Fetch != nil {
		h.Trace = in.Item.Fetch.LoadTrace()
	}
}

// cachingDecisionFor resolves the storability of ONE fetch result from the
// subgraph's Cache-Control response header and the fetch's static
// configuration, and emits the runtime-private hint — at most once per result,
// because this runs once per merge hook.
func (r *requestCache) cachingDecisionFor(cfg *resolve.FetchCacheConfig, in resolve.MergeInput) cachingDecision {
	decision := resolveCaching(parseResponseCacheControl(in.ResponseHeaders), cachingInput{
		L1:          cfg.L1,
		L2:          r.useL2(cfg),
		Private:     cfg.Private,
		StaticTTL:   cfg.TTL,
		NegativeTTL: cfg.NegativeCacheTTL,
		MaxTTL:      cfg.MaxTTL,
	})
	if decision.UncacheablePrivate {
		r.observeUncacheablePrivate(cfg.SubgraphName, UncacheablePrivateResponseHeader)
	}
	return decision
}

// l1Put stores one L1 value; the caller passes a transaction-owned value
// (ParseBytes/StructuralCopy product — never a heap value smuggled into
// arena-noscan memory) and holds the transaction.
//
// PRIVATE VALUES LAND HERE UNPARTITIONED, deliberately: the map lives and dies
// with ONE request, and one request has ONE requester, so two identities can
// never meet in it — a second requester means a second request and therefore a
// second map. That is why privacy costs L2 entries but never in-request reuse,
// even for a request the partition provider cannot identify at all.
func (r *requestCache) l1Put(key string, value *astjson.Value) {
	if r.l1 == nil {
		r.l1 = make(map[string]*astjson.Value)
	}
	r.l1[key] = value
}

// PrepareFetch runs the lookup and folds the fetches it declines into the
// client cache answer: a cache-configured fetch that ends up with NO handle
// touches no layer, so its data reaches the response uncached.
func (r *requestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	decision, handle := r.prepareFetch(in)
	if handle == nil && in.Config != nil {
		r.foldUncacheableFetch(in.Config)
	}
	return decision, handle
}

// prepareFetch renders each item's key from its data, serves what L1 covers,
// resolves the remaining keys in ONE store read, runs the always-on coverage
// walk, and AND-reduces per-item hits into the decision. It opens exactly one
// CacheTransaction for all arena work.
func (r *requestCache) prepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	cfg := in.Config
	if cfg == nil || (!cfg.L1 && !r.useL2(cfg)) {
		return resolve.DecisionFetch, nil
	}
	if cfg.KeySpec.Scope == resolve.CacheScopeRootField {
		if !r.useL2(cfg) {
			// Root fields are L2 providers only; L1 never applies.
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

	tx := in.Arena.Begin()
	defer tx.Commit()

	template, ok := r.newFetchKeyTemplate(cfg, in)
	if !ok {
		return resolve.DecisionFetch, nil
	}
	state := &handleState{cfg: cfg}
	items := r.prepareItems(tx, state, template, subgraphOf(in.Item), in.Items)
	shadowStash := stashShadowReads(tx, cfg, items)
	allCovered := allItemsCovered(items)

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
		Decision:    decision,
		WasHit:      allCovered,
		Shadow:      shadowStash != nil,
		ShadowStash: shadowStash,
		Items:       items,
	}
	r.registerHandle(handle, state, in)
	return decision, handle
}

// newFetchKeyTemplate resolves the fetch's store access and builds its key
// template from it. ok=false when the fetch has nothing left to key: no
// representation node, or neither layer once a private fetch without a
// requester identity has lost L2. It must run inside the fetch's transaction —
// the access resolution touches the request-shared provider cache.
func (r *requestCache) newFetchKeyTemplate(cfg *resolve.FetchCacheConfig, in resolve.PrepareFetchInput) (cacheKeyTemplate, bool) {
	access := r.resolveL2Access(cfg, in.HeaderHash)
	if !cfg.L1 && !access.enabled {
		return cacheKeyTemplate{}, false
	}
	return newCacheKeyTemplate(r.ctx, cfg, in.HeaderHash, access)
}

// stashShadowReads moves every would-be-served value of a shadow-configured
// fetch into the stash, keyed by item index; nil when the config is not in
// shadow mode or nothing was selected.
func stashShadowReads(tx *resolve.CacheTransaction, cfg *resolve.FetchCacheConfig, items []resolve.ItemCacheState) map[int]resolve.ShadowCacheEntry {
	var shadowStash map[int]resolve.ShadowCacheEntry
	for i := range items {
		entry := shadowStashEntry(tx, cfg, &items[i])
		if entry == nil {
			continue
		}
		if shadowStash == nil {
			shadowStash = make(map[int]resolve.ShadowCacheEntry)
		}
		shadowStash[i] = *entry
	}
	return shadowStash
}

// allItemsCovered is the full-hit condition: every item selected a cached
// value. It runs AFTER the shadow stash, which clears what it takes — a shadow
// fetch therefore never counts as covered.
func allItemsCovered(items []resolve.ItemCacheState) bool {
	for i := range items {
		if items[i].FromCache == nil {
			return false
		}
	}
	return true
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
	entry := &resolve.ShadowCacheEntry{
		CachedValue:  tx.StructuralCopy(state.FromCache),
		CacheKey:     state.RenderedKey,
		RemainingTTL: state.RemainingTTL,
		CacheTTL:     cacheTTL,
	}
	state.FromCache = nil
	state.RemainingTTL = 0
	state.NegativeHit = false
	state.ServedFromLayer = ""
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

	tx := in.Arena.Begin()
	defer tx.Commit()

	access := r.resolveL2Access(cfg, in.HeaderHash)
	if !access.enabled {
		// Root fields live in L2 alone, so a private coordinate without a
		// requester identity has no cache at all this request.
		return resolve.DecisionFetch, nil
	}
	key := rootFieldCacheKey(cfg, in.HeaderHash, r.ctx, access.partition)

	state := &handleState{cfg: cfg}
	entry := r.storeGetMany(subgraphOf(in.Item), []string{key})[0]
	var fromCache *astjson.Value
	var servedFreshness time.Duration
	if entry.OK {
		if envelope, ok := decodeEnvelope(tx, entry.Value); ok && r.servableScope(cfg, envelope) {
			if cfg.ProvidesData != nil && covers(r.ctx, envelope.Data, cfg.ProvidesData) {
				fromCache = envelope.Data
				servedFreshness = r.servedFreshness(envelope)
			}
		}
	}
	if cfg.ShadowMode {
		// The root-field shadow asymmetry: read, then force-refetch WITHOUT a
		// stash or compare — the plain DecisionFetch below makes a compare
		// structurally impossible, and the normal write path overwrites L2.
		fromCache = nil
	}

	// One key for the whole fetch means one tag set, shared by its items.
	tags := rootFieldTags(cfg)
	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	for _, item := range in.Items {
		state := resolve.ItemCacheState{
			Item:        item,
			RenderedKey: key,
			Tags:        tags,
			FromCache:   fromCache,
		}
		if fromCache != nil {
			state.RemainingTTL = entry.RemainingTTL
			state.ServedFreshness = servedFreshness
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
	r.registerHandle(handle, state, in)
	return decision, handle
}

// prepareBatchFetch is the batch arm: one ItemCacheState per UNIQUE
// representation (BatchStats bucket), keyed and looked up individually, with
// the original batch position recorded for the splice and the
// partial realign. Full-batch semantics: ALL covered serves, ANY uncovered
// refetches everything.
func (r *requestCache) prepareBatchFetch(in resolve.PrepareFetchInput, cfg *resolve.FetchCacheConfig) (resolve.Decision, *resolve.FetchCacheHandle) {
	if len(in.BatchStats) == 0 {
		// The loader's empty-batch skip normally prevents this call entirely;
		// an empty batch has nothing to serve or write.
		return resolve.DecisionFetch, nil
	}
	tx := in.Arena.Begin()
	defer tx.Commit()

	template, ok := r.newFetchKeyTemplate(cfg, in)
	if !ok {
		return resolve.DecisionFetch, nil
	}
	// One representative per unique representation; an empty bucket keys off a
	// nil node and stays a miss.
	representations := make([]*astjson.Value, len(in.BatchStats))
	for i, bucket := range in.BatchStats {
		if len(bucket) > 0 {
			representations[i] = bucket[0]
		}
	}
	state := &handleState{cfg: cfg}
	items := r.prepareItems(tx, state, template, subgraphOf(in.Item), representations)
	for i := range items {
		items[i].BatchIndex = i
	}
	shadowStash := stashShadowReads(tx, cfg, items)
	allCovered := allItemsCovered(items)

	decision := resolve.DecisionFetch
	var batchFetchKeep []bool
	switch {
	case shadowStash != nil:
		decision = resolve.DecisionFetchShadow
	case allCovered:
		decision = resolve.DecisionSkipFullHit
	default:
		// Entity policies expose the knob as EnablePartialCacheLoad;
		// root-field policies as PartialBatchLoad — either enables the batch
		// partial path. Shadow wins over partial (read-never-serve).
		if (cfg.EnablePartialCacheLoad || cfg.PartialBatchLoad) && !cfg.ShadowMode {
			// SOME buckets covered: mark the missing ones and go partial.
			// keep[i] mirrors the bucket order (which is the representations
			// order); the LOADER assembles the reduced input from the
			// already-rendered segments — no bytes are parsed back.
			keep := make([]bool, len(items))
			anyCovered := false
			for i := range items {
				keep[i] = items[i].FromCache == nil
				if !keep[i] {
					anyCovered = true
				}
			}
			if anyCovered {
				decision = resolve.DecisionFetchPartial
				batchFetchKeep = keep
			}
		}
	}
	handle := &resolve.FetchCacheHandle{
		Decision:       decision,
		WasHit:         allCovered,
		BatchEntityKey: true,
		Shadow:         shadowStash != nil,
		ShadowStash:    shadowStash,
		BatchFetchKeep: batchFetchKeep,
		Items:          items,
	}
	r.registerHandle(handle, state, in)
	return decision, handle
}

// prepareItems resolves the fetch's items in one pass: render every key, serve
// what L1 already covers, and resolve the remaining keys with a SINGLE store
// read. The returned states align positionally with items; an item whose key
// does not render is a plain miss — the fetch input could not have rendered
// either.
func (r *requestCache) prepareItems(tx *resolve.CacheTransaction, state *handleState, template cacheKeyTemplate, subgraph string, items []*astjson.Value) []resolve.ItemCacheState {
	cfg := state.cfg
	states := make([]resolve.ItemCacheState, len(items))
	// keys carries the batch read; pending[n] is the state keys[n] answers.
	var keys []string
	var pending []int
	for i, item := range items {
		states[i].Item = item
		rendered, ok := template.render(item)
		if !ok {
			continue
		}
		states[i].L1Key = rendered.L1
		states[i].RenderedKey = rendered.L2
		if rendered.L2 != "" {
			// Tags address store entries, so an L1-only fetch derives none. They
			// are taken here, from the same pre-merge item the keys rendered
			// from, which is what keeps a written and a served entry's tags
			// identical.
			states[i].Tags = template.entityTags(item)
		}
		if r.serveFromL1(tx, cfg, &states[i]) {
			continue
		}
		if rendered.L2 == "" {
			// L1-only: a miss is a plain fetch; there is no L2 to read.
			continue
		}
		keys = append(keys, rendered.L2)
		pending = append(pending, i)
	}
	if len(keys) == 0 {
		return states
	}
	entries := r.storeGetMany(subgraph, keys)
	for n, i := range pending {
		r.applyStoreEntry(tx, state, &states[i], entries[n])
	}
	return states
}

// The layers an item can be served from, as recorded on ItemCacheState for
// trace output — and read back by the client-answer fold, which counts a
// request-lifetime hit as no contribution at all.
const (
	servedFromL1 = "l1"
	servedFromL2 = "l2"
)

// serveFromL1 serves the item from the request-lifetime cache when a covering
// value (or the negative sentinel) sits under its RAW PREIMAGE key, which
// SHORT-CIRCUITS the store read: an L1 hit costs no hashing, no parsing, and no
// marshaling. Reports whether the item was served.
func (r *requestCache) serveFromL1(tx *resolve.CacheTransaction, cfg *resolve.FetchCacheConfig, state *resolve.ItemCacheState) bool {
	if !cfg.L1 {
		return false
	}
	stored := r.l1[state.L1Key]
	if stored == nil {
		return false
	}
	// A null is the L1 negative sentinel: the entity is KNOWN missing within
	// this request, so it serves without a coverage walk.
	negative := stored.Type() == astjson.TypeNull
	if !negative && !covers(r.ctx, stored, cfg.ProvidesData) {
		return false
	}
	state.FromCache = tx.StructuralCopy(stored)
	state.NegativeHit = negative
	state.ServedFromLayer = servedFromL1
	return true
}

// applyStoreEntry accepts (or rejects) ONE store read for its item: the
// negative sentinel serves as a known-missing entity, a covering value serves
// as a hit and populates L1, and everything else — miss, undecodable bytes, a
// foreign privacy scope, uncovered value — leaves the item a miss.
func (r *requestCache) applyStoreEntry(tx *resolve.CacheTransaction, state *handleState, item *resolve.ItemCacheState, entry Entry) {
	if !entry.OK {
		return
	}
	cfg := state.cfg
	envelope, ok := decodeEnvelope(tx, entry.Value)
	if !ok {
		// Undecodable stored bytes are treated as a miss; the write path
		// refreshes the entry.
		return
	}
	if !r.servableScope(cfg, envelope) {
		// The scope guard runs BEFORE the sentinel branch: a private "this
		// entity does not exist" is as requester-specific as a value.
		return
	}
	cached := envelope.Data
	switch {
	case cached.Type() == astjson.TypeNull:
		// A TOP-LEVEL null cached value is the negative sentinel: the entity is
		// KNOWN to not exist, so the item is served as null without a coverage
		// walk (there is nothing to cover).
		item.FromCache = cached
		item.NegativeHit = true
	case covers(r.ctx, cached, cfg.ProvidesData):
		// The served value stays in NORMALIZED (stored) form on the handle;
		// OnFetchSkipped denormalizes it to the requesting aliases at splice
		// time.
		item.FromCache = cached
	default:
		return
	}
	item.RemainingTTL = entry.RemainingTTL
	item.ServedFreshness = r.servedFreshness(envelope)
	item.ServedFromLayer = servedFromL2
	// Only SERVED values may enter the shared L1. Under shadow the selection is
	// a probe the stash clears before anything can serve it — leaking it into L1
	// would let a sibling L1 reader serve a value shadow mode never served.
	if cfg.L1 && !cfg.ShadowMode {
		r.l1Put(item.L1Key, tx.StructuralCopy(item.FromCache))
	}
}

// OnFetchSkipped splices the cached values into the merge targets at the
// surfaced merge path, inside one CacheTransaction; the per-target denormalize
// walk guards against aliasing when one cached value serves multiple targets.
// A served hit owes NO writes: the read key is the write key, so the entry it
// came from is already the one a write would target.
func (r *requestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil {
		return nil
	}
	state := r.states[h]
	if state == nil {
		return nil
	}
	cfg := state.cfg
	tx := in.Arena.Begin()
	defer tx.Commit()

	for i := range h.Items {
		item := &h.Items[i]
		r.foldServedItem(cfg, item)
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
		if err := r.spliceCachedItem(tx, cfg.ProvidesData, item, targets, in.MergePath); err != nil {
			return err
		}
	}
	return nil
}

// spliceCachedItem serves ONE covered item: splice the denormalized cached
// value into every merge target. Shared by OnFetchSkipped and the partial arm.
func (r *requestCache) spliceCachedItem(tx *resolve.CacheTransaction, provides *resolve.Object, item *resolve.ItemCacheState, targets []*astjson.Value, fetchMergePath []string) error {
	if item.FromCache == nil {
		return nil
	}
	if item.FromCache.Type() == astjson.TypeNull {
		// A negative hit splices NOTHING and writes nothing: a real
		// successful-but-empty entity fetch leaves the merge targets untouched
		// (mergeResult early-returns), and the resolvable then renders the
		// null bubble and its non-null error exactly as it would uncached.
		// Replacing the target with null here would make the cached response
		// DIFFER from the uncached one — caching must never change the
		// response.
		return nil
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
		if len(fetchMergePath) > 0 {
			if _, err := tx.MergeValuesWithPath(target, cached, fetchMergePath...); err != nil {
				return err
			}
		} else if _, err := tx.MergeValues(target, cached); err != nil {
			return err
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
	state := r.states[h]
	if state == nil {
		return nil
	}
	cfg := state.cfg
	if h.Decision == resolve.DecisionFetchPartial {
		// The partial arm owns splice + realign + writes and runs BEFORE the
		// failure gate: on a failed partial fetch the covered splice must
		// still happen (the cached data is valid; the loader already rendered
		// the fetch errors), and only the fetched subset is skipped.
		return r.onPartialBatchResult(h, in, state)
	}
	if in.FetchFailed || in.HasErrors {
		// All failure signals block ALL writes — including negative ones: a
		// transport/parse failure or errored response is a transient error,
		// never a proof of nonexistence (FetchFailed wins over EmptyEntity).
		// Nothing was stored, so the response may not be either.
		r.foldUncacheableFetch(cfg)
		return nil
	}
	decision := r.cachingDecisionFor(cfg, in)
	r.foldFetchedResult(h, in, decision)
	if in.EmptyEntity && in.ResponseData != nil && in.ResponseData.Type() == astjson.TypeNull {
		r.writeNegativeSentinel(h, in, decision)
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

	if cfg.KeySpec.Scope == resolve.CacheScopeRootField {
		// One whole-response value under the fetch's single key, written once
		// (the items share it).
		if decision.TTL <= 0 {
			return nil
		}
		toStore := in.ResponseData
		if cfg.ProvidesData != nil && cfg.ProvidesData.HasAliases {
			toStore = normalizeToSchema(tx, r.ctx, toStore, cfg.ProvidesData)
		}
		r.deferSet(h.Items[0].RenderedKey, toStore, decision.TTL, decision.Scope, h.Items[0].Tags)
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
	for itemIndex := range h.Items {
		item := &h.Items[itemIndex]
		itemToStore := in.ResponseData
		if h.BatchEntityKey {
			if item.BatchIndex < 0 || item.BatchIndex >= len(batch) {
				continue
			}
			itemToStore = batch[item.BatchIndex]
		}
		if len(in.MergePath) > 0 {
			// The response merges into the item at the merge path; the value to
			// cache is the entity BELOW that path, never the wrapper.
			entity := itemToStore.Get(in.MergePath...)
			if entity == nil {
				continue
			}
			itemToStore = entity
		}
		r.writeFetchedValue(tx, decision, state, item, itemToStore)
	}
	return nil
}

// writeNegativeSentinel caches the ONE non-failure that still writes: a
// SUCCESSFUL fetch that legitimately returned no entity, so repeated lookups
// for a nonexistent entity skip the network. The sentinel passes the same
// storability gates as a value — an emptiness answer can be no-store or
// requester-dependent too — but never takes its lifetime from max-age.
func (r *requestCache) writeNegativeSentinel(h *resolve.FetchCacheHandle, in resolve.MergeInput, decision cachingDecision) {
	if !decision.L1 && decision.NegativeTTL <= 0 {
		return
	}
	tx := in.Arena.Begin()
	defer tx.Commit()
	for i := range h.Items {
		item := &h.Items[i]
		item.FromCache = tx.Null()
		item.NegativeHit = true
		if decision.L1 && item.L1Key != "" {
			// Within the request the nonexistence is a fact — the L1 sentinel
			// needs no TTL knob.
			r.l1Put(item.L1Key, tx.Null())
		}
		if decision.NegativeTTL > 0 && item.RenderedKey != "" {
			// The sentinel describes no fields, so it stores no data. It carries
			// the entity's full tag set all the same: purging an entity must
			// clear its "does not exist" record too, or the purge would leave
			// the entity invisible until the sentinel expires.
			r.deferSet(item.RenderedKey, nil, decision.NegativeTTL, decision.Scope, item.Tags)
		}
	}
}

// writeFetchedValue caches one FRESH per-item value: normalize to the stored
// form (schema names + argument suffixes; HasAliases is the fast-path gate),
// then feed each layer the fetch result's storability decision permits, under
// its own key from that one rendering. A layer whose key did not render is
// skipped — the read and the write use the same key, so there is nothing to
// write it under. Shared by OnFetchResult and the partial arm.
func (r *requestCache) writeFetchedValue(tx *resolve.CacheTransaction, decision cachingDecision, state *handleState, item *resolve.ItemCacheState, itemToStore *astjson.Value) {
	if itemToStore == nil || itemToStore.Type() == astjson.TypeNull {
		// A null element is a MISSING entity, not a value: storing it would
		// fabricate a negative sentinel under the positive TTL (bypassing the
		// NegativeCacheTTL knob). Negative caching rides the loader's
		// whole-response EmptyEntity signal only.
		return
	}
	provides := state.cfg.ProvidesData
	toStore := itemToStore
	if provides != nil && provides.HasAliases {
		toStore = normalizeToSchema(tx, r.ctx, itemToStore, provides)
	}
	if decision.L1 && item.L1Key != "" {
		// write-L1 before write-L2; POINTER store, zero marshaling.
		r.l1Put(item.L1Key, tx.StructuralCopy(toStore))
	}
	if decision.TTL > 0 && item.RenderedKey != "" {
		r.deferSet(item.RenderedKey, toStore, decision.TTL, decision.Scope, item.Tags)
	}
}

// FlushTraces passes every not-yet-observed handle to the observer (trace
// assembly, counters) — single-threaded, no lock, no arena. The resolver calls
// it right before the response renders so extensions.trace carries the cache
// sections; EndRequest runs the same pass for whatever remains, so each handle
// is observed exactly once either way.
func (r *requestCache) FlushTraces() {
	if r.obs == nil {
		return
	}
	for h, state := range r.states {
		if state.observed {
			continue
		}
		state.observed = true
		r.obs.OnFetchObserved(h)
	}
}

// EndRequest flushes the request's deferred L2 writes in ONE store call —
// bytes only, no lock, no arena, no transaction — and finalizes observability.
// It runs once, single-threaded, after the root tree and every defer group have
// resolved.
func (r *requestCache) EndRequest() {
	// Finalize observability for handles not already flushed pre-render.
	r.FlushTraces()
	if len(r.deferred) > 0 {
		if err := r.store.SetMany(r.storeContext(), r.deferred); err != nil {
			// Dropped writes never reach the response: the values the request
			// rendered came from the subgraphs, and the next request repopulates
			// the entries. The flush spans every fetch of the request, so the
			// event carries no single subgraph.
			r.observeStoreError("SetMany", "", len(r.deferred), err)
		}
	}
	r.deferred = nil
	if r.obs != nil {
		r.obs.EndRequest(r.ctx)
	}
}

// deferSet queues ONE L2 write for the request-end flush. The value is encoded
// into the storage envelope here — with the TTL it is written under, the write
// moment, and the scope it belongs to — so only plain bytes cross into the
// deferred set. A nil data queues the negative sentinel. The tags ride on the
// Item, never inside the envelope: a private entry carries the same tags as its
// public counterpart would, so one purge reaches every partition of an entity.
func (r *requestCache) deferSet(key string, data *astjson.Value, ttl time.Duration, scope string, tags []string) {
	cc := cacheControl{
		TTL:     ttl,
		Created: time.Now(),
		Scope:   scope,
	}
	r.deferred = append(r.deferred, Item{
		Key:   key,
		Value: encodeEnvelope(data, cc),
		TTL:   ttl,
		Tags:  tags,
	})
	if r.aggregate != nil {
		// The queue is the single funnel every written entry passes, so the CDN
		// union collects exactly the entries the response really produced.
		r.aggregate.foldTags(tags)
	}
}
