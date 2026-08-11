package cache

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The client cache answer: ONE per request, folded per fetch and read after
// resolution through resolve.Context.CacheResponseInfo. Each fetch contributes
// the freshness the response may still be served under — what is LEFT of a
// served entry's lifetime, or the lifetime a fresh write got — and ZERO when
// it was not cacheable; one zero makes the whole response no-store. Two
// exceptions: an L1-served part contributes nothing (its originating fetch
// folded already), and a fetch with NO cache configuration is a zero that
// cannot stand alone — an operation the cache never touched has no policy
// rather than no-store.

// responseAggregate accumulates the client cache answer of one request.
//
// It guards itself with a mutex instead of riding the CacheTransaction lock the
// rest of the request state uses: the folds sit where a fetch's cacheability is
// DECIDED — the merge hooks' failure gates and the prepare give-ups included —
// and those points run outside any open transaction. The lock is uncontended in
// practice (one fold per fetch) and never held across arena work.
type responseAggregate struct {
	// emitCacheControl gates the freshness/privacy fold, emitTags the tag union;
	// an aggregate exists only while at least one of them is on.
	emitCacheControl bool
	emitTags         bool

	mu            sync.Mutex
	hasPolicy     bool
	noStore       bool
	uncachedFetch bool
	hasFreshness  bool
	freshness     time.Duration
	private       bool
	tags          map[string]struct{}
}

// fold folds ONE part into the answer: a non-positive freshness makes the whole
// response no-store, a positive one lowers the response's freshness to the
// minimum, and a private part marks the whole response private.
func (a *responseAggregate) fold(freshness time.Duration, private bool) {
	if !a.emitCacheControl {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hasPolicy = true
	a.private = a.private || private
	if freshness <= 0 {
		a.noStore = true
		return
	}
	if !a.hasFreshness || freshness < a.freshness {
		a.hasFreshness = true
		a.freshness = freshness
	}
}

// noteUncachedFetch records that one fetch of this response ran outside the
// cache. It is not a fold of its own: on its own it says nothing, and next to
// any cached part it makes the response no-store.
func (a *responseAggregate) noteUncachedFetch() {
	if !a.emitCacheControl {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.uncachedFetch = true
}

// foldTags adds one entry's invalidation tags to the response's union. The
// union is a set: entries written and served under the same tags — every
// variant of one entity, every fetch of one subgraph — collapse into one
// purgeable address.
func (a *responseAggregate) foldTags(tags []string) {
	if !a.emitTags || len(tags) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tags == nil {
		a.tags = make(map[string]struct{}, len(tags))
	}
	for _, tag := range tags {
		a.tags[tag] = struct{}{}
	}
}

// info renders the accumulated answer. The tag union is SORTED here, at read
// time, so one response's header is byte-identical however its fetches
// interleaved.
func (a *responseAggregate) info() resolve.CacheResponseInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	info := resolve.CacheResponseInfo{
		HasPolicy: a.hasPolicy,
		Private:   a.private,
		// The uncached-fetch zero answers only once a cached part gave the
		// response something to restrict; on its own there is nothing to say.
		NoStore: a.hasPolicy && (a.noStore || a.uncachedFetch),
	}
	if !info.NoStore && a.hasFreshness {
		info.MaxAge = a.freshness
	}
	if len(a.tags) > 0 {
		info.Tags = slices.Sorted(maps.Keys(a.tags))
	}
	return info
}

// CacheResponseInfo implements resolve.CacheResponseInfoSource: the request's
// client cache answer, readable after resolution — including after EndRequest,
// which touches nothing this reads.
func (r *requestCache) CacheResponseInfo() resolve.CacheResponseInfo {
	if r.aggregate == nil {
		return resolve.CacheResponseInfo{}
	}
	return r.aggregate.info()
}

// OnUncachedFetch records one fetch of this request that ran with no cache
// configuration. It is the hook that may open a request — the loader calls it
// before the load — and it costs nothing while no client answer is computed.
func (r *requestCache) OnUncachedFetch() {
	if r.aggregate == nil {
		return
	}
	r.aggregate.noteUncachedFetch()
}

// foldServedItem folds ONE item served from a store entry: what is left of the
// entry's lifetime bounds the response, and its tags join the CDN union. An
// item served from the request-lifetime layer is skipped — see the package
// comment above on why that is not a zero contribution. An entry the clock has
// already caught up with contributes zero and makes the response no-store:
// freshness that is gone promises nothing.
func (r *requestCache) foldServedItem(cfg *resolve.FetchCacheConfig, item *resolve.ItemCacheState) {
	if r.aggregate == nil || item.FromCache == nil || item.ServedFromLayer == servedFromL1 {
		return
	}
	r.aggregate.fold(item.ServedFreshness, cfg.Private)
	r.aggregate.foldTags(item.Tags)
}

// foldFetchedResult folds ONE fresh subgraph result: the lifetime its entries
// were stored under. The tags of those entries join the union as they are
// queued, in deferSet — exactly the entries that were really written.
func (r *requestCache) foldFetchedResult(h *resolve.FetchCacheHandle, in resolve.MergeInput, decision cachingDecision) {
	if r.aggregate == nil {
		return
	}
	r.aggregate.fold(fetchedFreshness(h, in, decision), decision.Private)
}

// foldUncacheableFetch folds a cache-CONFIGURED fetch whose data reached the
// response without landing in any entry: the controller declined it outright
// (no handle — no derivable key, or a private coordinate without a requester
// identity), or its result failed and blocked every write.
func (r *requestCache) foldUncacheableFetch(cfg *resolve.FetchCacheConfig) {
	if r.aggregate == nil {
		return
	}
	r.aggregate.fold(0, cfg.Private)
}

// servedFreshness is what is left of a served entry's lifetime, computed from
// the entry's OWN record (created + ttl - now) rather than from the store's
// remaining-TTL report, which backends may not keep. Zero — and unread — when
// no client answer is being computed.
func (r *requestCache) servedFreshness(envelope storedEnvelope) time.Duration {
	if r.aggregate == nil {
		return 0
	}
	return time.Until(envelope.Created.Add(envelope.TTL))
}

// fetchedFreshness is the freshness one fresh result contributes: the lifetime
// its entries were written under, or ZERO when the result did not become
// entries at all — a failed or empty response, a storability decision that
// permits no write, or a fetch whose items did not all render a store key
// (an L1-only fetch, an unidentified private one, an unrenderable item).
func fetchedFreshness(h *resolve.FetchCacheHandle, in resolve.MergeInput, decision cachingDecision) time.Duration {
	ttl := decision.TTL
	if in.ResponseData == nil || in.ResponseData.Type() == astjson.TypeNull {
		// A response with no data becomes an entry in exactly one case: a fetch
		// that legitimately resolved no entity writes the negative sentinel.
		ttl = 0
		if in.EmptyEntity && in.ResponseData != nil {
			ttl = decision.NegativeTTL
		}
	}
	if ttl <= 0 {
		return 0
	}
	for i := range h.Items {
		if h.Items[i].RenderedKey == "" {
			return 0
		}
	}
	return ttl
}
