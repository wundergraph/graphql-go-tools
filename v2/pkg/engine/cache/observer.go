package cache

import (
	"sync"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TraceObserver is the production CacheObserver: it accumulates per-fetch
// cache trace from the opaque FetchCacheHandle and attaches the assembled
// CacheTrace to the fetch's ART trace at request end. The observer is
// composed INSIDE the controller — the loader never calls it — and a nil
// observer on the controller records nothing at zero cost. Metrics/analytics
// export and the walker-inlined per-field hooks stay follow-ups (the scope
// guard); OnEntity/OnFieldValue are deliberate no-ops.
type TraceObserver struct {
	// mu guards compares: CompareShadow runs inside per-request hook
	// transactions, and one TraceObserver instance serves MANY concurrent
	// requests.
	mu       sync.Mutex
	compares map[*resolve.FetchCacheHandle][]resolve.CacheShadowCompareTrace
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
	for itemIndex, entry := range h.ShadowStash {
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
		keys := make([]string, 0, len(item.RenderedKeys))
		for _, key := range item.RenderedKeys {
			keys = append(keys, traceKey(key, h.HashAnalyticsKeys))
		}
		trace.Items = append(trace.Items, resolve.CacheItemTrace{
			Keys:              keys,
			ServedFrom:        item.ServedFromLayer,
			Hit:               item.FromCache != nil,
			NegativeHit:       item.NegativeHit,
			RemainingTTLNano:  int64(item.SelectedRemainingTTL),
			WriteReason:       string(item.WriteReason),
			PendingCandidates: len(item.PendingCandidates),
		})
	}
	h.Trace.CacheTrace = trace
}

func (o *TraceObserver) OnEntity(*resolve.FetchCacheHandle, *astjson.Value)       {}
func (o *TraceObserver) OnFieldValue(resolve.GraphCoordinate, resolve.FieldValue) {}

// traceKey returns the key as-is, or its 16-hex xxhash64 when the policy asks
// for hashed key material in analytics/trace output.
func traceKey(key string, hash bool) string {
	if !hash || key == "" {
		return key
	}
	return hashHex([]byte(key))
}
