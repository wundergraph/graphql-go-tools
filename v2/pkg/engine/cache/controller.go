package cache

import (
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
		store:   c.store,
		obs:     c.obs,
		ctx:     ctx,
		configs: make(map[*resolve.FetchCacheHandle]*resolve.FetchCacheConfig),
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
}

// deferredSet is one pending L2 write, held as bytes until EndRequest.
type deferredSet struct {
	key   string
	value []byte
	ttl   time.Duration
}

// useL2 reports whether this fetch participates in L2 through this controller.
func (r *requestCache) useL2(cfg *resolve.FetchCacheConfig) bool {
	return cfg != nil && cfg.L2 && r.store != nil
}

// PrepareFetch renders the candidate key from the item data, looks L2 up,
// runs the always-on coverage walk, and AND-reduces per-item hits into the
// decision. It opens exactly one CacheTransaction for all arena work.
func (r *requestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	cfg := in.Config
	if !r.useL2(cfg) {
		return resolve.DecisionFetch, nil
	}
	if cfg.ShadowMode {
		// Shadow mode (read-never-serve) lands with task 12; until then a
		// shadow-configured fetch behaves as a plain miss so no cached value
		// can ever be served.
		return resolve.DecisionFetch, nil
	}
	if cfg.KeySpec.Scope != resolve.CacheScopeEntity {
		// Root-field caching lands with task 13.
		return resolve.DecisionFetch, nil
	}
	if in.BatchStats != nil {
		// Batch entity caching lands with task 10.
		return resolve.DecisionFetch, nil
	}
	if len(in.Items) == 0 {
		return resolve.DecisionFetch, nil
	}
	templates := newCacheKeyTemplates(cfg, in.HeaderHash)
	if len(templates) == 0 {
		return resolve.DecisionFetch, nil
	}
	// Task 07 handles the single-candidate case; multi-key selection lands
	// with task 08.
	template := templates[0]

	tx := in.Arena.Begin()
	defer tx.Commit()

	items := make([]resolve.ItemCacheState, 0, len(in.Items))
	allCovered := true
	for _, item := range in.Items {
		state := resolve.ItemCacheState{Item: item}
		key, ok := template.render(item)
		if ok {
			state.RenderedKeys = []string{key}
			if value, remaining, hit := r.store.Get(key); hit {
				// Parse the cached bytes ONCE, onto the transaction's arena.
				if cached, err := tx.ParseBytes(value); err == nil {
					state.FromCacheCandidates = []resolve.CacheCandidate{{
						Value:        append([]byte(nil), value...),
						RemainingTTL: remaining,
					}}
					if covers(cached, cfg.ProvidesData) {
						state.FromCache = cached
						state.SelectedRemainingTTL = remaining
					}
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

// OnFetchSkipped splices the chosen cached values into the merge targets at
// the surfaced merge path, inside one CacheTransaction; StructuralCopy guards
// against aliasing when one cached value serves multiple targets.
func (r *requestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if h == nil || r.configs[h] == nil {
		return nil
	}
	tx := in.Arena.Begin()
	defer tx.Commit()

	for _, item := range h.Items {
		if item.FromCache == nil || item.Item == nil {
			continue
		}
		cached := tx.StructuralCopy(item.FromCache)
		if cached.Type() == astjson.TypeNull {
			// Negative sentinels land with task 11; a null value has nothing
			// to splice.
			continue
		}
		if len(in.MergePath) > 0 {
			if _, err := tx.MergeValuesWithPath(item.Item, cached, in.MergePath...); err != nil {
				return err
			}
		} else if _, err := tx.MergeValues(item.Item, cached); err != nil {
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
	cfg := r.configs[h]
	if cfg == nil {
		return nil
	}
	if in.FetchFailed || in.HasErrors {
		return nil
	}
	if in.ResponseData == nil || in.ResponseData.Type() == astjson.TypeNull {
		// Includes the EmptyEntity case for now; the negative-cache sentinel
		// write lands with task 11.
		return nil
	}
	tx := in.Arena.Begin()
	defer tx.Commit()

	for _, item := range h.Items {
		itemToStore := in.ResponseData
		if len(in.MergePath) > 0 {
			// The response merges into the item at the merge path; the value to
			// cache is the entity BELOW that path (D4), never the wrapper.
			entity := itemToStore.Get(in.MergePath...)
			if entity == nil {
				continue
			}
			itemToStore = entity
		}
		// Marshal ONCE per item; the deferred set holds bytes only.
		value := itemToStore.MarshalTo(nil)
		for _, key := range item.RenderedKeys {
			r.deferSet(key, value, cfg.TTL)
		}
	}
	return nil
}

// EndRequest flushes the deferred L2 writes — bytes only, no lock, no arena,
// no transaction — and finalizes observability. It runs once, single-threaded,
// after the root tree and every defer group have resolved.
func (r *requestCache) EndRequest() {
	for _, set := range r.deferred {
		r.store.Set(set.key, set.value, set.ttl)
	}
	r.deferred = nil
	if r.obs != nil {
		r.obs.EndRequest(r.ctx)
	}
}

func (r *requestCache) deferSet(key string, value []byte, ttl time.Duration) {
	r.deferred = append(r.deferred, deferredSet{
		key:   key,
		value: append([]byte(nil), value...),
		ttl:   ttl,
	})
}
