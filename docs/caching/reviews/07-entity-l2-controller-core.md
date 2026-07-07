# Reviewer notes — task 07: entity L2 controller core (single candidate)

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/07-entity-l2-controller-core.md](../tasks/07-entity-l2-controller-core.md).
Spec background: RFC-1 §3, §4.4, §5, §6; appendix rows A/B/C/D-core/F/K/L/O; deviations D2, D4, D5.

## What this commit adds

The first working `RequestCache`: an L2-only entity controller for the single-candidate case — lookup → coverage → decision → gated write → request-end flush.
An end-to-end L2 entity hit now works over real plans: request 1 misses and writes, request 2 serves from the cache with ZERO network for the cached fetch.

## Decisions made

- Port source is the first-pass controller, cut down to the task's scope with EXPLICIT deferral gates (each with a task-reference comment): shadow-configured fetches behave as plain misses until task 12 (read-never-serve holds trivially — no reads at all), root-field scope until 13, batch until 10, negative sentinels until 11, L1 until 17, multi-candidate selection/reorder/backfill until 08–09.
  `PrepareFetch` uses `Candidates[0]` only (the single-candidate contract).
- NO `Mode` enum (the first pass had NoOp/L1/L2/L1L2): the controller is the L2 module; enablement derives from `cfg.L2 && store != nil`.
  Task 17 composes L1 without a mode switch.
- `cacheKeyTemplate` is a named type (the task's "one CacheKeyTemplate per candidate — the sole source of read/write/invalidate keys"), wrapping (prefix, representation) with a best-effort `render`; keys are `<prefix>:<16-hex pooled-xxhash64>`; numbers canonicalize to their JSON string in the preimage so `1` and `1.0` cannot split the key space; a representation that renders zero key fields is unrenderable (defense in depth on top of the task-06 builder guard).
- D4 applied structurally: the merge hooks read `MergeInput.MergePath` — the write extracts the entity BELOW the path before storing, the splice merges the cached value AT the path (`ItemCacheState.EntityMergePath` stays unset; the prepare phase has no path input).
- `ttlForConfig` was not ported: it special-cased `MutationTTLOverride`, and mutation caching is out-of-core (D12); writes use `cfg.TTL` directly.
- The first pass parsed cached bytes and recorded `FromCacheCandidates` even on parse failure paths; here a malformed cached value records NOTHING (miss, no candidate) and never panics (unit row).
- `resolve.NewTransactionBeginner(arena, db)` is new EXPORTED API: controller unit tests (and any out-of-tree controller's tests) need to construct the transaction seam; the loader keeps wiring its own internally.
- The lock-discipline invariant is documented at the `requestCache` struct: all mutable fields are guarded by the transaction's `DataBuffer.Lock` (external lock, no internal mutex); `EndRequest` is single-threaded and touches bytes only.

## What was implemented

- `v2/pkg/engine/cache/controller.go` — `Store`, `Controller`/`NewController`/`BeginRequest`, `requestCache` with the four hooks, `deferredSet` + `deferSet` + flush.
- `v2/pkg/engine/cache/cache_key_template.go` — `cacheKeyTemplate`/`newCacheKeyTemplates`/`render`, `renderRepresentationValue`, `cacheKeyPrefix`, `renderCacheKey`, pooled hashing.
- `v2/pkg/engine/cache/coverage.go` — `covers`/`coversNode` (response-name reads for now; task 09 extends the naming).
- `v2/pkg/engine/resolve/cache_transaction.go` — `NewTransactionBeginner`.
- `cachetesting/realish.go` — `StoreAdapter` (FakeStore → `cache.Store`) and `NewRealishCache` (the REAL controller over the in-memory store), completing the task-04 deferral.

Tests (row IDs in subtest names):

- `controller_test.go` (unit, constructed astjson + a local `testStore` — `cachetesting` cannot be imported by the package it imports):
  D rows (full hit incl. read-key==write-key, miss, coverage-fail with the candidate recorded but NOT served, nullability both ways, `ProvidesData == nil`, partial-hit AND-reduction, malformed-bytes no-panic, TTL expiry in a synctest bubble);
  F rows F1–F8 (clean write with exact op log and TTL; transport/empty/parse/status failures with `HasErrors == false` — the historical blocking bug — plus GraphQL errors, JSON-null, and empty-entity all producing ZERO Sets);
  K rows K1–K4 (accumulate then one flush batch, byte-snapshot isolation from later value mutation, empty flush);
  the D4 merge-path rows (store below the path, splice at the path, root splice) and the gate/nil-guard rows (O).
- `execution/cachingtesting/entity_l2_test.go` (e2e over real plans, public entry points, also run under `-race`):
  the end-to-end L2 hit (complete responses, per-datasource load counts via the new name-translating `PlanResult.LoadCount`, exact store-op log across both requests proving key fidelity, lazy single `BeginRequest`, observer begin/end counts, and C7 — LoaderHooks silent for the skipped fetch);
  dispatch rows with the recording fake (decision → hook routing, C8 handle pointer identity, hook-error propagation to the caller).

## What to look into (review focus)

- The write gate order in `OnFetchResult`: `FetchFailed || HasErrors` first, then `ResponseData == nil || TypeNull` — confirm no path can write after any failure signal.
- One transaction per hook: `PrepareFetch`/`OnFetchSkipped`/`OnFetchResult` each call `in.Arena.Begin()` exactly once with `defer tx.Commit()`; nothing arena-touching sits outside.
- The full-hit path relies on the task-02 loader seam (`skipLoad` + `fetchSkipped`); the e2e hit asserting the COMPLETE response is the no-spurious-error proof (C3/C6).
- The deferral gates: each unimplemented feature fails CLOSED (plain fetch, no serving, no write) — worth re-checking against tasks 08–13 scopes.
- `cachetesting` grew `StoreAdapter`/`NewRealishCache`; the unit tests keep a minimal local store double because of the import direction — flag if you prefer moving the op-log store into a third shared package instead.

## Verification evidence

- All unit + e2e rows pass; `-race` clean over `engine/cache`, `cachetesting`, and the execution harness tests.
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
