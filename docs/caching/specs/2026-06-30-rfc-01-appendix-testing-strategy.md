# RFC-1 Appendix: Testing strategy for the loader cache abstraction

Status: final for review.
Author: caching working group.
Companion: RFC-1 (`2026-06-30-rfc-01-loader-cache-abstraction.md`) and RFC-2 (`2026-06-30-rfc-02-caching-planner.md`).
Ground truth for this appendix: the coverage matrix (`scratchpad/caching-rfc/explore-coverage-scenarios.md`),
the supergraph + plan-drift design (`scratchpad/caching-rfc/explore-supergraph-plandrift.md`),
and the conventions survey (`scratchpad/caching-rfc/explore-conventions.md`).

This is the detailed plan for achieving ~100% unit-test coverage of EVERY caching loader code path,
using the RFC-1 interface fakes, with no network and no real cache backend.
It is written so a Codex session can build the harness and the catalog row-by-row,
with every row asserting on FULL values via `assert.Equal` (never `assert.Contains`, never fuzzy comparisons).

Target Go 1.25: tests use `t.Context()`, table tests, `t.Run`, type-safe atomics (`atomic.Int64`),
`slices`/`maps`/`cmp`, `for i := range n`, `errors.Is`, `wg.Go(fn)`,
and — for every time/TTL/ordering concern — the Go 1.25 stdlib `testing/synctest` (`synctest.Test`, `synctest.Wait`).
There is NO custom clock double and NO injectable `now func` seam anywhere in the strategy:
the cache and loader call REAL `time.Now`/`time.Since`/`time.Until`/`time.Duration` comparisons,
and `synctest` fakes the clock around them (§2.5).

---

> REVISION 2 (maintainer feedback — see PLAN §R2.10, which is AUTHORITATIVE and overrides this appendix on conflict).
> Drop the `.golden` / plan-drift-snapshot approach entirely: assert COMPLETE responses with full-value `assert.Equal` (never `assert.JSONEq`, never fuzzy).
> Build the integration/e2e framework ON TOP OF the `execution` package (reuse it, do not reimplement) and REUSE the existing `federationtesting` package, copying its approach.
> Create DEDICATED caching subgraphs (not `commerce` alone) with good NESTING and real VARIANCE of TTL and other cache options, and cover the full scenario matrix — including PARTIAL EXPIRY of some fields, partial fetch, root-field split, and alias/argument reuse — with fully-implemented assertions and NO placeholders.
> Every function of the (now modular) cache interfaces gets a meaningful unit test; HIGH coverage; no dead code.
> The `commerce`/`config_factory`/goldie material below is superseded where it conflicts.

## 1. Testing philosophy

### 1.1 Caching is interfaces, so the loader plus caching is unit-testable in-process with fakes

RFC-1 plugs caching into the loader through a small set of public interfaces only:
`CacheController` -> `RequestCache` (`PrepareFetch`/`OnFetchSkipped`/`OnFetchResult`/`EndRequest`),
the scoped `MergeArena.Begin() -> MergeSession -> Close()` lock,
the two-level opaque `*FetchCacheHandle`,
the `Decision` enum (including `DecisionFetchShadow`),
and the optional `CacheObserver`.
The loader branches on NOTHING but the `Decision` value and the `*FetchCacheHandle != nil` check (RFC-1 §3.1, §4.3).

That is the whole point of the abstraction for testing:
because the loader is mode-blind and talks only to interfaces,
every loader cache seam can be driven from in-process fakes,
and every controller code path (key render, coverage, freshness, reorder, write gate, backfill, shadow)
can be driven from an in-memory store fake,
so we reach ~100% path coverage WITHOUT a subgraph, an HTTP server, or a real L2 backend.
The two observable surfaces every scenario asserts on are
(1) the FAKE CACHE RECORD (an ordered, deterministic, normalized log of lifecycle/lookup/write calls and the returned `Decision`/handle), and
(2) the RESPONSE BYTES (the fully assembled GraphQL response, asserted as the exact JSON string).

### 1.2 Coverage target and how it is measured

Target: ~100% statement and branch coverage of the loader cache glue
(`cacheRequest`, `cachePrepare`, `cacheMerge`, `mergeArena`/`mergeSession`, the `mergeResult` carrier assignments,
`endCacheRequest`, the `clone`/`Free` cache reset, `FetchConfiguration.Equals`' cache clause),
plus the `resolve/cache` controller package.
Because the two test layers straddle the module boundary (§1.3), coverage is measured with TWO commands, each instrumenting the v2 packages under test via `-coverpkg`:
- the v2 controller-unit layer, run from `v2/`: `go test ./pkg/engine/resolve/cache/... -coverpkg=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache -covermode=atomic -race`;
- the execution plan-driven layer, run from `execution/`: `go test ./engine/... -run Caching -coverpkg=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve,github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache -covermode=atomic -race` (the execution tests drive the real v2 loader, so they instrument and report on the v2 loader glue by import path).
The cross-check in §5.10 maps each loader-glue branch to the row(s) that hit it, so a reviewer can confirm completeness without reading a coverage HTML report.

### 1.3 Where the tests live: TWO layers across the module boundary

The reviewer rule is "do NOT append tests to existing test files or add more test files to existing packages;
create SEPARATE NEW PACKAGES to group tests logically."
A second, harder constraint shapes the split: a real, drift-proof plan can ONLY be produced via real `wgc` composition + the execution engine's `config_factory` (§4),
and `config_factory` plus the cosmo proto (`nodev1`) live in the EXECUTION module (`execution/go.mod`, separate from `v2/go.mod`; execution imports v2).
`v2` must NOT take the cosmo-proto dependency.
So the strategy is exactly TWO test layers, each a NEW package, on opposite sides of that module boundary:

| Layer (NEW package) | Module | Sees | Holds | Inputs |
|---|---|---|---|---|
| (i) Controller unit tests — `v2/pkg/engine/resolve/cache`, white-box `package cache` and black-box `package cache_test` | v2 | white-box: unexported `cache` internals + in-package fakes; black-box: the public `cache` surface + `cachetesting` (`RealishCache`) | the controller LOGIC rows: key render, coverage walk, freshness sort, reorder, write/backfill gate, multi-key render-then-backfill, negative, shadow compare | CONSTRUCTED `*astjson.Value` + `FakeStore` + `RecordingObserver` (`resolve.CacheObserver`) + `synctest`, driving `cache.NewController`/`RealishCache` directly. NO plans. NO `resolve` loader. NO cosmo dependency. |
| (ii) Plan-driven loader+cache integration tests — a new `package …_test` in `execution/engine` | execution | the v2 PUBLIC `resolve` surface (`ResolveGraphQLResponse`/`ArenaResolveGraphQLResponse`/`ResolveGraphQLDeferResponse`), `plan`, `postprocess`, `config_factory`, and the v2 `cachetesting` fakes | the loader-seam + lifecycle + dispatch rows, the mode matrix, flush, `MergeSession` discipline, concurrency, defer, and the end-to-end hit/multi-key rows | a REAL plan from real `wgc` composition -> `config_factory` -> `config_factory`'s `plan.Configuration` -> v2 planner + postprocess (RFC-2 caching pass) -> golden -> the v2 loader driven against a FAKE in-process datasource + the cache controller + `synctest`. |

Module-boundary rationale (why this exact split):
- Layer (i) is pure controller logic and needs no plan and no loader, so it lives in v2 next to the code it tests, reaching the unexported `cache` internals directly as a greenfield white-box package — and it stays cosmo-free.
- Layer (ii) needs a PLAUSIBLE, drift-proof plan, which requires `config_factory` + the cosmo proto, which only exist in the execution module; so the plan-driven loader tests must live there.
  They drive the loader through the PUBLIC `resolve` entry points and assert on the two OBSERVABLE surfaces every scenario already uses — the FAKE CACHE RECORD (`fake.Calls()`) and the RESPONSE BYTES — so they need no unexported `resolve` access (which the module boundary would forbid anyway).
  The handful of seams the old plan once asserted through an `export_test.go` bridge (full-hit skip, prepare->merge handle threading, lock-once-per-hook) are now asserted OBSERVABLY instead: the recording fake owns the handle it returns, so it proves pointer identity prepare->merge itself; the gated datasource + `store.Ops()` prove no network ran on a full hit; and `-race` proves the lock discipline.
  No `export_test.go`, no hand-built federation plans.

This satisfies both reviewer asks at once: "validate that queries plan as the unit tests say" (a real planner over real composition, golden-snapshotted) and "test subgraphs are composable + planner config valid" (wgc composition + `config_factory`), while keeping `v2` free of the cosmo dependency.

---

## 2. The test doubles

The doubles split along the module boundary (§1.3).
The cosmo-FREE fakes that DRIVE THE LOADER — the recording cache, the in-memory `RealishCache` (which wraps the real `cache.Controller`), the gated datasource, the `FakeCacheController` — live in ONE v2 support package, `v2/pkg/engine/resolve/cache/cachetesting` (not a `_test` package); the execution layer (ii) imports it (execution imports v2).
Because `cachetesting` imports `cache` (for `RealishCache`), the white-box layer (i) — itself `package cache` — does NOT import `cachetesting` (that would cycle); instead it uses small IN-PACKAGE fakes (`FakeStore`, `RecordingObserver`) and `cache.NewController` directly. `FakeStore` and `RecordingObserver` are described once below and are duplicated as a tiny in-package fake for the white-box layer and a `cachetesting` export for the loader-driving layer.
The drift-proof `Plan(...)` harness (§4) is the ONLY double that needs `config_factory` + the cosmo proto, so it lives in a support file in the execution module beside `config_factory_federation.go`.
There is no fake clock at all — `synctest` is the clock (§2.5).
We REUSE the existing repo machinery wherever it exists and only add a seam where the survey proved none exists.

### 2.1 Reused, do not reinvent

- Real composition + planner config: `wgc router compose` driving a committed `config.json`, then `protojson.Unmarshal` into `nodev1.RouterConfig` and `engine.NewFederationEngineConfigFactory(ctx).BuildEngineConfiguration(&rc)` to obtain a VALID `plan.Configuration` — the exact pattern of `execution/engine/config_factory_federation_test.go:36-46` and `execution/engine/skipped_fetch_test.go:47-57`. This is the spine of §4; it is the reuse that makes the test subgraphs provably composable and the planner config provably valid.
- DataSource stubbing: the generated `MockDataSource` (`//go:generate mockgen ... DataSource`) and the `mockedDS(t, ctrl, expectedInput, responseData)` helper (`resolve_federation_test.go:19`), and the `TestingTB` abstraction (`resolve_federation_test.go:13`) so harnesses work for benchmarks too.
- JSON normalization for assertions: `compactJSONForAssert(t, input)` (`json_assert_test.go:9`); use it to normalize the EXPECTED response string so byte-for-byte `assert.Equal` is robust to insignificant whitespace, never `assert.JSONEq`/`assert.Contains`.
- Golden snapshots: `github.com/sebdah/goldie/v2` — ALREADY used in the execution module (`execution/engine/federation_integration_test.go:15,71`, `goldie.New(t, goldie.WithNameSuffix(".json"))`) and wrapped in v2 at `pkg/testing/goldie/goldie.go`; the plan-drift golden (§4) lives in the execution layer, so it uses `sebdah/goldie/v2` directly the same way the neighboring execution tests do.
- Key-hash assertions: the pooled xxhash digest pattern the loader already uses for batch keys (`pool.Hash64.Get()`, `keyGen.Reset(); keyGen.Write(bytes); keyGen.Sum64()`, `loader.go:1667-1669`); a test that asserts a rendered cache key recomputes it the SAME way rather than hard-coding a magic number, then `assert.Equal`s the full key string.
- Atomic call counters: `atomic.Int64` exactly as `loader_hooks_test.go` counts `OnLoad`/`OnFinished`.
- Fake time: the Go 1.25 stdlib `testing/synctest` (§2.5) — not a hand-rolled clock.

### 2.2 `fakeCacheController` — proves lazy, once-per-request init

```go
// cachetesting/fakes.go
type FakeCacheController struct {
    begins atomic.Int64
    rc     resolve.RequestCache // the RequestCache handed back on every BeginRequest
}

func (f *FakeCacheController) BeginRequest(*resolve.Context) resolve.RequestCache {
    f.begins.Add(1)
    return f.rc
}
func (f *FakeCacheController) Begins() int64 { return f.begins.Load() }
```

Used to assert `BeginRequest` is called EXACTLY once per request and lazily (only on the first cache-eligible fetch), rows B1/B2/M1.

### 2.3 `FakeRequestCache` (recording) — the pure loader-seam driver

This double records every call with FULL, NORMALIZED inputs and returns a SCRIPTED `(Decision, *FetchCacheHandle)`.
It is what lets us assert what the loader hands the cache and how it dispatches the decision, WITHOUT any cache logic.

```go
// A normalized, comparable record — pointers/arena values are projected to bytes/strings
// so the whole slice is asserted with one assert.Equal (rule 11). No Contains, ever.
type Call struct {
    Op           string            // "Prepare" | "Skipped" | "Result" | "End"
    FetchPath    string            // item.ResponsePath, deterministic per fetch
    Items        int               // len(in.Items)
    InputBytes   string            // PrepareFetchInput.Input (canonical pre-injection, §3.6)
    HeaderHash   uint64            // PrepareFetchInput.HeaderHash
    ResponseData string            // MergeInput.ResponseData marshaled, "" when nil
    HasErrors    bool
    FetchFailed  bool
    EmptyEntity  bool
    StatusCode   int
    Decision     resolve.Decision  // echoed from the scripted return (Prepare only)
}

type FakeRequestCache struct {
    mu     sync.Mutex
    calls  []Call
    script map[string]ScriptedDecision // keyed by FetchPath
    errs   map[string]error            // OnFetch* error injection, keyed by FetchPath+Op
}

type ScriptedDecision struct {
    Decision resolve.Decision
    Handle   *resolve.FetchCacheHandle // nil for NO-OP (DecisionFetch,nil)
}

func (f *FakeRequestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
    s := f.script[pathOf(in.Item)]
    f.record(Call{Op: "Prepare", FetchPath: pathOf(in.Item), Items: len(in.Items),
        InputBytes: string(in.Input), HeaderHash: in.HeaderHash, Decision: s.Decision})
    return s.Decision, s.Handle
}
// OnFetchSkipped / OnFetchResult / EndRequest record analogously and return f.errs[...].

func (f *FakeRequestCache) Calls() []Call { f.mu.Lock(); defer f.mu.Unlock(); return slices.Clone(f.calls) }
```

`record` takes `f.mu`, so the slice is race-free under parallel fetches.
For parallel scenarios where call ORDER is non-deterministic, tests normalize by sorting `Calls()` with `slices.SortFunc` on `(FetchPath, Op)` before the `assert.Equal` (the "normalize then assert structural equality" rule), so the assertion stays a full-value `assert.Equal` and never degrades to `Contains`.

### 2.4 `RealishCache` (in-memory L1+L2) — the end-to-end behavior driver

A real `RequestCache` (produced by a real `cache.Controller` over a `map[string][]byte` store) parameterized by mode,
so the hit/miss/backfill/negative/shadow/multi-key rows exercise the ACTUAL candidate-render, coverage, freshness, reorder, and write logic.

```go
type Mode uint8
const ( ModeNoop Mode = iota; ModeL1; ModeL2; ModeL1L2 ) // shadow is a per-policy bool, cross-cutting (RFC-1 §5)

func NewRealishCache(tb testing.TB, mode Mode, store *FakeStore, obs resolve.CacheObserver) resolve.CacheController
```

It takes no clock: the real controller calls `time.Now`/`time.Since`/`time.Until` directly, and `synctest` (§2.5) fakes those for the rows that need TTL control.

It records the SAME `Call` log as the recording fake (so the catalog asserts both surfaces uniformly) plus a store-op log (`Get`/`Set`) described in §2.6.

### 2.5 `testing/synctest` — the fake clock (NO custom double, NO injectable now)

The conventions survey is explicit: NO clock abstraction exists in the repo; `time.Now()` is called directly.
The Go 1.25 stdlib `testing/synctest` makes that a feature, not a problem, so the strategy adds NO clock seam at all:
the `resolve/cache` controller keeps calling REAL `time.Now`/`time.Since`/`time.Until` and comparing `time.Duration` TTLs directly,
and tests that care about time run inside a `synctest` bubble where that real clock is faked.
This is why the appendix proposes NO `cache.WithClock(...)` option and NO `now func()` field — there is nothing to inject.

How a bubble works (the only three rules that matter here):
- `synctest.Test(t, func(t *testing.T){ … })` runs the body in a bubble with a FAKE clock that starts at a fixed instant and advances ONLY when every goroutine in the bubble is durably blocked.
- `synctest.Wait()` blocks the caller until all OTHER bubble goroutines are durably blocked — the deterministic-ordering primitive (used with the gates of §2.7, NOT with real latency).
- Inside the bubble, `time.Sleep(d)` advances the fake clock by `d` instantaneously (no real wall-clock wait), so TTL EXPIRY is exercised by sleeping PAST the TTL and then re-reading.

TTL-expiry pattern (replaces every "advance the FakeClock" step):

```go
func TestCaching_Negative_ExpiresAfterTTL(t *testing.T) { // row G4
    synctest.Test(t, func(t *testing.T) {
        store := cachetesting.NewFakeStore()
        cc := cachetesting.NewRealishCache(t, cachetesting.ModeL2, store, nil)
        // … write a negative entry with NegativeCacheTTL = 30s (real time.Now() is the fake clock) …

        time.Sleep(31 * time.Second) // fake time jumps past the TTL, instantly
        synctest.Wait()
        // … re-run the fetch: the entry is now treated as expired -> miss -> network runs …
    })
}
```

Hard constraints `synctest` imposes on these tests (call them out so the harness stays bubble-safe):
- ALL goroutines must be STARTED inside the bubble (the loader's per-fetch / per-defer-group goroutines are spawned by the public entry point invoked within the body, so they are in the bubble).
- ONLY in-process fakes are allowed: the gated/fake datasource (§2.7) returns scripted bytes with no socket; a real `net/http` transport is NOT bubble-aware and is forbidden inside a bubble.
- Use `time.Sleep` ONLY to advance the fake clock for TTL; NEVER as a latency knob to order concurrent fetches — ordering is done with gates + `synctest.Wait()` (§2.7, §6).

Required by rows that control time: G4 (negative expiry), F8/F9 (TTL stamped / mutation override), G1/G3 (negative write/hit TTL), H2/H3 (shadow `CacheAge`), and the multi-candidate freshness rows D7-D10 (different remaining TTLs produced by writing/seeding at different fake instants).

### 2.6 `FakeStore` — the L2 backend, TTL driven by the synctest clock

The store models an L2 backend with its own expiry: each entry records an ABSOLUTE `ExpiresAt` computed from `time.Now()` at write time, and `Get` consults `time.Now()` so an entry past its expiry is a MISS.
Inside a `synctest` bubble (§2.5) `time.Now()` is the fake clock, so both EXPIRY and the per-candidate RemainingTTL fall out of fake time with NO separate clock plumbing.

```go
type StoredEntry struct { Value []byte; ExpiresAt time.Time } // ExpiresAt = time.Now().Add(ttl) at write
type FakeStore struct {
    mu    sync.Mutex
    data  map[string]StoredEntry
    ops   []StoreOp // {Kind:"Get"|"Set", Key string, Value string, TTL time.Duration} — normalized, full-value asserted
}
// Seed writes an entry as if Set with this ttl at the CURRENT fake instant; remaining TTL is then ExpiresAt-time.Now().
func (s *FakeStore) Seed(key string, v []byte, ttl time.Duration)
// Get returns (miss) when time.Now() >= ExpiresAt; otherwise the value and remaining = ExpiresAt - time.Now().
func (s *FakeStore) Get(key string) (StoredEntry, bool)
func (s *FakeStore) Ops() []StoreOp
```

For the multi-candidate freshness ordering rows D7-D10, distinct remaining TTLs are produced inside the bubble either by seeding with different TTLs or by `time.Sleep`ing between seeds so the earlier entry has decayed further — both are exact and deterministic because the fake clock only moves on `time.Sleep`.
The EXPIRY rows (G4) `time.Sleep` past the TTL; the shadow-age rows (H2/H3) `time.Sleep` to age an entry before the compare reads `CacheAge`.
Tests assert `assert.Equal(t, []StoreOp{...}, store.Ops())` — every `Get` (key, in order) and every `Set` (key + EXACT bytes + EXACT ttl).

### 2.7 Gate channels — deterministic fetch ordering (NOT latency)

Commit e509453b mandates GATES, not sleeps/latency, for deterministic ordering across parallel and defer fetches; `synctest` makes this airtight.
The gate wraps a canned response and blocks `Load` until released, optionally signaling arrival first.
A gated `Load` that is parked on `<-g.Release` is a DURABLY blocked goroutine, so inside a bubble the test can call `synctest.Wait()` to be SURE every other fetch has reached its gate before it releases one — that is how "fetch A fully merges and writes L1 BEFORE fetch B prepares" is made deterministic without any real latency.
The gate is an in-process fake (no socket), so it is bubble-safe (§2.5).

```go
type GatedDataSource struct {
    Name    string
    Resp    []byte
    Err     error
    Arrived chan<- string  // optional: fetch announces it reached the network
    Release <-chan struct{} // test closes/sends to let the fetch proceed
}
func (g *GatedDataSource) Load(ctx context.Context, h http.Header, in []byte) ([]byte, error) {
    if g.Arrived != nil { g.Arrived <- g.Name }
    <-g.Release
    return g.Resp, g.Err
}
func (g *GatedDataSource) LoadWithFiles(context.Context, http.Header, []byte, []*httpclient.FileUpload) ([]byte, error) {
    panic("cache tests never upload files")
}
```

This lets a test force "fetch A fully merges and writes L1 BEFORE fetch B prepares",
which is exactly the ordering rows M2/M3/N1/N2 need to prove cross-fetch and cross-defer-group L1 reuse.

### 2.8 `RecordingObserver` — shadow / analytics, and the nil path

```go
type RecordingObserver struct {
    mu       sync.Mutex
    compares []ShadowCompare // {CacheKey, EntityType, IsFresh, CacheAge time.Duration} — full-value asserted
}
func (o *RecordingObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, s resolve.MergeSession) { ... }
```

v1 ships the observer NIL, so the shadow rows assert BOTH the wired-observer path AND that a nil observer force-fetches but records nothing (rows H6, H7).
The `CacheAge` field is exact and assertable because it is `time.Now() - entry.writtenAt` against the synctest fake clock (§2.5): the shadow rows `time.Sleep` a known amount to age the entry, then assert the recorded `CacheAge` equals that amount.

---

## 3. The clean test supergraph and the scenario -> element table

### 3.1 `commerce` — 3 subgraphs, 2 cross-subgraph entities, real wgc composition

This is the canonical products/reviews/users federation supergraph the repo already plans in `loader_test.go:20-33`,
minimally extended (a second `@key(fields:"sku")` on `Product`, plus by-key root fields `product(upc:)`/`user(id:)`),
so ONE graph hits every matrix row.
The multi-`@key`-on-one-type shape is itself an existing convention (`graphql_datasource_test.go:5069-5074`), so this reuses, not invents.

Crucially, the SDLs are NOT `const` strings — they are real subgraph files, COMPOSED by `wgc`, exactly mirroring `execution/engine/testdata/config_factory_federation/` (§4).
A new directory `execution/engine/testdata/cache_commerce/` holds:
- `account_sdl.graphql`, `product_sdl.graphql`, `review_sdl.graphql` — the three subgraph SDLs (the extension over `config_factory_federation` is the second `Product @key(fields:"sku")` and the `product(upc:)`/`user(id:)` root fields);
- `graph.yaml` — one `{ name, routing_url, schema.file }` per subgraph, like `testdata/config_factory_federation/graph.yaml`;
- `compose.sh` — `npx -y wgc@latest router compose -i graph.yaml -o config.json` then `jq . config.json`, like `testdata/config_factory_federation/compose.sh`;
- `config.json` — the COMMITTED composed supergraph config.

Re-running `compose.sh` IS the composability guard: `wgc` fails if the three subgraphs do not compose, so a committed `config.json` is proof the graph is composable under federation. There is no hand-authored supergraph SDL and no "validated with rover" claim to drift — the committed artifact is the validation.

- Products (owns `Product @key(fields:"upc") @key(fields:"sku")`): `topProducts(first:Int=5): [Product!]!`, `product(upc:String!): Product`.
- Reviews (owns local `Review`; references `Product` and `User`): `latestReviews(first:Int=5): [Review!]!`; contributes `Product.reviews`; `User @key(fields:"id", resolvable:false)`.
- Accounts (owns `User @key(fields:"id")`): `user(id:ID!): User`.

### 3.2 Scenario-group -> supergraph element (from the design)

| Coverage scenario | Supergraph element | Representative operation |
|---|---|---|
| Single-`@key` entity | `User @key(id)` (Accounts owns, Reviews references) | `{ user(id:"1"){ name } }` |
| Multi-`@key` entity (best-effort) | `Product @key(upc) @key(sku)` | `{ topProducts{ upc sku name } }` |
| Multi-key BACKFILL (candidate unrenderable at lookup) | `product(upc:)` returns `sku` only in the response | `{ product(upc:"1"){ name sku } }` |
| Entity reference across subgraphs (fetch+cache in A/C, reuse in B) | `Review.product`, `Review.author` | `{ latestReviews{ body product{ name } author{ name } } }` |
| Root-field-cached query | `topProducts`, `latestReviews` | `{ topProducts{ name price } }` |
| Root field that RE-USES the entity cache (`EntityKeyMapping`) | `product(upc:)`, `user(id:)` | `{ product(upc:"1"){ name } }` after `topProducts` primed upc="1" |
| Batch entity fetch | `topProducts{ reviews }`, `latestReviews{ author }` | `{ topProducts{ name reviews{ body } } }` |
| Defer + caching reuse | `Product.reviews` deferred under `topProducts` | `{ topProducts{ name ... @defer { reviews{ body author{ name } } } } }` |

### 3.3 The stage toggle mirrors the mandated implementation order

The same composed `config.json` drives every stage; only the `postprocess.EnableCaching` provider wiring changes (which datasources get a cache config provider), via a `CacheStage` enum that mirrors the mandated order
(structure -> L2 entities -> L2 root fields -> L2 root-reuses-entity -> L1):

```go
type CacheStage uint8
const (
    StageNoop CacheStage = iota // providers empty: strict NO-OP (RFC-1 §10, RFC-2 G11)
    StageL2Entities             // policy on Product, User only
    StageL2RootFields           // + topProducts, latestReviews
    StageL2RootReusesEntity     // + product(upc:), user(id:)  (EntityKeyMapping rows)
    StageL1                     // flip L1 eligibility; defer query proves request-lifetime shared L1
)
```

Each implementation phase lands its catalog rows at the matching stage,
so the suite grows monotonically and never tests a capability before it exists.

---

## 4. Plan-drift prevention harness: real wgc composition + config_factory

### 4.1 Why this harness exists

A plan-driven test (layer ii, §1.3) needs a PLAUSIBLE loader input — a `*resolve.GraphQLResponse` whose fetch tree carries the RFC-2-stamped `FetchCacheConfig`.
There are two drift risks, and this harness closes BOTH:
- a hand-built `*GraphQLResponse` literal drifts from the real planner/stamper (nothing keeps it honest);
- a hand-built `plan.Configuration` (DataSources with hand-typed `FederationMetaData.Keys`/`Requires`/`Provides`, root/child nodes, field configs) drifts from a real, composable supergraph — and silently lets a test assert against a configuration the real graph could never produce.

The fix is to derive EVERYTHING from a real `wgc`-composed supergraph through the execution engine's `config_factory`, exactly as the existing federation tests do, and then golden-snapshot the resulting plan:
- composition is real (`compose.sh` -> committed `config.json`, §3.1), so the subgraphs are PROVABLY composable;
- the planner configuration is built by `config_factory`, so it is PROVABLY a valid `plan.Configuration` for that graph;
- the plan is produced by the real v2 planner + postprocess (with the RFC-2 caching pass), then snapshotted, so any planner/stamper change is a review-visible golden diff.

This reuses the live, proven pipeline rather than inventing one. The exact reference files (read them to stay accurate):
- `execution/engine/config_factory_federation.go`: `NewFederationEngineConfigFactory(ctx, opts...)` (`:80`) and `BuildEngineConfiguration(*nodev1.RouterConfig) (Configuration, error)` (`:127`), which calls `createPlannerConfiguration` (`:152`) to map `RouterConfig.EngineConfig` -> `plan.Configuration` (`Fields`/`Types`/`DataSources`), and `dataSourceMetaData` (`:309`) which builds each datasource's real `FederationMetaData.Keys`/`Requires`/`Provides` from `in.Keys`/`in.Requires`/`in.Provides`.
- `execution/engine/config_factory_federation_test.go`: the load pattern — `os.ReadFile("testdata/.../config.json")` (`:36`), `protojson.Unmarshal(data, &rc)` into `nodev1.RouterConfig` (`:40-41`), `BuildEngineConfiguration(&rc)` (`:42`); `skipped_fetch_test.go:47-57` shows the same with a per-test URL rewrite.
- `execution/engine/testdata/config_factory_federation/`: `account_sdl.graphql`/`product_sdl.graphql`/`review_sdl.graphql`, `graph.yaml`, `compose.sh`, `config.json` — the template the new `cache_commerce/` directory copies (§3.1).
- imports: `nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"`, `"google.golang.org/protobuf/encoding/protojson"` (both already used across `execution/engine`).

### 4.2 The `Plan(...)` harness (execution-module support file)

`config_factory` and `nodev1` are execution-module-only (§1.3), so the harness lives in the execution module beside `config_factory_federation.go`.
It obtains a `plan.Configuration` from `config_factory`, runs the real planner + postprocess, golden-snapshots the plan, then swaps in fakes:

```go
// execution/engine/cachetesting_plan.go  (execution module; imports the cosmo proto + config_factory)
type PlanResult struct {
    Response *resolve.GraphQLResponse // == plan.SynchronousResponsePlan.Response, the loader's input
    Fakes    *cachetesting.FakeRegistry // (subgraph,input) -> canned bytes / GatedDataSource (the v2 fakes)
}

// Plan composes -> config_factory -> real planner + postprocess (RFC-2 caching pass) ->
// golden-snapshots the plan -> swaps each fetch's transport for an in-process fake.
func Plan(tb testing.TB, stage cachetesting.CacheStage, query string, responses map[string]string) PlanResult {
    tb.Helper()

    // 1. Real composition output -> RouterConfig -> valid plan.Configuration (the §4.1 reuse).
    data, err := os.ReadFile("testdata/cache_commerce/config.json")
    require.NoError(tb, err)
    var rc nodev1.RouterConfig
    require.NoError(tb, protojson.Unmarshal(data, &rc))

    f := NewFederationEngineConfigFactory(tb.Context())
    conf, err := f.BuildEngineConfiguration(&rc)
    require.NoError(tb, err)
    cfg := conf.PlannerConfig() // small EXPORTED accessor added to engine.Configuration (§4.3)

    // 2. Definition is the COMPOSED supergraph schema, not a hand-typed SDL.
    def := unsafeparser.ParseGraphqlDocumentString(rc.EngineConfig.GraphqlSchema)
    op := unsafeparser.ParseGraphqlDocumentString(query)
    require.NoError(tb, asttransform.MergeDefinitionWithBaseSchema(&def))

    norm := astnormalization.NewWithOpts(
        astnormalization.WithExtractVariables(),
        astnormalization.WithInlineFragmentSpreads(),
        astnormalization.WithRemoveFragmentDefinitions(),
        astnormalization.WithRemoveUnusedVariables(),
        astnormalization.WithEnableDefer(),
    )
    var report operationreport.Report
    norm.NormalizeOperation(&op, &def, &report)
    astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
    require.False(tb, report.HasErrors(), report.Error())

    // 3. Real planner + postprocess WITH the RFC-2 caching pass, gated per stage.
    planner, err := plan.NewPlanner(cfg)
    require.NoError(tb, err)
    raw := planner.Plan(&op, &def, "", &report)
    require.False(tb, report.HasErrors(), report.Error())

    proc := postprocess.NewProcessor(
        postprocess.EnableCaching(cacheProvidersForStage(cfg, stage), federationByDS(cfg), &def),
    )
    proc.Process(raw)

    resp := planResponse(tb, raw) // handles Synchronous + Defer + Subscription plan kinds

    // 4. Visibility + drift guard: golden the plan shape AND what RFC-2 stamped.
    goldie.New(tb, goldie.WithNameSuffix(".golden")).Assert(tb, tb.Name(), []byte(renderPlanWithCache(resp)))

    // 5. Only mutation of the planner output: transport -> in-process fake (§4.4).
    fakes := cachetesting.NewFakeRegistry(responses)
    cachetesting.SwapDataSources(resp.Fetches, fakes)
    return PlanResult{Response: resp, Fakes: fakes}
}
```

`renderPlanWithCache` writes `resp.Fetches.QueryPlan().PrettyPrint()` (`fetchtree.go:165,256`)
plus, per fetch, `cfg.String()` and a compact `KeySpec` dump (both required by RFC-1 §3.6),
so the golden shows BOTH the plan shape and the stamped cache config; `goldie -update` regenerates it.
`postprocess.EnableCaching(providers, federation, definition)` is RFC-2's option (RFC-2 §5.1): `cacheProvidersForStage` returns providers only for the datasources active at `stage` (so `StageNoop` passes an empty map -> the caching pass is a no-op), and `federationByDS` reads each datasource's `FederationMetaData` out of `cfg.DataSources`.

### 4.3 The one exported accessor (refactor-in-place)

`engine.Configuration.plannerConfig` is unexported (`engine_config.go:23`), and `BuildEngineConfiguration` returns it wrapped (`config_factory_federation.go:140-143`).
The harness needs the `plan.Configuration` out of it, so add ONE small exported accessor next to the existing `DataSources()`/`FieldConfigurations()` getters (`engine_config.go:62-68`):

```go
// PlannerConfig exposes the built plan.Configuration for plan-driven tests.
func (e *Configuration) PlannerConfig() plan.Configuration { return e.plannerConfig }
```

This is a refactor-in-place in the execution engine package; no behavior change.

### 4.4 No hand-built plans, anywhere

There are NO hand-built `*GraphQLResponse` trees and NO hand-built `plan.Configuration` in the suite.
Layer (i) (the v2 controller unit tests) needs no plan at all — it drives the cache controller with constructed `*astjson.Value` inputs (§1.3).
Layer (ii) always goes through `Plan(...)`, which has exactly one source of truth for any plausible plan (the real planner over the real composition) and a golden that makes any legitimate change reviewable.
The loader-seam dispatch that an earlier draft asserted with tiny hand-built `SingleFetch`/`EntityFetch` literals is instead asserted on the smallest real plan `Plan(...)` can produce (a single root-field query, a single by-key entity query), so even the seam fixtures cannot drift.

### 4.5 The DataSource swap

`SwapDataSources` (a v2 fake helper) walks the fetch tree (`FetchTreeNode.Item`/`ChildNodes`, kinds at `fetchtree.go:19-24`)
and replaces each fetch's `DataSource` field (settable, `fetch.go:170,210,258`) with a fake/gated in-process source keyed by `(Info, Input)`.
The tree SHAPE stays the planner's; only the transport is faked — which is also what keeps the plan-driven tests bubble-safe under `synctest` (no real socket, §2.5).

---

## 5. The full scenario catalog (checklist)

Every row from the matrix, grouped as the matrix groups them.
Each row asserts on the FULL recorded calls (`assert.Equal(t, []Call{...}, fake.Calls())` and/or `assert.Equal(t, []StoreOp{...}, store.Ops())`)
AND the FULL response bytes (`assert.Equal(t, compactJSONForAssert(t, want), buf.String())`).
No `assert.Contains`, no `assert.JSONEq`, no fuzzy comparison anywhere.
`[ttl]` marks a row that runs inside a `synctest` bubble and controls time (seed/write TTLs, `time.Sleep` past an expiry, or age an entry — §2.5); `[gate]` marks a row that uses gate channels + `synctest.Wait()` for deterministic ordering (§2.7). There is no `[clock]` marker — `synctest` is the clock.

### 5.1 A. Loader-seam gates — NO-OP and config gating (driven by `FakeRequestCache`)

- [ ] **A1** controller nil: fake never constructed; `cacheRequest()` nil; zero `BeginRequest`; response == non-cached baseline.
- [ ] **A2** controller set, all `Cache` nil: zero `PrepareFetch`; `BeginRequest` NOT called; response == baseline.
- [ ] **A3** `Cache != nil` but `!L1 && !L2`: `cachePrepare` returns at the guard; zero `PrepareFetch`; response == baseline.
- [ ] **A4** `Cache.L1` true but controller nil: `cacheRequest()` nil; zero `PrepareFetch`; response == baseline (lower gate wins).
- [ ] **A5** `prepared.skipLoad` pre-set (null parent / auth reject / render error): `cachePrepare` early-returns; `cacheHandle` stays nil; `cacheMerge` no-ops.
- [ ] **A6** unknown fetch type (default switch arm): `cfg` stays nil; zero `PrepareFetch`.
- [ ] **A7** controller set AND `Cache.L2` true: `BeginRequest` once, `PrepareFetch` called — the ONLY combination that activates caching.

### 5.2 B. Lazy request-lifetime init + lifecycle

- [ ] **B1** `BeginRequest` lazy + once: 3 eligible fetches -> exactly 1 `BeginRequest`; all 3 `PrepareFetch` get the SAME `RequestCache`.
- [ ] **B2** `BeginRequest` skipped when no eligible fetch: never called.
- [ ] **B3** `EndRequest` once per sync request (`ResolveGraphQLResponse`); `ctx.requestCache` nilled after.
- [ ] **B4** `EndRequest` once on `ArenaResolveGraphQLResponse` (proves the arena entry also wires the defer).
- [ ] **B5** `endCacheRequest` no-op when caching unused: `requestCache==nil` -> no `EndRequest`, no panic.
- [ ] **B6** `clone` resets request cache: `cpy.requestCache==nil`, `cpy.cacheController` still set.
- [ ] **B7** `Free` is defensive: `endCacheRequest` invoked, then `cacheController` nilled.
- [ ] **B8** double `endCacheRequest` idempotent (B3 then B7): `EndRequest` fires exactly once total.

### 5.3 C. PrepareFetch decision dispatch (scripted `FakeRequestCache`, one row per decision)

- [ ] **C1** `DecisionFetch` + handle: `skipLoad` false; network runs; `cacheMerge`->`OnFetchResult`; normal fetched response.
- [ ] **C2** `DecisionFetch, nil`: handle nil; `cacheMerge` returns at `handle==nil`; NO `OnFetchResult`.
- [ ] **C3** `DecisionSkipFullHit`: `skipLoad=true` AND `res.fetchSkipped=true`; `loadPhase` early-returns; `OnFetchSkipped`; cached response, NO "failed to fetch" error.
- [ ] **C4** `DecisionFetchShadow` (handle `Shadow=true`): `skipLoad` false; network runs; `OnFetchResult`; FRESH response (cache value never served).
- [ ] **C5** `DecisionFetchPartial` (v2 seam): v1 branch no-op; `skipLoad` false; dispatches to `OnFetchSkipped` Partial arm (asserts the seam compiles + dispatches).
- [ ] **C6** skip-path spurious-error guard: `DecisionSkipFullHit` with empty `res.out` MUST NOT render `renderErrorsFailedToFetch`; response has data, no errors array (the §4.7 #1 invariant).
- [ ] **C7** OnLoad/OnFinished not fired on full hit: with `LoaderHooks` set, `res.loaderHookContext` nil -> `OnFinished` NOT called (§4.7 #3, atomic-counter assert == 0).
- [ ] **C8** handle threaded prepare->merge: the SAME `*FetchCacheHandle` instance the recording fake returned from `PrepareFetch` reaches `OnFetch*` (pointer identity asserted BY the fake itself — it owns and records the handle pointer on both calls, so no unexported `resolve` access is needed).

### 5.4 D. Hit determination — coverage / freshness / reorder (`RealishCache`, seeded L2)

- [ ] **D1** full hit, single covering candidate: `Get`->hit; coverage passes; `SkipFullHit`; reorder skipped; spliced.
- [ ] **D2** miss (no entry): `Get`->miss; `Fetch`; `PendingCandidates` empty; `OnFetchResult` writes.
- [ ] **D3** coverage FAIL (entry missing a required field): treated as MISS -> `Fetch`; assert the stale partial value was NOT served.
- [ ] **D4** null accepted only where Nullable: `field:null`, spec `Nullable=true` -> coverage passes -> full hit.
- [ ] **D5** null rejected at non-nullable spec node -> miss/Fetch.
- [ ] **D6** alias/arg-aware mismatch: value cached under `friends_<hashA>`, request is `friends(first:20)`=`friends_<hashB>` -> miss/Fetch (the §9.1 correctness-hole guard). `[ttl]`
- [ ] **D7** multi-candidate freshness pick: 2 entries different `RemainingTTL`; both `Get`s recorded; freshest (larger TTL) chosen; `SelectedRemainingTTL`==larger. `[ttl]`
- [ ] **D8** freshness tie / known-beats-unknown: known TTL beats unknown; stable tie order. `[ttl]`
- [ ] **D9** merge-synthesis when no single covers: union covers -> merged value; `NeedsWriteback=true`; `MustWriteBack=true`; reorder RUN; canonical rewrite queued. `[ttl]`
- [ ] **D10** older-candidate fallback: union does not cover, an older single covers -> accepted; `NeedsWriteback=true`. `[ttl]`
- [ ] **D11** reorder to selection order: chosen value rebuilt to `ProvidesData` order; extra fields appended; response key order matches selection order.
- [ ] **D12** AND-reduction all covered -> `SkipFullHit` (batch/multi-item).
- [ ] **D13** AND-reduction one uncovered -> whole fetch flips to `Fetch` (all-or-nothing v1).
- [ ] **D14** `ProvidesData==nil` disables walk: treated as miss/Fetch; no panic.

### 5.5 E. Multi-key candidates — best-effort render-then-backfill (see §8 for sketches)

- [ ] **E1** all candidates renderable at lookup: 2 `RenderedKeys`; `Get` under BOTH; `PendingCandidates` empty.
- [ ] **E2** hit on a NON-primary key: candidate[0] misses, candidate[1] hits; both `Get`s recorded; serves from candidate[1].
- [ ] **E3** some renderable at lookup, more after response -> backfill ALL: lookup `Get` once (cand0); `PendingCandidates`=[cand1]; after fetch re-render cand1; `Set` under BOTH keys; `WriteReason` backfill for cand1.
- [ ] **E4** none renderable at lookup -> skip cache: zero `RenderedKeys`; no `Get`; `Fetch`; candidates re-rendered + written post-fetch.
- [ ] **E5** backfill on read-hit (`MustWriteBack`): full hit on cand0; cand1 renderable from the HIT value; `OnFetchSkipped` `Set`s the missing key (no network).
- [ ] **E6** refresh vs backfill reason tags: `Set` records carry correct `WriteReason` (metadata only, does not gate).
- [ ] **E7** single-`@key` entity = one-element candidate list: exactly 1 candidate, 1 `RenderedKey` (degenerate of E1).

### 5.6 F. Write / backfill gate — `OnFetchResult` (`!FetchFailed && !HasErrors && ResponseData != nil && Type()!=Null`)

- [ ] **F1** clean success writes: `Set` with fresh bytes + `cfg.TTL`.
- [ ] **F2** transport failure (`res.err!=nil` -> `FetchFailed`, `ResponseData==nil`): ZERO `Set`; assert gate keys off `FetchFailed`/`ResponseData==nil` NOT `HasErrors` (the §3.3 blocking-bug guard).
- [ ] **F3** empty body (`len(res.out)==0`): ZERO `Set`.
- [ ] **F4** parse failure (`res.response==nil`): ZERO `Set`; no panic.
- [ ] **F5** GraphQL errors (`HasErrors`): ZERO `Set` (transient error must not persist).
- [ ] **F6** data == JSON null (not EmptyEntity): ZERO positive `Set` (blocked by `Type()!=Null`).
- [ ] **F7** status fallback (non-2xx + shape-mismatch null): gate blocks.
- [ ] **F8** TTL stamped from config: `Set` ttl == `cfg.TTL` exactly. `[ttl]`
- [ ] **F9** mutation TTL override: `Set` ttl == `MutationTTLOverride`, not `cfg.TTL`. `[ttl]`

### 5.7 G. Negative caching (`EmptyEntity`)

- [ ] **G1** negative WRITE on empty entity (`EmptyEntity`, `NegativeCacheTTL>0`, not FetchFailed): `Set` a NULL sentinel under the key with `NegativeCacheTTL`. `[ttl]`
- [ ] **G2** negative write SKIPPED when `NegativeCacheTTL==0`: ZERO `Set`.
- [ ] **G3** negative HIT served: `Get`->null sentinel; `NegativeHit=true`; `FromCache=TypeNull`; merge skipped; `SkipFullHit`; null entity, no network. `[ttl]`
- [ ] **G4** negative entry expired -> miss: `time.Sleep` past the `NegativeCacheTTL` inside the synctest bubble; entry treated as expired; network runs. `[ttl]`
- [ ] **G5** EmptyEntity AND FetchFailed: gate blocks (FetchFailed wins) -> NO negative write.
- [ ] **G6** null-bubble suppression preserved: loader's `setSkipErrors`/null-bubble path untouched; no synthetic non-null error.

### 5.8 H. Shadow mode (`RecordingObserver`, entity-only compare)

- [ ] **H1** shadow L2 read + stash + force-fetch: `Get` hit; `ShadowStash[i]` populated; `DecisionFetchShadow`; `handle.Shadow=true`; network STILL runs; FRESH response.
- [ ] **H2** shadow compare MATCH: `time.Sleep` to age the seeded entry; `CompareShadow` BEFORE writes; `IsFresh=true`; recorded `CacheAge`==slept duration; order compare->write-L1->write-L2; L2 overwritten. `[ttl]`
- [ ] **H3** shadow compare MISMATCH: `IsFresh=false`; L2 overwritten with fresh. `[ttl]`
- [ ] **H4** shadow L1 hit wins (no shadow): L1 `SkipFullHit`, no refetch, no `CompareShadow` (shadow is L2-only).
- [ ] **H5** root-field shadow asymmetry: force-refetch + overwrite L2 but `CompareShadow` NOT recorded (entity-only).
- [ ] **H6** shadow compare no-ops without analytics: nil observer / `ProvidesData==nil` -> still force-fetches, records nothing.
- [ ] **H7** NO-OP / L1-only never yields `DecisionFetchShadow` (mode-matrix assert).

### 5.9 I/J/K/L. Fetch shapes, modes, flush, MergeSession

- [ ] **I1-I8** entity (`EntityFetch`) / root-field (`SingleFetch`) / batch (`BatchEntityFetch`) shapes: entity-scope keys + `EntityMergePath`; root-field-scope key + `cfg.L1=false`; `BatchEntityKey=true` + per-element multi-key + `BatchIndex`; batch full-hit/all-miss/mixed (mixed -> full re-fetch, NO partial realign in v1); batch empty short-circuit; root->entity L1 promotion is a v1 miss.
- [ ] **J1-J7** modes NO-OP / L1 / L2 / L1+L2 over the SAME query: assert the loader branches ONLY on `Decision`; L1 hit short-circuits L2 `Get`; L2 hit populates L1; mode-blindness (data-equal responses).
- [ ] **K1-K4** EndRequest flush: deferred writes ACCUMULATE then flush ONE batch per cache instance at `EndRequest`; write-through flushes immediately (empty `EndRequest` flush); flush holds BYTES not `*Value` (no `MergeSession` at flush); nothing-to-flush case.
- [ ] **L1-L7** MergeSession: `Begin()` takes `DataBuffer.Lock` once / `Close()` releases once (under `-race`); `ParseBytes` lazy + malformed->error no panic; `StructuralCopy` avoids aliasing; `MergeValues`/`WithPath` discard `changed`; `NewObject`/`NewArray`/`String`/`Null` arena-backed; no session opened from `loadPhase`.

### 5.10 O/P. Error/edge paths and `Equals` nil-safety

- [ ] **O1** `OnFetchSkipped` error -> `cacheMerge` returns it, `resolveSingle` propagates (wrapped).
- [ ] **O2** `OnFetchResult` error -> propagated.
- [ ] **O3** nested-merge branch unhooked (dead `res.nestedMergeItems`): cache hooks do NOT fire; no double-handle.
- [ ] **O4** `EmptyEntity` computed only when `res.response!=nil`: full-hit skip guards before `isEmptyEntityFetch`; no nil-deref.
- [ ] **O5** malformed cached bytes in `PrepareFetch`: `ParseBytes` errors; candidate skipped; no panic.
- [ ] **O6** header-hash nil-guard: no `SubgraphHeadersBuilder` -> `HeadersForSubgraphRequest` returns `(nil,0)`; key uses hash 0; no panic.
- [ ] **O7** cache-key fidelity: key derived from `PrepareFetchInput.Input` (canonical pre-injection), NOT post-prepare bytes; read key == write key (byte-identical invariant).
- [ ] **P1** both `Cache==nil` -> prior dedup result.
- [ ] **P2** one nil one set -> not equal.
- [ ] **P3** both set, equal -> dedup.
- [ ] **P4** both set, differ in any field (TTL / CacheName / a candidate Representation) -> not equal (`slices.EqualFunc` over `Candidates` via `Object.Equals`).
- [ ] **P5** KeySpec candidate deep-compare: differ only in one candidate's `Representation` -> distinguished.

### 5.11 Coverage cross-check (every loader-glue branch is hit)

- `cacheRequest`: nil controller A1/A4; lazy create B1; reuse existing (B1 second fetch).
- `cachePrepare`: `skipLoad` pre-set A5; Single/Entity/Batch arms I1/I2/I3; default arm A6; `cfg==nil` A2; `!L1&&!L2` A3; rc nil A4; decision arms C1-C5.
- `cacheMerge`: handle nil C2/A5; SkipFullHit/Partial->Skipped C3/C5; Fetch/Shadow->Result C1/C4; `res.response!=nil` guard O4; `FetchFailed` composition F2/F3/F4; error return O1/O2.
- `mergeResult` carriers (`response`/`responseData`/`responseHasErrors`): read by F*/G*/H*; dead-when-off A1.
- `endCacheRequest`/`clone`/`Free`: B3-B8, N6.
- `MergeSession` ops: L1-L7.
- Decisions: Fetch C1/C2; SkipFullHit C3; FetchPartial C5; FetchShadow C4/H1.

---

## 6. Concurrency tests (run under `-race`, ordered by gates inside a synctest bubble)

These run with the `-race` detector and use GATE channels (§2.7), never latency, for deterministic ordering (commit e509453b).
They live in the execution plan-driven layer (they need real plans + the public defer entry points) and run inside a `synctest.Test` bubble: every loader goroutine is spawned by the public entry point invoked within the body (so all goroutines are IN the bubble), the datasources are in-process gated fakes (no socket, bubble-safe), and ordering is forced with gates + `synctest.Wait()` rather than latency.
They prove the per-defer-group loaders share ONE request-lifetime L1 on `Context` under the scoped `MergeSession` lock,
and that lazy init and parallel writes are race-free.

- [ ] **M1** parallel fetches, lazy init race-free: 1 group, N parallel eligible fetches; `BeginRequest` EXACTLY once (lazy-init under `DataBuffer.Lock`); no race. `[gate]`
- [ ] **M2** parallel writes to shared L1: 2 parallel entity fetches, same type, both miss; both write L1 under the lock; first-writer-wins / merge-on-collision; no corruption. `[gate]`
- [ ] **M3** per-defer-group loaders share ONE L1: initial loader + defer-group loader, same `Context`; L1 written by the initial fetch is visible to the defer group's fetch (shared `ctx.requestCache` by reference). `[gate]`
- [ ] **M4** each hook = single lock acquisition: each `PrepareFetch`/`OnFetch*` takes `DataBuffer.Lock` once for its whole multi-op sequence; no per-op relock; no deadlock. `[gate]`
- [ ] **M5** error propagation under parallel: one parallel fetch's `OnFetchResult` errors -> propagated out of the group (the commit c619ff12 path); other fetches unaffected. `[gate]`

Sketch for the ordering primitive (M3, the load-bearing "L1 reuse across loaders" proof):

```go
func TestCaching_SharedL1_AcrossDeferGroups(t *testing.T) {
    synctest.Test(t, func(t *testing.T) {
        store := cachetesting.NewFakeStore()
        pr := Plan(t, cachetesting.StageL1,
            `{ topProducts { name ... @defer { reviews { product { name } } } } }`, /* responses */ nil)
        // wire the initial Product fetch to a GatedDataSource that signals + waits,
        // and the deferred Product _entities fetch to a fake that MUST hit L1 (asserted via store.Ops()==no Get to network)

        ctx := resolve.NewContext(t.Context())
        ctx.SetCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL1, store, nil)) // no clock arg

        var buf bytes.Buffer
        r := resolve.New(t.Context(), resolve.ResolverOptions{})
        // ResolveGraphQLDeferResponse spawns the per-defer-group loader goroutines INSIDE the bubble.
        _, err := r.ResolveGraphQLDeferResponse(ctx, pr.Response, nil, &buf)
        require.NoError(t, err)

        synctest.Wait() // all bubble goroutines durably blocked: the request is fully resolved

        assert.Equal(t, compactJSONForAssert(t, wantFrames), buf.String()) // full response
        assert.Equal(t, wantStoreOps, store.Ops())                         // deferred fetch served from L1, no second network read
    })
}
```

Ordering is forced with the §2.7 gates: the test releases the initial Product fetch, calls `synctest.Wait()` to be sure it has fully merged and written L1, and only then releases the deferred fetch — so "L1 reuse across loaders" is deterministic with no latency.
Where parallel ordering is genuinely free (M1/M2), the recorded `Calls()`/`Ops()` are sorted with `slices.SortFunc` before `assert.Equal`,
so the assertion remains a full-value structural comparison and never weakens to `Contains`.

---

## 7. Defer + caching combined

The interaction RFC-1 calls out explicitly: an entity cached by the initial fetch served to a deferred fetch,
a deferred fetch populating L1, and cache-vs-defer ordering / gates.
Driven via the PUBLIC `ResolveGraphQLDeferResponse`, with gates for deterministic frame order.

FIXTURE GAP (first pass, commit D3): the `commerce` supergraph could NOT express N1/N2/M3 (an initial fetch whose L1 entry is reused by a same-entity deferred fetch in a LATER group) without weakening the proof —
a duplicate initial/deferred selection of the same entity normalizes to a synchronous plan (no defer group), and a nested-defer shape produces a real defer group but the initial selection does not COVER the deferred selection, so `optimizeL1Cache` correctly narrows `l1:false` and nothing is reused.
Proving cross-defer-group L1 SERVING needs a richer fixture: an initial entity fetch whose `ProvidesData` is a strict superset of a deferred same-entity fetch across groups.
M1/M2 (lazy-init-once, parallel writes to the shared L1) ARE provable on `commerce` and are covered; N1/N2/M3 remain open pending that fixture.

- [ ] **N1** entity cached by initial fetch served to deferred fetch: deferred `PrepareFetch` -> L1 full hit -> `SkipFullHit`; deferred subgraph NOT hit; initial frame + deferred frame, deferred served from cache. `[gate]`
- [ ] **N2** deferred fetch populates L1 visible to a later group: group B hits L1 written by group A (visible because scheduled after A's `cacheMerge`). `[gate]`
- [ ] **N3** single `EndRequest` after ALL defer groups: exactly ONE `EndRequest` flushes deferred L2 writes from the initial fetch AND every group. `[gate]`
- [ ] **N4** defer ordering not broken by cache hits: `SkipFullHit` on one branch does not reorder defer frames; gates still drive deterministic emit order. `[gate]`
- [ ] **N5** cache + defer gate interplay: a deferred fetch's lookup/merge happens inside its group's locked region; no cross-group arena race (under `-race`). `[gate]`
- [ ] **N6** subscription event isolation: two events via `clone`; each has its OWN L1 (clone nilled `requestCache`); no cross-event bleed.

N1 asserts the full two-frame response bytes AND the full `Call` log showing the deferred fetch decided `SkipFullHit` with zero network `Get` to its subgraph.
N3 asserts `assert.Equal(t, int64(1), fakeController.endCount.Load())` plus the full backend `Ops()` after the request.

---

## 8. Multi-key tests (best-effort render-then-backfill)

The multi-key model: render every renderable candidate at lookup, look up under ALL rendered keys (a hit on ANY serves),
re-render the previously-unrenderable candidates from fresh data at write, backfill ALL renderable keys (RFC-1 §3.6/§3.7, RFC-2 §6.3).
Driven by `RealishCache` over `Product @key(upc) @key(sku)` (multi) and `User @key(id)` (single).

- [ ] **E1** all renderable at lookup (`topProducts{upc sku name}`): both candidates render -> 2 `RenderedKeys`; `Get` under `Product:upc=…` and `Product:sku=…`; `PendingCandidates` empty; hit on either serves. Assert the full ordered `Ops()` shows BOTH `Get`s.
- [ ] **E2** hit on a NON-primary key: seed only the `sku` key; `Get(upc)`->miss, `Get(sku)`->hit; serves from `sku`; full response from cache.
- [ ] **E3** backfill ALL after response (`product(upc:"1"){name sku}`): lookup renders only the `upc` candidate (arg-derived) -> one `Get`; `PendingCandidates`=[sku]; MISS; real fetch returns `sku`; `OnFetchResult` re-renders `sku` and `Set`s under BOTH keys; assert `Ops()` == `[Get upc, Set upc refresh, Set sku backfill]` with exact bytes + ttl.
- [ ] **E4** none renderable at lookup: all candidate fields absent in selected data -> zero `RenderedKeys`, no `Get`, `Fetch`; candidates re-rendered + written post-fetch.
- [ ] **E5** backfill on read-hit (`MustWriteBack`, no network): full hit on `upc`; `sku` renderable from the served value; `OnFetchSkipped` `Set`s the `sku` key; assert `Ops()` == `[Get upc, Set sku backfill]`, response from cache.
- [ ] **E6** refresh vs backfill reason tags: a pre-populated key rewritten == `refresh`, an absent key populated == `backfill`; assert the `WriteReason` on each `StoreOp`.
- [ ] **E7** single-`@key` entity (`user(id:)`): exactly 1 candidate, 1 `RenderedKey`; degenerate of E1, proves the multi-key code handles the one-element list.

Each multi-key row asserts the EXACT ordered `[]StoreOp` (so a missing/extra `Get` or `Set`, a wrong key, wrong bytes, or wrong `WriteReason` fails)
AND the full response bytes, per rule 11.

---

## 9. Unified test structure and naming convention

So every caching test looks the same and a reviewer can read any one of them cold.

### 9.1 Naming

- Plan-driven rows (layer ii, the execution-module `package …_test`): `TestCaching_<Area>_<Scenario>`, e.g. `TestCaching_Negative_HitServed`, `TestCaching_MultiKey_BackfillAll`.
- Controller unit rows (layer i, the v2 white-box `package cache`): `TestController_<Unit>_<Case>`, e.g. `TestController_Coverage_NullRejectedAtNonNullable`.
- Subtests use descriptive `t.Run("<full sentence>", ...)`; matrix IDs (A1, D7, …) appear in the subtest name in brackets so the catalog maps 1:1 to test output, e.g. `t.Run("[D7] freshest candidate is chosen", ...)`.

### 9.2 Table shape (modern Go 1.25)

```go
func TestCaching_Decision_Dispatch(t *testing.T) {
    tests := []struct {
        name      string
        stage     cachetesting.CacheStage
        query     string
        responses map[string]string
        script    map[string]cachetesting.ScriptedDecision
        wantCalls []cachetesting.Call
        wantBody  string
    }{
        {name: "[C1] miss fetches and writes", /* ... */},
        {name: "[C3] full hit skips network", /* ... */},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            pr := Plan(t, tt.stage, tt.query, tt.responses) // execution-module harness (§4.2)
            fake := cachetesting.NewRecordingCache(tt.script) // v2 fake
            ctx := resolve.NewContext(t.Context())
            ctx.SetCacheController(fake)

            var buf bytes.Buffer
            r := resolve.New(t.Context(), resolve.ResolverOptions{})
            _, err := r.ResolveGraphQLResponse(ctx, pr.Response, nil, &buf)
            require.NoError(t, err)

            assert.Equal(t, tt.wantCalls, fake.Calls())                          // FULL recorded calls
            assert.Equal(t, cachetesting.Compact(t, tt.wantBody), buf.String())  // FULL response bytes
        })
    }
}
```

Rows marked `[ttl]`/`[gate]` wrap the body in `synctest.Test(t, func(t *testing.T){ … })` (§2.5, §6); rows with no time/ordering concern (most decision-dispatch rows) need no bubble.

### 9.3 Assertion conventions (rule 11, restated for this suite)

- ALWAYS `assert.Equal` on the WHOLE value: the full `[]Call`, the full `[]StoreOp`, the full response string.
- NEVER `assert.Contains`, `assert.JSONEq`, `assert.GreaterOrEqual`, or any substring/fuzzy match.
- Non-deterministic bits are NORMALIZED, then asserted structurally:
  - pointers and arena `*astjson.Value` are projected to marshaled bytes/strings in the `Call`/`StoreOp` records (§2.3, §2.6);
  - free parallel ordering is made deterministic by gates (§2.7) or by sorting the record slice with `slices.SortFunc` before the single `assert.Equal`;
  - rendered cache keys are recomputed with the repo's xxhash pattern (§2.1) and asserted as full strings, never hard-coded magic numbers.
- `require.*` for preconditions that must abort (`require.NoError`, `require.False(report.HasErrors())`); `assert.*` for the checks.
- Use `t.Context()` for the resolver context, `t.Helper()` in every harness function, table tests + `t.Run` everywhere.

### 9.4 What each layer owns (so there is no duplication)

Two SUPPORT packages, built once:
- `v2/pkg/engine/resolve/cache/cachetesting` (v2, cosmo-free) owns the loader-driving fakes — the recording cache, `RealishCache` (wraps the real `cache.Controller`), `GatedDataSource`, `FakeCacheController` — plus `FakeStore`/`RecordingObserver`, the `CacheStage` enum, and the `Call`/`StoreOp` record types. Imported by layer (ii) (execution imports v2). There is no clock here — `synctest` is the clock. The white-box layer (i) does NOT import it (cycle: `cachetesting` imports `cache`); layer (i) uses tiny in-package `FakeStore`/`RecordingObserver` fakes instead.
- the execution-module support file owns the `Plan(...)` harness (real `wgc` composition -> `config_factory` -> planner + postprocess(caching) -> golden, §4) and the `cache_commerce` testdata, because only the execution module may depend on `config_factory` + the cosmo proto.

Two TEST layers (§1.3):
- Layer (i) — v2 controller unit tests — owns the controller LOGIC rows driven on the cache surface directly with CONSTRUCTED `*astjson.Value` + `FakeStore` + `synctest`, no plan: coverage/freshness/reorder/key-render (D), write/backfill gate (F), multi-key render-then-backfill (E), negative (G), shadow-compare (H compare-side), and the `FetchCacheConfig.Equals` nil-safety (P). It splits into white-box `package cache` for rows that touch unexported internals (using in-package `FakeStore`/`RecordingObserver`) and black-box `package cache_test` for rows driven through `RealishCache` (which legally imports `cachetesting`).
- Layer (ii) — the execution-module `package …_test` — owns everything that needs the real loader against a real plan, driven via the PUBLIC `ResolveGraphQLResponse`/`ArenaResolveGraphQLResponse`/`ResolveGraphQLDeferResponse`: the loader-seam dispatch + lifecycle (A/B/C), flush + `MergeSession` discipline (K/L, asserted observably + under `-race`), the mode matrix (J), the end-to-end hit/multi-key behavior, defer + caching (N), and concurrency (M). Time-sensitive and ordered rows run inside a `synctest` bubble (§2.5, §6).
- The few seams an earlier draft reached through unexported `resolve` access are asserted OBSERVABLY in layer (ii) — the recording fake proves prepare->merge handle identity, the gated datasource + `store.Ops()` prove no-network-on-hit, and `-race` proves lock discipline — so no `export_test.go` bridge exists.
