# RFC-1: Loader cache abstraction for the defer branch

Status: final for review.
Author: caching working group.
Branch under change: `feat/eng-7770-add-defer-support-part-4`, code under `v2/pkg/engine/resolve/`.
Ground truth: `scratchpad/caching-rfc/DOSSIER.md` (CURRENT defer-branch + OLD caching-base evidence, cited as `file:line`).

This RFC is the synthesis of three independent drafts (A minimal-opaque, B typed-explicit, C layered-ports) and three judge reviews.
The aggregate judge scores were close (A 159, B 153, C 141 summed across reviewers, with two reviewers preferring A and one preferring B).
The structural base is **B (typed-explicit)**, because RFC-1's core deliverable is the compile-checked cross-RFC OUTPUT CONTRACT that RFC-2 must satisfy, and B's typed per-fetch config plus typed two-level handle make that contract concrete and reviewable.
Onto B we graft, surgically, the ideas the reviewers identified as strictly better:

- from A: the full-hit skip mechanic that reuses the existing `mergeResult` early-return (the one blocking bug in B), folding `ProvidesData` into the per-fetch config instead of re-adding `FetchInfo.ProvidesData`, and folding the end-of-tree flush into a request-end lifecycle hook so every root (including the defer-entry path B and C miss) is covered;
- from C: a dedicated `CacheObserver` port to quarantine analytics, trace, and shadow-compare (the churn that drove OLD's 461 references) off the lookup/write surface, a per-candidate `CacheKeyTemplate` as the "sole source of truth for read/write/invalidate keys" (one per candidate in the multi-key model), and the cache-key-fidelity contract promoted from a flagged risk to a hard clause.

Every must-fix raised by the reviewers is resolved in §4, §6, §10, and §11, with file:line proof for the three correctness-critical ones (skip path, nested-merge, OnLoad/OnFinished).

---

## 1. Summary and goals

### 1.1 What this RFC delivers

RFC-1 adds a clean abstraction layer to the loader so that L1 + L2 response + entity caching can be (re-)implemented entirely OUTSIDE `loader.go`.
The OLD implementation re-authored the loader's control flow at ~461 cache call-sites and ~1433 inserted lines, with config bolted onto `FederationMetaData` (DOSSIER §1).
This RFC replaces that with one small, mostly-additive commit that exposes a single mode-blind cache working surface, a self-contained per-fetch cache configuration, and a two-level opaque handle, such that four interchangeable cache implementations (NO-OP, L1-only, L2-only, L1+L2) plus shadow mode all ride the SAME interface and the loader never learns which is active.

### 1.2 Goals

- G1. ONE small, mostly-additive commit to `loader.go` plus minimal other `resolve` files, inserting calls at the existing prepare/load/merge seams (HR1).
- G2. ONE cache abstraction such that NO-OP / L1-only / L2-only / L1+L2 are all implementations of it, with the loader blind to the mode and a nil controller zero-cost (HR2).
- G3. Cover every runtime capability in DOSSIER §2 (HR3).
- G4. A two-level opaque handle, per-fetch state plus per-item payload, mapped 1:1 against OLD `CacheKey`/`cachedData` (HR4).
- G5. Correct under defer's per-defer-group Loader plus `DataBuffer.Lock` plus a shared arena (HR5).
- G6. A self-contained caching configuration decoupled from `FederationMetaData`, behind a `CacheConfigProvider` port (HR6).
- G7. A justified `responseData` surfacing decision (HR7), a module boundary with an `Equals` contract (HR8), a migration note (HR9), and impossible-to-misconfigure NO-OP gating (HR10).
- G8. Merging RFC-1's commit ALONE — before any cache implementation exists — changes runtime behavior in ZERO ways.

### 1.3 Non-goals

- The caching planner pass that populates the per-fetch config: that is RFC-2.
  RFC-1 defines only the OUTPUT contract RFC-2 must satisfy (§8).
- Any concrete cache implementation: NO-OP / L1 / L2 / L1+L2 / shadow are all implemented in a follow-up impl PR, in a new `resolve/cache` package behind the interfaces defined here.
- Partial entity / partial batch realign, walker-inlined analytics, and subscription / mutation caching are staged to v2; their loader SEAMS exist in v1 (§9).

---

## 2. Background

### 2.1 Why the OLD coupling failed

The OLD branch did not add hooks, it re-authored the loader.
`resolveParallel` became a hand-rolled 6-phase pipeline (OLD `loader.go:266-503`) and `resolveSingle` a sequential mirror, with key derivation, lookup, merge, analytics, and tracing interleaved at ~461 call-sites, ~60 cache fields hand-threaded on the `result` struct (OLD `loader.go:137-192`), and ~12 on `Loader` (DOSSIER §1).
That only worked because OLD kept the arena and parser main-thread-only.
Configuration was bolted onto `FederationMetaData`: 6 collections plus 4 interface methods plus datasource forwarders (DOSSIER §5.1).
The single biggest source of churn was analytics and trace, interleaved at nearly every cache site (DOSSIER §2.7) — which is why this RFC gives it its own port (§3.5).

### 2.2 What the defer branch already gives us

The CURRENT defer branch already did the structural work that makes a clean plug-in possible (DOSSIER §1, §3.1).
Each fetch's lifecycle is split into three phases with explicit lock discipline (CURRENT `loader.go`):

- `preparePhase` (`loader.go:318`) builds the input and selects the merge `items`, under `DataBuffer.Lock`.
- `loadPhase` (`loader.go:349`) does the network with NO lock, and early-returns when `prepared.skipLoad` is set (`loader.go:350-352`).
- `mergePhase` (`loader.go:360`) parses and merges the response into the tree, under `DataBuffer.Lock`, then calls `callOnFinished` (`loader.go:376-378`).

`resolveSingle` (`loader.go:381-401`) orchestrates the three, carrying state on `preparedFetch` (`loader.go:403-412`) with a `skipLoad` flag that is the exact structural equivalent of OLD `cacheSkipFetch`.
The OLD 6-phase pipeline is structurally incompatible because thread-safety is inverted — defer parses and merges inside goroutines under a mutex, OLD forbade goroutine arena use (DOSSIER §7 risk 1) — so caching must be re-expressed against these three seams, NOT ported.

The other defining constraint is that the Loader is per-defer-group.
`resolveDeferSingle` creates a fresh `NewLoader` per group sharing only the request `DataBuffer` and arena (CURRENT `resolve.go:605-612`), and the initial loader is separate (`resolve.go:504`).
The request `*Context` is shared by reference across all groups, with its only defer-time mutation (`ctx.subgraphErrors`) written exclusively under `dc.db.Lock()` (verified in the comment at CURRENT `resolve.go:698-704`) — this is the exact precedent RFC-1 reuses to home the shared cache state (§6).

---

## 3. The abstraction

All contract types live in the `resolve` package, in two new files `cache_controller.go` and `cache_config.go` (pure additions).
They reference only `resolve` types plus `astjson`, `time`, and `arena`; they import NO federation types.
The cache LOGIC (every implementation, key rendering, transforms, coverage/freshness/reorder, analytics) lives in a NEW sibling package `v2/pkg/engine/resolve/cache` that imports `resolve` one-way; `resolve` never imports it (§7.1, HR8).
The wiring mirrors `LoaderHooks` (CURRENT `loader.go:40-48`) and `SetAuthorizer` / `SetRateLimiter` (CURRENT `context.go:189/226`): an interface value reached through `Context`, nil-checked at every call site, fed value objects.

### 3.1 The two-tier controller (and why it is still ONE abstraction)

The loader sees exactly ONE mode-blind working surface, `RequestCache`.
A second, tiny interface, `CacheController`, exists only to scope that surface to a request — it is the lifecycle/factory tier, the exact analogue of how `Authorizer` is long-lived while producing per-call decisions.
This is "one abstraction" in the sense HR2 demands: NO-OP / L1-only / L2-only / L1+L2 are all implementations of `RequestCache`, and the loader branches on nothing but the `Decision` value it returns.

```go
// CacheController is the long-lived, integrator-supplied lifecycle port. The
// router sets one via Context.SetCacheController, exactly as it sets an Authorizer.
// A nil controller is the global NO-OP and is zero-cost. NO-OP, L1-only, L2-only,
// and L1+L2 (with shadow as a cross-cutting L2 variant) are NOT distinguished here;
// they are all distinguished only by the RequestCache the controller hands back.
type CacheController interface {
	// BeginRequest is called once per request, lazily, under DataBuffer.Lock, the
	// first time a cache-eligible fetch is prepared. It returns the request-lifetime
	// shared working surface, which is then shared by reference across every
	// per-defer-group Loader of this request (HR3-h, HR5). The returned value
	// owns all per-request mutable state (the L1 map, mutation-inheritance flags,
	// the analytics accumulator, the deferred L2 write set), so nothing mutable
	// hangs on the long-lived controller and there is no cross-request sharing.
	BeginRequest(ctx *Context) RequestCache
}

// RequestCache is the ONE mode-blind working surface the loader talks to. Its
// PrepareFetch / OnFetchSkipped / OnFetchResult methods are invoked from
// resolveSingle with NO loader lock held; each one acquires DataBuffer.Lock
// itself, ONCE, by opening a MergeSession from the passed MergeArena (§3.4) and
// holding it for its whole arena sequence. EndRequest runs once, single-threaded,
// after the whole fetch tree (root + every defer group) has resolved, and needs
// no lock. Because each hook holds DataBuffer.Lock for the duration of its
// MergeSession, implementations need no internal lock for their request-lifetime
// (arena-touching) state; the session's DataBuffer.Lock is its guard (§6).
type RequestCache interface {
	// PrepareFetch runs from resolveSingle after the render phase, with no loader
	// lock held; it opens a MergeSession (§3.4) for its arena work. It best-effort
	// renders every candidate key that CAN be rendered from the data available now
	// (some candidates may be unrenderable, which is allowed), looks the cache up
	// under ALL rendered keys (a hit on ANY serves the item), runs the always-on
	// per-item coverage + multi-candidate freshness + reorder walk (DOSSIER §2.10),
	// and returns a Decision plus the two-level handle the merge/flush steps read.
	// A NO-OP returns DecisionFetch + nil.
	PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle)

	// OnFetchSkipped runs from resolveSingle (after mergePhase) when PrepareFetch
	// returned DecisionSkipFullHit (or DecisionFetchPartial, v2). It opens a
	// MergeSession, splices the chosen, already-reordered cached values into items,
	// and MAY emit best-effort multi-key write-back / backfill writes
	// (handle.MustWriteBack). The fetch did not hit the network (DOSSIER §2.2, §2.5).
	OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error

	// OnFetchResult runs from resolveSingle (after mergePhase) after a real network
	// fetch (DecisionFetch or DecisionFetchShadow, or the fetched subset of
	// DecisionFetchPartial). It opens a MergeSession, does negative-hit detection,
	// best-effort multi-key render-then-backfill (re-render the still-unrendered
	// candidate keys from the fresh data, then L1/L2 write or defer), and — when
	// h.Shadow — the shadow compare BEFORE the write-back, preserving the OLD
	// compare -> write-L1 -> write-L2 order (DOSSIER §2.11).
	OnFetchResult(h *FetchCacheHandle, in MergeInput) error

	// EndRequest runs once after the root tree AND every defer group have resolved,
	// single-threaded. It flushes batched L2 writes (one Set per cache instance) and
	// finalizes analytics/trace. It needs no lock and no arena (§6.4).
	EndRequest()
}
```

Note on the pair vs "ONE abstraction" (reviewer must-fix): the loader holds and calls `RequestCache` only; `CacheController` never appears at a hot call site, it is a one-method factory the loader invokes once per request to obtain the working surface.
Collapsing them would force per-request mutable state onto a long-lived integrator object, which is exactly the cross-request-sharing race that sank one of the alternative drafts (reviewer must-fix on draft C's lifecycle).

### 3.2 The decision enum (including FetchShadow)

```go
// Decision is what PrepareFetch tells the loader to do. It is the ONLY cache
// concept the loader branches on.
type Decision uint8

const (
	// DecisionFetch: miss, or caching disabled for this fetch. loadPhase fetches
	// normally; the handle may be nil.
	DecisionFetch Decision = iota

	// DecisionSkipFullHit: every item is covered by a covering cache value (the
	// AND-reduction of the per-item coverage walk, DOSSIER §2.10). The loader skips
	// the network load. NOT terminal for the cache: OnFetchSkipped may still emit
	// best-effort multi-key backfill/refresh L2 writes (handle.MustWriteBack, §2.2).
	DecisionSkipFullHit

	// DecisionFetchPartial: some items covered, some not. Fetch only the missed
	// subset, then splice the cached subset and realign. STAGED to v2 (§9); a v1
	// controller never returns this.
	DecisionFetchPartial

	// DecisionFetchShadow: cfg.ShadowMode AND the L2 read hit (DOSSIER §2.11). The
	// loader treats it EXACTLY like a miss — skipLoad stays false, full render,
	// write-back — so it is byte-identical to DecisionFetch on the loader side. The
	// only deltas live in the handle (Shadow + ShadowStash) and the extra compare
	// step inside OnFetchResult.
	DecisionFetchShadow
)
```

The enum needs `DecisionFetchShadow` because shadow decouples "read a cache value" from "serve a cache value" (DOSSIER §2.11): a full L2 hit must still fetch (so not `SkipFullHit`), partial is force-disabled (so not `FetchPartial`), and a plain `Fetch` would lose the L2-read-having-run, the stash, and the post-merge compare.

### 3.3 Inputs to the hooks

```go
type PrepareFetchInput struct {
	Ctx        *Context
	Item       *FetchItem
	Items      []*astjson.Value   // merge targets (selectItemsForPath, loader.go:480)
	Config     *FetchCacheConfig  // RFC-2 output; nil => not cached (the gate, §10)
	BatchStats [][]*astjson.Value // unique-rep -> targets, for batch fetches
	Input      []byte             // canonical pre-injection rendered bytes (§3.6, cache-key fidelity)
	HeaderHash uint64             // subgraph header hash, the sole L2 vary-by knob (§3.6)
	Arena      MergeArena         // scoped-lock arena facade; open ONE MergeSession via Begin() (§3.4)
}

type MergeInput struct {
	Item         *FetchItem
	Items        []*astjson.Value
	ResponseData *astjson.Value     // parsed response DATA sub-path, surfaced from mergeResult (HR7, §7); nil => no parseable data was produced (transport/empty/parse failure), an authoritative no-write gate
	BatchStats   [][]*astjson.Value
	HasErrors    bool               // subgraph response carried GraphQL errors (one of five write gates, §3.3)
	FetchFailed  bool               // transport failure / empty body / parse failure: res.err != nil || len(res.out) == 0 || res.response == nil (the two §2.6 signals the prior draft dropped); an authoritative no-write gate
	EmptyEntity  bool               // isEmptyEntityFetch over the FULL parsed response, for negative-hit detection (§3.7); a SUCCESSFUL-but-empty entity fetch, NOT a failure — it ROUTES to the negative-cache write, it does not block writes
	StatusCode   int                // status-fallback signal (§2.6)
	Arena        MergeArena         // scoped-lock arena facade; open ONE MergeSession via Begin() (§3.4)
}
```

The write gate, made explicit (reviewer must-fix; DOSSIER §2.2 "Gate writes on success", §2.6).
`MergeInput` now surfaces ALL FIVE failure signals DOSSIER §2.6 enumerates, where the prior draft surfaced only three:
`FetchFailed` folds the two it dropped — `res.err` (transport failure) and empty `res.out` — plus the parse-failure case (`res.response == nil`);
`HasErrors` is the GraphQL error-path match;
`EmptyEntity` is the successful-but-empty entity fetch;
`StatusCode` is the status fallback.
These signals gate the REAL-FETCH write path (`OnFetchResult`): the authoritative rule a controller MUST follow before persisting a fetched value is `!in.FetchFailed && !in.HasErrors && in.ResponseData != nil && in.ResponseData.Type() != astjson.TypeNull`.
`FetchFailed` and `HasErrors` block ALL fetched-value writes, including negative writes, because a transport/empty/parse failure or an errored response must never be cached (it would persist a transient error, DOSSIER §2.6);
`EmptyEntity` is the ONE non-failure that still writes — it routes to the negative-cache sentinel path when `cfg.NegativeCacheTTL > 0` (a successful fetch that legitimately returned no entity).
`ResponseData == nil` is the structural backstop for the EARLY failure paths: the transport (`res.err`), empty-body (`len(out) == 0`), and parse-failure paths return before `res.responseData` is assigned (§4.4), so the data pointer is nil exactly when no data was parsed at all; the later status-fallback / shape-mismatch paths run AFTER the assignment and leave a JSON-`null` (non-nil pointer), which is blocked instead by the `Type() != astjson.TypeNull` clause plus `StatusCode`/`HasErrors`, while the legitimate null-data / empty-entity case is the one routed by `EmptyEntity`.
This is why `HasErrors` alone is NOT the gate: the transport/empty/parse paths reach `OnFetchResult` with `HasErrors == false` because `responseHasErrors` is assigned only after them (~`loader.go:680`), so without `FetchFailed`/`ResponseData == nil` a contract-following controller would cache a failed fetch (the reviewer's blocking bug).
These signals do NOT constrain the cache-hit write-back path (`OnFetchSkipped`): there was no network fetch, so `res.response`/`FetchFailed` are not about a failed fetch — the best-effort multi-key backfill/refresh writes that path emits operate on already-validated cached values gated by `handle.MustWriteBack` (§3.7, DOSSIER §2.2), not by these signals.

`ProvidesData` is NOT a parameter: it is folded into `FetchCacheConfig.ProvidesData` (§3.6), so the loader never references `*Object` for caching, and there is no `FetchInfo.ProvidesData` re-add and therefore no `Object`/`Field` `Copy()` drift (DOSSIER §7 risk 9).
The controller reads `cfg.ProvidesData` at lookup time inside `PrepareFetch`; it is request-independent plan data consumed as a runtime decision input (DOSSIER §2.10).

### 3.4 The merge-arena facade

The facade is a SCOPED (transactional) lock, not a per-op one.
A cache merge over several items is a multi-op sequence (the §2.10 candidate-parse -> merge-synthesis -> reorder ladder, then the splice or write), so a facade that took `DataBuffer.Lock` per method call would re-acquire the lock dozens of times for one fetch.
Instead the controller opens a `MergeSession` ONCE via `Begin()`, runs ALL its arena ops on that session, and releases the lock with `Close()` (via `defer`).
A plain `Lock()`/`Unlock()` pair would work, but the `Begin() -> MergeSession -> Close()` shape makes "arena ops only while the lock is held" impossible to misuse: every arena/parser op is a method ON the session, so there is no way to touch the arena without a held lock, and there is no closure-allocation overhead of a `Transaction(func())` callback.

```go
// MergeArena is the narrow, lock-guarded facade over the Loader's jsonArena. The
// cache controller may touch arena memory ONLY through a MergeSession, which it
// opens by calling Begin() ONCE at the top of a hook (PrepareFetch /
// OnFetchSkipped / OnFetchResult) and releases with Close() (via defer). Begin()
// acquires DataBuffer.Lock; Close() releases it. The session is the SINGLE scoped
// lock taken around the whole multi-op sequence — the candidate-parse ->
// merge-synthesis -> reorder pipeline (DOSSIER §2.10) and the splice/write —
// instead of re-locking per op, which reduces lock contention. A MergeArena/
// MergeSession must never be retained past the hook, and never be opened from the
// off-lock loadPhase (DOSSIER §7 risk 7).
type MergeArena interface {
	// Begin enters the locked merge region and returns the session through which
	// all arena/parser ops run. It acquires DataBuffer.Lock once for the caller.
	Begin() MergeSession
}

// MergeSession is the scoped, lock-held handle for one multi-op arena sequence.
// Every arena/parser op is a method here, so a controller cannot touch the arena
// without a held session ("ops only while locked" is unmisusable by construction).
// Close releases DataBuffer.Lock and MUST be called (via defer) exactly once.
type MergeSession interface {
	ParseBytes(b []byte) (*astjson.Value, error)                    // lazy candidate parse onto the arena
	StructuralCopy(v *astjson.Value) *astjson.Value                 // avoid MergeValues aliasing corruption (OLD loader_cache_transform.go:22-48)
	MergeValues(dst, src *astjson.Value) (*astjson.Value, error)
	MergeValuesWithPath(dst, src *astjson.Value, path ...string) (*astjson.Value, error)
	NewObject() *astjson.Value
	NewArray() *astjson.Value
	String(s string) *astjson.Value
	Null() *astjson.Value
	Close() // releases DataBuffer.Lock; use via defer
}
```

The contract: every arena op requires a held `MergeSession`; the arena and parser are touched ONLY inside a session; `Begin()`/`Close()` bracket exactly one scoped lock region per hook.
The implementation is a tiny loader-side value, `type mergeArena struct{ a arena.Arena; db *DataBuffer }`, built on demand by `l.mergeArena()`; `Begin()` calls `l.dataBuffer.Lock()` and returns a session bound to `l.jsonArena`, and `Close()` calls `l.dataBuffer.Unlock()`.
The session discards the `changed` bool that `astjson.MergeValuesWithPath` returns today (CURRENT `loader.go:724`), matching how defer already discards it (DOSSIER §7 risk 18).
It is never constructed on the no-op path (it is built only after the controller is known non-nil), and the loader does NOT hold `DataBuffer.Lock` when it calls a cache hook (§4.3, §6.4), so the session's `Begin()` is the single acquisition.

### 3.5 The observer port (analytics / trace / shadow-compare isolation)

Analytics, trace, and the shadow comparison are the churn-prone concern that dominated OLD's 461 references (DOSSIER §2.7).
RFC-1 quarantines them behind their own port so they evolve with zero impact on the lookup/write surface.

```go
// CacheObserver is the optional analytics/trace/shadow-compare port. It is composed
// INSIDE a RequestCache implementation (the loader never calls it directly). A nil
// observer means no observability and is zero-cost. Defined in v1; the walker hooks
// (OnEntity/OnFieldValue) are wired in v2 to avoid colliding with defer's walker
// rewrite (DOSSIER §4.3, §7 risk 10).
type CacheObserver interface {
	BeginRequest(ctx *Context)
	EndRequest(ctx *Context)
	// OnFetchObserved derives per-fetch trace + counters from the finished handle,
	// so trace and analytics read the same opaque state the writer used.
	OnFetchObserved(h *FetchCacheHandle)
	// CompareShadow runs the DOSSIER §2.11 staleness probe; the writer calls it
	// before overwriting L2, preserving compare -> write-L1 -> write-L2 order. It
	// runs inside OnFetchResult's already-open MergeSession (it does not open its
	// own). It no-ops unless analytics is enabled and cfg.ProvidesData != nil, and
	// it records only for entity fetches (root-field shadow force-refetches without
	// comparing, the OLD asymmetry at OLD loader_cache.go applyRootFetchL2Results).
	CompareShadow(h *FetchCacheHandle, fresh *astjson.Value, s MergeSession)
	// OnEntity / OnFieldValue are the resolvable-walker hooks (DOSSIER §2.7); v2.
	OnEntity(h *FetchCacheHandle, entity *astjson.Value)
	OnFieldValue(coordinate GraphCoordinate, value FieldValue)
}
```

Keeping `CacheObserver` off the loader surface is what prevents the ~22 trace fields and the per-result analytics accumulators from leaking back into `loader.go` (DOSSIER §2.7); they live on the handle (§4) and are read only here.

### 3.6 The self-contained per-fetch config (runtime side)

```go
// FetchCacheConfig is the self-contained, federation-free per-fetch cache config
// stamped by RFC-2 onto SingleFetch / EntityFetch / BatchEntityFetch. The loader
// carries it forward-only (it never reads a field, only hands it to the controller);
// the cache package interprets it. A nil *FetchCacheConfig means caching disabled
// for this fetch, which is the gate that makes a not-run planner pass behave as
// NO-OP (HR10). It imports NO federation types: federation @key selection sets are
// a plan-time INPUT only, frozen into KeySpec by RFC-2 (HR6, §7).
type FetchCacheConfig struct {
	L1 bool // participate in per-request L1 entity cache
	L2 bool // participate in cross-request L2 cache

	CacheName                   string
	TTL                         time.Duration
	NegativeCacheTTL            time.Duration // > 0 enables negative caching (DOSSIER §2.6)
	IncludeSubgraphHeaderPrefix bool          // fold the subgraph header hash into the L2 key
	EnablePartialCacheLoad      bool          // partial L1/entity (v2)
	PartialBatchLoad            bool          // partial batch realign (v2)
	ShadowMode                  bool          // L2 read-but-never-serve probe (DOSSIER §2.11)
	HashAnalyticsKeys           bool

	KeySpec CacheKeySpec // frozen multi-key derivation, DATA only, no federation types

	// ProvidesData is the field tree the fetch returns. It is request-independent
	// plan data, but it is consumed at RUNTIME by the coverage walk in PrepareFetch
	// (DOSSIER §2.10), not only at plan time. Folding it here (rather than re-adding
	// FetchInfo.ProvidesData) keeps loader.go from referencing *Object and sidesteps
	// the Object/Field Copy() drift (DOSSIER §7 risk 9).
	ProvidesData *Object

	// Mutation populate inheritance (request-lifetime carry; DOSSIER §2.8, §7 risk 3).
	PopulateL2OnMutation bool
	MutationTTLOverride  time.Duration
}

// Equals lets FetchConfiguration.Equals (plan dedup) deep-compare cache config.
// Only ever called with both receivers non-nil (the nil case is handled at the
// call site, §3.8). KeySpec is pure data, so this is a structural walk.
func (c *FetchCacheConfig) Equals(other *FetchCacheConfig) bool

// String renders a compact, nil-safe summary for the plan pretty-printer (which
// already guards nils, commit 921e48ae in fetchtree.go) and for logs.
func (c *FetchCacheConfig) String() string

type CacheScope uint8

const (
	CacheScopeRootField CacheScope = iota
	CacheScopeEntity
)

// CacheKeySpec is DATA ONLY (no renderer, no federation types). It models the
// MULTI-KEY identity of an entity / root field: an entity type may declare MORE
// THAN ONE @key set, so Candidates is a LIST of candidate key templates. None is
// "required" — each candidate is INDEPENDENTLY and BEST-EFFORT renderable from
// whatever data is available at the moment it is rendered. The cache package
// builds ONE CacheKeyTemplate PER candidate; each template is the SOLE source of
// truth for read, write, and invalidate of THAT key, so the three paths cannot
// silently diverge (the byte-identical-keys invariant, DOSSIER §2.3). Frozen ONCE
// from @key at plan time.
type CacheKeySpec struct {
	Scope     CacheScope
	TypeName  string
	FieldName string

	// Candidates is the list of candidate key templates (one per @key set). At
	// LOOKUP time PrepareFetch renders every candidate it CAN render from the data
	// at hand (some may be unrenderable because their fields are absent — allowed)
	// and looks the cache up under ALL rendered keys; a hit on ANY serves the item.
	// At WRITE time OnFetchResult re-renders the candidates that were unrenderable
	// at lookup, now from the fresh response data, and populates all keys that are
	// renderable (best-effort multi-key render-then-backfill, §2.2). The
	// interfaceObject / entityInterface __typename remap is baked INTO each
	// candidate's Representation, so there is no separate TypenameOverride field.
	Candidates []CacheKeyCandidate

	// EntityKeyMappings (root-arg <-> @key) contribute additional candidates rather
	// than a separate key space: a key derivable from root-field args is renderable
	// at lookup, while one derivable only from the returned entity data becomes
	// renderable at write — both are just candidates in the best-effort model (§2.2).
	EntityKeyMappings []EntityKeyMapping
}

// CacheKeyCandidate is ONE candidate @key template, frozen from a single @key set
// at plan time. Representation is the *resolve.Object key template that RFC-2's
// shared representationvariable.BuildRepresentationVariableNode produces from that
// one @key set (RFC-2 §6.1): a federation-pointer-free node tree with the
// interfaceObject / entityInterface __typename remap already baked in, so the
// candidate is a complete key template on its own. It subsumes the old single-key
// KeyFields + TypenameOverride pair. A candidate renders only when every field its
// Representation references is present in the data at hand; an unrenderable
// candidate is skipped at lookup and retried at write, never an error.
type CacheKeyCandidate struct {
	Representation *Object // the frozen @key representation node for this one candidate
}

// CacheWriteReason is metadata only; it does NOT gate writes (DOSSIER §2.2).
type CacheWriteReason string

const (
	CacheWriteReasonRefresh  CacheWriteReason = "refresh"  // a key already populated, re-written with fresh data
	CacheWriteReasonBackfill CacheWriteReason = "backfill" // a candidate key unrenderable/absent at lookup, populated now
)
```

`EntityKeyMapping` moves into `resolve` as a value type (the OLD `EntityKeyMappingConfig`, OLD `caching.go:96`), so `FetchCacheConfig` stays self-contained and `Equals` is a structural `slices.Equal` walk over pure data.
`Equals` over `CacheKeySpec` walks `Candidates` with `slices.EqualFunc`, comparing each candidate's `Representation` via the existing `Object.Equals` shape walk (CURRENT `node_object.go:42`), so two fetches that differ in any candidate key are not deduped.

Cache-key fidelity (DOSSIER §7 risk 17, promoted to a contract clause).
The loader passes `PrepareFetchInput.Input` as the canonical pre-injection rendered bytes, and `PrepareFetchInput.HeaderHash` as the subgraph header hash from the nil-guarded wrapper `ctx.HeadersForSubgraphRequest(name)` (verified CURRENT `context.go:105-111`; it nil-checks `SubgraphHeadersBuilder` and returns `(nil, 0)` when unset, so cache-key derivation never panics on a request with no header builder).
The controller derives all keys from these plus the `@key`/arg values, and folds the header hash into the L2 key as the sole vary-by knob (there is no Cache-Control/max-age/vary concept, DOSSIER §2.3).
This keys off the same canonical form that single-flight dedups on (risk 16), NOT off the post-prepare bytes that `executeSourceLoad` mutates (`extensions` at CURRENT `loader.go:1940`, `fetch_reasons` at `1956`).

### 3.7 The two-level opaque handle (HR4)

The handle replaces the ~60 OLD `result` cache fields with one carrier on `preparedFetch`.
It is a concrete `resolve` struct, not `any`, for debuggability and to avoid boxing on the merge path — but the loader treats it strictly opaquely: it stores it on `preparedFetch`, threads it back to the merge hook, and never reads a field.
It is two-level — per-fetch state PLUS a slice of per-item payloads — because flattening loses multi-candidate freshness selection, per-entity coverage, and the per-item multi-key render-then-backfill state (DOSSIER §3.3).

```go
// FetchCacheHandle is the per-fetch opaque cache state, carried on preparedFetch.
// Allocated by PrepareFetch ONLY when the controller actually touches the fetch; a
// nil controller means the loader never allocates one.
type FetchCacheHandle struct {
	Decision       Decision                 // what PrepareFetch decided (drives the merge dispatch + debuggability)
	WasHit         bool                     // OLD res.cacheSkipFetch (a covering value was found)
	MustWriteBack  bool                     // OLD res.cacheMustBeUpdated (a hit still needs L2 writes, §2.2)
	BatchEntityKey bool                     // batch-entity-key mode (per-element multi-key render on write)
	Shadow         bool                     // FetchShadow: run compare in OnFetchResult before write-back
	ShadowStash    map[int]ShadowCacheEntry // OLD res.shadowCachedValues, keyed by item index
	Items          []ItemCacheState         // per-item payload (OLD res.l1CacheKeys / res.l2CacheKeys)
	Analytics      any                      // observer-owned accumulators; opaque even here
}

// String renders {decision:SkipFullHit items:3 hits:3 writeback:1 shadow:false}
// for logs and panics.
func (h *FetchCacheHandle) String() string

// ItemCacheState is the per-item (per-CacheKey) payload, one per merge target.
// Mapped against OLD resolve.CacheKey + embedded cachedData (caching.go:27-65),
// with the OLD requested-vs-rendered-key + missing-key framing REPLACED by the
// best-effort multi-key model: RenderedKeys are the candidate keys that rendered
// at lookup; PendingCandidates are the candidates that did not, retried at write.
type ItemCacheState struct {
	Item                 *astjson.Value      // OLD CacheKey.Item        — splice target / write source (itemToStore)
	FromCache            *astjson.Value      // OLD CacheKey.FromCache   — chosen+reordered value; nil=miss, TypeNull=negative hit
	RenderedKeys         []string            // candidate keys SUCCESSFULLY rendered at lookup (the renderable subset of OLD CacheKey.Keys); looked up (hit on ANY serves) and the default write set; L1 prefix-free, L2 prefixed
	PendingCandidates    []CacheKeyCandidate // candidates NOT renderable at lookup (fields absent); re-rendered from fresh data in OnFetchResult and, if now renderable, added to the write set (best-effort backfill, §2.2) — generalizes OLD CacheKey.missingKeys + the requested/rendered split
	FromCacheCandidates  []CacheCandidate    // OLD cachedData.fromCacheCandidates — freshest-first cached entries matched across RenderedKeys (DOSSIER §2.10)
	SelectedRemainingTTL time.Duration       // OLD cachedData.fromCacheRemainingTTL — cache-age analytics/trace
	NeedsWriteback       bool                // OLD cachedData.fromCacheNeedsWriteback — merged/older chosen -> rewrite canonical
	EntityMergePath      []string            // OLD CacheKey.EntityMergePath — root<->entity wrap/extract
	BatchIndex           int                 // OLD CacheKey.BatchIndex  — original batch position for realign
	NegativeHit          bool                // OLD CacheKey.NegativeCacheHit — subgraph-null sentinel routing
	WriteReason          CacheWriteReason    // refresh / backfill (metadata only)
}

type CacheCandidate struct {
	Value        []byte        // OLD fromCacheCandidate.value (lazily parsed via MergeSession.ParseBytes)
	RemainingTTL time.Duration // OLD fromCacheCandidate.remainingTTL
}

type ShadowCacheEntry struct {
	CachedValue  *astjson.Value // OLD shadowCacheEntry.cachedValue, on the merge arena
	CacheKey     string
	RemainingTTL time.Duration
}
```

Phase ownership: `PrepareFetch` WRITES every field except `NegativeHit` / `WriteReason` (those are stamped in `OnFetchResult` from the fresh response); `PrepareFetch` writes `PendingCandidates`, and `OnFetchResult` re-renders and clears them from the fresh data (the multi-key backfill, §2.2); `OnFetchSkipped` / `OnFetchResult` / `EndRequest` READ the payload.
The field-by-field mapping to OLD `CacheKey` / `cachedData` is the inline column above (HR4).

### 3.8 The Equals call site

`SingleFetch` embeds `FetchConfiguration` (CURRENT `fetch.go:96-98`), and `EqualSingleFetch` deduplicates single fetches via `l.FetchConfiguration.Equals(&r.FetchConfiguration)` (CURRENT `fetch.go:80`, `283-309`).
The new field is a POINTER `Cache *FetchCacheConfig` (nil = the unambiguous "not cached" gate, and most fetches are uncached so no inline struct is carried), and the `Equals` clause is nil-safe at the call site, so the method is only ever invoked with both receivers non-nil:

```go
// inside FetchConfiguration.Equals (CURRENT fetch.go:283-309)
if (fc.Cache == nil) != (other.Cache == nil) {
	return false
}
if fc.Cache != nil && !fc.Cache.Equals(other.Cache) {
	return false
}
```

This avoids the nil-pointer-receiver footgun (a reviewer flagged draft C's nil-receiver `Equals`) while keeping the comparison compile-checked (a reviewer flagged that an `any`-typed config would force a reflect/assert comparison).
In v1 `Cache` is nil at dedup time, because RFC-2 stamps it AFTER `createConcreteSingleFetchTypes` (DOSSIER §6.4), so both sides are nil and the clause returns the prior result — zero behavior change.
The clause exists so the contract holds the instant any producer stamps config earlier (DOSSIER §7 risk 11).
`EntityFetch` / `BatchEntityFetch` carry the same `Cache *FetchCacheConfig` field but are built post-dedup, so they need no `Equals` of their own.

---

## 4. The one loader/resolve commit (HR1)

This is ONE commit.
It compiles as a no-op: every hook is guarded by `l.ctx.cacheController == nil`, every new field is zero-valued, and merging it alone changes runtime behavior in zero ways (§10.2, G8).
A second, behavior-neutral move-commit relocates the cache files into the new `cache` package (§7.1); it touches no runtime behavior.

### 4.1 File-by-file edit list

| File | Edit | Why |
|---|---|---|
| `cache_controller.go` (NEW) | `CacheController`, `RequestCache`, `Decision`, `PrepareFetchInput`, `MergeInput`, `MergeArena`, `MergeSession`, `CacheObserver`, `FetchCacheHandle`, `ItemCacheState`, `CacheCandidate`, `ShadowCacheEntry`, plus the `mergeArena`/`mergeSession` impl. | The runtime contract + scoped-lock facade (§3). Pure additions. |
| `cache_config.go` (NEW) | `FetchCacheConfig`, `CacheKeySpec`, `CacheKeyCandidate`, `CacheScope`, `CacheWriteReason`, `EntityKeyMapping`. | The self-contained multi-key config (§3.6, §7). Each `CacheKeyCandidate` carries a `*resolve.Object` Representation (the existing resolve type), so no `KeyField` type is needed. Pure additions. |
| `loader.go` (a) | `preparedFetch` (`403-412`): add `cacheHandle *FetchCacheHandle`. | Carry the opaque handle across phases. |
| `loader.go` (b) | `result` (`99-142`): add `responseData *astjson.Value`, `responseHasErrors bool`, and `response *astjson.Value` (the FULL parsed response, the input `isEmptyEntityFetch` needs). | Surface the response data sub-path, the error flag, AND the full response to the merge hook (HR7, §7). |
| `loader.go` (c) | `resolveSingle` (`381-401`): after `preparePhase` (lock released) call `l.cachePrepare(prepared)`; after `mergePhase` call `l.cacheMerge(prepared)`. | Seams S1 + S4, OUTSIDE the phase locks so the controller's `MergeSession` is the single `DataBuffer.Lock` acquisition (§3.4, §6.4). |
| `loader.go` (d) | `preparePhase` (`318-347`): NO change (it renders under `DataBuffer.Lock` and releases on return). | Lock released before the cache lookup. |
| `loader.go` (e) | `mergeResult`: assign `res.response = response` right after the successful parse (`628-635`), `res.responseData` (`645-650`), and `res.responseHasErrors` (after the `hasErrors` block ~`680`), each where already computed. | Surface the data sub-path, the full response, and the error flag; no signature change (HR7). |
| `loader.go` (f) | `mergePhase` (`360-379`): NO change (it merges under `DataBuffer.Lock` and releases on return). | `cacheMerge` runs after it, lock-free, so the splice/write session is the single acquisition. |
| `loader.go` (g) | NEW helpers: `cacheRequest`, `cachePrepare`, `cacheMerge`, `mergeArena`. | Keep all branching out of the phase bodies. |
| `loader.go` (h) | `loadPhase` (`349-358`): NO change. | Reuse `prepared.skipLoad` (S3 = 0 edits). |
| `context.go` (i) | `Context` (`42-44`): add `cacheController CacheController` and `requestCache RequestCache`. | The port + the shared-state home (§6). |
| `context.go` (j) | `SetCacheController` setter near `SetAuthorizer` (`189`). | Router wires the controller, mirroring `SetAuthorizer`. |
| `context.go` (k) | `endCacheRequest()` method: if `requestCache != nil` call `EndRequest()` then nil it. | The single request-end lifecycle hook (§4.5). |
| `context.go` (l) | `clone` (`283-305`): `cpy.requestCache = nil`. | A cloned resolution (subscription event, `resolve.go:1103`) gets its OWN L1 (§6.3). |
| `context.go` (m) | `Free` (`307-322`): defensively call `endCacheRequest()`, then nil `cacheController`. | Teardown. |
| `fetch.go` (n) | `FetchConfiguration` (`255-281`): add `Cache *FetchCacheConfig`; `EntityFetch` (`206-215`) and `BatchEntityFetch` (`166-175`): add `Cache *FetchCacheConfig`. | RFC-2 stamps config on all three concrete types (DOSSIER §6.4). |
| `fetch.go` (o) | `FetchConfiguration.Equals` (`283-309`): the nil-safe clause of §3.8. | Plan dedup must see cache config (HR8). |
| `resolve.go` (p) | `defer ctx.endCacheRequest()` at ALL FOUR request entry functions: `ResolveGraphQLResponse` (sync, `339`; flush at `360`), `ArenaResolveGraphQLResponse` (sync-arena, `383`; flush at `425`), `ResolveGraphQLDeferResponse` (defer root, `472`), and `executeSubscriptionUpdate` (subscription, `878`; loader at `910`/`912`). | Single request-end flush/finalize, covering BOTH sync entries (so the Arena sync path also flushes L2) and the defer-entry path (§4.5). |

That is TWO behavior-touching call sites (`cachePrepare` and `cacheMerge`, both in `resolveSingle`, outside the phase locks), plus FOUR request-end lifecycle `defer`s (one per entry function), plus the setter / struct fields / lazy-init.
Everything else is type declarations and zero-valued fields.

### 4.2 Call-site / LOC budget vs OLD

- OLD: ~461 cache call-sites, ~1433 inserted lines in `loader.go`, ~60 `result` cache fields (DOSSIER §1).
- RFC-1: 2 behavior call-sites + 4 lifecycle defers (one per entry function) + 4 helpers + 1 nil-checked lazy-init.
  Net new lines in existing files (`loader.go` / `context.go` / `fetch.go` / `resolve.go`): roughly 70-100, of which ~40 are the helper bodies.
  The two new contract files are reviewable in isolation.
- Reduction factor on call-sites: ~461 -> ~6, roughly two orders of magnitude.

### 4.3 S1 — cache lookup after the render phase

`preparePhase` and `mergePhase` are UNCHANGED: each takes `DataBuffer.Lock` for its own render / merge work and releases it on return (CURRENT `loader.go:319-320`, `361-362`).
The cache hooks are inserted in `resolveSingle`, which orchestrates the three phases, so they run with NO loader lock held — the controller's `MergeSession` is therefore the SINGLE `DataBuffer.Lock` acquisition around its multi-op sequence (§3.4, §6.4).

Before (CURRENT `loader.go:381-401`):

```go
prepared, err := l.preparePhase(item)
// ... err / nil-prepared handling ...
if err := l.loadPhase(ctx, prepared); err != nil {
	return errors.WithStack(err)
}
return l.mergePhase(prepared)
```

After (two cache calls bracket the network, both lock-free):

```go
prepared, err := l.preparePhase(item) // renders under DataBuffer.Lock; lock released on return
// ... err / nil-prepared handling unchanged ...
l.cachePrepare(prepared)              // S1: lookup + coverage + decision (no-op when controller/cfg nil)
if err := l.loadPhase(ctx, prepared); err != nil {
	return errors.WithStack(err)
}
if err := l.mergePhase(prepared); err != nil {
	return errors.WithStack(err)
}
return l.cacheMerge(prepared)         // S4: write / skip-splice / shadow (no-op when handle nil)
```

`cachePrepare` derives the per-fetch config from the fetch type and bails on every render-skip:

```go
// cacheRequest lazily obtains the request-lifetime working surface. The once-create
// runs under DataBuffer.Lock so it is race-free across the parallel fetches of a
// group and across per-defer-group Loaders, given exactly one DataBuffer per
// request (§6.2). It does NOT hold the lock across the hook: PrepareFetch /
// OnFetch* open their OWN MergeSession for arena work (§3.4).
func (l *Loader) cacheRequest() RequestCache {
	if l.ctx.cacheController == nil {
		return nil
	}
	l.dataBuffer.Lock()
	defer l.dataBuffer.Unlock()
	if l.ctx.requestCache == nil {
		l.ctx.requestCache = l.ctx.cacheController.BeginRequest(l.ctx)
	}
	return l.ctx.requestCache
}

func (l *Loader) cachePrepare(prepared *preparedFetch) {
	if prepared == nil || prepared.skipLoad {
		return // render already decided to skip — NOT a cache decision (see below)
	}
	var cfg *FetchCacheConfig
	switch f := prepared.item.Fetch.(type) {
	case *SingleFetch:
		cfg = f.Cache
	case *EntityFetch:
		cfg = f.Cache
	case *BatchEntityFetch:
		cfg = f.Cache
	}
	if cfg == nil || (!cfg.L1 && !cfg.L2) {
		return // zero-cost: no controller call, no key render, no lookup, no allocation
	}
	rc := l.cacheRequest()
	if rc == nil {
		return
	}
	_, hash := l.ctx.HeadersForSubgraphRequest(prepared.res.ds.Name) // nil-guarded wrapper (CURRENT context.go:105-111); returns (nil, 0) when no header builder is set
	decision, handle := rc.PrepareFetch(PrepareFetchInput{
		Ctx: l.ctx, Item: prepared.item, Items: prepared.items, Config: cfg,
		BatchStats: prepared.res.batchStats, Input: prepared.input, HeaderHash: hash,
		Arena: l.mergeArena(), // PrepareFetch opens ONE MergeSession from this (§3.4)
	})
	prepared.cacheHandle = handle
	switch decision {
	case DecisionSkipFullHit:
		// Full hit: skip the network AND the merge. Setting BOTH flags reuses the
		// existing mergeResult early-return (loader.go:620) so no spurious error is
		// rendered, and skips the network via the existing loadPhase guard.
		prepared.skipLoad = true
		prepared.res.fetchSkipped = true
	case DecisionFetchPartial:
		// v2: send only the missed subset, then OnFetchSkipped splices the cached
		// subset and mergeResult merges the fresh subset (§9).
	case DecisionFetch, DecisionFetchShadow:
		// skipLoad stays false; the handle (and h.Shadow) drives the merge.
	}
}
```

The `prepared.skipLoad` guard is load-bearing: every existing render-skip site sets `prepared.skipLoad` (null parent CURRENT `loader.go:1451`, render error `1461`, auth/rate-limit reject `1470`, skip-null/empty item `1507/1519/1529`, trace-mode skips `1547/1726`, empty batch `1701`), so the cache lookup never runs for a fetch the loader already decided to skip.
Those sites set `res.fetchSkipped` for the trace branches at `loader.go:1545` and `1724` inside `preparePhase`, which runs (and returns) strictly before `cachePrepare`, so our `cachePrepare` (which may set `res.fetchSkipped` for a full hit) runs after them and cannot corrupt the `LoadSkipped` trace.

Full-hit modelling — why both flags, grafted from draft A and verified.
Setting `res.fetchSkipped = true` makes `mergeResult` return `nil` at its existing guard `if res.fetchSkipped { return nil }` (CURRENT `loader.go:620-622`), WITHOUT falling through to `len(res.out) == 0` at `623` and rendering `renderErrorsFailedToFetch(emptyGraphQLResponse)` at `624`.
This is the one-line fix for the blocking skip-path bug a reviewer found in the typed-explicit base (which set only `skipLoad`): on a full hit `res.out` is empty, so without `res.fetchSkipped` every cache hit would render a spurious error.
Setting `skipLoad` (already required) makes `loadPhase` early-return (CURRENT `loader.go:350`).
The cached, already-reordered values are then spliced by `OnFetchSkipped` (§4.4), so no synthetic bytes are produced and no re-parse happens.

### 4.4 S4 — `mergeResult` carrier + `cacheMerge` (write / splice / shadow)

`mergeResult` stashes the values it already computes onto `res` (no signature change, HR7, §7).
Placement is exact so both fields are set before every null-data early return that a write path cares about:

```go
// CURRENT loader.go:628-635, immediately after the parse succeeds (and BEFORE
// every subsequent early return), one free pointer assignment of the full response:
res.response = response // NEW — the input isEmptyEntityFetch needs (it does response.Get("data","_entities"))
// CURRENT loader.go:645-650, after responseData is computed:
res.responseData = responseData // NEW
// ... after the hasErrors block (CURRENT loader.go:652-680), before the
// null-data branch at 683 and the isEmptyEntityFetch early-return at 686-688:
res.responseHasErrors = hasErrors // NEW
```

`res.response` is assigned right after `astjson.ParseBytesWithArena` returns successfully (CURRENT `loader.go:628`), so it is the EXACT value `isEmptyEntityFetch` consumes (`response.Get("data","_entities")`, CURRENT `loader.go:686/788-799`), and it is nil on every pre-parse failure path (`res.err` at `601`, empty `res.out` at `623`, parse error at `628-635`), which is precisely the `FetchFailed`/`ResponseData == nil` signal the write gate keys off (§3.3).
All three assignments are dead weight when caching is off (written, never read), preserving the no-op guarantee (§10.2); none changes control flow.

`mergePhase` is UNCHANGED: it runs `mergeResult` then `callOnFinished` under `DataBuffer.Lock` and returns, releasing the lock.
`resolveSingle` then calls `cacheMerge` (§4.3) with the lock free, so `OnFetchSkipped` / `OnFetchResult` open their OWN `MergeSession` for the splice / write (§3.4):

```go
func (l *Loader) cacheMerge(prepared *preparedFetch) error {
	h := prepared.cacheHandle
	if h == nil {
		return nil // controller nil, or this fetch not cached
	}
	res := prepared.res
	in := MergeInput{
		Item: prepared.item, Items: prepared.items,
		ResponseData: res.responseData, BatchStats: res.batchStats,
		HasErrors:   res.responseHasErrors,
		FetchFailed: res.err != nil || len(res.out) == 0 || res.response == nil, // transport/empty/parse failure (§3.3)
		StatusCode:  res.statusCode, Arena: l.mergeArena(), // OnFetch* opens ONE MergeSession from this
	}
	if res.response != nil {
		// Only meaningful (and only nil-safe) when a response actually parsed;
		// on a full-hit skip res.response is nil and EmptyEntity is irrelevant
		// because cacheMerge dispatches to OnFetchSkipped, which ignores it.
		in.EmptyEntity = isEmptyEntityFetch(prepared.item, res.response)
	}
	rc := l.cacheRequest() // already created during cachePrepare; non-nil because h != nil
	switch h.Decision {
	case DecisionSkipFullHit, DecisionFetchPartial:
		return rc.OnFetchSkipped(h, in)
	default: // DecisionFetch, DecisionFetchShadow
		return rc.OnFetchResult(h, in)
	}
}
```

The merge hook is gated only on `prepared.cacheHandle != nil`, so it never fires for fetches the controller did not touch (auth/null-parent skips never produced a handle, because `cachePrepare` is gated on `!prepared.skipLoad`).
On a full hit, `mergePhase` returned with `mergeResult` having early-returned `nil` (fetchSkipped) and `callOnFinished` a no-op (the cache hit set no `loaderHookContext`, §4.7), so `cacheMerge` dispatches to `OnFetchSkipped`, which opens a `MergeSession` and splices each `ItemCacheState.FromCache` (already reordered to selection order in `PrepareFetch`) into `ItemCacheState.Item`, and may emit best-effort multi-key backfill / refresh L2 writes when `MustWriteBack`.
On a real fetch, `cacheMerge` dispatches to `OnFetchResult`, which applies the full write gate `!in.FetchFailed && !in.HasErrors && in.ResponseData != nil && in.ResponseData.Type() != astjson.TypeNull` (§3.3) before persisting anything, does negative-hit detection (driven by `EmptyEntity`, the ONE non-failure that still writes a sentinel), re-renders the still-unrendered candidate keys (`ItemCacheState.PendingCandidates`) from the fresh data and writes ALL renderable keys to L1/L2 (or defers, §2.2), and — when `h.Shadow` — runs `CompareShadow` before the writes.
The gate cannot be reduced to `!HasErrors`: a transport, empty-body, or parse failure reaches `OnFetchResult` with `HasErrors == false` (because `responseHasErrors` is assigned only at ~`loader.go:680`, after those early returns) and `ResponseData == nil`, so `FetchFailed` (equivalently `ResponseData == nil`) is the load-bearing signal that stops a failed fetch from being cached (the reviewer's blocking write-gate bug).

### 4.5 S6 — request-end flush via `EndRequest` (and why this covers every root)

The reviewers showed that wiring the flush into `LoadGraphQLResponseData` (as two of the drafts did) MISSES the defer paths: the defer-entry loader fetches via `loader.ResolveFetchNode(response.Response.Fetches)` (CURRENT `resolve.go:510`), and each defer group via `groupLoader.ResolveFetchNode(group.Fetches)` (CURRENT `resolve.go:615`), neither of which calls `LoadGraphQLResponseData`.

RFC-1 instead folds the flush into `EndRequest`, called once per request via `defer ctx.endCacheRequest()` at ALL FOUR request entry functions (two sync, one defer, one subscription):

- sync: `ResolveGraphQLResponse` (CURRENT `resolve.go:339`, calls `LoadGraphQLResponseData` at `362`) AND `ArenaResolveGraphQLResponse` (CURRENT `resolve.go:383`, calls `LoadGraphQLResponseData` at `425`) — both sync entries get the defer, so the arena sync path also flushes L2;
- defer: `ResolveGraphQLDeferResponse` (CURRENT `resolve.go:472`), so the flush runs after `resolveDeferTree` returns (called at CURRENT `resolve.go:570`), i.e. after the parallel `g.Wait()` inside `resolveDeferTree` (CURRENT `resolve.go:715`), single-threaded;
- subscription: `executeSubscriptionUpdate` (CURRENT `resolve.go:878`; loader at `910`, `LoadGraphQLResponseData` at `912`).

`ctx.endCacheRequest()` no-ops when `requestCache` is nil, so it is free when caching was never used.
Because the request-lifetime surface is shared by reference across all groups (§6), one `EndRequest` flushes the deferred L2 writes contributed by the initial fetch AND every defer group.

Justification for deferring the flush to request end (a reviewer must-fix).
The deferred L2 write set holds BYTES, marshaled inside `OnFetchResult` under the lock, so `Flush`/`EndRequest` needs no arena and no lock (§6.4).
L2 writes are a side effect that populates the cache for FUTURE requests; they are not part of the current response, which for incremental delivery is already on the wire (the initial frame is flushed at CURRENT `resolve.go:536`).
So persisting them at request end is correct and simplest.
A per-defer-group early flush (to make L2 visible sooner) is a v2 option (§9); it would add one `Flush` call after each group's render but does not change correctness.

### 4.6 What does NOT change

- `loadPhase` (CURRENT `loader.go:349-358`): zero change; full hits already early-return via `skipLoad`, and the off-lock phase never touches cache state (S3 budget = 0).
- `mergeResult` control flow: zero new branches; only the three additive carrier assignments of §4.4 (`res.response`, `res.responseData`, `res.responseHasErrors`) and the reused `fetchSkipped` early-return.
- The batch dedup loop (CURRENT `loader.go:1645-1693`) and the `len(batchStats) == len(batch)` assertion (`743`): zero change in v1 (full-batch); partial-batch makes the loop cache-aware later (§9).
- The nested-merge branch (CURRENT `loader.go:366-374`): zero change; not hooked in v1, and proven safe (§4.7).
- `preparePhase` / `mergePhase` bodies: zero change; the cache hooks run in `resolveSingle` after each phase returns (§4.3), so the phase lock scopes only the render / merge, never the cache work.
- `NewLoader` signature: unchanged; the shared surface lives on `Context` (`requestCache`) and is created lazily inside `cacheRequest` under the lock, so the 5 `NewLoader` call sites (CURRENT `resolve.go:359/422/504/612/910`) are untouched.

### 4.7 Three correctness invariants, with proof

These are the reviewer-flagged correctness risks; each is resolved against verified CURRENT code, not hand-waved.

1. Skip-path spurious error (blocking).
   Resolved by setting `res.fetchSkipped = true` together with `prepared.skipLoad` on `DecisionSkipFullHit` (§4.3), which hits the existing early-return at CURRENT `loader.go:620-622` and never reaches the empty-response error at `623-624`.

2. Nested-merge gap.
   `res.nestedMergeItems` is declared and read in `mergePhase` (CURRENT `loader.go:115`, `366-369`) but is NEVER assigned anywhere in the repository (verified: a repo-wide search for any assignment to `nestedMergeItems` returns nothing).
   It is a forward-looking, currently-dead field on the defer branch.
   Therefore no cached fetch can ever land on the nested-merge branch in v1, so leaving it unhooked is correct, not a silent data risk.
   Forward guard: when a future change begins populating `nestedMergeItems`, the cache hook must be added INSIDE that loop with per-nested-item handle binding (a distinct `FetchCacheHandle` per nested item), NOT by reusing one whole-fetch handle across each nested item; this RFC records that requirement so the gap cannot be reintroduced silently.

3. OnLoad / OnFinished invariant.
   On `DecisionSkipFullHit`, `loadPhase` is skipped, so `executeSourceLoad` never runs, so `res.loaderHookContext` stays nil (it is set only at CURRENT `loader.go:2072-2079`, inside `executeSourceLoad`).
   `callOnFinished` is guarded by `res.loaderHookContext != nil` (CURRENT `loader.go:453`), so a cache hit does NOT fire `OnFinished` — which is exactly the documented contract: `OnFinished` is called only when `OnLoad` was (CURRENT `loader.go:45-47`).
   No extra guard is needed; a cache hit behaves like every other skip (auth reject, null parent) with respect to LoaderHooks.

---

## 5. The four modes plus shadow ride the SAME interface

The mode is controller-internal config, invisible to `loader.go` (DOSSIER §3.5).
Shadow is not a fifth mode but a cross-cutting variant of any L2-enabled mode.

| Mode | `PrepareFetch` returns | `OnFetchResult` / `OnFetchSkipped` | `EndRequest` |
|---|---|---|---|
| NO-OP | `DecisionFetch, nil` (no key render, no lookup, no handle) | not called (handle nil) | nothing |
| L1-only | render L1 key, coverage-walk the shared L1 map; all covered -> `SkipFullHit` | `OnFetchResult`: write normalized entities to shared L1; `OnFetchSkipped`: splice from L1 | nothing |
| L2-only | render every renderable candidate key, `Get` under ALL of them, multi-candidate select + reorder; all covered -> `SkipFullHit` (+ `MustWriteBack` when a candidate was unrenderable/absent or the value was synthesized) | `OnFetchResult`: re-render pending candidates from fresh data, `Set` all renderable keys or defer; `OnFetchSkipped`: splice + multi-key backfill/refresh | flush deferred Sets, one per cache instance |
| L1+L2 | render both; L1 -> L2 -> subgraph order, coverage at each layer | write L1 + L2; splice + writeback | flush L2 |
| + shadow (L2 variant) | L2 read + stash into `ShadowStash`, return `DecisionFetchShadow` (skipLoad stays false) | `OnFetchResult`: `CompareShadow` (compare -> write-L1 -> write-L2) | flush L2 (fresh) |

The no-op guarantee is structural, not configured (HR2): when `cacheController` is nil the loader never enters cache code, never allocates a handle, never touches a map, never builds the arena facade.
A NO-OP or L1-only controller never produces a `DecisionFetchShadow`, so that case is invisible to those modes.
`DecisionSkipFullHit` is the AND-reduction of the per-item coverage walk (DOSSIER §2.10), not a single per-fetch lookup; one uncovered item degrades to `DecisionFetchPartial` (partial mode) or `DecisionFetch` (all-or-nothing).

### 5.1 Capability coverage (HR3 a-i)

- (a) Pre-fetch lookup + best-effort multi-key render + runtime `ProvidesData` coverage validation + multi-candidate freshness selection + reorder-before-merge (DOSSIER §2.10): entirely inside `PrepareFetch`, driven by `cfg.KeySpec.Candidates` + `cfg.ProvidesData`; it renders every candidate key it can (the renderable subset into `ItemCacheState.RenderedKeys`, the rest into `PendingCandidates`), looks the cache up under all rendered keys, then the candidate-collect -> freshness-sort -> coverage-validate -> reorder walk runs through the `MergeSession` and writes the per-item `ItemCacheState`. Always-on, gated only by `cfg.ProvidesData != nil`, not by partial mode.
- (b) Shadow via the `FetchShadow` decision (DOSSIER §2.11): `DecisionFetchShadow` + `handle.Shadow` + `handle.ShadowStash`; `CacheObserver.CompareShadow` in `OnFetchResult` before the writes; entity-fetch-only comparison, root-field force-refetch only (the OLD asymmetry).
- (c) Post-fetch write as best-effort multi-key render-then-backfill (DOSSIER §2.2): `OnFetchResult` (real fetch) and `OnFetchSkipped` (hit that still writes back via `MustWriteBack`) re-render the candidate keys that were unrenderable at lookup (`ItemCacheState.PendingCandidates`) from the fresh data and, if more keys are now renderable, populate the cache for ALL renderable keys (otherwise just the `RenderedKeys` from lookup); `ItemCacheState.{RenderedKeys,PendingCandidates,NeedsWriteback,WriteReason}` carry the contract. This generalizes and replaces the OLD requested-vs-rendered-key + missing-key-set framing.
- (d) Negative caching (DOSSIER §2.6): `ItemCacheState.{NegativeHit, FromCache=TypeNull}`; `MergeInput` surfaces ALL FIVE §2.6 failure signals — `FetchFailed` (transport/empty/parse, the two the prior draft dropped + parse), `HasErrors`, `EmptyEntity`, `StatusCode`, and `ResponseData == nil` — so `OnFetchResult` reproduces the OLD null detection AND correctly distinguishes a successful-but-empty entity fetch (negative write) from a transport/empty/parse failure (no write at all, §3.3); the loader's `setSkipErrors`/`isEmptyEntityFetch` paths are untouched.
- (e) Partial / batch splice-merge: the loader SEAM exists in v1 (`DecisionFetchPartial`, `MergeInput.BatchStats` in both hooks, `ItemCacheState.BatchIndex`); the realign + dedup-loop rewrite is staged to v2 (§9).
- (f) End-of-tree batched L2 flush (DOSSIER §2.2): `EndRequest` (§4.5).
- (g) Analytics / trace observer (DOSSIER §2.7): the `CacheObserver` port (§3.5); v1 ships it nil (no-op), v2 wires the walker hooks.
- (h) Per-request shared L1 (request-lifetime cache state) visible across per-defer-group Loaders: the `RequestCache` lives on the by-reference-shared `Context` (§6).
- (i) Lifecycle (begin/end request): `CacheController.BeginRequest` (lazy, once) and `RequestCache.EndRequest` (once, after the whole tree).

---

## 6. Thread-safety under defer

### 6.1 The constraints (DOSSIER §2.8, §7 risks 1/2/7)

- The Loader is per-defer-group: `resolveDeferSingle` builds a fresh `NewLoader` per group (CURRENT `resolve.go:612`); the initial loader is separate (`resolve.go:504`); they share only the request `DataBuffer` and `arena`.
- There is exactly ONE `DataBuffer` per request; its `Lock()` serializes every prepare and merge across the initial tree and every defer group.
- The `jsonArena` is shared across groups and serialized ONLY by `DataBuffer.Lock` (CURRENT `resolve.go:607-611`); off-lock arena use is a data race.

### 6.2 Where the shared state hangs and how it is created

The request-lifetime shared cache state — the `RequestCache`, owning the per-request L1 entity store, the mutation-inheritance flags, the analytics accumulator, and the deferred L2 write set — lives on `Context`, in the unexported `requestCache` field, NOT on any per-group Loader.
It exists for the lifetime of ONE request and is shared by reference across that request's per-defer-group Loaders.

This per-request L1 store is intrinsic to L1 caching (it is how the L1 store survives across defer groups, the final implementation phase) and has NOTHING to do with the OLD `@requestScoped` directive feature: that directive (and its `requestScopedL1` map, `RequestScopedFieldPolicy`/`RequestScopedFieldConfig` types, pre-planning widening rewrite, and inject/export hooks) is OUT OF SCOPE and removed entirely from both RFCs; only the request-lifetime sharing of the L1 store is retained, renamed away from "request scoped" to avoid confusion.

| Shared request-lifetime cache state | Lives on | Guard |
|---|---|---|
| L1 entity store (OLD `loader.go:349`) | `RequestCache` on `Context` | `DataBuffer.Lock` |
| mutation populate-inheritance flags (OLD `loader.go:215/219`) | `RequestCache` on `Context` | `DataBuffer.Lock` |
| analytics accumulator (OLD `context.go:55`) | `RequestCache` on `Context` | `DataBuffer.Lock` (accumulate) / single-thread (finalize) |
| deferred L2 write set (OLD `loader.go:677`) | `RequestCache` on `Context` | `DataBuffer.Lock` (append) / single-thread (flush) |

`Context` is shared by reference across defer groups — the parallel defer branch does NOT clone the Context, its only defer-time mutation (`ctx.subgraphErrors`) being written exclusively under `dc.db.Lock()` (verified comment CURRENT `resolve.go:698-704`).
So L1 populated by one group is visible to groups scheduled after it (HR3-h).

`requestCache` is created lazily, ONCE, under `DataBuffer.Lock`, inside `cacheRequest` (called from `cachePrepare` in `resolveSingle`, §4.3).
`cacheRequest` takes `DataBuffer.Lock` for just the once-create read-and-set of `ctx.requestCache`, so the lazy `BeginRequest` is race-free across the parallel fetches of a group and across per-defer-group Loaders, with no extra mutex and no `NewLoader` signature change.
This correctness depends on there being exactly one `DataBuffer` per request, which is verified (CURRENT `resolve.go:503/612` both pass the same `db`); if a future change introduced a second `DataBuffer` per request, the lazy-init would need a dedicated mutex.

### 6.3 Subscription isolation

`Context.clone` (CURRENT `context.go:283-305`, used for the subscription-event re-resolve at CURRENT `resolve.go:1103`) sets `cpy.requestCache = nil`, so a genuinely separate resolution builds its own L1 — correct isolation across subscription events.
The `cacheController` port is copied by value (`cpy := *c`), so the clone still has caching available; only the per-request mutable surface is reset.
Subscription cache wiring (the trigger lifecycle, invalidation) is itself v2 (§9); RFC-1 fixes only the clone seam so events cannot share L1 once subscription caching lands.

### 6.4 How it is lock-guarded, and the off-lock rule

- `PrepareFetch`, `OnFetchSkipped`, `OnFetchResult` run from `resolveSingle` with NO loader lock held, and EACH opens ONE `MergeSession` via `MergeArena.Begin()` (which takes `DataBuffer.Lock`) for its whole arena sequence, releasing it with `Close()` (§3.4).
  So all mutation of the shared maps, all arena allocation, and all candidate parse / merge-synthesis / reorder happen under the session's single `DataBuffer.Lock` hold; the controller needs no internal lock for its request-lifetime state.
  The session is a SCOPED lock taken ONCE around the multi-op sequence, not re-acquired per arena op — that is the contention reduction the scoped facade buys (§3.4).
- The off-lock `loadPhase` never touches the controller, the maps, or the arena; the only thing caching contributes to it is the `skipLoad` bool set during prepare. Opening a `MergeSession` from `loadPhase` is forbidden (DOSSIER §7 risk 7).
- A `MergeArena`/`MergeSession` is handed to the controller only for the duration of one hook; the controller must not retain it past the call, and every arena op is a `MergeSession` method so there is no way to touch the arena outside a held session.
  v1 does the L2 `Get` inside `PrepareFetch`'s session (matching the dossier's "splice under lock", DOSSIER §2.1); moving the L2 read off-lock is a v2 optimization that would do the network `Get` BEFORE `Begin()` and parse into the arena only inside the session (DOSSIER §7 risk 7) — the same `Begin()`/`Close()` API, still one acquisition.
- The GC-arena hazard (a heap `*Value` stored inside arena-noscan memory, CURRENT `loader.go:218-227`) is avoided because every cached `*Value` stored in arena containers is produced via `MergeSession.ParseBytes`/`StructuralCopy`, exactly as the OLD `populateL1Cache` did on the main thread.
- `EndRequest` runs single-threaded after the whole tree resolves (§4.5); the deferred L2 write set holds bytes, so the flush needs neither lock nor arena.

---

## 7. Self-contained caching configuration

### 7.1 Module boundary (HR8)

- A NEW package `v2/pkg/engine/resolve/cache` holds ALL implementation: the controller implementations (NO-OP is a nil controller; plus L1 / L2 / L1+L2), the `CacheKeyTemplate` and the two OLD templates, the `CacheKeyVariableRenderer`, the transform pipeline (OLD `loader_cache_transform.go`), candidate selection / coverage validation / reorder, analytics (OLD `cache_analytics.go`), the dormant `circuit_breaker.go`, and subscription/trigger cache (OLD `trigger_cache.go`).
  It imports `resolve`; `resolve` never imports it (no cycle).
- `resolve` keeps ONLY: the contract types (§3, two new files), the opaque `cacheHandle` slot on `preparedFetch`, the `Cache *FetchCacheConfig` slot on the three fetch structs, the `cacheController`/`requestCache` fields on `Context`, and the four glue helpers in `loader.go`.
  No cache LOGIC lives in `resolve`.
- On the defer branch the OLD cache files were already deleted (DOSSIER §7 risk 8), so this is greenfield: the cache package is purely additive, there is no literal file move of runtime code, only a clean re-home of the logic from the OLD branch.

Why `FetchConfiguration.Equals` forces `FetchCacheConfig.Equals` (HR8): plan dedup deep-compares fetch configs (CURRENT `fetch.go:283-309`, DOSSIER §7 risk 11).
If two otherwise-identical fetches differ only in cache policy, they must not dedup to one (silent policy loss), and two identical fetches with equal-but-distinct config instances must still dedup (a dedup miss otherwise).
So the config exposes a concrete, compile-checked `Equals(*FetchCacheConfig) bool`, called via the nil-safe clause of §3.8.
The runtime HANDLE (§3.7) needs no `Equals`: it is per-execution, never part of a cached plan.

### 7.2 The plan-side policy model (parallel to FederationMetaData)

This is the dossier §5.4 model, defined as its OWN types in a `cacheconfig` (plan-side) package, never inside `FederationMetaData`:

```go
type CachingConfiguration struct { // sibling of FederationMetaData on the DataSource config, subgraph-local
	Entities      []EntityCachePolicy
	RootFields    []RootFieldCachePolicy
	Mutations     []MutationCachePolicy
	Subscriptions []SubscriptionCachePolicy
	KeySpecs      []CacheKeySpec // frozen multi-key specs (candidate key templates) from @key at plan time
}

type EntityCachePolicy struct {
	TypeName, CacheName         string
	TTL, NegativeCacheTTL       time.Duration
	IncludeSubgraphHeaderPrefix bool
	EnablePartialCacheLoad      bool
	HashAnalyticsKeys           bool
	ShadowMode                  bool
}
type RootFieldCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	ShadowMode, PartialBatchLoad   bool
}
type MutationCachePolicy struct {
	FieldName              string
	Invalidate, PopulateL2 bool
	TTLOverride            time.Duration
}
type SubscriptionCachePolicy struct {
	TypeName, FieldName, CacheName string
	TTL                            time.Duration
	IncludeSubgraphHeaderPrefix    bool
	EnableInvalidationOnKeyOnly    bool
}
```

### 7.3 The `CacheConfigProvider` port

```go
// CacheConfigProvider exposes caching policy WITHOUT touching the FederationInfo
// interface. A datasource config returns nil when caching is not configured, so a
// NO-OP build never constructs any caching type (DOSSIER §5.5). It replaces the 4
// caching methods the OLD branch bolted onto FederationInfo (DOSSIER §5.1).
type CacheConfigProvider interface {
	EntityPolicy(typeName string) (EntityCachePolicy, bool)
	RootFieldPolicy(typeName, fieldName string) (RootFieldCachePolicy, bool)
	MutationPolicy(fieldName string) (MutationCachePolicy, bool)
	SubscriptionPolicy(typeName, fieldName string) (SubscriptionCachePolicy, bool)
	KeySpec(scope CacheScope, typeName, fieldName string) (CacheKeySpec, bool)
}

// New accessor on the datasource config, parallel to (not on) FederationInfo:
//   func (d dataSourceConfiguration[T]) Caching() CacheConfigProvider // nil => no caching
```

### 7.4 The federation boundary, explicitly

```
FederationMetaData.Keys (@key selection sets) + interfaceObject/entityInterface remaps + EntityKeyMapping
        │  PLAN-TIME INPUT ONLY (read once, by value, by RFC-2)
        ▼
CacheKeySpec  (frozen: Scope, TypeName, FieldName, Candidates []CacheKeyCandidate (each a *Object Representation with __typename remap baked in), EntityKeyMappings)
        │  packed into
        ▼
resolve.FetchCacheConfig  (federation-free runtime config; imports NO federation types)
        │  carried on the fetch structs, consumed by the controller
        ▼
loader.go  (forward-only; never reads a field)
```

- KEY-DERIVATION coupling is legitimate and plan-time only: `@key` selection sets, entity membership (`HasEntity`), interfaceObject/entityInterface `__typename` remaps, and root-arg <-> `@key` mappings are read ONCE by RFC-2 and frozen into `CacheKeySpec` (DOSSIER §5.3).
- POLICY coupling is removed: no `fedConfig.EntityCacheConfig`/`RootFieldCacheConfig` lookups at decision time; policy comes from `CacheConfigProvider` only.
- The RUNTIME side (`FetchCacheConfig`, the loader, the controller) imports NO federation type; `@key` reaches the runtime only as the already-frozen `CacheKeySpec` inside `FetchCacheConfig`.
- The cache package builds ONE `CacheKeyTemplate` per candidate in `CacheKeySpec.Candidates`; each template is the sole source of truth for read, write, and invalidate of THAT candidate key, enforcing the byte-identical-keys invariant (DOSSIER §2.3) across the best-effort multi-key lookup and backfill.

The OLD runtime contract `resolve.FetchCacheConfiguration` was ALREADY federation-free (DOSSIER §5.2), so the decoupling is a plan-time refactor — the runtime never imported federation in the first place.

### 7.5 Migration note for external config producers (HR9)

Today cosmo populates caching fields ON `FederationMetaData`, and caching rides the `FederationInfo` interface plus `dataSourceConfiguration[T]` forwarders (DOSSIER §5.1, §5.5).
On THIS branch those fields no longer exist: defer already removed the per-fetch `Caching`, `FetchInfo.ProvidesData`, `ExecutionOptions.Caching`, and the `FederationMetaData` caching collections (DOSSIER §7 risk 8) — so there is nothing to be backward-compatible WITH on the engine types, which makes the migration cleaner.

Recommended path:

- Provide a one-release compatibility shim in the plan layer, `cacheconfig.FromFederation(...) CachingConfiguration`, that reads cosmo's existing caching source-of-truth and produces the new `CachingConfiguration`, so existing cosmo deployments keep working unchanged.
- In parallel, migrate cosmo's config producer to populate `CachingConfiguration` directly and to return a real provider from `dataSourceConfiguration[T].Caching()`.
- Once cosmo is migrated, drop the shim.
- Preserve the behavior-critical rules during translation (DOSSIER §5.5): `RootFieldCachePolicy`'s all-root-fields-share-identical-policy-or-L2-disabled rule; the dual mutation-populate semantics (`EnableEntityL2CachePopulation` reused for single-subgraph populate); and that a true NO-OP now needs the structural gate (§10), which the OLD `DisableEntityCaching` lacked (it only disabled L2 while still building L1 and key templates).
- Because both gates default OFF (§10), cosmo can ship the engine upgrade with no controller set first (pure no-op), then wire `SetCacheController` and populate `CachingConfiguration` in a later, isolated change.

This is primarily RFC-2's responsibility; RFC-1 fixes the boundary so the migration is a plan-time refactor, never a runtime one.

---

## 8. The OUTPUT contract for RFC-2

RFC-2 (the caching planner pass) MUST satisfy exactly this loader-side contract.
If it does, the loader integration in this RFC consumes its output with zero further loader edits.

1. Per concrete fetch type (`*SingleFetch`, `*EntityFetch`, `*BatchEntityFetch`), stamp a fully-populated `*resolve.FetchCacheConfig` (§3.6) onto the fetch's `Cache` field, running AFTER `createConcreteSingleFetchTypes` so all three carry it (DOSSIER §6.4).
   A nil `Cache` means caching off for that fetch and is the gate of §10.
2. Self-contained AND multi-key: `FetchCacheConfig` and its `CacheKeySpec` contain NO federation types or pointers into `FederationMetaData`; ALL of an entity's `@key` sets are read once and frozen by value into `CacheKeySpec.Candidates` as a LIST of candidate key templates, NONE marked required (each is best-effort renderable at runtime, §3.6); a single-key entity simply yields a one-element list (§7.4, DOSSIER §6.5).
3. Carry the full runtime payload the controller needs: the L1/L2 enable flags, `CacheName`, `TTL`, `NegativeCacheTTL`, `IncludeSubgraphHeaderPrefix`, `EnablePartialCacheLoad`, `PartialBatchLoad`, `ShadowMode`, `HashAnalyticsKeys`; the frozen `CacheKeySpec` (the list of `Candidates []CacheKeyCandidate`, each carrying a `*Object` Representation with the `__typename` remap baked in — the multiple @key sets, none required — plus `EntityKeyMappings`); the mutation `PopulateL2OnMutation`/`MutationTTLOverride`; and the fetch-level `ProvidesData *Object` consumed at RUNTIME by the coverage walk (DOSSIER §2.10), packed into the config rather than re-added as a `FetchInfo` field.
4. `Equals`-stable: two fetches with differing cache config must produce `FetchCacheConfig` values that `Equals` distinguishes (§3.8), or plan dedup corrupts cache behavior (§7.1).
5. Request-independent: the postprocessed plan is cached and reused across requests (CURRENT `execution_engine.go:304-305`); write only static config, and derive all per-request key material (variable values, header hash, global prefix) at loader prepare time inside `PrepareFetch`, not baked in.
6. Walk EVERY fetch tree (root + each defer group), respecting `DeferID` as fetch identity (`EqualSingleFetch` guard CURRENT `fetch.go:50-53`).
7. Gate off entirely when caching is NO-OP (the pass does not run, or stamps nil), composing with RFC-1's runtime nil-check (§10).
8. Provide the plan-side `CacheConfigProvider` (§7.3) as the policy source, decoupled from `FederationInfo`/`dataSourceConfiguration[T]`.

RFC-2's own structure (one additive `FetchTreeProcessor` after `createConcreteSingleFetchTypes`, plus keeping `optimize_l1_cache.go`) is specified in DOSSIER §6 and is out of scope here.
The OLD `@requestScoped` directive feature (and its pre-planning widening rewrite) is OUT OF SCOPE for both RFCs and is not part of this contract.

---

## 9. v1 vs v2 staging

| Capability | v1 | v2 |
|---|---|---|
| NO-OP / L1-only / L2-only / L1+L2 full-response + full-batch caching | yes | — |
| Per-item coverage + multi-candidate freshness + reorder (DOSSIER §2.10) | yes (always-on, runs inside `PrepareFetch`, gated only by `ProvidesData != nil`) | — |
| Full-hit splice + best-effort multi-key render-then-backfill / refresh | yes | — |
| Negative caching | yes | — |
| Shadow mode | yes (entity-fetch compare; root-field force-refetch only, the OLD asymmetry) | — |
| End-of-tree batched L2 flush | yes (`EndRequest`) | — |
| Per-request shared L1 (request-lifetime state) across defer groups | yes | — |
| Analytics / trace | `CacheObserver` port present, nil (no-op) | full accumulators + walker `OnEntity`/`OnFieldValue` hooks |
| Partial L1 entity caching | SEAM only (`DecisionFetchPartial`) | impl: cache-aware dedup loop (CURRENT `loader.go:1645-1693`) + response realign |
| Partial batch load | SEAM only (`ItemCacheState.BatchIndex`, `MergeInput.BatchStats`) | impl: filtered variables + realign (OLD `caching.go:299`, `loader_cache.go:1591`) |
| Nested-merge (`res.nestedMergeItems`) cache interplay | not hooked (proven dead, §4.7) | hook with per-nested-item handle binding, IF a writer is introduced |
| Subscription / mutation invalidation | out (clone seam fixed, §6.3) | trigger `OnTriggerInit`/`OnTriggerEvent` hook + `EntityCachePopulation` plan annotation |

Critical staging note (DOSSIER §2.4 caveat, OQ 1): "full-batch needs zero loop change" is true ONLY of the loader's dedup/realign loop and the variable-filter path.
Hit determination is per-item even for full-batch: the controller's candidate-collect -> freshness-sort -> coverage-validate -> reorder walk ships in v1 regardless, and its AND-reduction yields `DecisionSkipFullHit`.
Staging only defers the realign / variable-filter seam.

Partial seam fidelity (a reviewer must-fix): when `DecisionFetchPartial` is implemented in v2, BOTH the fresh-fetch merge (`mergeResult` over the filtered response) AND the cached-subset splice (`OnFetchSkipped`-style) occur — it is not splice-only.

---

## 10. NO-OP gating (HR10)

### 10.1 Two independent gates compose

| | planner NO-OP (every `Cache` nil) | planner ON (configs stamped) |
|---|---|---|
| runtime NO-OP (`cacheController == nil`) | nothing happens (the common default) | configs present but never consulted -> NO-OP |
| runtime ON (controller set) | controller present but `PrepareFetch` never called (cfg nil) -> NO-OP | caching active |

1. Runtime gate (RFC-1): `l.ctx.cacheController == nil` ⇒ `cacheRequest` returns nil ⇒ both call sites skip ⇒ the loader path is byte-identical to today, with no key render, no lookup, no handle allocation.
2. Per-fetch gate (RFC-1 + RFC-2): `item.Fetch.Cache == nil` (or `!L1 && !L2`) ⇒ `cachePrepare` returns before calling `PrepareFetch`, even when a controller is present.
3. Planner gate (RFC-2): if the post-pass does not run, every fetch's `Cache` stays nil.

The lower of the two gates always wins, and the lowest is NO-OP, so "L1-only runtime + NO-OP planner" or "configs stamped + no controller" both degrade to a full NO-OP by construction.
A real cache requires BOTH a non-nil controller AND a non-nil per-fetch config.
There is no third place caching can switch on: the loader's only knowledge of caching is the two hook calls plus the request-end flush, all guarded by these gates.
This is the gate the OLD branch lacked: `DisableEntityCaching` only disabled L2 while still building L1 and key templates (DOSSIER §5.5).

### 10.2 The commit is a strict no-op before any cache exists

With no `SetCacheController` call, `l.ctx.cacheController` is nil everywhere, every `Cache` field is nil, and the `FetchConfiguration.Equals` clause sees both-nil and returns its prior result.
The three additive `result` fields (`response`, `responseData`, `responseHasErrors`) are written by `mergeResult` but never read.
The `preparedFetch.cacheHandle` field stays nil.
The contract types compile standalone (they reference only `resolve` types plus `astjson`/`time`/`arena`).
Therefore merging RFC-1's commit ALONE — before any `cache`-package implementation exists — changes runtime behavior in ZERO ways (G8).

---

## 11. Risks and open questions

Carried from DOSSIER §7/§8, with how this design addresses each:

- Risk 1 (pipeline gone / thread-safety inverted): designed against the 3 phases, never ported (§2.2, §4).
- Risk 2 (per-group Loader / L1 lifecycle): one `RequestCache` on the by-reference-shared `Context`, guarded by `DataBuffer.Lock` (§6).
- Risk 3 (mutation populate inheritance): explicit request-lifetime carry on `RequestCache` + `FetchCacheConfig.PopulateL2OnMutation`/`MutationTTLOverride` (§3.6, §6.2).
- Risk 5 (responseData not surfaced): carrier-widening on `result`, set where already computed (§7 below, §4.4).
- Risk 7 (off-lock arena race): arena reachable only through a `MergeSession` whose `Begin()`/`Close()` bracket a single `DataBuffer.Lock` hold per hook; never opened from `loadPhase`; L2 read inside the prepare session in v1 (§3.4, §6.4).
- Risk 9 (Object/Field Copy drift): no `Object`/`Field` fields added in v1 (`ProvidesData` folded into config, analytics staged), so no `Copy()` edit in this commit (§3.3, §3.6).
- Risk 11 (Equals deep-compares config): concrete `FetchCacheConfig.Equals` + nil-safe clause (§3.8, §7.1).
- Risk 15 (`result` field collision, `fetchSkipped` vs `cacheSkipFetch`): unified onto `fetchSkipped` + `skipLoad` (§4.3, §4.7).
- Risk 16 (single-flight precedence): the cache short-circuits BEFORE single-flight; a miss/partial falls through to single-flight on the canonical input, and the L2 key is derived from the same canonical pre-injection form single-flight dedups on (§3.6) — flagged for the impl PR to wire precedence explicitly.
- Risk 17 (cache-key fidelity vs post-prepare input mutation): promoted to a hard contract clause — keys derived from `PrepareFetchInput.Input` (canonical pre-injection bytes) + `HeaderHash`, independent of the post-prepare `extensions`/`fetch_reasons` injection at CURRENT `loader.go:1940/1956` (§3.6).
- Risk 18 (astjson `MergeValues` 3->2 returns): the `MergeSession` facade hides the `changed` bool, matching how defer already discards it (§3.4).

Open questions for review:

- OQ 1 (partial scope): v1 ships full-response/full-batch + the always-on per-item coverage walk; partial realign is v2 (§9).
- OQ 5 (analytics scope): v1 ships the `CacheObserver` port nil; full analytics + the walker hooks are a separate additive change to avoid the defer walker collision (§3.5, §9).
- OQ 6/7 (request-lifetime state home + defer L1 visibility): state lives on `Context` (the established setter pattern), shared by reference; a defer group's L1 writes are visible to fetches scheduled AFTER that group's `cacheMerge` — confirm this ordering is the intended L1 semantics.
- OQ 10 (subscription/mutation): out of v1; the clone-nil seam is fixed now (§6.3), the trigger lifecycle hook is v2.

---

## Appendix A: HR checklist

| HR | Where | One-line resolution |
|---|---|---|
| HR1 | §4 | 2 behavior call-sites + 4 lifecycle defers (one per entry function); ~70-100 LOC in existing files; file-by-file table (§4.1). |
| HR2 | §3.1, §5 | ONE mode-blind `RequestCache`; nil controller zero-cost; factory tier justified, not a second loader-facing surface. |
| HR3 | §5.1 | a-i each mapped to a hook/port + seam. |
| HR4 | §3.7 | typed two-level `FetchCacheHandle` + `ItemCacheState`, mapped 1:1 to OLD `CacheKey`/`cachedData`. |
| HR5 | §6 | `RequestCache` on by-ref `Context`, lazy-init + all hooks under `DataBuffer.Lock`, `MergeArena` off-lock-forbidden, clone-nil. |
| HR6 | §7 | `FetchCacheConfig` federation-free, `CacheKeySpec` frozen, `CacheConfigProvider` port, boundary diagram (§7.4). |
| HR7 | §7-below, App. B | widen `result` with `response` (full, for `isEmptyEntityFetch`) + `responseData` + `responseHasErrors`, set where already computed; not re-parse, not signature change. |
| HR8 | §3.8, §7.1 | new `cache` package one-way; concrete compile-checked `Equals` forced by `FetchConfiguration.Equals`. |
| HR9 | §7.5 | one-release `FromFederation` shim; both gates default off so cosmo migrates in stages. |
| HR10 | §10 | two independent gates compose; half-on is impossible by construction. |
| no-op-before-impl | §10.2 | controller nil + all `Cache` nil + unused result fields ⇒ zero behavior change. |

## Appendix B: responseData surfacing decision (HR7)

Choice: WIDEN THE RESULT CARRIER (add `result.response` + `result.responseData` + `result.responseHasErrors`), NOT widen `mergeResult`'s signature, and NOT re-parse `res.out`.

- Re-parsing `res.out` in the hook is wrong on cost and correctness: it duplicates the parse on every cached fetch, and forces the hook to re-derive `SelectResponseDataPath`, the batch array, and the null/empty-entity decisions `mergeResult` already computes (CURRENT `loader.go:645-712`); two derivations of "what is the response data" is how read/write key spaces silently diverge (DOSSIER §2.3).
- Widening `mergeResult`'s signature is the larger, more error-prone diff: it has ~15 return points, and threading the extra return values ripples through the `nestedMergeItems` loop and every caller (CURRENT `loader.go:366-378`).
- Three fields, not two, because the cache needs TWO different views of the response: `responseData` is the DATA sub-path (`response.Get(SelectResponseDataPath...)`, CURRENT `loader.go:646`) the controller writes to cache, while `isEmptyEntityFetch` (the negative-hit / write-gate signal, HR3-d) consumes the FULL response (`response.Get("data","_entities")`, CURRENT `loader.go:686/788-799`). Surfacing both as carrier fields lets the loader keep `isEmptyEntityFetch` as the SINGLE source of truth for empty-entity detection — `cacheMerge` computes `EmptyEntity` from `res.response` (guarded by `res.response != nil`) rather than re-implementing the rule in the cache package, which would be a second derivation prone to drift (DOSSIER §2.3).
- Widening the carrier is minimal and safe: `result` is already the per-fetch state object passed through all three phases and already carries `batchStats` (CURRENT `loader.go:113`); the assignments set exactly the values `mergeResult` already computed, with `res.response` set right after the parse (CURRENT `loader.go:628-635`, before every early return) and `res.responseData`/`res.responseHasErrors` placed so both precede the null-data early returns at CURRENT `loader.go:686-688/711` (so negative-cache detection sees the right flags); `response`/`responseData` are arena-backed and read by `OnFetchResult` inside the `MergeSession` it opens in `cacheMerge` (the request-lifetime `jsonArena` is not reset between `mergePhase` and `cacheMerge`, so the pointers stay valid); and all three fields are dead weight when caching is off, preserving the no-op guarantee.
- Because `res.response` is assigned only on a successful parse, `res.response == nil` (and equivalently `res.responseData == nil`) is the structural fetch-failed signal the write gate keys off (§3.3), so the same carrier widening that surfaces the data ALSO supplies the failed-fetch no-write gate — one decision, three uses (write data, empty-entity routing, fetch-failed gate).
- The shadow compare (DOSSIER §2.11) needs the same `responseData`, so one decision serves it too.

## Appendix C: OLD-capability -> new-home mapping (proving nothing is dropped)

| OLD capability / call-site cluster (DOSSIER §) | OLD site | New home |
|---|---|---|
| Render L1/L2 keys (read/write/invalidate, one format) (§2.3) | `prepareCacheKeys`, `rootFieldL2CachePrefix`, invalidate path | one `CacheKeyTemplate` per candidate in `CacheKeySpec.Candidates`, in `cache` pkg (§7.4) |
| Per-entity L1 lookup + skip (§2.1) | `tryL1CacheLoad` | `RequestCache.PrepareFetch` -> `DecisionSkipFullHit` |
| Bulk/sequential L2 read (§2.1) | `bulkL2Lookup`/`tryL2CacheLoad` | `RequestCache.PrepareFetch` (inside its `MergeSession`, v1) |
| Full-hit short-circuit (§2.1) | `res.cacheSkipFetch` | `prepared.skipLoad` + `res.fetchSkipped` (§4.3) |
| Coverage validation (§2.10) | `validateItemHasRequiredData` family | inside `PrepareFetch`, from `cfg.ProvidesData` |
| Multi-candidate freshness + reorder (§2.10) | `resolveMultiCandidateCacheValue`, `reorderCacheValueToSelectionOrder` | inside `PrepareFetch`, via the `MergeSession`; written to `ItemCacheState` |
| Full-hit splice (§2.5) | `loader.go:1517-1540` merge | `RequestCache.OnFetchSkipped` |
| Batch hit/partial/empty merge (§2.5) | `mergeBatchCacheHit`/`PartialResponse`/`EmptyResponse` | `OnFetchSkipped` (full-batch v1; partial v2) |
| L1 entity store + root->entity promotion (§2.2) | `populateL1Cache`, `populateL1CacheForRootFieldEntities` | `OnFetchResult` -> shared L1 on `RequestCache` |
| L2 set + write-set + key-sync + TTL (§2.2) | `updateL2Cache`/`prepareL2CacheSet`/`prepareL2WriteKeys` | `OnFetchResult` (write-through or defer) |
| Best-effort multi-key render-then-backfill (re-render unrendered candidates from data, populate all renderable keys, reason) (§2.2) | `shouldWriteRequestedKey`/`shouldWriteRenderedKey`, `CacheWriteReason` | `OnFetchSkipped`/`OnFetchResult`, via `ItemCacheState.{RenderedKeys,PendingCandidates,WriteReason,NeedsWriteback}` |
| Deferred batched flush (§2.2) | `writeL2CacheSetContributors` | `RequestCache.EndRequest` (§4.5) |
| Negative caching (§2.6) | `cacheKeysToNegativeEntries`, `NegativeCacheHit` | `ItemCacheState.{NegativeHit,FromCache=Null}`; `MergeInput.EmptyEntity` |
| Variable/representation remap for partial batch (§2.4) | `filterBatchVariablesForPartialFetch`, `CacheKey.BatchIndex` | `DecisionFetchPartial` + `ItemCacheState.BatchIndex` (seam v1, impl v2) |
| Transform pipeline schema<->alias (§2.5) | `loader_cache_transform.go` | `cache` pkg, via `MergeSession.StructuralCopy` |
| Analytics accumulators + ~22 trace fields (§2.7) | `result` analytics fields, `cacheTrace*`, `buildCacheTrace` | `FetchCacheHandle.Analytics` + `CacheObserver` (§3.5) |
| Walker-inlined analytics (§2.7) | `resolvable.go:830-880/672-708` | `CacheObserver.OnEntity`/`OnFieldValue` (v2) |
| Subgraph fetch timing (§2.7) | `executeSourceLoad` timing | `CacheObserver` (v2) |
| Per-request L1 entity store (request-lifetime, §2.8) | `l.l1Cache` | `RequestCache` on `Context`, lock-guarded (§6.2) |
| Mutation populate inheritance (§2.8) | `enableMutationL2CachePopulation` etc. | `RequestCache` flags + `cfg.PopulateL2OnMutation`/`MutationTTLOverride` |
| Lifecycle init/finalize (§2.9) | `LoadGraphQLResponseData` init, `Free`, `initCacheAnalytics` | `CacheController.BeginRequest` (lazy) / `RequestCache.EndRequest` |
| Invalidation, subscription per-event cache (§2.9) | `processExtensionsCacheInvalidation`, `trigger_cache.go` | trigger lifecycle hook (v2); helpers moved to `cache` pkg |
| Shadow read + stash + force-fetch (§2.11) | `prepareL2LookupState`/`saveShadowCachedValue` | `DecisionFetchShadow` + `FetchCacheHandle.{Shadow,ShadowStash}` |
| Shadow compare + event (§2.11) | `compareShadowValues`, `ShadowComparisonEvent` | `CacheObserver.CompareShadow` (compare -> write-L1 -> write-L2) |
| Circuit breaker (dormant) (§4.3) | `circuit_breaker.go` | `cache` pkg (move-only) |
| Config on `FederationMetaData` / `FederationInfo` (§5.1) | 6 collections + 4 methods + forwarders | `CachingConfiguration` + `CacheConfigProvider` (§7.2-7.3) |
