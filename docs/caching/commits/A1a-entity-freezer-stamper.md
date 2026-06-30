# Commit A1a — entity key-spec freezer + entity config stamper

Plan item: `docs/caching/PLAN.md` §3, A1 (first of two parts).
RFC sections: RFC-2 §6, §6.1, §6.3 (the freezer + multi-key model), §7, §7.1 (the stamper).
Phase: A (L2 entities).

## Problem

Entity fetches carried no cache config, so the runtime would have no keys and no policy.
The plan-side producer must freeze every resolvable `@key` set into a self-contained multi-key `CacheKeySpec` and stamp a `*FetchCacheConfig` onto entity fetches — reading federation `@key` info ONLY as a plan-time input, by value.

## Solution

Fill two of the three S3 skeletons; `ProvidesData` stays nil (the P1 visitor port is A1b) and the root-field arm stays nil (B1). Nothing consumes the stamped config yet (the controller is A2), so this is a safe increment.

- `cacheKeySpecFreezer.freeze` entity path (RFC-2 §6.1): for `CacheScopeEntity`, gate on `fed.HasEntity(typeName)`, read ALL resolvable `@key` sets via `fed.RequiredFieldsByKey`, sort deterministically by `SelectionSet` string, and build one `CacheKeyCandidate{Representation}` per set via the shared `representationvariable.BuildRepresentationVariableNode` (S1). None is required (best-effort multi-key). Returns `(zero,false)` for non-entity scope (root-field freezing is B1) and for no-`@key` types. `EntityKeyMappings` is stubbed nil (`TODO(C1)`).
- `cacheConfigStamper` entity arm (RFC-2 §7): `stamp` now SETS `f.Cache` on the concrete `*SingleFetch`/`*EntityFetch`/`*BatchEntityFetch`. `buildConfig` looks up `provider.EntityPolicy`, freezes the entity `KeySpec`, and assembles `L1=true`, `L2 = TTL>0 || NegativeCacheTTL>0`, the scalar policy fields, and the frozen spec. `cfg.ProvidesData = pd[info]` (nil this commit) with `ComputeHasAliases` folded in (guarded). The root-field arm returns nil (`TODO(B1)`). Final gate: nil when nothing is active.
- `resolve.ComputeHasAliases` re-added (additive; OLD-verbatim semantics): sets `Object.HasAliases` when any descendant field has an alias (`OriginalName`) or `CacheArgs`.
- Execution harness `cacheProvidersForStage`: returns a real fake provider for `StageL2Entities` (entity policy `{CacheName:"entities", TTL:1m}` for `Product`/`User`, keyed to every datasource id); `StageNoop` stays empty.

## Key decisions

- Multi-key best-effort: every resolvable `@key` set on the entity becomes one independent candidate; a subgraph that resolves the entity by only one of several `@key`s yields exactly the keys it can render (e.g. the reviews subgraph resolves `Product` by `upc` only -> 1 candidate, even though `Product` declares `@key(upc) @key(sku)`).
- Deterministic candidate order: sort `@key` sets by selection-set string before building (so `sku` precedes `upc`).
- No federation pointer escapes the freezer: the shared builder allocates fresh `resolve` nodes; a post-freeze mutation of the source `FederationMetaData` leaves the frozen spec unchanged (tested).
- L2 is derived structurally from TTL (`TTL>0 || NegativeCacheTTL>0`), per RFC-2 §7.1.

## Tests

- `postprocess/cache_key_spec_freezer_test.go`: full `CacheKeySpec` for single (`User @key(id)`), composite (`@key("a b")`), nested (`@key("info { id }")`), and MULTIPLE (`Product @key(upc) @key(sku)` -> two candidates, deterministically ordered) `@key` sets; no-`@key` -> `(zero,false)`; and a post-freeze `FederationMetaData` mutation that re-asserts equality (no pointer aliasing).
- `postprocess/cache_config_stamper_test.go`: full `*FetchCacheConfig` stamped on an entity fetch from a fake provider; nil where the provider returns no policy.
- `execution/engine/caching_entity_golden_test.go`: `TestCaching_StageL2Entities_Golden` over `{ topProducts { upc name reviews { body } } }` (yields a Product entity fetch on the reviews subgraph); the golden shows that fetch carrying `Cache{l1:true l2:true ttl:1m scope:Entity type:Product candidates:1}` with `providesData:false` (nil until A1b).

Verification:

- `cd v2 && go build ./pkg/...` — clean.
- `cd v2 && go test ./pkg/engine/postprocess/... ./pkg/engine/plan/... ./pkg/engine/resolve/... -count=1` — PASS; the StageNoop planner no-op golden is byte-identical.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (StageNoop unchanged + new StageL2Entities golden + the existing seam rows).
- `cd v2 && go vet ./pkg/engine/postprocess/... ./pkg/engine/resolve/...` — clean.

## Reviewer guidance

- No federation pointer escapes the freezer (one-file review of `cache_key_spec_freezer.go`; the mutation test pins it).
- The stamper stamps the right concrete types after `createConcreteSingleFetchTypes`; nil where no policy.
- The StageNoop no-op golden is unchanged (no provider -> byte-identical).
- `ProvidesData` is intentionally nil this commit (A1b ports the P1 visitor); root-field stamping and `EntityKeyMappings` are TODO (B1/C1).
