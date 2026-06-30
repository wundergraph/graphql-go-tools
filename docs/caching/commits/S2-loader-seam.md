# Commit S2 — RFC-1 contract types + the no-op loader seam

Plan item: `docs/caching/PLAN.md` §2, S2.
RFC sections: RFC-1 §3 (the abstraction), §4 (the one loader/resolve commit), §7.4 (boundary), §10 (NO-OP gating), Appendix B (responseData surfacing).
Phase: 0 (Structure, pure no-op).

## Problem

The loader had no cache abstraction.
RFC-2 (the planner) and the future `resolve/cache` controller need the runtime OUTPUT contract to exist and be wired as a strict no-op first,
so the caching implementation can live entirely outside `loader.go`.

## Solution

One mostly-additive commit that compiles and behaves identically (RFC-1 §4, G8); no cache implementation.

New contract files (package `resolve`, pure additions):

- `cache_controller.go`: `CacheController`, `RequestCache`, `Decision` (incl. `DecisionFetchShadow`), `PrepareFetchInput`, `MergeInput`, `MergeArena`, `MergeSession`, `CacheObserver`, `FetchCacheHandle` (+ nil-safe `String`), `ItemCacheState`, `CacheCandidate`, `ShadowCacheEntry`, plus the loader-side `mergeArena`/`mergeSession` scoped-lock facade.
- `cache_config.go`: `FetchCacheConfig` (+ `Equals`, nil-safe `String`), `CacheKeySpec` (+ `Equals`), `CacheKeyCandidate`, `CacheScope`, `CacheWriteReason`, `EntityKeyMapping`/`EntityFieldMapping`.

Loader/resolve seam (additive, all guarded):

- `loader.go`: `preparedFetch.cacheHandle`; `result.{response, responseData, responseHasErrors}`; the three carrier assignments in `mergeResult` (set where the values are already computed, `res.response` right after a successful parse so it is nil on every pre-parse failure path); the two lock-free call sites in `resolveSingle` (`cachePrepare` after the render phase, `cacheMerge` after the merge phase); helpers `cacheRequest` / `cachePrepare` / `cacheMerge` / `mergeArena`.
- `context.go`: `cacheController` / `requestCache` fields, `SetCacheController`, `endCacheRequest`, `clone` sets `requestCache=nil`, `Free` defensively tears down.
- `fetch.go`: `Cache *FetchCacheConfig` on `FetchConfiguration` (embedded by `SingleFetch`), `EntityFetch`, `BatchEntityFetch`; the nil-safe cache clause in `FetchConfiguration.Equals`.
- `resolve.go`: `defer ctx.endCacheRequest()` at all FOUR entry functions (`ResolveGraphQLResponse`, `ArenaResolveGraphQLResponse`, `ResolveGraphQLDeferResponse`, `executeSubscriptionUpdate`).

## Key decisions

- Two composed gates: a hook fires only when BOTH `cacheController != nil` AND a per-fetch `Cache` with `L1||L2` exists; the `mergeArena` facade is built only on the non-nil-controller path (RFC-1 §10).
- `DecisionSkipFullHit` sets BOTH `prepared.skipLoad` and `prepared.res.fetchSkipped`, reusing the existing `mergeResult` early-return so a full hit renders no spurious "failed to fetch" error (RFC-1 §4.3).
- `ProvidesData` is folded into `FetchCacheConfig` (no `FetchInfo.ProvidesData` re-add), so `loader.go` never references `*Object` for caching.
- `Cache` is a POINTER; nil is the gate.

Deviations from the RFC's literal code (necessary for this branch's actual APIs, both behavior-neutral):

- `CacheKeySpec.Equals` compares `EntityKeyMappings` with `slices.EqualFunc` (not `slices.Equal`), because `EntityKeyMapping` carries a `[]string` argument path and is therefore not a comparable type.
- `MergeSession.StructuralCopy` delegates to `astjson.DeepCopy` because this module pins astjson v1.1.0, which does not expose a `StructuralCopy`; a full deep copy is strictly safe against the merge-aliasing the method exists to prevent. (Revisit in A2 if a more surgical copy is wanted.)

## Tests

- New in-package file `cache_config_test.go` (the documented engine-package in-package exception, CODING_GUIDELINES §4.1):
  `FetchConfiguration.Equals` cache clause P1-P5 (both nil -> dedup; one nil -> not equal; both equal -> dedup; scalar differs -> not equal; one candidate `Representation` differs -> not equal via `slices.EqualFunc` + `Object.Equals`);
  `FetchCacheConfig.String` and `FetchCacheHandle.String` nil-safe rendering, asserting the exact strings.
  All full-value `assert.Equal`.

Verification (run from `v2/`):

- `go build ./pkg/...` — clean (no import cycle; facade compiles).
- `go test ./pkg/engine/resolve/... -count=1` — PASS (full existing suite + new tests). This is the runtime NO-OP invariant: the existing suite is byte-for-byte behavior-identical with no `SetCacheController`.
- `go test ./pkg/engine/postprocess/... ./pkg/engine/plan/... ./pkg/engine/datasource/graphql_datasource/... -count=1` — PASS (the `fetch.go` change does not disturb dedup/plan goldens).
- `go vet ./pkg/engine/resolve/...` — clean.

## Reviewer guidance

- Merging this alone must change runtime behavior in ZERO ways — verified by the unchanged-green resolve suite with no controller.
- The two `resolveSingle` call sites are OUTSIDE the phase locks (so the controller's `MergeSession` is the single `DataBuffer.Lock` acquisition per hook).
- `clone`/`Free` reset the per-request surface (`requestCache`); `cacheController` is copied by value on clone and nilled on `Free`.
- No `*Object` is referenced by the loader for caching (`ProvidesData` lives in the config).
