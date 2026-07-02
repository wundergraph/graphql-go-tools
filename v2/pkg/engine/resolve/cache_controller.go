package resolve

import (
	"fmt"
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
	// best-effort renders every candidate key that CAN be rendered from the data
	// available now, looks the cache up under all rendered keys, runs the per-item
	// coverage / freshness / reorder walk, and returns a Decision plus the opaque
	// handle the merge step reads. A NO-OP returns DecisionFetch and a nil handle.
	PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle)

	// OnFetchSkipped runs after the merge phase when PrepareFetch returned
	// DecisionSkipFullHit (or DecisionFetchPartial). It splices the chosen,
	// already-reordered cached values into the merge targets and may emit
	// best-effort multi-key write-back / backfill writes. The fetch did not hit
	// the network.
	OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error

	// OnFetchResult runs after the merge phase following a real network fetch
	// (DecisionFetch or DecisionFetchShadow). It applies the write gate
	// (!FetchFailed && !HasErrors && ResponseData != nil && Type() != Null;
	// EmptyEntity is the one non-failure that still writes the negative
	// sentinel), re-renders pending candidate keys from the fresh data, and
	// persists or defers the writes. When h.Shadow it runs the shadow compare
	// before the write-back.
	OnFetchResult(h *FetchCacheHandle, in MergeInput) error

	// EndRequest runs once after the root tree AND every defer group have
	// resolved, single-threaded. It flushes batched L2 writes and finalizes
	// analytics/trace. It needs no lock and no arena.
	EndRequest()
}

// Decision is what PrepareFetch tells the loader to do. It is the ONLY cache
// concept the loader branches on.
type Decision uint8

const (
	// DecisionFetch: miss, or caching disabled for this fetch. loadPhase fetches
	// normally; the handle may be nil.
	DecisionFetch Decision = iota

	// DecisionSkipFullHit: every item is covered by a covering cache value. The
	// loader skips the network load. NOT terminal for the cache: OnFetchSkipped
	// may still emit best-effort multi-key backfill/refresh writes.
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

// PrepareFetchInput carries everything PrepareFetch needs to render candidate
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
	MergePath   []string
	HasErrors   bool
	FetchFailed bool
	EmptyEntity bool
	StatusCode  int
	Arena       TransactionBeginner
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
	MustWriteBack  bool     // a hit still needs best-effort L2 writes
	BatchEntityKey bool     // batch-entity-key mode (per-element multi-key render on write)
	// PartialInput is the reduced fetch input for DecisionFetchPartial: the
	// original rendered input with the CACHED representations filtered out, so
	// the subgraph receives only the missing ones. The loader swaps it in as
	// the network input (single-flight then dedups on the reduced input).
	PartialInput []byte
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
// {decision:SkipFullHit items:3 hits:3 writeback:1 shadow:false}.
func (h *FetchCacheHandle) String() string {
	if h == nil {
		return "<nil>"
	}
	hits := 0
	writeback := 0
	for i := range h.Items {
		if h.Items[i].FromCache != nil {
			hits++
		}
		if h.Items[i].NeedsWriteback {
			writeback++
		}
	}
	return fmt.Sprintf("{decision:%s items:%d hits:%d writeback:%d shadow:%t}", h.Decision, len(h.Items), hits, writeback, h.Shadow)
}

// ItemCacheState is the per-item cache payload, one per merge target.
// PrepareFetch writes every field except NegativeHit / WriteReason (stamped in
// OnFetchResult from the fresh response); the merge hooks read it.
type ItemCacheState struct {
	Item      *astjson.Value // splice target / write source
	FromCache *astjson.Value // chosen + reordered cached value; nil = miss, TypeNull = negative hit

	// RenderedKeys are the candidate keys successfully rendered at lookup; looked
	// up (a hit on ANY serves the item) and the default write set. L1 and L2
	// SHARE these keys — each key is derived once and reused for both layers.
	RenderedKeys []string

	// PendingCandidates are the candidates NOT renderable at lookup (fields
	// absent); re-rendered from fresh data in OnFetchResult and, if now
	// renderable, added to the write set (best-effort backfill).
	PendingCandidates []CacheKeyCandidate

	FromCacheCandidates  []CacheCandidate // freshest-first cached entries matched across RenderedKeys
	SelectedRemainingTTL time.Duration    // remaining TTL of the chosen value, for analytics/trace
	NeedsWriteback       bool             // merged/older value chosen -> rewrite the canonical entry
	// ServedFromLayer is "l1" or "l2" when FromCache was selected (trace only).
	ServedFromLayer string
	EntityMergePath []string         // the fetch's merge path for root<->entity wrap/extract
	BatchIndex      int              // original batch position for realign
	NegativeHit     bool             // subgraph-null sentinel routing
	WriteReason     CacheWriteReason // refresh / backfill (metadata only, never gates writes)
}

// CacheCandidate is one cached entry matched for an item at lookup, kept as
// bytes and lazily parsed inside a CacheTransaction.
type CacheCandidate struct {
	Value        []byte
	RemainingTTL time.Duration
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
