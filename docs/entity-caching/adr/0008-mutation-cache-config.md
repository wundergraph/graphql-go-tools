# ADR-0008: Mutation cache config caching support

## Status

Proposed

## Context

The foundation ([ADR-0001](0001-foundation.md)) establishes the entity-caching seam: a thin `entityCache` collaborator on the loader,
the `LoaderCache` (L2) interface, the `CacheKeyTemplate` key seam, the `ProvidesData *Object` alias-aware field shape,
the per-fetch `cacheSkipFetch` / `cacheMustBeUpdated` booleans, and the StructuralCopy isolation discipline.
Entity and root-field config (ADR-0006, ADR-0007) sit on top: they bind types and root fields to a named cache, a TTL, and a key template,
and drive the query-path read/write of L1 and L2.

Mutations need different cache behavior than queries, and the existing seam does not express it.
The problems this configuration concept solves:

1. **Mutations must never serve stale reads.**
A mutation observes or changes state, so reading the answer from L2 would be a correctness bug.
The rule is unconditional: mutations always skip L2 *reads* and always fetch fresh from the subgraph.

2. **Mutation-triggered entity fetches must not silently pollute the cache.**
A mutation like `updateUser` typically resolves the root mutation field, then issues a follow-up entity fetch (`_entities`) to fill the rest of the selection.
That entity fetch is an ordinary cacheable entity fetch, but writing its result to L2 by default is dangerous,
because the mutation may have left the entity in a transient or partially-committed shape.
So mutation-triggered L2 *writes* must be **off by default** and **opt-in per mutation field**,
with a TTL that can differ from the entity's default (the `@cachePopulate(maxAge:)` use case).

3. **A successful mutation must be able to evict what it changed.**
After `updateUser` succeeds, the L2 entry for that `User` is now stale and must be deleted so the next query re-fetches.
The engine must detect which entity (or list of entities) the mutation returned, build the exact same cache key the read path would build, and delete it.

None of these are wire directives.
They arrive as Go configuration, supplied per subgraph through `SubgraphCachingConfig`:
`MutationFieldCaching{FieldName, EnableEntityL2CachePopulation, TTL}` controls opt-in population,
and `MutationCacheInvalidationConfiguration` controls post-success eviction.
The full contract — config-struct fields, the populate gate, the invalidation matching rules, list handling, and analytics —
lives in [../directives/mutation-cache-config.md](../directives/mutation-cache-config.md).

## Decision

Mutation cache config is implemented as **plan-time fetch annotations plus existing resolver hooks**, with **no new loader hot-path code**.
It reuses the five seams of ADR-0001 rather than adding a sixth.

### How it plugs into the foundation seam

**Plan-time metadata (new, additive).**
The planner attaches two pieces of configuration to the relevant fetches inside the existing `FetchCacheConfiguration`:

- On the **root mutation fetch**, two fields gate population:
  `EnableMutationL2CachePopulation bool` and `MutationCacheTTLOverride time.Duration`.
  These come from `MutationFieldCaching{EnableEntityL2CachePopulation, TTL}` for the matching `FieldName`.
- On the **mutation fetch that returns an entity** (or on the root fetch for single-subgraph mutations),
  a `*MutationEntityImpactConfig` describes what to evict or populate after success.
  Its shape: `{EntityTypeName, KeyFields []KeyField, CacheName, InvalidateCache bool, PopulateCache bool, PopulateTTL, IncludeSubgraphHeaderPrefix}`.

`MutationEntityImpactConfig` is a plain data struct, not a `plan` type, so the resolver does not pull a `plan` dependency in — consistent with the `EntityCacheInvalidationConfig` split in ADR-0001 §7.4.

**Resolver hooks (existing, not new).**
Mutation behavior is expressed entirely through the foundation's existing seams:

- **Read-skip** rides the *operation type*, not a cache hook.
  When `FetchInfo.OperationType == ast.OperationTypeMutation`, the L2 read path is simply not entered, so no `bulkL2Lookup` / `tryL2CacheLoad` Get is issued.
  This is the unconditional "mutations skip L2 reads" rule, and it costs nothing because it is a guard, not a new branch in the merge funnel.
- **Write-gate** rides the existing post-fetch write seam (`updateL2Cache`).
  The loader carries a per-mutation flag `enableMutationL2CachePopulation`, set when the root mutation fetch's `EnableMutationL2CachePopulation` is true,
  and **inherited by the follow-up entity fetch** (the entity fetch propagates the flag from the root mutation fetch during sequential resolution).
  `updateL2Cache` only writes when this flag is set; when it writes, it uses `MutationCacheTTLOverride` if non-zero, otherwise the entity's default TTL.
  When the flag is false, mutations produce **zero L2 operations** — no get, no set.
- **Invalidation / populate** rides the existing invalidation seam (the fifth seam of ADR-0001), at the same point as extension-based invalidation.
  After `mergeResult` folds the mutation response into the response tree, `detectMutationEntityImpact(result, fetchInfo, responseData)` runs:
  1. It returns `nil` for non-mutation operations, nil `FetchInfo`, missing `MutationEntityImpactConfig`, nil `ProvidesData`, missing `caches`, or a non-object/non-array entity payload — pure guard clauses that keep the disabled path free.
  2. It navigates `ProvidesData` to the entity object under the mutation root field (`navigateProvidesDataToField`), then builds the entity cache key from the response data and `KeyFields` (`buildEntityKeyValue` → `buildMutationEntityCacheKey`).
  3. The key passes through the **same transform pipeline** as every other key — `IncludeSubgraphHeaderPrefix` header-hash prefix, then `GlobalCacheKeyPrefix`, then `L2CacheKeyInterceptor` — so the deleted/written key is byte-identical to the read-path key. This is the load-bearing footgun ADR-0001 calls out; mutation invalidation must reproduce it exactly or it targets the wrong entry.
  4. If `InvalidateCache` is set, it deletes the key(s) and returns the deleted-key set.
  5. If `PopulateCache` is set (the single-subgraph case with no follow-up entity fetch to inherit the population flag), it projects the entity through `ProvidesData` (`structuralCopyNormalized`) and writes it to L2 — but only when `EnableL2Cache` is on.
  6. **Lists** are handled by iterating array items, building one key per object item and skipping non-object items, so a `deleteUsers: [User]` mutation evicts every returned entity.

**No loader hot-path changes.**
The walker dispatch, the four-phase parallel machinery, and `mergeResult`'s signature are untouched.
Mutation read-skip is a guard on operation type, the write-gate is one boolean checked inside the already-existing `updateL2Cache`,
and impact detection hangs off the already-existing post-merge invalidation seam.
All mutation-specific logic lives in the `entityCache` collaborator files, not inline in the resolution loop.

### Analytics

When `EnableCacheAnalytics` is on, `detectMutationEntityImpact` records one `MutationEvent` per impacted entity
(`MutationRootField`, `EntityType`, `EntityCacheKey` as the display key without the global prefix, `HadCachedValue`, `IsStale`, hashes, byte sizes).
Critically, analytics **does not issue a mutation-time cache read** to compute staleness — the read path is avoided even with analytics on,
so `HadCachedValue` is reported as false and `CachedHash`/`CachedBytes` are zero.
Records are emitted whether or not `InvalidateCache` is set (recording-without-deleting is a valid analytics-only mode).

### How the PR stacks

This work ships as **gqtools PR 13 / PR-CACHE-INVALIDATION**, stacked on the foundation PR and on the entity + root-field config PRs (ADR-0006, ADR-0007).
It depends on them because mutation invalidation builds entity-shaped keys (needs `@key` and the entity key template)
and mutation population writes through the same L2 projection (needs `ProvidesData` and the entity cache config).
Because it adds only plan-time annotations and reuses existing hooks, the loader diff is mechanical and the PR is independently reviewable against the now-frozen seam.

## Consequences

### Positive

- **Correctness by default.** Mutations never serve stale reads and never write to L2 unless explicitly opted in, so adding the feature cannot silently corrupt a query-side cache.
- **Self-cleaning cache.** A mutation can evict exactly the entities it touched, including whole lists, keeping downstream queries fresh without a manual invalidation API.
- **Zero new hot-path surface.** Read-skip is a guard on operation type, the write-gate is one boolean inside `updateL2Cache`, and impact detection reuses the post-merge invalidation seam — the loader stays as in ADR-0001.
- **TTL flexibility.** `MutationCacheTTLOverride` lets mutation-triggered writes use a shorter lifetime than the entity default, supporting the `@cachePopulate(maxAge:)` pattern.

### Negative / costs

- **Key-pipeline duplication risk.** Mutation invalidation must reproduce the full key-transform pipeline (header prefix → global prefix → interceptor); any drift between the read path and the invalidation path leaks stale entries. This is mitigated by routing both through the same key-building helpers, but it remains a place where the two must stay in lockstep.
- **Population is post-success only.** L2 population for mutations happens after the mutation returns; a mutation that errors writes nothing, which is correct but means there is no speculative caching.
- **Flag inheritance coupling.** The follow-up entity fetch inherits `enableMutationL2CachePopulation` from the root mutation fetch via sequential propagation; this implicit inheritance must be preserved or mutation-triggered entity writes silently stop.

### Performance implications

- A mutation with population disabled (the default) issues **zero** L2 operations, so the cache layer is effectively free on the mutation path.
- Invalidation is at most one `Delete` batch per mutation (one key, or one per list element); population is at most one `Set`. There is never a mutation-time `Get`.
- Analytics deliberately skips the staleness read, so enabling analytics on the mutation path does not add a cache round-trip.

### What becomes possible for later directives

- The post-merge impact-detection seam this directive uses is the same one **subscription population** (ADR-0009) hangs its per-event populate/invalidate logic off, so subscriptions reuse this pattern rather than inventing a new one.
- Extension-based invalidation (subgraph-supplied `cacheInvalidation` keys) and mutation invalidation share the delete path and the de-duplication optimization (skip a delete when the same key is being written), so both can coexist on one fetch.

## Alternatives considered

### A. Make mutation L2 population opt-out (write by default, suppress per field)

Treat mutation-triggered entity fetches like any query fetch and write to L2 unless a field opts out.
**Rejected.**
A mutation can leave an entity in a transient state, and a wrong default that writes that state to a cross-request cache is a silent correctness bug that is hard to detect.
Opt-in is the safe default; the cost is one explicit config field per mutation that wants population.

### B. Add a dedicated mutation hook (a sixth loader seam)

Introduce a `tryMutationCacheEffects` hook in the loader's hot path, separate from the existing invalidation seam.
**Rejected.**
It violates the ADR-0001 constraint of keeping the loader at five seams.
Mutation effects are post-merge cache writes/deletes, which is exactly what the existing invalidation seam already does for extension-based invalidation; reusing it keeps the loader diff minimal and the disabled path unchanged.

### C. Let the router invalidate manually after a mutation

Expose mutation results to the router and have the router call `Delete` itself.
**Rejected.**
The router cannot reliably reconstruct the entity cache key — it would have to reproduce the entity key template, the header-hash prefix, the global prefix, and the interceptor in exactly the engine's byte order.
That is the documented manual-invalidation footgun; pushing it onto the router for the common mutation case multiplies the chance of targeting the wrong key and leaving stale entries.
The engine owns key construction, so the engine owns mutation invalidation.

### D. Always skip both L2 reads and writes for mutations (no population at all)

Make mutations purely cache-invalidating with no ability to populate.
**Rejected.**
Some mutations (single-subgraph `@cachePopulate`) usefully prime the cache with their just-written entity so the immediate follow-up read hits.
A blanket no-write rule would forbid that, and the opt-in `EnableEntityL2CachePopulation` plus `PopulateCache` path supports it without compromising the safe default.

## References

- [ADR-0001: Foundation](0001-foundation.md) — the integration seam, L1/L2 model, key-transform pipeline, StructuralCopy invariants.
- [../directives/mutation-cache-config.md](../directives/mutation-cache-config.md) — the detailed mutation cache config contract.
- [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) — directive taxonomy and PR mapping.
- `v2/pkg/engine/resolve/mutation_cache_test.go` — `detectMutationEntityImpact`, `buildMutationEntityCacheKey`, `MutationCacheTTLOverride`, list-invalidation, and population behavior.
- `v2/pkg/engine/resolve/extensions_cache_invalidation_test.go` — shared delete path and mutation-event recording for extension-driven invalidation.
- `docs/entity-caching/ENTITY_CACHING_INTEGRATION.md` — router-facing config (`MutationFieldCacheConfiguration`, `MutationCacheInvalidationConfiguration`).
