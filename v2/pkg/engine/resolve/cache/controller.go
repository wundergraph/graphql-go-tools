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
		store:   c.store,
		mode:    c.mode,
		obs:     c.obs,
		ctx:     ctx,
		configs: make(map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig),
	}
}

type requestCache struct {
	store    Store
	mode     Mode
	obs      resolve.CacheObserver
	ctx      *resolve.Context
	deferred []deferredSet
	configs  map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig
	// Mutated only under the MergeSession's DataBuffer.Lock per RFC-1 section 6.4.
}

type deferredSet struct {
	key   string
	value []byte
	ttl   time.Duration
}

func (r *requestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	if r.mode != ModeL2 || r.store == nil || in.Config == nil || !in.Config.L2 {
		// TODO(D2): implement L1 and L1+L2 modes.
		return resolve.DecisionFetch, nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	cfg := in.Config
	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	allCovered := len(in.Items) > 0
	prefix := cacheKeyPrefix(cfg, in.HeaderHash)
	for _, item := range in.Items {
		state := resolve.ItemCacheState{Item: item}
		key, ok := renderFirstCandidateKey(session, cfg, item, prefix)
		if ok {
			state.RenderedKeys = []string{key}
			value, remaining, hit := r.store.Get(key)
			if hit && cfg.ProvidesData != nil {
				cached, err := session.ParseBytes(value)
				if err == nil && coversWithContext(in.Ctx, cached, cfg.ProvidesData) {
					state.FromCache = cached
					state.SelectedRemainingTTL = remaining
				}
			}
		}
		if state.FromCache == nil {
			allCovered = false
		}
		items = append(items, state)
	}

	decision := resolve.DecisionFetch
	if allCovered {
		decision = resolve.DecisionSkipFullHit
	}
	handle := &resolve.FetchCacheHandle{
		Decision: decision,
		WasHit:   allCovered,
		Items:    items,
	}
	r.configs[handle] = cfg
	return decision, handle
}

func (r *requestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil || r.mode != ModeL2 {
		return nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	for _, item := range h.Items {
		if item.Item == nil || item.FromCache == nil || item.FromCache.Type() == astjson.TypeNull {
			continue
		}
		cached := session.StructuralCopy(item.FromCache)
		if len(item.EntityMergePath) > 0 {
			if _, err := session.MergeValuesWithPath(item.Item, cached, item.EntityMergePath...); err != nil {
				return err
			}
			continue
		}
		if _, err := session.MergeValues(item.Item, cached); err != nil {
			return err
		}
	}
	// TODO(A2b): best-effort multi-candidate read-hit backfill/writeback.
	return nil
}

func (r *requestCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil || r.mode != ModeL2 {
		return nil
	}
	session := in.Arena.Begin()
	defer session.Close()

	if in.FetchFailed || in.HasErrors || in.ResponseData == nil || in.ResponseData.Type() == astjson.TypeNull {
		return nil
	}
	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	ttl := cfg.TTL
	if cfg.PopulateL2OnMutation && cfg.MutationTTLOverride > 0 {
		ttl = cfg.MutationTTLOverride
	}
	for _, item := range h.Items {
		if len(item.RenderedKeys) == 0 {
			// TODO(A2b): re-render pending candidates from the fresh response.
			continue
		}
		itemToStore := in.ResponseData
		if len(item.EntityMergePath) > 0 {
			if entity := in.ResponseData.Get(item.EntityMergePath...); entity != nil {
				itemToStore = entity
			}
		}
		copied := session.StructuralCopy(itemToStore)
		bytes := append([]byte(nil), copied.MarshalTo(nil)...)
		r.deferred = append(r.deferred, deferredSet{
			key:   item.RenderedKeys[0],
			value: bytes,
			ttl:   ttl,
		})
	}
	// TODO(A3/A4): negative caching and shadow compare/write ordering.
	return nil
}

func (r *requestCache) EndRequest() {
	for _, set := range r.deferred {
		r.store.Set(set.key, set.value, set.ttl)
	}
	r.deferred = nil
	if r.obs != nil {
		r.obs.EndRequest(r.ctx)
	}
}

func renderFirstCandidateKey(session resolve.MergeSession, cfg *resolve.FetchCacheConfig, item *astjson.Value, prefix string) (string, bool) {
	if len(cfg.KeySpec.Candidates) == 0 {
		return "", false
	}
	return renderEntityKey(session, cfg.KeySpec.Candidates[0].Representation, item, prefix)
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
