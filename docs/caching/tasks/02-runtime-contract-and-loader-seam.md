# Task 02 — Runtime cache contract + no-op loader seam + CacheTransaction

Phase: 0 (Structure).
Dependencies: none (independent of task 01).
References: RFC-1 §3, §4, §6, §10; deviations D2, D4, D5, D8 (PLAN §7).

## Problem

The loader has no cache abstraction.
The controller package and the planner passes need the runtime OUTPUT contract to exist and be wired as a strict no-op first, so every later task plugs into stable seams.

## Scope

One mostly-additive change that compiles and behaves identically with no controller set; no cache implementation.

Contract types (new files in `resolve`):

- `resolve/cache_controller.go`: `CacheController` (`BeginRequest(ctx) RequestCache`), `RequestCache` (`PrepareFetch`, `OnFetchSkipped`, `OnFetchResult`, `EndRequest`), `Decision` (`DecisionFetch`, `DecisionSkipFullHit`, `DecisionFetchPartial`, `DecisionFetchShadow`), `PrepareFetchInput`, `MergeInput`, `CacheObserver`, `FetchCacheHandle`, `ItemCacheState`, `CacheCandidate`, `ShadowCacheEntry`.
- `resolve/cache_transaction.go`: the `CacheTransaction` abstraction (D2) — `TransactionBeginner` with `Begin() *CacheTransaction`; methods on the transaction: `ParseBytes`, `StructuralCopy`, `MergeValues`, `MergeValuesWithPath`, `NewObject`, `NewArray`, `String`, `Null`, `Commit()`.
  `Begin()` acquires `DataBuffer.Lock` once; `Commit()` releases it (call via `defer`).
  Every arena/parser op is a method ON the transaction, so the arena cannot be touched without a held transaction; a transaction must never be retained past the hook or opened from the off-lock load phase.
- `resolve/cache_config.go`: `FetchCacheConfig` (+ nil-safe `Equals`, `String`), `CacheKeySpec`, `CacheKeyCandidate` (one frozen `@key` representation per candidate), `CacheScope`, `CacheWriteReason`, `EntityKeyMapping`.

Fetch polymorphism (D8):

- `fetch.go`: add `Cache *FetchCacheConfig` to `FetchConfiguration`, `EntityFetch`, `BatchEntityFetch`.
- Extend the `Fetch` interface with cache accessors and shape predicates, e.g. `CacheConfig() *FetchCacheConfig`, `SetCacheConfig(*FetchCacheConfig)`, `IsEntityFetch() bool`, `IsBatchEntityFetch() bool`, implemented on all concrete types.
  All caching code (loader seam, passes, controller) uses these methods; NO `switch` over concrete fetch types.
  While here, improve other fetch-type-switch sites encountered in the code being touched (e.g. re-express existing switches through the new predicates where it is a clear simplification) — surgical, one commit note per site.
- `FetchConfiguration.Equals`: the nil-safe cache clause (`(a==nil)!=(b==nil)` → not equal; both non-nil → `a.Equals(b)`), so plan dedup can never lose or conflate cache policy.

Loader seam (`loader.go`, `resolve.go`, `context.go`):

- `preparedFetch.cacheHandle *FetchCacheHandle`.
- `result` carriers: `response`, `responseData`, `responseHasErrors` — assigned in `mergeResult` where already computed (full response right after the successful parse; data sub-path after it is computed; error flag after the hasErrors block), so `response == nil` is the structural fetch-failed signal.
- `MergeInput` also carries the fetch's post-processing MERGE PATH (D4), so entity/batch values splice at the correct target, not silently at the item root.
- Two behavior call sites in `resolveSingle`, both OUTSIDE the phase locks: `cachePrepare` after the prepare phase, `cacheMerge` after the merge phase; helpers `cacheRequest` (lazy once-per-request `BeginRequest` under `DataBuffer.Lock`), `cachePrepare`, `cacheMerge`.
- Full-hit modeling: `DecisionSkipFullHit` sets BOTH `prepared.skipLoad` (skips the network) and `res.fetchSkipped` (reuses the existing `mergeResult` early-return so no spurious "failed to fetch" error is rendered).
- `cachePrepare` early-returns when `prepared.skipLoad` is already set (render-skip sites win) and when `cfg == nil || (!cfg.L1 && !cfg.L2)`.
- `context.go`: `cacheController`/`requestCache` fields, `SetCacheController` (mirrors `SetAuthorizer`), `endCacheRequest()` (idempotent), `clone` sets `requestCache = nil` (subscription-event isolation), `Free` calls `endCacheRequest` defensively.
- `resolve.go`: `defer ctx.endCacheRequest()` at ALL FOUR entry functions (`ResolveGraphQLResponse`, `ArenaResolveGraphQLResponse`, `ResolveGraphQLDeferResponse`, `executeSubscriptionUpdate`).

Write-gate signals surfaced on `MergeInput` (all five): `FetchFailed` (transport / empty body / parse failure), `HasErrors`, `EmptyEntity` (computed via the loader's existing `isEmptyEntityFetch`, only when a response parsed), `StatusCode`, and `ResponseData == nil`.

Integration posture (per maintainer feedback): the seam MAY be more integrated with the loader than a detached six-call-site design where that makes L1/L2/partial easier to reason about; keep the hooks as the default shape but do not contort the loader to avoid touching it.

## Tests

Pure, self-contained (no harness yet):

- `FetchConfiguration.Equals` cache clause: appendix rows P1–P5 (both nil, one nil, equal, differ in any field, differ in one candidate `Representation`).
- Nil-controller no-op: an existing-style loader test with no controller stays byte-identical; the full existing resolve suite passes.
- `FetchCacheConfig.String` / `FetchCacheHandle.String` render nil-safely.
- `Fetch` interface methods: table test asserting `CacheConfig`/predicates across all concrete fetch types.
- `CacheTransaction`: `Begin`/`Commit` lock-once semantics compile-checked here; behavioral lock-discipline rows land with task 04/07 under `-race`.

## Acceptance criteria

- [ ] Merging this task ALONE changes runtime behavior in ZERO ways (no controller → no cache code entered, no allocation, no lock).
- [ ] The two hook call sites are outside the phase locks; the transaction is the single `DataBuffer.Lock` acquisition per hook.
- [ ] `clone`/`Free` reset the per-request surface; `endCacheRequest` is idempotent.
- [ ] No `switch` over concrete fetch types anywhere in the new code.
- [ ] Every new type carries a responsibility doc comment.
- [ ] Lint-clean in `v2`.

## Reviewer guidance

- The write gate must key off `FetchFailed`/`ResponseData == nil`, never `HasErrors` alone (transport/empty/parse failures reach the merge hook with `HasErrors == false`).
- Verify the merge-path surfacing (D4) carries the real fetch merge path and is nil/empty-safe.
- Verify no `*Object` is referenced by `loader.go` (`ProvidesData` lives inside the config, consumed by the controller).
