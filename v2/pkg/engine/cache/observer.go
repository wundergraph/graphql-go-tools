package cache

import (
	"maps"
	"slices"
	"sync"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TraceObserver is the production CacheObserver: it accumulates per-fetch
// cache trace from the opaque FetchCacheHandle and attaches the assembled
// CacheTrace to the fetch's ART trace at request end. The observer is
// composed INSIDE the controller — the loader never calls it — and a nil
// observer on the controller records nothing at zero cost. OnEntity and
// OnFieldValue are deliberate no-ops.
type TraceObserver struct {
	// mu guards compares and storeErrors: CompareShadow runs inside per-request
	// hook transactions, and one TraceObserver instance serves MANY concurrent
	// requests.
	mu          sync.Mutex
	compares    map[*resolve.FetchCacheHandle][]resolve.CacheShadowCompareTrace
	storeErrors []StoreError
	// uncacheablePrivate records the private results the store never received,
	// one entry per occurrence.
	uncacheablePrivate []UncacheablePrivate
	// scopeMismatches counts the entries discarded because their recorded scope
	// is not the one the reading fetch keys under, per subgraph and per STORED
	// scope — the two directions of configuration drift.
	scopeMismatches map[ScopeMismatch]int
}

// UncacheablePrivate is one private result that never reached the store: the
// subgraph that produced it and the reason it stayed out.
type UncacheablePrivate struct {
	Subgraph string
	Reason   string
}

// ScopeMismatch identifies one direction of privacy-scope drift: entries of a
// subgraph found under StoredScope while the reading fetch keys under the other
// one.
type ScopeMismatch struct {
	Subgraph    string
	StoredScope string
}

// StoreError is one recorded store failure: the op that failed, the subgraph
// whose keys it carried (empty for the request-end write flush), the number of
// keys in the call, and the store's error.
type StoreError struct {
	Op       string
	Subgraph string
	KeyCount int
	Err      error
}

// NewTraceObserver builds the ART trace observer.
func NewTraceObserver() *TraceObserver {
	return &TraceObserver{
		compares: make(map[*resolve.FetchCacheHandle][]resolve.CacheShadowCompareTrace),
	}
}

func (o *TraceObserver) BeginRequest(*resolve.Context) {}
func (o *TraceObserver) EndRequest(*resolve.Context)   {}

// CompareShadow materializes the shadow staleness probe: cached-vs-fresh byte
// equality per stashed entry, with the entry's age (CacheTTL - RemainingTTL).
// Results are computed EAGERLY into plain values — nothing arena-owned
// survives the transaction — and drained by OnFetchObserved.
func (o *TraceObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, tx *resolve.CacheTransaction) {
	if h == nil || len(h.ShadowStash) == 0 {
		return
	}
	var batch []*astjson.Value
	if h.BatchEntityKey && fresh != nil {
		batch = fresh.GetArray()
	}
	compares := make([]resolve.CacheShadowCompareTrace, 0, len(h.ShadowStash))
	for _, itemIndex := range slices.Sorted(maps.Keys(h.ShadowStash)) {
		entry := h.ShadowStash[itemIndex]
		freshValue := fresh
		if h.BatchEntityKey {
			freshValue = nil
			if itemIndex >= 0 && itemIndex < len(h.Items) {
				if batchIndex := h.Items[itemIndex].BatchIndex; batchIndex >= 0 && batchIndex < len(batch) {
					freshValue = batch[batchIndex]
				}
			}
		}
		freshBytes := []byte("null")
		if freshValue != nil {
			freshBytes = freshValue.MarshalTo(nil)
		}
		compares = append(compares, resolve.CacheShadowCompareTrace{
			Key:          traceKey(entry.CacheKey, h.HashAnalyticsKeys),
			IsFresh:      string(entry.CachedValue.MarshalTo(nil)) == string(freshBytes),
			CacheAgeNano: int64(entry.CacheTTL - entry.RemainingTTL),
		})
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.compares[h] = append(o.compares[h], compares...)
}

// OnFetchObserved assembles the finished handle's CacheTrace and attaches it
// to the fetch's ART trace. It runs at EndRequest — single-threaded per
// request, no lock (beyond draining the cross-request compare map), no arena.
func (o *TraceObserver) OnFetchObserved(h *resolve.FetchCacheHandle) {
	if h == nil {
		return
	}
	o.mu.Lock()
	compares := o.compares[h]
	delete(o.compares, h)
	o.mu.Unlock()
	if h.Trace == nil {
		// Tracing disabled: the drain above still prevents cross-request
		// accumulation; nothing is emitted.
		return
	}
	trace := &resolve.CacheTrace{
		Decision:       h.Decision.String(),
		Hit:            h.WasHit,
		Shadow:         h.Shadow,
		ShadowCompares: compares,
	}
	for i := range h.Items {
		item := &h.Items[i]
		trace.Items = append(trace.Items, resolve.CacheItemTrace{
			Key:              traceKey(itemTraceKey(item), h.HashAnalyticsKeys),
			ServedFrom:       item.ServedFromLayer,
			Hit:              item.FromCache != nil,
			NegativeHit:      item.NegativeHit,
			RemainingTTLNano: int64(item.RemainingTTL),
		})
	}
	h.Trace.CacheTrace = trace
}

// OnStoreError records one failed store round trip. Failures are degradations,
// never request errors: the reads they lost are served from the origin and the
// writes they lost are dropped.
func (o *TraceObserver) OnStoreError(op string, subgraph string, keyCount int, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.storeErrors = append(o.storeErrors, StoreError{
		Op:       op,
		Subgraph: subgraph,
		KeyCount: keyCount,
		Err:      err,
	})
}

// OnUncacheablePrivate records one private result whose store entry is
// silently never written — exactly the kind of invisible cache miss an operator
// needs surfaced, with the reason naming the knob that would fix it.
func (o *TraceObserver) OnUncacheablePrivate(subgraph string, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.uncacheablePrivate = append(o.uncacheablePrivate, UncacheablePrivate{
		Subgraph: subgraph,
		Reason:   reason,
	})
}

// UncacheablePrivate returns the recorded private results the store never
// received, in occurrence order.
func (o *TraceObserver) UncacheablePrivate() []UncacheablePrivate {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.uncacheablePrivate)
}

// OnScopeMismatch counts one entry discarded for carrying the other privacy
// scope. A steady count means two deployments disagree about a subgraph's
// scope; every such entry is refetched from the origin, never served.
func (o *TraceObserver) OnScopeMismatch(subgraph string, storedScope string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.scopeMismatches == nil {
		o.scopeMismatches = make(map[ScopeMismatch]int)
	}
	o.scopeMismatches[ScopeMismatch{Subgraph: subgraph, StoredScope: storedScope}]++
}

// ScopeMismatches returns the discarded-entry counts per subgraph and stored
// scope.
func (o *TraceObserver) ScopeMismatches() map[ScopeMismatch]int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return maps.Clone(o.scopeMismatches)
}

// StoreErrors returns the recorded store failures in occurrence order.
func (o *TraceObserver) StoreErrors() []StoreError {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.storeErrors)
}

func (o *TraceObserver) OnEntity(*resolve.FetchCacheHandle, *astjson.Value)       {}
func (o *TraceObserver) OnFieldValue(resolve.GraphCoordinate, resolve.FieldValue) {}

// itemTraceKey identifies the item's cache entry in trace output: its L2 key,
// or — for an L1-only fetch, which derives no store key — the hash of its raw
// preimage, so the entity's @key VALUES never reach a trace. An item whose key
// did not render has neither and traces without a key.
func itemTraceKey(item *resolve.ItemCacheState) string {
	if item.RenderedKey != "" {
		return item.RenderedKey
	}
	if item.L1Key == "" {
		return ""
	}
	return hashHex([]byte(item.L1Key))
}

// traceKey returns the key as-is, or its 16-hex xxhash64 when the policy asks
// for hashed key material in analytics/trace output.
func traceKey(key string, hash bool) string {
	if !hash || key == "" {
		return key
	}
	return hashHex([]byte(key))
}
