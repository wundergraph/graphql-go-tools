package resolve

import (
	"fmt"
	"net/http"
	"time"

	"github.com/wundergraph/astjson"
)

// CacheController is the long-lived, integrator-supplied cache lifecycle port.
// The router sets one via Context.SetCacheController, exactly as it sets an
// Authorizer. A nil controller is the global NO-OP and is zero-cost: the loader
// never enters cache code, never allocates a handle, never takes a lock.
// NO-OP, L1-only, L2-only, and L1+L2 are not distinguished here; they are all
// distinguished only by the RequestCache the controller hands back.
type CacheController interface {
	// BeginRequest is called once per request, lazily, under DataBuffer.Lock, the
	// first time a cache-eligible fetch is prepared. It returns the request-lifetime
	// shared working surface, which is then shared by reference across every
	// per-defer-group Loader of this request. The returned value owns all
	// per-request mutable state, so nothing mutable hangs on the long-lived
	// controller and there is no cross-request sharing.
	BeginRequest(ctx *Context) RequestCache
}

// RequestCache is the ONE mode-blind working surface the loader talks to. Its
// PrepareFetch / OnFetchSkipped / OnFetchResult methods are invoked from
// resolveSingle with NO loader lock held; each one opens exactly ONE
// CacheTransaction (which acquires DataBuffer.Lock) for its whole arena
// sequence and releases it with Commit. EndRequest runs once, single-threaded,
// after the whole fetch tree (root + every defer group) has resolved, and
// needs no lock and no arena.
type RequestCache interface {
	// PrepareFetch runs after the prepare phase, with no loader lock held. It
	// renders each item's ONE key from the fetch's representation node, looks the
	// cache up under it, runs the per-item coverage walk, and returns a Decision
	// plus the opaque handle the merge step reads. A NO-OP returns DecisionFetch
	// and a nil handle.
	PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle)

	// OnUncachedFetch runs BEFORE the load for every fetch the loader executes
	// without a cache config — a fetch the cache neither reads nor writes. It
	// carries no state and decides nothing: it reports that part of this
	// response came from outside the cache, which bounds what the client may be
	// told about the response as a whole. It is the one hook that can be the
	// FIRST call of a request, so it begins the request surface like any other.
	// It holds no lock and must not touch the arena.
	OnUncachedFetch()

	// OnFetchSkipped runs after the merge phase when PrepareFetch returned
	// DecisionSkipFullHit. It splices the cached values into the merge targets.
	// The fetch did not hit the network, and a pure hit owes no writes: the read
	// key IS the write key, so there is nothing left to populate.
	OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error

	// OnFetchResult runs after the merge phase following a real network fetch
	// (DecisionFetch, DecisionFetchShadow, or DecisionFetchPartial — the
	// partial arm splices the cached items and realigns + merges the fetched
	// ones in this same hook). It applies the write gate
	// (!FetchFailed && !HasErrors && ResponseData != nil && Type() != Null;
	// EmptyEntity is the one non-failure that still writes the negative
	// sentinel) and persists or defers one write per item. When h.Shadow it runs
	// the shadow compare before the write-back.
	OnFetchResult(h *FetchCacheHandle, in MergeInput) error

	// EndRequest runs once after the root tree AND every defer group have
	// resolved, single-threaded. It flushes batched L2 writes and finalizes
	// analytics/trace. It needs no lock and no arena — and it MUST NOT touch
	// one: on the arena entry paths it runs via defer AFTER the request arena
	// is released back to its pool, so dereferencing any arena-owned
	// astjson.Value still held on a handle (ItemCacheState.Item / FromCache,
	// ShadowStash values) reads reset or reused memory. Everything EndRequest
	// consumes must be plain heap data (strings, durations, copied bytes).
	EndRequest()
}

// CacheTraceFlusher is the optional RequestCache extension that attaches the
// per-fetch cache traces AHEAD of EndRequest. The resolver calls it right
// before the response renders (only when the trace ships in the response
// extensions), because the trace extension serializes during Resolve while
// EndRequest runs after the response has been written. Idempotent per handle;
// EndRequest's own observation pass skips handles already flushed. Same
// no-arena contract as EndRequest.
type CacheTraceFlusher interface {
	FlushTraces()
}

// Decision is what PrepareFetch tells the loader to do. It is the ONLY cache
// concept the loader branches on.
type Decision uint8

const (
	// DecisionFetch: miss, or caching disabled for this fetch. loadPhase fetches
	// normally; the handle may be nil.
	DecisionFetch Decision = iota

	// DecisionSkipFullHit: every item is covered by a covering cache value. The
	// loader skips the network load and OnFetchSkipped only splices — with one
	// key per item a served hit owes no writes.
	DecisionSkipFullHit

	// DecisionFetchPartial: some items covered, some not. Fetch only the missed
	// subset, then splice the cached subset and realign.
	DecisionFetchPartial

	// DecisionFetchShadow: shadow-mode L2 read hit. The loader treats it exactly
	// like a miss — skipLoad stays false, full fetch, full merge; the only deltas
	// live in the handle and the extra compare step inside OnFetchResult.
	DecisionFetchShadow
)

// String renders the Decision for logs and test assertions.
func (d Decision) String() string {
	switch d {
	case DecisionFetch:
		return "Fetch"
	case DecisionSkipFullHit:
		return "SkipFullHit"
	case DecisionFetchPartial:
		return "FetchPartial"
	case DecisionFetchShadow:
		return "FetchShadow"
	default:
		return fmt.Sprintf("Decision(%d)", d)
	}
}

// PrepareFetchInput carries everything PrepareFetch needs to render the item
// keys, look the cache up, and decide. Input is the canonical pre-injection
// rendered bytes and HeaderHash the subgraph header hash — together the sole
// key material, so read and write keys derive from the same canonical form.
type PrepareFetchInput struct {
	Ctx        *Context
	Item       *FetchItem
	Items      []*astjson.Value
	Config     *FetchCacheConfig
	BatchStats [][]*astjson.Value
	Input      []byte
	HeaderHash uint64
	Arena      TransactionBeginner
}

// MergeInput carries the post-merge view of one fetch to OnFetchSkipped /
// OnFetchResult. It surfaces all five write-gate signals: FetchFailed
// (transport / empty body / parse failure), HasErrors, EmptyEntity, StatusCode,
// and ResponseData == nil. FetchFailed and HasErrors block ALL fetched-value
// writes; EmptyEntity is the one non-failure that still writes (the negative
// sentinel); ResponseData == nil is the structural backstop for the early
// failure paths. The gate can never reduce to !HasErrors alone: transport,
// empty-body, and parse failures reach the merge hook with HasErrors == false.
type MergeInput struct {
	Item         *FetchItem
	Items        []*astjson.Value
	ResponseData *astjson.Value
	BatchStats   [][]*astjson.Value
	// MergePath is the fetch's post-processing merge path, so entity/batch
	// values splice at the correct target instead of silently at the item root.
	MergePath []string
	// ResponseHeaders are the subgraph HTTP response's headers, the runtime
	// source of the fetch's Cache-Control. They are NOT cloned: the hook reads
	// them synchronously and must retain nothing. nil when the fetch produced no
	// HTTP response — a transport failure, a non-HTTP datasource, or a skipped
	// load — and the storability decision then falls back to static
	// configuration.
	ResponseHeaders http.Header
	HasErrors       bool
	FetchFailed     bool
	EmptyEntity     bool
	StatusCode      int
	Arena           TransactionBeginner
}

// CacheObserver is the optional analytics/trace/shadow-compare port. It is
// composed INSIDE a RequestCache implementation (the loader never calls it
// directly), so verbose observability evolves with zero impact on the
// lookup/write surface. A nil observer means no observability and is zero-cost.
type CacheObserver interface {
	BeginRequest(ctx *Context)
	EndRequest(ctx *Context)
	// OnFetchObserved derives per-fetch trace + counters from the finished
	// handle, so trace and analytics read the same opaque state the writer used.
	OnFetchObserved(h *FetchCacheHandle)
	// CompareShadow runs the shadow staleness probe; the writer calls it before
	// overwriting L2, preserving compare -> write-L1 -> write-L2 order. It runs
	// inside OnFetchResult's already-open CacheTransaction (it does not open its
	// own).
	CompareShadow(h *FetchCacheHandle, fresh *astjson.Value, tx *CacheTransaction)
	// OnStoreError reports ONE failed store round trip: the op ("GetMany" /
	// "SetMany"), the subgraph whose keys it carried (empty for the request-end
	// flush, which spans every fetch), how many keys the call held, and the
	// store's error. A failed read serves every one of its keys as a miss and a
	// failed write is dropped, so the request itself is unaffected.
	OnStoreError(op string, subgraph string, keyCount int, err error)
	// OnUncacheablePrivate reports private data the store never received. The
	// result stays in the request-lifetime layer (per-request is per-requester)
	// but no shared entry materializes, which is otherwise an invisible cache
	// miss. Emitted at most ONCE per fetch, with the reason naming the remedy:
	// "response-private" — a statically-public fetch was answered with
	// `Cache-Control: private`, so declare the scope statically; or
	// "no-identity" — a statically-private fetch ran without a requester
	// identity, so supply a PrivatePartitionProvider or key by subgraph headers.
	OnUncacheablePrivate(subgraph string, reason string)
	// OnScopeMismatch reports a stored entry whose recorded privacy scope
	// ("public" or "private") is not the one the reading fetch derives its keys
	// under — configuration drift between the writing and the reading
	// deployment. The entry is discarded as a miss, never served.
	OnScopeMismatch(subgraph string, storedScope string)
	// OnEntity / OnFieldValue are the resolvable-walker observability hooks.
	OnEntity(h *FetchCacheHandle, entity *astjson.Value)
	OnFieldValue(coordinate GraphCoordinate, value FieldValue)
}

// FetchCacheHandle is the per-fetch opaque two-level cache state, carried on
// preparedFetch. It is allocated by PrepareFetch only when the controller
// actually touches the fetch; the loader stores it, threads it back to the
// merge hook, and never reads a field beyond Decision.
type FetchCacheHandle struct {
	Decision       Decision // what PrepareFetch decided (drives the merge dispatch)
	WasHit         bool     // a covering cache value was found
	BatchEntityKey bool     // batch-entity-key mode (one key per unique representation)
	// BatchFetchKeep marks, per unique batch bucket, whether the NETWORK fetch
	// must include its representation (DecisionFetchPartial: true = still
	// missing, false = served from cache). The loader assembles the reduced
	// input from the already-rendered representation segments BEFORE any bytes
	// are parsed back — nil means "fetch all".
	BatchFetchKeep []bool
	// Trace is the fetch's ART trace destination (nil when tracing is off);
	// the observer attaches the assembled CacheTrace to it at request end.
	Trace *DataSourceLoadTrace
	// HashAnalyticsKeys mirrors the policy knob so the observer hashes key
	// material in trace output when set.
	HashAnalyticsKeys bool
	Shadow            bool                     // shadow mode: run compare in OnFetchResult before write-back
	ShadowStash       map[int]ShadowCacheEntry // stashed L2 reads for the shadow compare, keyed by item index
	Items             []ItemCacheState         // per-item payload, one per merge target
	Analytics         any                      // observer-owned accumulators; opaque even here
}

// String renders a compact, nil-safe summary for logs and panics, e.g.
// {decision:SkipFullHit items:3 hits:3 shadow:false}.
func (h *FetchCacheHandle) String() string {
	if h == nil {
		return "<nil>"
	}
	hits := 0
	for i := range h.Items {
		if h.Items[i].FromCache != nil {
			hits++
		}
	}
	return fmt.Sprintf("{decision:%s items:%d hits:%d shadow:%t}", h.Decision, len(h.Items), hits, h.Shadow)
}

// ItemCacheState is the per-item cache payload, one per merge target.
// PrepareFetch writes every field except NegativeHit (stamped in OnFetchResult
// from the fresh response); the merge hooks read it.
// Item and FromCache may be arena-owned: they are valid inside the fetch-phase
// hooks only and MUST NOT be dereferenced at EndRequest time (a nil-check is
// fine — see RequestCache.EndRequest).
type ItemCacheState struct {
	Item      *astjson.Value // splice target / write source
	FromCache *astjson.Value // cached value; nil = miss, TypeNull = negative hit

	// RenderedKey is the item's L2 store key, derived from the fetch's
	// representation node; "" when the item could not render one, or when the
	// fetch does not use L2 (then no store key is derived at all).
	RenderedKey string
	// L1Key is the item's request-lifetime cache key: the raw, unhashed
	// preimage the L2 key derives from, without the format version and without
	// the header hash (nothing persists and the map is per-requester). "" when
	// the item could not render one, or in root-field scope, which is L2-only.
	L1Key string
	// Tags are the invalidation tags the item's store entry is indexed under,
	// derived from the same rendering as RenderedKey and therefore identical
	// whether the item ends up written or served from the store. nil when the
	// fetch does not use L2 — tags address store entries and nothing else.
	Tags []string

	RemainingTTL time.Duration // remaining TTL of the cached value, for analytics/trace
	// ServedFreshness is what is left of a served store entry's lifetime, taken
	// from the entry's OWN freshness record rather than from what the store
	// reports. It bounds the client cache answer; 0 when the item was not
	// served from the store or when no client answer is being computed.
	ServedFreshness time.Duration
	// ServedFromLayer is "l1" or "l2" when FromCache was selected (trace only).
	ServedFromLayer string
	BatchIndex      int  // original batch position for realign
	NegativeHit     bool // subgraph-null sentinel routing
}

// ShadowCacheEntry is one stashed L2 read kept for the shadow compare: the
// value that WOULD have been served, compared against the fresh fetch before
// the write-back.
type ShadowCacheEntry struct {
	CachedValue  *astjson.Value
	CacheKey     string
	RemainingTTL time.Duration
	// CacheTTL is the policy TTL the entry was written with, so the observer
	// can derive the entry's age (CacheTTL - RemainingTTL) without re-deriving
	// config.
	CacheTTL time.Duration
}
