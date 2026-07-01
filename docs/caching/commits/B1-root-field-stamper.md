# Commit B1 — root-field stamper + root-field freezer scope

Plan item: `docs/caching/PLAN.md` §4, B1.
RFC sections: RFC-2 §6 (freezer), §7 (stamper), §7.2 (the root-field all-or-nothing rule).
Phase: B (L2 root fields).

## Problem

Root-field fetches carried no cache config, and a merged fetch may mix root-field policies. v1 must cache a root-field fetch only when it is safe — and decline rather than mis-cache when policies differ.

## Solution

Fill the root-field arms left as TODO in A1a (plan-side only; runtime is B2).

- `cacheConfigStamper` root-field arm: `rootFieldPolicyForAllRootFields(provider, info)` walks `info.RootFields` and returns `(policy, true)` ONLY when every root field resolves to an IDENTICAL `RootFieldCachePolicy` (`sameRootFieldCachePolicy` over `CacheName`/`TTL`/`IncludeSubgraphHeaderPrefix`/`ShadowMode`/`PartialBatchLoad`); any uncached sibling or differing policy -> `(zero, false)` and the stamper leaves `Cache` nil. When a policy applies, it stamps `L1=false`, `L2 = TTL>0`, the policy scalars, and the root-field `KeySpec`.
- `cacheKeySpecFreezer` root-field scope: `freeze(CacheScopeRootField, info)` returns `CacheKeySpec{Scope: RootField, TypeName, FieldName, Candidates: nil}` (a plain root field has no `@key` representation; the runtime keys it from the canonical fetch `Input`).
- Execution harness `cacheProvidersForStage(StageL2RootFields)`: the fake provider now also returns `RootFieldPolicy{CacheName:"rootfields", TTL:1m}` for `(Query, topProducts)` and `(Query, latestReviews)`.

## Key decisions

- Conservative all-or-nothing decline reproduces OLD behavior additively in the post-plan stamper, with ZERO path-builder edits. Per-root-field isolation (the optimization that lets differing root fields each cache) is RFC-03, out of scope; v1 declines.
- `L1=false`: root fields are L2-only in v1 (they do not populate or read the request-lifetime L1 entity store); root->entity L1 promotion is deferred to v2.
- L2 derived from `TTL>0`.

## Tests

- `postprocess/cache_config_stamper_test.go`: a single cached root field -> full `*FetchCacheConfig` (`L1:false`, `L2:true`, root-field `KeySpec`); a MIXED-policy fetch -> `Cache` nil (the conservative decline). Full-value `assert.Equal`.
- `postprocess/cache_key_spec_freezer_test.go`: root-field freeze returns `CacheKeySpec{Scope: RootField, TypeName, FieldName, Candidates: nil}`.
- Execution `caching_rootfield_golden_test.go` `TestCaching_StageL2RootFields_Golden` over `{ topProducts { upc name } }`: the root fetch carries `Cache{l1:false l2:true scope:RootField type:Query field:topProducts candidates:0 providesData:true}`.

Verification:

- `cd v2 && go test ./pkg/engine/postprocess/... ./pkg/engine/plan/... ./pkg/engine/resolve/... -count=1` — PASS; StageNoop planner no-op golden byte-identical.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (StageNoop + StageL2Entities goldens unchanged; new StageL2RootFields golden).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/postprocess/...` — clean; zero diff to the five forbidden visitor files.

## Reviewer guidance

- Confirm a mixed-policy / cached+uncached fetch declines L2 (Cache nil).
- Confirm `path_builder_visitor.go` shows zero diff (no isolation hook; RFC-03 is separate).
- The runtime root-field controller path is B2.
