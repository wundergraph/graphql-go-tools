package cache

import (
	"cmp"
	"slices"
	"time"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// Store is the L2 backend the controller talks to. Get returns the value and its
// remaining TTL; ok=false on miss/expiry.
type Store interface {
	Get(key string) (value []byte, remainingTTL time.Duration, ok bool)
	Set(key string, value []byte, ttl time.Duration)
}

type Mode uint8

const (
	ModeNoop Mode = iota
	ModeL1
	ModeL2
	ModeL1L2
)

type Controller struct {
	store Store
	mode  Mode
	obs   resolve.CacheObserver
}

func NewController(store Store, mode Mode, obs resolve.CacheObserver) *Controller {
	return &Controller{store: store, mode: mode, obs: obs}
}

func (c *Controller) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	if c.obs != nil {
		c.obs.BeginRequest(ctx)
	}
	return &requestCache{
		store:              c.store,
		mode:               c.mode,
		obs:                c.obs,
		ctx:                ctx,
		configs:            make(map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig),
		prefixes:           make(map[*resolve.FetchCacheHandle]string),
		renderedMissedKeys: make(map[*resolve.FetchCacheHandle][][]string),
	}
}

type requestCache struct {
	store              Store
	mode               Mode
	obs                resolve.CacheObserver
	ctx                *resolve.Context
	deferred           []deferredSet
	configs            map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig
	prefixes           map[*resolve.FetchCacheHandle]string
	renderedMissedKeys map[*resolve.FetchCacheHandle][][]string
	// Mutated only under the MergeSession's DataBuffer.Lock per RFC-1 section 6.4.
}

type deferredSet struct {
	key    string
	value  []byte
	ttl    time.Duration
	reason resolve.CacheWriteReason
}

const negativeCacheSentinel = "null"

func (r *requestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	if r.mode != ModeL2 || r.store == nil || in.Config == nil || !in.Config.L2 {
		// TODO(D2): implement L1 and L1+L2 modes.
		return resolve.DecisionFetch, nil
	}
	if in.BatchStats != nil && len(in.BatchStats) == 0 {
		return resolve.DecisionFetch, nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	cfg := in.Config
	prefix := cacheKeyPrefix(cfg, in.HeaderHash)
	if len(in.BatchStats) > 0 {
		items := make([]resolve.ItemCacheState, 0, len(in.BatchStats))
		missedByItem := make([][]string, 0, len(in.BatchStats))
		allCovered := true
		mustWriteBack := false
		for i, bucket := range in.BatchStats {
			var representative *astjson.Value
			if len(bucket) > 0 {
				representative = bucket[0]
			}
			state, missedKeys, itemMustWriteBack := r.prepareItemCacheState(in.Ctx, session, cfg, representative, prefix)
			state.BatchEntityKey = true
			state.BatchIndex = i
			if state.FromCache == nil {
				allCovered = false
			}
			if itemMustWriteBack {
				mustWriteBack = true
			}
			items = append(items, state)
			missedByItem = append(missedByItem, missedKeys)
		}
		decision := resolve.DecisionFetch
		if allCovered {
			decision = resolve.DecisionSkipFullHit
		}
		handle := &resolve.FetchCacheHandle{
			Decision:       decision,
			WasHit:         allCovered,
			MustWriteBack:  allCovered && mustWriteBack,
			BatchEntityKey: true,
			Items:          items,
		}
		r.configs[handle] = cfg
		r.prefixes[handle] = prefix
		r.renderedMissedKeys[handle] = missedByItem
		return decision, handle
	}

	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	missedByItem := make([][]string, 0, len(in.Items))
	allCovered := len(in.Items) > 0
	mustWriteBack := false
	for _, item := range in.Items {
		state, missedKeys, itemMustWriteBack := r.prepareItemCacheState(in.Ctx, session, cfg, item, prefix)
		if state.FromCache == nil {
			allCovered = false
		}
		if itemMustWriteBack {
			mustWriteBack = true
		}
		items = append(items, state)
		missedByItem = append(missedByItem, missedKeys)
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
	r.prefixes[handle] = prefix
	r.renderedMissedKeys[handle] = missedByItem
	return decision, handle
}

func (r *requestCache) prepareItemCacheState(ctx *resolve.Context, session resolve.MergeSession, cfg *resolve.FetchCacheConfig, item *astjson.Value, prefix string) (resolve.ItemCacheState, []string, bool) {
	state := resolve.ItemCacheState{Item: item}
	missedKeys := make([]string, 0, len(cfg.KeySpec.Candidates))
	mustWriteBack := false
	for _, candidate := range cfg.KeySpec.Candidates {
		key, ok := renderEntityKey(session, candidate.Representation, item, prefix)
		if !ok {
			state.PendingCandidates = append(state.PendingCandidates, candidate)
			mustWriteBack = true
			continue
		}
		state.RenderedKeys = append(state.RenderedKeys, key)
		value, remaining, hit := r.store.Get(key)
		if !hit {
			missedKeys = append(missedKeys, key)
			mustWriteBack = true
			continue
		}
		state.FromCacheCandidates = append(state.FromCacheCandidates, resolve.CacheCandidate{
			Value:        append([]byte(nil), value...),
			RemainingTTL: remaining,
		})
		cached, err := session.ParseBytes(value)
		if err == nil && cached.Type() == astjson.TypeNull && state.FromCache == nil {
			// v1 uses a literal JSON null as the negative-cache sentinel.
			state.FromCache = cached
			state.SelectedRemainingTTL = remaining
			state.NegativeHit = true
		}
	}
	if len(state.FromCacheCandidates) > 0 {
		slices.SortStableFunc(state.FromCacheCandidates, func(a, b resolve.CacheCandidate) int {
			return compareCacheCandidateFreshness(a.RemainingTTL, b.RemainingTTL)
		})
		if state.NegativeHit {
			// Negative hits are already covering values and must not run the positive
			// ProvidesData coverage walk.
		} else if selectMultiCandidateCacheValue(ctx, session, &state, cfg.ProvidesData) {
			state.FromCache = reorderCacheValueToSelectionOrder(ctx, session, state.FromCache, cfg.ProvidesData)
		}
		if state.NeedsWriteback {
			mustWriteBack = true
		}
	}
	return state, missedKeys, mustWriteBack
}

func (r *requestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil || r.mode != ModeL2 {
		return nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	ttl := ttlForConfig(cfg)
	prefix := r.prefixes[h]
	missedByItem := r.renderedMissedKeys[h]
	for i, item := range h.Items {
		if item.FromCache == nil {
			continue
		}
		targets := []*astjson.Value{item.Item}
		if h.BatchEntityKey {
			targets = nil
			if item.BatchIndex >= 0 && item.BatchIndex < len(in.BatchStats) {
				targets = in.BatchStats[item.BatchIndex]
			}
		}
		for _, target := range targets {
			if target == nil {
				continue
			}
			cached := session.StructuralCopy(item.FromCache)
			if item.NegativeHit && cached.Type() == astjson.TypeNull {
				*target = *cached
				continue
			}
			if cached.Type() == astjson.TypeNull {
				continue
			}
			if len(item.EntityMergePath) > 0 {
				if _, err := session.MergeValuesWithPath(target, cached, item.EntityMergePath...); err != nil {
					return err
				}
			} else if _, err := session.MergeValues(target, cached); err != nil {
				return err
			}
		}
		bytes := append([]byte(nil), item.FromCache.MarshalTo(nil)...)
		if item.NeedsWriteback {
			for _, key := range item.RenderedKeys {
				r.deferSet(key, bytes, ttl, resolve.CacheWriteReasonRefresh)
			}
		} else if i < len(missedByItem) {
			for _, key := range missedByItem[i] {
				r.deferSet(key, bytes, ttl, resolve.CacheWriteReasonBackfill)
			}
		}
		for _, candidate := range item.PendingCandidates {
			key, ok := renderEntityKey(session, candidate.Representation, item.FromCache, prefix)
			if !ok {
				key, ok = renderEntityKey(session, candidate.Representation, item.Item, prefix)
			}
			if ok {
				r.deferSet(key, bytes, ttl, resolve.CacheWriteReasonBackfill)
			}
		}
	}
	return nil
}

func (r *requestCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil || r.mode != ModeL2 {
		return nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	if in.FetchFailed || in.HasErrors {
		return nil
	}
	if in.EmptyEntity && in.ResponseData != nil && in.ResponseData.Type() == astjson.TypeNull {
		if cfg.NegativeCacheTTL <= 0 {
			return nil
		}
		nullValue := session.Null()
		for i := range h.Items {
			h.Items[i].FromCache = nullValue
			h.Items[i].NegativeHit = true
			for _, key := range h.Items[i].RenderedKeys {
				r.deferSet(key, []byte(negativeCacheSentinel), cfg.NegativeCacheTTL, resolve.CacheWriteReasonRefresh)
			}
		}
		return nil
	}
	if in.ResponseData == nil || in.ResponseData.Type() == astjson.TypeNull {
		return nil
	}
	ttl := ttlForConfig(cfg)
	prefix := r.prefixes[h]
	var batch []*astjson.Value
	if h.BatchEntityKey {
		batch = in.ResponseData.GetArray()
		if batch == nil {
			return nil
		}
	}
	for _, item := range h.Items {
		itemToStore := in.ResponseData
		if h.BatchEntityKey {
			if item.BatchIndex < 0 || item.BatchIndex >= len(batch) {
				continue
			}
			itemToStore = batch[item.BatchIndex]
		}
		if len(item.EntityMergePath) > 0 {
			if entity := itemToStore.Get(item.EntityMergePath...); entity != nil {
				itemToStore = entity
			}
		}
		copied := session.StructuralCopy(itemToStore)
		bytes := append([]byte(nil), copied.MarshalTo(nil)...)
		for _, key := range item.RenderedKeys {
			r.deferSet(key, bytes, ttl, resolve.CacheWriteReasonRefresh)
		}
		for _, candidate := range item.PendingCandidates {
			key, ok := renderEntityKey(session, candidate.Representation, itemToStore, prefix)
			if ok {
				r.deferSet(key, bytes, ttl, resolve.CacheWriteReasonBackfill)
			}
		}
	}
	// TODO(A3/A4): negative caching and shadow compare/write ordering.
	return nil
}

func (r *requestCache) EndRequest() {
	for _, set := range r.deferred {
		if recorder, ok := r.store.(writeReasonRecorder); ok {
			recorder.RecordWriteReason(set.key, set.reason)
		}
		r.store.Set(set.key, set.value, set.ttl)
	}
	r.deferred = nil
	if r.obs != nil {
		r.obs.EndRequest(r.ctx)
	}
}

type writeReasonRecorder interface {
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

func ttlForConfig(cfg *resolve.FetchCacheConfig) time.Duration {
	if cfg.PopulateL2OnMutation && cfg.MutationTTLOverride > 0 {
		return cfg.MutationTTLOverride
	}
	return cfg.TTL
}

func renderFirstCandidateKey(session resolve.MergeSession, cfg *resolve.FetchCacheConfig, item *astjson.Value, prefix string) (string, bool) {
	if len(cfg.KeySpec.Candidates) == 0 {
		return "", false
	}
	return renderEntityKey(session, cfg.KeySpec.Candidates[0].Representation, item, prefix)
}

func selectMultiCandidateCacheValue(ctx *resolve.Context, session resolve.MergeSession, state *resolve.ItemCacheState, providesData *resolve.Object) bool {
	if len(state.FromCacheCandidates) == 0 || providesData == nil {
		return false
	}
	parsed := make([]*astjson.Value, len(state.FromCacheCandidates))
	for i, candidate := range state.FromCacheCandidates {
		value, err := session.ParseBytes(candidate.Value)
		if err != nil {
			continue
		}
		parsed[i] = value
	}
	if parsed[0] != nil && coversWithContext(ctx, parsed[0], providesData) {
		state.FromCache = parsed[0]
		state.SelectedRemainingTTL = state.FromCacheCandidates[0].RemainingTTL
		return true
	}
	if len(state.FromCacheCandidates) <= 1 {
		return false
	}

	var merged *astjson.Value
	for i := len(parsed) - 1; i >= 0; i-- {
		if parsed[i] == nil {
			continue
		}
		current := session.StructuralCopy(parsed[i])
		if merged == nil {
			merged = current
			continue
		}
		if _, err := session.MergeValues(merged, current); err != nil {
			merged = nil
			break
		}
	}
	if merged != nil && coversWithContext(ctx, merged, providesData) {
		state.FromCache = merged
		state.SelectedRemainingTTL = state.FromCacheCandidates[0].RemainingTTL
		state.NeedsWriteback = true
		return true
	}

	for i := 1; i < len(parsed); i++ {
		if parsed[i] == nil {
			continue
		}
		if coversWithContext(ctx, parsed[i], providesData) {
			state.FromCache = parsed[i]
			state.SelectedRemainingTTL = state.FromCacheCandidates[i].RemainingTTL
			state.NeedsWriteback = true
			return true
		}
	}
	return false
}

func compareCacheCandidateFreshness(a, b time.Duration) int {
	aKnown := a > 0
	bKnown := b > 0
	switch {
	case aKnown && bKnown:
		return cmp.Compare(b, a)
	case aKnown:
		return -1
	case bKnown:
		return 1
	default:
		return 0
	}
}

func reorderCacheValueToSelectionOrder(ctx *resolve.Context, session resolve.MergeSession, value *astjson.Value, node resolve.Node) *astjson.Value {
	if value == nil || node == nil {
		return value
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return value
		}
		reordered := session.NewObject()
		seen := make(map[string]struct{}, len(typed.Fields))
		for _, field := range typed.Fields {
			fieldName := cacheFieldName(ctx, field)
			fieldValue := value.Get(fieldName)
			if fieldValue == nil {
				continue
			}
			reordered.Set(nil, fieldName, reorderCacheValueToSelectionOrder(ctx, session, fieldValue, field.Value))
			seen[fieldName] = struct{}{}
		}
		obj, err := value.Object()
		if err != nil {
			return value
		}
		obj.Visit(func(key []byte, fieldValue *astjson.Value) {
			fieldName := string(key)
			if _, ok := seen[fieldName]; ok {
				return
			}
			reordered.Set(nil, fieldName, fieldValue)
		})
		return reordered
	case *resolve.Array:
		if value.Type() != astjson.TypeArray {
			return value
		}
		items, err := value.Array()
		if err != nil {
			return value
		}
		reordered := session.NewArray()
		for i, item := range items {
			reordered.SetArrayItem(nil, i, reorderCacheValueToSelectionOrder(ctx, session, item, typed.Item))
		}
		return reordered
	default:
		return value
	}
}

// cacheKeyPrefix returns the visible key prefix. The final store key is
// "<prefix>:<16-hex xxhash64>", hashing "<prefix>:<rendered entity-key JSON>".
func cacheKeyPrefix(cfg *resolve.FetchCacheConfig, headerHash uint64) string {
	if cfg == nil {
		return ""
	}
	if cfg.IncludeSubgraphHeaderPrefix {
		return cfg.CacheName + ":h" + hex64(headerHash)
	}
	return cfg.CacheName
}

func renderEntityKey(session resolve.MergeSession, representation *resolve.Object, item *astjson.Value, prefix string) (string, bool) {
	_ = session
	if representation == nil || item == nil {
		return "", false
	}
	typename := item.Get("__typename")
	if typename == nil {
		if representation.TypeName == "" {
			return "", false
		}
		typename = astjson.StringValue(nil, representation.TypeName)
	}
	keyObj := astjson.ObjectValue(nil)
	keyObj.Set(nil, "__typename", typename)
	keysObj := astjson.ObjectValue(nil)
	renderedFields := 0
	for _, field := range representation.Fields {
		name := string(field.Name)
		if name == "__typename" {
			continue
		}
		value, ok := renderRepresentationValue(field.Value, item.Get(name))
		if !ok {
			return "", false
		}
		keysObj.Set(nil, name, value)
		renderedFields++
	}
	if renderedFields == 0 {
		return "", false
	}
	keyObj.Set(nil, "key", keysObj)
	jsonBytes := keyObj.MarshalTo(nil)
	preimage := make([]byte, 0, len(prefix)+1+len(jsonBytes))
	if prefix != "" {
		preimage = append(preimage, prefix...)
	}
	preimage = append(preimage, ':')
	preimage = append(preimage, jsonBytes...)
	return prefix + ":" + hashHex(preimage), true
}

func renderRepresentationValue(node resolve.Node, value *astjson.Value) (*astjson.Value, bool) {
	if value == nil || value.Type() == astjson.TypeNull {
		return nil, false
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return nil, false
		}
		out := astjson.ObjectValue(nil)
		rendered := 0
		for _, field := range typed.Fields {
			name := string(field.Name)
			if name == "__typename" {
				continue
			}
			child, ok := renderRepresentationValue(field.Value, value.Get(name))
			if !ok {
				return nil, false
			}
			out.Set(nil, name, child)
			rendered++
		}
		return out, rendered > 0
	default:
		if value.Type() == astjson.TypeNumber {
			return astjson.StringValue(nil, string(value.MarshalTo(nil))), true
		}
		return value, true
	}
}

func coversWithContext(ctx *resolve.Context, value *astjson.Value, obj *resolve.Object) bool {
	if value == nil || obj == nil {
		return false
	}
	for _, field := range obj.Fields {
		fieldValue := value.Get(cacheFieldName(ctx, field))
		if fieldValue == nil {
			return false
		}
		if !coversNode(ctx, fieldValue, field.Value) {
			return false
		}
	}
	return true
}

func coversNode(ctx *resolve.Context, value *astjson.Value, node resolve.Node) bool {
	switch typed := node.(type) {
	case *resolve.Scalar:
		return value.Type() != astjson.TypeNull || typed.Nullable
	case *resolve.Object:
		if value.Type() == astjson.TypeNull {
			return typed.Nullable
		}
		if value.Type() != astjson.TypeObject {
			return false
		}
		return coversWithContext(ctx, value, typed)
	case *resolve.Array:
		if value.Type() == astjson.TypeNull {
			return typed.Nullable
		}
		if value.Type() != astjson.TypeArray {
			return false
		}
		if typed.Item == nil {
			return true
		}
		items, err := value.Array()
		if err != nil {
			return false
		}
		for _, item := range items {
			if !coversNode(ctx, item, typed.Item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func cacheFieldName(ctx *resolve.Context, field *resolve.Field) string {
	name := string(field.Name)
	if len(field.OriginalName) > 0 {
		name = string(field.OriginalName)
	}
	if len(field.CacheArgs) == 0 {
		return name
	}
	return name + computeArgSuffix(ctx, field.CacheArgs)
}

func computeArgSuffix(ctx *resolve.Context, args []resolve.CacheFieldArg) string {
	sorted := args
	if !slices.IsSortedFunc(sorted, func(a, b resolve.CacheFieldArg) int {
		return cmp.Compare(a.Name, b.Name)
	}) {
		sorted = slices.Clone(args)
		slices.SortFunc(sorted, func(a, b resolve.CacheFieldArg) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	h := pool.Hash64.Get()
	for i, arg := range sorted {
		if i > 0 {
			_, _ = h.WriteString(",")
		}
		_, _ = h.WriteString(arg.Name)
		_, _ = h.WriteString(":")
		var value *astjson.Value
		if ctx != nil && ctx.Variables != nil {
			variableName := arg.VariableName
			if ctx.RemapVariables != nil {
				if mapped, ok := ctx.RemapVariables[variableName]; ok {
					variableName = mapped
				}
			}
			value = ctx.Variables.Get(variableName)
		}
		if value == nil {
			_, _ = h.WriteString("null")
		} else {
			_, _ = h.Write(value.MarshalTo(nil))
		}
	}
	sum := h.Sum64()
	pool.Hash64.Put(h)
	return "_" + hex64(sum)
}

func hashHex(value []byte) string {
	h := pool.Hash64.Get()
	_, _ = h.Write(value)
	sum := h.Sum64()
	pool.Hash64.Put(h)
	return hex64(sum)
}

func hex64(sum uint64) string {
	var buf [16]byte
	const digits = "0123456789abcdef"
	for i := 15; i >= 0; i-- {
		buf[i] = digits[sum&0xf]
		sum >>= 4
	}
	return string(buf[:])
}
