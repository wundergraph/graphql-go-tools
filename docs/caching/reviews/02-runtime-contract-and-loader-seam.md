# Reviewer notes — task 02: runtime contract + loader seam + CacheTransaction

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/02-runtime-contract-and-loader-seam.md](../tasks/02-runtime-contract-and-loader-seam.md).
Spec background: RFC-1 §3, §4, §6, §10; deviations D2, D4, D5, D8 (PLAN §7).

## What this commit adds

The complete runtime cache CONTRACT (types the loader and the future controller share) plus the loader SEAM, wired as a strict no-op.
No cache implementation exists yet; with no `SetCacheController` the loader path is byte-identical to before this commit.
Every later runtime task (07–12, 17–19) plugs into these seams without touching the loader again.

Three new contract files in `resolve`:

- `cache_controller.go` — `CacheController` (lifecycle port), `RequestCache` (the ONE mode-blind working surface: `PrepareFetch` / `OnFetchSkipped` / `OnFetchResult` / `EndRequest`), `Decision` enum, `PrepareFetchInput`, `MergeInput`, `CacheObserver`, `FetchCacheHandle` + `ItemCacheState` + `CacheCandidate` + `ShadowCacheEntry` (the opaque two-level handle).
- `cache_transaction.go` — `TransactionBeginner` + concrete `CacheTransaction` (D2: the RFC's `MergeArena`/`MergeSession` renamed and reshaped; `Begin()` takes `DataBuffer.Lock` once, ops are methods on the transaction, `Commit()` releases).
- `cache_config.go` — `FetchCacheConfig` (nil-safe `Equals`/`String`), `CacheKeySpec`, `CacheKeyCandidate`, `CacheScope`, `CacheWriteReason`, `EntityKeyMapping`/`EntityFieldMapping`.

## Decisions made (and deviations applied)

- D2 applied: `CacheTransaction` is a CONCRETE struct with unexported fields, so the arena is untouchable outside a held transaction by construction; `Commit()` (not `Close()`) releases the lock.
- D8 applied: `cachePrepare` reads the config via the new `Fetch.CacheConfig()` interface method — NOT the RFC's `switch` over concrete fetch types.
  The `Fetch` interface gains `CacheConfig()`, `SetCacheConfig(...)`, `IsEntityFetch()`, `IsBatchEntityFetch()`, implemented on all three concrete types.
- D4 applied: `MergeInput.MergePath` carries `res.postProcessing.MergePath`, so entity/batch values can splice at the correct non-root target (consumed from task 07 on).
- `FetchCacheConfig.Equals` is nil-safe ITSELF (`c == nil || other == nil` → pointer equality) in addition to the nil-safe clause at the `FetchConfiguration.Equals` call site; the task file asks for a nil-safe `Equals`, and belt-and-braces here avoids the nil-receiver footgun RFC-1 §3.8 flags.
- `ShadowCacheEntry` carries the RFC's three fields only (the first pass had added a fourth, `CacheTTL`); task 12 adds fields if the shadow compare actually needs them.
  Same for `ItemCacheState`: the first-pass extra `BatchEntityKey` per-item flag was NOT ported (it lives on the handle).
- Full-hit modeling: `DecisionSkipFullHit` sets BOTH `prepared.skipLoad` (skips the network via the existing `loadPhase` guard) and `res.fetchSkipped` (reuses the existing `mergeResult` early-return so no spurious "failed to fetch" error is rendered for the empty `res.out`).
- `cachePrepare` runs only when `!prepared.skipLoad` (render-skip sites win — a fetch the loader already decided to skip is not a cache decision) and only when `cfg != nil && (cfg.L1 || cfg.L2)`.
- `cacheRequest` creates the request-lifetime surface lazily, ONCE, under `DataBuffer.Lock` (race-free across parallel fetches and per-defer-group Loaders because there is exactly one `DataBuffer` per request); it does NOT hold the lock across the hook.
- The three `result` carrier fields (`response`, `responseData`, `responseHasErrors`) are assigned in `mergeResult` exactly where each value is already computed, per RFC-1 Appendix B; they are dead weight when caching is off.
- `isEmptyEntityFetch` stays the single source of truth for empty-entity detection: `cacheMerge` calls it (guarded by `res.response != nil`) instead of re-deriving the rule cache-side.
- Sanctioned fetch-switch cleanup survey: `preparePhase`'s switch dispatches to type-specific prepare functions that need the concrete types (not caching code, not simplifiable by the new predicates), and `isEmptyEntityFetch` already dispatches polymorphically via `FetchKind()`; no site qualified for a clear-simplification rewrite, so none was touched.

## What was implemented

- Contract files as above (pure additions).
- `fetch.go`: `Cache *FetchCacheConfig` on `FetchConfiguration` / `EntityFetch` / `BatchEntityFetch`; the four new `Fetch` interface methods on all three concrete types; the nil-safe cache clause in `FetchConfiguration.Equals`.
- `loader.go`: `preparedFetch.cacheHandle`; the two behavior call sites in `resolveSingle` (`cachePrepare` after the prepare phase, `cacheMerge` after the merge phase, both OUTSIDE the phase locks); helpers `cacheRequest` / `cachePrepare` / `cacheMerge` / `cacheTransactions`; the three `mergeResult` carrier assignments.
- `context.go`: `cacheController`/`requestCache` fields, `SetCacheController` (mirrors `SetAuthorizer`), idempotent `endCacheRequest()`, `clone` resets `requestCache` (subscription-event isolation), `Free` calls `endCacheRequest` defensively and nils the controller.
- `resolve.go`: `defer ctx.endCacheRequest()` at ALL FOUR entry functions (`ResolveGraphQLResponse`, `ArenaResolveGraphQLResponse`, `ResolveGraphQLDeferResponse`, `executeSubscriptionUpdate`), so the request-end flush covers sync, defer, and subscription paths.
- Compile fixes: the three test-only `Fetch` stubs (`mockFetchWithInfo`, `stubFetch`, `nilInfoFetch`) got the four new methods.

New tests (dedicated files, full-value `assert.Equal`):

- `cache_config_test.go` — `FetchCacheConfig.Equals` nil-safety + a 24-row mutation table proving EVERY field participates; `FetchConfiguration.Equals` cache clause rows P1–P5; exact `String()` renders (nil / populated / zero value); `CacheScope.String`.
- `cache_fetch_test.go` — cache accessor + predicate table across all three concrete fetch types; a pin that `SingleFetch` stores the config in the embedded `FetchConfiguration` (what plan dedup compares).
- `cache_controller_test.go` — `Decision.String` exhaustive; `FetchCacheHandle.String` nil / zero / populated / shadow, exact strings.
- `cache_noop_test.go` — the no-op gates asserted OBSERVABLY: a real loader run (mocked datasource) with no controller, and with a controller but no per-fetch config, both produce the exact expected response and never call `BeginRequest`; `endCacheRequest` idempotency; `clone` resets `requestCache` while keeping the controller port.

## What to look into (review focus)

- WRITE GATE plumbing (the RFC's blocking-bug class): `MergeInput.FetchFailed` is `res.err != nil || len(res.out) == 0 || res.response == nil` and `res.response` is assigned ONLY after a successful parse — confirm a controller following the documented gate can never cache a transport/empty/parse failure even though `HasErrors` is false on those paths.
- Lock discipline: both hook call sites are in `resolveSingle` AFTER the phase functions return (locks released); `cacheRequest` takes the lock only for the once-create; `CacheTransaction.Begin/Commit` is the single acquisition per hook. Nothing cache-related runs inside `preparePhase`/`mergePhase`/`loadPhase`.
- No-op strictness: with a nil controller the only new work on the hot path is one `CacheConfig()` interface call returning nil per fetch (plus three dead field writes in `mergeResult`); confirm that is acceptable as "zero behavior change" (it allocates nothing and branches identically).
- `loader.go` references no `*Object` for caching (`ProvidesData` lives inside the config, consumed by the future controller) — grep guard per task reviewer guidance.
- The nested-merge branch (`res.nestedMergeItems`) is intentionally NOT hooked: the field is never assigned anywhere in the repo (RFC-1 §4.7 proof); flag if that ever changes.

## Verification evidence

- New contract tests: all pass (see commit CI output); full `resolve` package suite: ok (8.9s).
- Full `v2` suite: 40 packages ok, exit 0; `execution` module: 5 packages ok, exit 0 (no-op gate: entire existing behavior unchanged).
- `go test -race ./pkg/engine/resolve/`: ok, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`, which the local binary lacks): 0 issues; `gci`/`gofmt` clean.
