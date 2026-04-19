# Entity Caching Acceptance Criteria

Two-level entity caching system for GraphQL federation: L1 (per-request, in-memory) eliminates
redundant entity fetches within a single request; L2 (cross-request, external) shares cached
entities across requests via external stores like Redis.

## L1 Cache (Per-Request, In-Memory)

### AC-L1-01: Request-scoped isolation
Each GraphQL request gets its own L1 cache instance (a fresh `sync.Map` on the Loader).
No data leaks between requests. The cache is discarded when the request completes.

Tests:
- `v2/pkg/engine/resolve/l1_cache_test.go:24` — `TestL1Cache / "L1 hit - same entity fetched twice in same request"`

### AC-L1-02: Entity fetches only
L1 caches entity fetch results (fetches with `@key`-based representations), not root field
query results. Root fields never _read_ from L1 — they use L2 for cross-request caching.
However, root fields that return entities can _populate_ L1 (see AC-L1-08), so that a
subsequent entity fetch within the same request can hit L1.

Tests:
- `execution/engine/federation_caching_l1_test.go:56` — `TestL1CacheReducesHTTPCalls / "L1 enabled - entity fetches use L1 cache"`

### AC-L1-03: Cache keys use only @key fields
L1 cache keys are derived exclusively from the entity's `@key` directive fields
(see AC-KEY-01 for canonical format). `@requires` fields are never included because
they vary per consuming subgraph and would fragment the cache.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:632` — `TestCachingRenderEntityQueryCacheKeyTemplate / "single entity with typename and id"`

### AC-L1-04: Main-thread L1 check; full hit skips goroutine
L1 lookup happens in Phase 1 (`prepareCacheKeys` + `tryL1CacheLoad`) on the main thread,
before any goroutine is spawned. When every entity in a fetch batch is found in L1, the
fetch sets `cacheSkipFetch=true` and no goroutine is spawned for that fetch. The cached
values are used directly, saving both the goroutine allocation and the network call.

Tests:
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go:899` — `TestL1CacheSkipsParallelFetch`
- `execution/engine/federation_caching_l1_test.go:449` — `TestL1CacheSelfReferentialEntity / "L1 enabled - sameUserReviewers fetch entirely skipped via L1 cache"`

### AC-L1-06: Disabled by default
L1 caching must be explicitly enabled per-request via
`ctx.ExecutionOptions.Caching.EnableL1Cache = true`. When disabled, every entity fetch
goes through the normal L2/subgraph path.

Tests:
- `execution/engine/federation_caching_l1_test.go:93` — `TestL1CacheReducesHTTPCalls / "L1 disabled - more accounts calls without cache"`

### AC-L1-07: StructuralCopy on L1 read and write
Every L1 cache write StructuralCopies the value onto `l.jsonArena`.
Entity L1 uses `structuralCopyNormalizedPassthrough` — renames aliases
to schema names via an ephemeral `astjson.Transform` while keeping ALL
source fields (including @key fields not in ProvidesData) via
`Transform.Passthrough`.
This preserves field accumulation across fetches: fetch 1 stores `{name}`,
fetch 2 merges `{email}`, L1 has `{name, email}` for fetch 3.

Every L1 cache read uses `structuralCopyDenormalizedPassthrough` —
restores aliases while preserving all accumulated fields.
StructuralCopy clones container nodes on the arena while aliasing leaf
nodes from the source.
This gives the consumer a structurally independent value and prevents
pointer aliasing during JSON merge for self-referential entities.
Strings are always eagerly decoded (no lazy mutation), making aliased
leaf values safe for concurrent reads.

L2 writes use non-passthrough `structuralCopyNormalized` which projects
to ProvidesData fields only (rename + drop unlisted fields).

Merges into an existing L1 entry use the working-copy-and-swap pattern:
StructuralCopy the existing entry into a working copy,
run `astjson.MergeValues` against the working copy,
and store either the working copy (on success) or the fresh incoming
value (on merge failure).
The live cache entry pointer is never mutated in place,
so a partial `MergeValues` failure cannot corrupt sibling L1 keys
pointing at the same entry.

Tests:
- `execution/engine/federation_caching_l1_test.go:344` — `TestL1CacheSelfReferentialEntity`
- `v2/pkg/engine/resolve/loader_cache_phase2_test.go:21` — `TestL1Cache_RootFieldPromotionWithAliases` (alias-aware StructuralCopy on root-field promotion)
- `v2/pkg/engine/resolve/loader_cache_phase2_test.go:147` — `TestExportRequestScopedFields_MergeWorkingCopyOnFailure` (working-copy-and-swap on merge failure)
- `v2/pkg/engine/resolve/loader_cache_transform_test.go` — `TestStructuralCopyNormalized_*` (alias/arg-suffix normalize + denormalize)
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go` — `TestL1CacheFieldAccumulation` (3-fetch field accumulation with passthrough)

### AC-L1-09: Union-based L1 optimization
The postprocessor (`optimize_l1_cache.go`) computes the **union** of all
ancestor providers' ProvidesData fields when deciding whether to enable
L1 for a fetch.
If no single provider covers the consumer's field needs,
the union of all prior providers (same entity type, in dependency chain)
is checked.
This enables L1 for fetches whose required fields are spread across
multiple prior fetches.
A fetch is enabled as a writer if it contributes to a union that covers
any descendant consumer.

Tests:
- `v2/pkg/engine/postprocess/optimize_l1_cache_test.go` — `TestOptimizeL1Cache_Union_*` (9 tests: basic, insufficient, overlapping, 4-fetch chain, etc.)
- `execution/engine/federation_caching_l1_test.go` — `TestL1CacheEntityUnionOptimization` (6 E2E subtests using CacheEntity type)

### AC-L1-08: Root field entity population
When a root field query (e.g., `topProducts`) returns entities, those entities are
extracted and stored in L1 using their `@key`-based cache keys. This means a subsequent
entity fetch for the same entity within the same request can hit L1 instead of making
another subgraph call. Requires `RootFieldL1EntityCacheKeyTemplates` to be configured.

If the client's query doesn't select the `@key` fields (e.g., omits `id`), the cache key
is produced with an empty key object (`{"__typename":"Product","key":{}}`) and the entity
is silently stored under this degraded key. It will never match a real entity fetch, so the
behavior is benign but wasteful.

When the root field is aliased (e.g., `myUser: user(id: $id)`), the entity cache key
template path uses the alias (`myUser`), not the schema field name (`user`), because
the response JSON is keyed by the alias.

Tests:
- `execution/engine/federation_caching_l1_test.go:667` — `TestL1CacheRootFieldEntityListPopulation`
- `v2/pkg/engine/resolve/l1_cache_test.go:1813` — `TestPopulateL1CacheForRootFieldEntities_MissingKeyFields`
- `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_entity_key_mapping_test.go:871` — `aliased root fields use alias in entity cache key path` (verifies alias-based path in `RootFieldL1EntityCacheKeyTemplates`)

### AC-L1-09: Argument-variant coexistence via field merging
When the same entity is fetched with different field arguments (e.g., `friends(first:5)`
and `friends(first:20)`), each variant gets a unique suffixed field name
(e.g., `friends_<hash1>`, `friends_<hash2>`). When a second fetch for the same entity
arrives, L1 merges the new fields into the existing cached entity using first-writer-wins
semantics, so all arg variants coexist in a single cached entity.

L2 also performs arg-variant merging during `updateL2Cache`: before writing a new entity,
existing cached fields from other arg variants are merged in via `MergeValues` so they
are not lost (see AC-L2-08).

Tests:
- `execution/engine/federation_caching_entity_field_args_test.go:129` — `TestEntityFieldArgsCaching`
- `v2/pkg/engine/resolve/l1_cache_test.go:2609` — `TestMergeEntityFields` (6 subtests: new field added, existing preserved, nil dst/src, non-object, multiple fields coexist)

## L1/L2 Interaction Ordering

### AC-L1L2-01: L1 checked before L2; L1 hit skips L2 entirely
Within a single request, L1 is always checked first (Phase 1, main thread). When L1 has
a hit, L2 is never consulted and no subgraph call is made. This holds regardless of L2
TTL state — even if the L2 entry is expired, stale, or missing, an L1 hit is authoritative.

L1 is always fresh within a request because it is populated from the current request's own
subgraph fetches (or root field entity extraction), not from L2. L1 and L2 are independent
caches with different scopes:
- L1: per-request, in-memory, populated by fetches within the current request
- L2: cross-request, external, populated after successful subgraph calls

Tests:
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go:496` — `TestL1L2CacheEndToEnd / "L1+L2 - L1 hit prevents L2 lookup"` (two sequential entity fetches: first populates L1+L2, second hits L1 with zero L2 operations)
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go:605` — `TestL1L2CacheEndToEnd / "L1+L2 - L1 miss, L2 hit provides data"` (L1 miss falls through to L2)
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go:698` — `TestL1L2CacheEndToEnd / "L1+L2 - cross-request: L1 isolated, L2 shared"` (new request has empty L1, uses L2)
- `v2/pkg/engine/resolve/l1_l2_cache_e2e_test.go:899` — `TestL1CacheSkipsParallelFetch` (L1 hit prevents goroutine spawn for parallel fetch)

## L2 Cache (Cross-Request, External)

### AC-L2-01: External cache via LoaderCache interface
L2 caching delegates to user-provided implementations of the `LoaderCache` interface
(`Get`/`Set`/`Delete`). Typical backends: Redis, Memcached. Multiple named cache instances
are supported (e.g., different Redis clusters for different entity types).

Tests:
- `execution/engine/federation_caching_l2_test.go:20` — `TestL2CacheOnly / "L2 enabled - miss then hit across requests"`

### AC-L2-02: L2 reads use main-thread bulk Get; HTTP runs in goroutines
Within `resolveParallel`, L2 cache reads are issued by `bulkL2Lookup` on the main
thread: one bulk `cache.Get` per cache instance, covering every fetch in the batch
that routes to that instance. Parsed values are materialized on `l.parser` /
`l.jsonArena` and distributed back to each fetch's `l2CacheKeys[].FromCache`.
Only the fallback subgraph HTTP calls run in parallel goroutines (Phase 2HTTP);
those goroutines do HTTP only and do not touch the arena or cache.

Because a single bulk Get now covers the whole batch, **a bulk Get failure causes
every fetch in the batch to fall back to the subgraph** (documented behavior change
from the old per-fetch isolation). Each affected fetch is marked
`cacheMustBeUpdated`, its `cacheTraceL2GetError` is set, and a
`CacheOperationError` is recorded per fetch in `l2CacheOpErrors`.

`LoaderCache` implementations still must be safe for concurrent access because
`Set` / `Delete` operations (write-side) continue to run from Phase 4 and may
overlap across concurrent router requests.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:828` — `TestCacheLoadSequential / "two sequential calls - miss then hit"`

### AC-L2-03: Configurable TTL per entity type
Each entity type (or root field) can have its own TTL configured via
`EntityCacheConfiguration.TTL`. The TTL is passed to `LoaderCache.Set()`. If the cache
backend supports TTL introspection, it returns `RemainingTTL` on `Get` for analytics.

Tests:
- `execution/engine/federation_caching_test.go:1386` — `TestFederationCaching / "TTL expiry"`

### AC-L2-04: L2 keys follow canonical format with optional prefix
L2 cache keys use the canonical entity key format (see AC-KEY-01) or root field key
format (see AC-KEY-02), with an optional header hash prefix (AC-KEY-03) and optional
global prefix (AC-KEY-07) prepended for cache isolation.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:632` — `TestCachingRenderEntityQueryCacheKeyTemplate`
- `v2/pkg/engine/resolve/cache_key_test.go:13` — `TestCachingRenderRootQueryCacheKeyTemplate`

### AC-L2-05: Disabled by default
L2 caching must be explicitly enabled per-request via
`ctx.ExecutionOptions.Caching.EnableL2Cache = true` AND configured per-subgraph with
entity/root field cache configurations.

Tests:
- `execution/engine/federation_caching_l2_test.go:191` — `TestL2CacheOnly / "L2 disabled - no external cache operations"`

### AC-L2-06: Normalization before storage
Before writing to L2, field names are normalized: aliases are replaced with original
schema field names, and fields with arguments get an xxhash suffix appended.
This ensures cached data is query-independent and can be reused across different
GraphQL operations that request the same entity.

Normalization uses ephemeral `astjson.Transform` descriptors built inline via
`structuralCopyNormalized(value, providesData)`.
The Transform walks `FetchInfo.ProvidesData` and emits one `TransformEntry` per
aliased or arg-suffixed field.
Transforms are built into reusable `l.transformEntries` / `l.transforms` slabs
(resliced to [:0] before each use) and consumed by
`l.parser.StructuralCopyWithTransform` — no stored transforms on `result`.

L2 writes use non-passthrough normalization (projects to ProvidesData fields only).
L1 writes use passthrough normalization (renames aliases but keeps all fields).
L2 reads stay verbatim at parse time; denormalization is applied at the
materialization site via `structuralCopyDenormalized` so the writeback merge
in `updateL2Cache` can preserve fields outside the current selection (see AC-L2-08).

Tests:
- `v2/pkg/engine/resolve/loader_cache_transform_test.go` — `TestStructuralCopyNormalized_*` (7 tests: nil, alias, nested, array, arg-suffix, request-scoped invariant, mixed)
- `execution/engine/federation_caching_entity_field_args_test.go` — `TestEntityFieldArgsCaching` (E2E arg-hash normalization)
- `v2/pkg/engine/resolve/loader_cache_transform_test.go:174` — `TestBuildNormalizeTransform_MixedAliases`
- `v2/pkg/engine/resolve/loader_cache_phase2_test.go:125` — `TestL2WritePreservesFieldsOutsideSelection` (verbatim parse preserves fields outside selection for writeback merge)

### AC-L2-07: Validation before serving cached data
When reading from L2, the cached entity is validated against the `ProvidesData` schema
(the set of fields the current fetch expects). Every required field must be present; if
any are missing, the cached entry is treated as a miss and the entity is refetched from
the subgraph.

Tests:
- `execution/engine/federation_caching_l2_test.go:504` — `TestPartialEntityCaching / "only configured entities are cached"`
- `v2/pkg/engine/resolve/l1_cache_test.go:2159` — `TestValidateItemHasRequiredData` (22 subtests: nil, scalars, nullable/non-nullable, nested objects, arrays, CacheArgs suffixed lookup, empty arrays)
- `v2/pkg/engine/resolve/l1_cache_test.go:1953` — `TestValidateFieldDataWithAliases` (validates using original name on normalized cache data)

### AC-L2-08: Failed validation preserves old entity for field merging
When L2 validation fails (cached entity is missing fields the current query needs), the
old cached entity is preserved in `FromCache`. After the subgraph returns fresh data, the
old and new entities are merged so that previously-cached fields from other arg variants
are not lost. The merged result is then written back to L2.

Enforced by the verbatim-parse rule in `bulkL2Lookup`: cached entries are parsed without
applying the denormalize Transform at parse time, so `l2CacheKeys[i].FromCache` retains
every field that was in the cached value even if the current query selects a narrower
set. The denormalize Transform is applied only at the L2-to-response materialization
site for `l1CacheKeys[i].FromCache`, leaving `l2CacheKeys[i].FromCache` in cache-shape
for the writeback merge in `updateL2Cache`.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:605` — `TestCacheLoadSequential / "single entity fetch with cache miss"`
- `v2/pkg/engine/resolve/loader_cache_phase2_test.go:125` — `TestL2WritePreservesFieldsOutsideSelection` (writeback merge preserves fields outside current selection)

## Negative Caching

### AC-NEG-01: Null entity responses cached as negative sentinels
When a subgraph returns `null` for an entity in `_entities` (entity not found, no errors),
and `NegativeCacheTTL > 0` is configured for that entity type, the null result is stored in
L2 as a sentinel value (`"null"` bytes). On subsequent requests, the sentinel is recognized
as a negative cache hit and served without calling the subgraph.

This prevents repeated subgraph lookups for non-existent entities (e.g., a deleted product
that is still referenced by other entities).

Tests:
- `v2/pkg/engine/resolve/negative_cache_test.go:60` — `TestNegativeCaching / "null entity stored as negative sentinel and served on second request"`

### AC-NEG-02: Disabled by default (NegativeCacheTTL = 0)
When `NegativeCacheTTL` is 0 (default), null entity responses are NOT cached. Each request
re-fetches from the subgraph, preserving the pre-negative-caching behavior.

Tests:
- `v2/pkg/engine/resolve/negative_cache_test.go:229` — `TestNegativeCaching / "negative caching disabled when NegativeCacheTTL is 0"` (subgraph called twice, no sentinel stored)

### AC-NEG-03: Separate TTL for negative sentinels
Negative cache entries use `NegativeCacheTTL` (not the regular entity `TTL`) when calling
`LoaderCache.Set()`. This allows shorter TTLs for negative entries (e.g., 5s) compared to
regular entity data (e.g., 60s), so deleted entities are re-checked sooner.

Tests:
- `v2/pkg/engine/resolve/negative_cache_test.go:353` — `TestNegativeCaching / "negative cache sentinel uses NegativeCacheTTL not regular TTL"`

### AC-NEG-04: Per-entity-type opt-in
Negative caching is configured per entity type via `EntityCacheConfiguration.NegativeCacheTTL`.
Different entity types can have different negative cache TTLs, or have it disabled entirely
(TTL = 0).

## Cache Key Construction

### AC-KEY-01: Entity key format
Entity cache keys use the canonical format `{"__typename":"T","key":{...}}` where the
key object contains only the fields declared in the entity's `@key` directive. Composite
keys (multiple fields) and nested keys are supported.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:632` — `TestCachingRenderEntityQueryCacheKeyTemplate`
- `v2/pkg/engine/resolve/cache_key_test.go:1125` — `TestDerivedEntityCacheKey / "dot-notation entity key field"` (single-level nesting)
- `v2/pkg/engine/resolve/cache_key_test.go:1148` — `TestDerivedEntityCacheKey / "deeply nested dot-notation entity key field"` (multi-level nesting)
- `v2/pkg/engine/resolve/cache_key_test.go:1171` — `TestDerivedEntityCacheKey / "dot-notation shared prefix merges into same object"` (shared-prefix merge)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "flat key + composite key - all args present"` (flat + composite multi-key)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "flat key + nested composite key - all args present"` (flat + nested multi-key)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "nested composite key - structured argument input"` (structured input arg)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "two nested composite keys with structured args - both resolve"` (two nested keys)

### AC-KEY-02: Root field key format
Root field cache keys use `{"__typename":"Query","field":"fieldName","args":{...}}`.
Arguments are included when present. Root field keys can optionally map to entity keys
via `EntityKeyMappings` so that a root field query and an entity query share the same
cache entry.

When `EntityKeyMappings` is configured with multiple mappings, the system generates one
cache key per mapping whose arguments are all available. Mappings with missing arguments
are skipped — only the mappings where every argument resolves produce a key. This means
a root field with partial argument coverage generates fewer keys than one with full
coverage on the read path.

On the write path, the system uses smart cache key backfill (see AC-L2-BACKFILL section)
to make precise per-key write decisions based on final entity data. Requested missing keys
are backfilled when the final entity value proves them, and additional derived keys are
written when the entity data contains the mapped key fields.

Variable remapping (`ctx.RemapVariables`) applies to single-element argument paths only.
Multi-element paths (structured argument inputs like `["store", "id"]`) are not remapped.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:13` — `TestCachingRenderRootQueryCacheKeyTemplate`
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "flat key + composite key - only composite args present"` (partial arg coverage skips flat key)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "flat key + nested composite key - only nested args present"` (partial with nested keys)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "flat key + nested composite key with structured arg - only nested resolves"` (structured arg partial)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "two nested composite keys with structured args - only first resolves"` (two nested, one skipped)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "remap variables - flat key remapped"` (RemapVariables with entity key mapping)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "remap variables - multiple mappings only flat keys remapped"` (remap with multi-key)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "remap variables - structured arg path not remapped"` (multi-element path not remapped)
- `v2/pkg/engine/resolve/cache_key_test.go` — `TestDerivedEntityCacheKey / "remap variables - partial remap with multi-key"` (partial remap across mappings)
- `execution/engine/federation_caching_test.go` — `TestRootFieldCachingWithArgs / "entity key mapping - two root fields asymmetric key coverage"` (E2E: full-key write, partial-key read cross-lookup)
- `execution/engine/federation_caching_test.go` — `TestRootFieldCachingWithArgs_PartialKeyWrite / "entity key mapping - partial key write does not generate extra keys from response"` (E2E: partial-arg write backfills derived keys from response with Peek verification)
- `execution/engine/federation_caching_test.go` — `TestRootFieldCachingWithArgs_PartialKeyWrite / "entity key mapping - flat key cross-lookup from composite key write"` (E2E: flat key cross-lookup from composite write)

### AC-KEY-03: Subgraph header hash prefix
When `IncludeSubgraphHeaderPrefix` is enabled, the L2 cache key is prefixed with a hash
of the forwarded subgraph headers (e.g., auth tokens). Format: `{hash}:{json_key}`. This
ensures different auth contexts get separate cache entries, preventing data leakage
between tenants or users.

Tests:
- `execution/engine/federation_caching_test.go:418` — `TestFederationCaching / "two subgraphs - with subgraph header prefix"`

### AC-KEY-04: L2CacheKeyInterceptor transform
After the header prefix is applied, the key passes through an optional user-provided
`L2CacheKeyInterceptor` function. This allows custom transformations like adding tenant
prefixes or routing to different cache namespaces. The interceptor receives the subgraph
name and cache name as context.

Tests:
- `v2/pkg/engine/resolve/l2_cache_key_interceptor_test.go:80` — `TestL2CacheKeyInterceptor`

### AC-KEY-05: Field argument suffix for entity fields
When an entity field has arguments (e.g., `friends(first:5)`), the _field name in the
cached entity data_ gets an `_<16-hex-digit-xxhash>` suffix computed from the sorted,
canonicalized argument values. This ensures `friends(first:5)` and `friends(first:20)`
produce different field names _within_ the cached entity and don't overwrite each other.

Note: the suffix applies to field names in the stored JSON, not to the entity's L1 or L2
cache key. Cache keys are always derived from `@key` fields only (see AC-KEY-01).
Both L1 and L2 use the `cacheFieldName()` function to apply these suffixes during
normalization before storage and during denormalization on read.

Tests:
- `v2/pkg/engine/resolve/l1_cache_test.go:2502` — `TestComputeArgSuffix` (8 subtests: deterministic suffix, different values, null handling, sorted args, RemapVariables, object arg canonical JSON)

### AC-KEY-06: Canonical JSON for deterministic hashing
Argument values are serialized as canonical JSON (object keys sorted alphabetically,
arrays in order, scalars as-is) before hashing. This guarantees the same logical arguments
always produce the same hash, regardless of the JSON key order sent by the client.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:1979` — `TestWriteCanonicalJSON`

### AC-KEY-07: Global cache key prefix for schema versioning
When `CachingOptions.GlobalCacheKeyPrefix` is set, the prefix is prepended to all L2 cache
keys (both entity and root field). Format: `{prefix}:{rest_of_key}`. This allows the
router to separate cache entries by schema version — when the schema changes, a new prefix
automatically invalidates all old cache entries without explicit cache flushing.

The global prefix is applied as the outermost prefix, before the header hash prefix. When
both are active: `{global}:{header_hash}:{json_key}`. When only global prefix:
`{global}:{json_key}`.

The global prefix is applied consistently across all cache operations: L2 reads, L2 writes,
extension-based invalidation, mutation invalidation, and subscription populate/invalidate.

Tests:
- `v2/pkg/engine/resolve/l2_cache_key_interceptor_test.go:504` — `TestL2CacheKeyInterceptor / "global prefix is prepended to L2 keys"`
- `v2/pkg/engine/resolve/l2_cache_key_interceptor_test.go:597` — `TestL2CacheKeyInterceptor / "global prefix combined with interceptor"`

## Partial Cache Loading

### AC-PARTIAL-01: Default behavior (full refetch on any miss)
When `EnablePartialCacheLoad` is false (default), if any entity in a batch has a cache
miss, ALL entities in that batch are refetched from the subgraph. This keeps the cache
maximally fresh because every entity gets a new value on every batch that includes a miss.

Tests:
- `execution/engine/partial_cache_test.go:233` — `TestPartialCacheLoading / "L2 partial cache loading disabled - all entities fetched even with partial cache hit"`

### AC-PARTIAL-02: Partial loading fetches only missing entities
When `EnablePartialCacheLoad` is true, only entities with cache misses are included in the
subgraph fetch request. Cached entities are served directly from cache within their TTL.
The subgraph receives a smaller representations list containing only the missed entities.

Tests:
- `execution/engine/partial_cache_test.go:85` — `TestPartialCacheLoading / "L2 partial cache loading enabled - only missing entities fetched"`

### AC-PARTIAL-03: Freshness vs load tradeoff
Partial loading reduces subgraph load (fewer entities per request) at the cost of
potentially serving slightly stale data for the cached entities. Full refetch (default)
ensures maximum freshness but increases subgraph load.

Tests:
- `v2/pkg/engine/resolve/l1_cache_test.go:555` — `TestL1CachePartialLoading / "partial cache loading with L2 - only missing entities fetched"`

## Mutations and Cache Coherency

### AC-MUT-01: Mutations never read from L2
When the operation type is Mutation, the L2 cache is never consulted for reads. Mutations
always go to the subgraph to ensure they execute against live data. This prevents serving
stale cached data during write operations.

Tests:
- `execution/engine/federation_caching_test.go:2165` — `TestFederationCaching_MutationSkipsL2Read`
- `v2/pkg/engine/resolve/cache_load_test.go:2225` — `TestMutationSkipsL2Read` (unit test: mutation skips L2 read and always fetches from subgraph)

### AC-MUT-02: Mutations skip L2 writes by default
Mutation responses are not written to L2 cache by default. This is because mutation
responses often contain partial entity data that could overwrite a more complete cached
entity.

Tests:
- `execution/engine/federation_caching_test.go:2447` — `TestFederationCaching / "mutation skips L2 write by default without EnableEntityL2CachePopulation"`

### AC-MUT-03: Opt-in mutation L2 population
When `EnableMutationL2CachePopulation` is set to true for a specific mutation field, that
mutation's response IS written to L2. This is useful when a mutation returns a complete,
canonical entity representation that should update the cache.

Tests:
- `execution/engine/federation_caching_l2_test.go:1115` — `TestMutationCacheInvalidationE2E`

### AC-MUT-04: Mutation-triggered L2 invalidation
When `MutationCacheInvalidationConfiguration` is configured for a mutation, and the
mutation response contains an entity with `@key` fields, the corresponding L2 cache entry
is deleted. The cache key is constructed using the same pipeline as storage (typename +
key fields + header prefix + interceptor). Supports both single-entity responses (object)
and list responses (array) — each entity in the array is individually invalidated.

Tests:
- `execution/engine/federation_caching_l2_test.go:1115` — `TestMutationCacheInvalidationE2E`
- `v2/pkg/engine/resolve/mutation_cache_test.go:25` — `TestNavigateProvidesDataToField` (4 subtests: valid field, missing field, nil providesData, non-Object field)
- `v2/pkg/engine/resolve/mutation_cache_test.go:84` — `TestBuildEntityKeyValue` (4 subtests: simple key, composite key, nested key, missing field)
- `v2/pkg/engine/resolve/mutation_cache_test.go:139` — `TestBuildMutationEntityCacheKey` (3 subtests: basic key, with header prefix, with interceptor)
- `v2/pkg/engine/resolve/mutation_cache_test.go:230` — `TestDetectMutationEntityImpact` (includes array response invalidation and non-object item skipping)

### AC-MUT-05: Pre-delete cache read for analytics
When both cache invalidation and analytics are enabled, the cached value is read BEFORE
the delete operation. This allows the analytics system to compare the stale cached value
against the fresh mutation response to measure staleness.

_Known limitation_: `LoaderCache.Delete()` returns only an error, not a success/existence
indicator. The analytics system cannot distinguish "key did not exist" from "key was
successfully deleted". This would require extending the `LoaderCache` interface.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_test.go` — `TestDetectMutationEntityImpact / "analytics enabled, no cached value records MutationEvent with HadCachedValue=false"`

### AC-MUT-06: Staleness detection via hash comparison
Mutation impact analytics computes xxhash of both the cached entity (pre-delete) and the
fresh mutation response (both filtered to `ProvidesData` fields only). If hashes differ,
the entity is marked as stale. This measures how often mutations actually change cached
data.

_Note_: This mechanism (xxhash of `ProvidesData`-filtered fields) is shared with
shadow mode staleness detection (AC-SHADOW-03). The trigger differs (mutation response
vs shadow mode) but the comparison logic is identical.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_test.go` — `TestDetectMutationEntityImpact / "analytics enabled, stale cached value records MutationEvent with IsStale=true"`

### AC-MUT-07: Mutation TTL override
When `MutationFieldCacheConfiguration.TTL` is non-zero, mutation-triggered L2 cache writes
use that TTL instead of the entity's default TTL (from `EntityCacheConfiguration`). When
zero, the entity's default TTL is used. This allows `@cachePopulate(maxAge: 60)` on mutation
fields to override the entity's default cache duration.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_test.go:717` — `TestMutationCacheTTLOverride / "mutation with TTL override uses override value"`
- `v2/pkg/engine/resolve/mutation_cache_test.go:717` — `TestMutationCacheTTLOverride / "mutation without TTL override uses entity default"`
- `v2/pkg/engine/resolve/mutation_cache_test.go:717` — `TestMutationCacheTTLOverride / "TTL override not applied when mutation L2 population disabled"`

## Extension-Based Invalidation

### AC-EXT-01: Subgraph-driven invalidation signals
Subgraphs can include cache invalidation keys in their response extensions:
`{"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`.
The engine processes these keys and deletes the corresponding L2 cache entries.

Tests:
- `execution/engine/federation_caching_ext_invalidation_test.go:14` — `TestFederationCaching_ExtensionsInvalidation / "mutation with extensions invalidation clears L2 cache"`

### AC-EXT-02: Key format matches storage format
Invalidation keys use the same `typename` + `key` structure as stored cache keys, ensuring
the correct entry is targeted for deletion.

Tests:
- `execution/engine/federation_caching_ext_invalidation_test.go:90` — `TestFederationCaching_ExtensionsInvalidation / "multiple entities invalidated in single response"`

### AC-EXT-03: Full key construction pipeline for deletion
The invalidation key goes through the same transformation pipeline as storage keys:
build JSON → apply header hash prefix → apply `L2CacheKeyInterceptor` → call
`cache.Delete()`. This ensures tenant-isolated keys are correctly invalidated.

Tests:
- `execution/engine/federation_caching_ext_invalidation_test.go:214` — `TestFederationCaching_ExtensionsInvalidation / "with subgraph header prefix"`

### AC-EXT-04: Works for queries and mutations
Extension-based invalidation is not restricted to mutation responses. A query response can
also include invalidation keys (e.g., when a subgraph detects data has changed since the
last cache write).

Tests:
- `execution/engine/federation_caching_ext_invalidation_test.go:178` — `TestFederationCaching_ExtensionsInvalidation / "query response triggers invalidation"`

### AC-EXT-05: Skip redundant delete-before-set
If the same entity key appears in both the invalidation keys and the cache write set of
the same fetch, the delete is skipped because the entry will be overwritten with fresh
data anyway. This avoids an unnecessary cache round-trip.

Tests:
- `v2/pkg/engine/resolve/extensions_cache_invalidation_test.go:11` — `TestExtensionsCacheInvalidation`

### AC-EXT-06: Prerequisites for extension invalidation
Extension-based invalidation requires: (1) L2 caching enabled, (2) `entityCacheConfigs`
present for the subgraph (to determine which named cache to delete from and whether header
prefix is needed), and (3) the `caches` map populated.

Tests:
- `execution/engine/federation_caching_ext_invalidation_test.go:121` — `TestFederationCaching_ExtensionsInvalidation / "mutation without extensions does not delete"`

## Subscription Caching

### AC-SUB-01: Populate mode writes entities to L2
In populate mode, each subscription event that returns entity data writes it to the L2
cache. This keeps the cache warm with real-time data, so subsequent queries can serve
the latest state without hitting the subgraph.

Tests:
- `execution/engine/federation_subscription_caching_test.go:330` — `TestFederationSubscriptionCaching / "subscription entity populates L2 - verified via cache"`

### AC-SUB-02: Invalidate mode deletes L2 entries
In invalidate mode, each subscription event triggers L2 cache deletion for the received
entity (identified by `@key` fields). This is used when the subscription delivers only
key fields (not full entity data), signaling that the cached version is stale.

Tests:
- `execution/engine/federation_subscription_caching_test.go:714` — `TestFederationSubscriptionCaching / "key-only subscription invalidates L2 cache"`

### AC-SUB-03: Base key pipeline for subscription cache operations
Subscription cache operations (both populate and invalidate) apply the cache key
pipeline: template rendering → global prefix → header hash prefix → `L2CacheKeyInterceptor`.
The base path (template rendering, populate, invalidate) is covered by existing tests.
Global prefix and `L2CacheKeyInterceptor` integration within subscriptions is verified
by the code path (shared with `prepareCacheKeys`) but not yet exercised by dedicated
trigger-level tests.

Tests:
- `v2/pkg/engine/resolve/trigger_cache_test.go:51` — `TestHandleTriggerEntityCache / "populate single entity"` (verifies base key pipeline for populate)
- `v2/pkg/engine/resolve/trigger_cache_test.go:224` — `TestHandleTriggerEntityCache / "invalidate mode deletes cache entry"` (verifies base key pipeline for invalidate)

### AC-SUB-04: Field-aware subscription config lookup
When multiple subscription fields return the same entity type, the plan visitor uses
`FindByTypeAndFieldName` to match the correct `SubscriptionEntityPopulationConfiguration`.
This prevents order-dependent config selection when subscriptions like `itemCreated` and
`itemUpdated` both produce configs for the same entity type with different TTLs.

Tests:
- `v2/pkg/engine/plan/federation_metadata_test.go` — `TestSubscriptionEntityPopulationConfigurations / "FindByTypeAndFieldName returns field-specific config"`
- `v2/pkg/engine/plan/federation_metadata_test.go` — `TestSubscriptionEntityPopulationConfigurations / "FindByTypeAndFieldName returns nil when field not found"`

## Shadow Mode

### AC-SHADOW-01: Never serves cached data; always fetches from subgraph
When shadow mode is enabled for an entity type, the subgraph is always called regardless
of cache state. L2 cached data is never used in the actual response — the client always
receives fresh data from the subgraph, even on a cache hit.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:1324` — `TestShadowMode_L2_AlwaysFetches`

### AC-SHADOW-02: Cache operations proceed normally
Despite not serving cached data, L2 reads and writes happen as usual. The cache stays
warm and populated. This allows measuring cache effectiveness without affecting
production traffic.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:1504` — `TestShadowMode_StalenessDetection`

### AC-SHADOW-03: Staleness detection via hash comparison
After both cached and fresh values are available, they are compared using xxhash. The
comparison uses only `ProvidesData` fields (the fields the fetch actually needs). Results
are recorded as `ShadowComparisonEvent` with `IsFresh` indicating whether cached data
matched.

_Note_: This mechanism (xxhash of `ProvidesData`-filtered fields) is shared with
mutation staleness detection (AC-MUT-06). The trigger differs (shadow mode vs mutation
response) but the comparison logic is identical.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:1504` — `TestShadowMode_StalenessDetection`

### AC-SHADOW-04: Per-field hash comparison
In addition to the whole-entity comparison (AC-SHADOW-03), shadow mode records individual
xxhash values for each non-key field of the cached entity (tagged as `FieldSourceShadowCached`).
During response rendering, the same fields from fresh subgraph data are hashed (tagged as
`FieldSourceSubgraph`). By comparing per-field hashes across these two sources, consumers
can identify exactly which fields went stale, enabling field-level staleness analysis.

Implementation: `loader_cache.go` iterates `ProvidesData` fields, computing xxhash per
field via `HashFieldValue`. The hashes appear in `CacheAnalyticsSnapshot.FieldHashes`.

Tests:
- `execution/engine/federation_caching_analytics_test.go:679` — `TestCacheAnalyticsE2E / "shadow all entities - always fetches"`
- `v2/pkg/engine/resolve/l1_cache_test.go:2017` — `TestComputeHasAliases` (4 subtests: no aliases, direct alias, nested alias, alias in array item)

### AC-SHADOW-05: L1 cache unaffected
Shadow mode only affects L2 behavior. L1 cache operates normally — it still caches and
serves entities within the same request, since L1 is always fresh (populated from the
current request's fetches).

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:1687` — `TestShadowMode_L1_WorksNormally`

## Thread Safety

### AC-THREAD-01: L1 on main thread with sync.Map
L1 cache reads (`Load`) and writes (`Store`) use `sync.Map` and occur on the main thread
only. The `sync.Map` provides safety for the concurrent `LoadOrStore` pattern used during
root field entity population.

Tests:
- `v2/pkg/engine/resolve/l1_cache_test.go:24` — `TestL1Cache / "L1 hit - same entity fetched twice in same request"`

### AC-THREAD-02: L2 implementations must be goroutine-safe
L2 `LoaderCache.Set()` and `Delete()` (write-side operations) are called from the main
thread during Phase 4 of `resolveParallel` and may overlap across concurrent router
requests. L2 `LoaderCache.Get()` is issued once per cache instance on the main thread
from `bulkL2Lookup` (Phase 2L2), so a single router request never concurrently reads
from the same cache instance — but concurrent router requests can, so `Get` still must
be goroutine-safe. Net requirement: implementers must ensure thread-safe access (e.g.,
connection pooling for Redis).

Tests:
- `execution/engine/federation_caching_test.go:1435` — `TestFederationCaching / "concurrency with different IDs"`

### AC-THREAD-03: Per-result analytics accumulation for write-side events
L2 read events (L2 key events, `L2 Get` fetch timings, cache Get errors) are accumulated
by `bulkL2Lookup` on the main thread in Phase 2L2 and folded directly into the collector.
Write-side and HTTP events — per-fetch `l2AnalyticsEvents`, `l2FetchTimings` for the HTTP
round trip, `l2ErrorEvents`, `l2CacheOpErrors`, and `l2EntitySources` — are accumulated
on the per-result slice either inside the Phase 2HTTP goroutine or during Phase 4 merge.
After `g.Wait()`, the main thread merges the per-result slices into the single analytics
collector via `MergeL2Events` / `MergeL2FetchTimings` / `MergeL2Errors` /
`MergeL2CacheOpErrors` / `MergeEntitySources`.

Tests:
- `v2/pkg/engine/resolve/cache_analytics_test.go:65` — `TestCacheAnalyticsCollector_MergeL2Events`

### AC-THREAD-04: Main-thread parsing on `l.jsonArena` via reusable `l.parser`
The JSON arena (`jsonArena`) uses a `MonotonicArena` which is NOT thread-safe, so all
astjson allocation happens on the main thread. `bulkL2Lookup` parses every L2 cache
entry onto `l.jsonArena` via the Loader-owned `l.parser` (an `astjson.Parser` whose
scratch slabs amortize across requests), and Phase 4 parses every subgraph HTTP response
onto the same arena. Phase 2HTTP goroutines only return a `[]byte` body and never touch
the arena, so there is no goroutine-arena pool, no cross-arena references in the
response tree, and no lifetime coupling between goroutines and response rendering.

The root-field L1 promotion path and entity L1 writes both DeepCopy onto `l.jsonArena`
before storing in `l1Cache`, so the stored `*astjson.Value` is always owned by the
Loader's own arena regardless of what arena the source value came from. This closes
the previous "cross-arena reference" hazard at the storage site rather than at the
goroutine boundary.

Tests:
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:21` — `TestCrossArenaMergeValuesCreatesShallowReferences` (documents the shallow merge semantics that motivate the always-DeepCopy rule)
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:83` — `TestGoroutineArenaLifetimeWithDeferredRelease`
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:137` — `Benchmark_CrossArenaGCSafety`
- `v2/pkg/engine/resolve/arena_thread_safety_bench_test.go:40` — `BenchmarkConcurrentArena`
- `v2/pkg/engine/resolve/arena_thread_safety_bench_test.go:61` — `BenchmarkPerGoroutineArena`
- `v2/pkg/engine/resolve/loader_arena_gc_test.go:102` — `Benchmark_ArenaGCSafety`
- `v2/pkg/engine/resolve/loader_arena_gc_test.go` — `TestLoaderArenaGC` family (verifies main-thread parsing on `l.jsonArena` preserves arena invariants)

## Error Handling

### AC-ERR-01: Cache errors are non-fatal
All cache operations (`Get`, `Set`, `Delete`) are non-fatal. A cache failure never causes
the GraphQL request to fail — the engine falls back to fetching from the subgraph.
When analytics is enabled, cache operation errors are recorded as `CacheOperationError`
events (see AC-ANA-06) so that infrastructure issues are visible to operators.

Tests:
- `execution/engine/federation_caching_l2_test.go:788` — `TestCacheNotPopulatedOnErrors`
- `v2/pkg/engine/resolve/cache_load_test.go:2077` — `TestL2CacheErrorResilience` (Get error falls through to fetch, Set error still returns correct response)

### AC-ERR-02: Subgraph errors prevent cache population
When a subgraph returns an error response, the result is NOT written to L2 cache. This
prevents caching error responses that would be served to subsequent requests.

Tests:
- `execution/engine/federation_caching_l2_test.go:788` — `TestCacheNotPopulatedOnErrors`

### AC-ERR-03: Graceful degradation on validation failure
When L2 returns a cached entity that fails `ProvidesData` validation (missing required
fields), the system gracefully refetches from the subgraph rather than serving incomplete
data. The old cached entity is preserved for field merging (AC-L2-08).

Tests:
- `execution/engine/federation_caching_l2_test.go:504` — `TestPartialEntityCaching / "only configured entities are cached"`

## L2 Circuit Breaker

### AC-CB-01: Configurable per-cache circuit breaker
Each named L2 cache can have a circuit breaker via `ResolverOptions.CacheCircuitBreakers`.
The breaker wraps the `LoaderCache` interface transparently — callers (loader, resolver)
don't need any changes.

Configuration:
- `FailureThreshold`: consecutive failures to trip open (default: 5)
- `CooldownPeriod`: duration in open state before half-open probe (default: 10s)

Tests:
- `v2/pkg/engine/resolve/circuit_breaker_test.go:44` — `TestCircuitBreaker` (7 subtests: stays closed below threshold, opens after N failures, open skips cache, half-open probe success/failure, concurrent safety, success resets count)

### AC-CB-02: Three-state lifecycle
The circuit breaker follows the standard Closed → Open → Half-Open pattern:
- **Closed**: all operations pass through to the underlying cache
- **Open**: `Get` returns `(nil, nil)` (all-miss), `Set`/`Delete` return `nil` (no-op)
- **Half-Open**: after `CooldownPeriod`, the next operation is allowed through as a probe;
  success closes the breaker, failure re-opens it

Tests:
- `v2/pkg/engine/resolve/circuit_breaker_test.go:44` — covers all three states and transitions

### AC-CB-03: Non-blocking failure isolation
When open, the breaker returns immediately without contacting the cache backend. This
prevents cascading failures when the cache is down (e.g., Redis timeout) from affecting
GraphQL request latency. The engine falls back to subgraph fetches transparently.

## Analytics

### AC-ANA-01: Event-level tracking
Every L1 and L2 read/write operation records a structured event containing: cache level
(L1/L2), entity type, cache key, data source name, byte size, and TTL. Events are
collected per-request in the `CacheAnalyticsCollector`.

Tests:
- `execution/engine/federation_caching_analytics_test.go:106` — `TestCacheAnalyticsE2E / "L2 miss then hit with analytics"`

### AC-ANA-02: Fetch timing instrumentation
Each subgraph HTTP call records: request duration, HTTP status code, time-to-first-byte,
and response body size. These timings are available in the snapshot for correlating cache
performance with fetch latency.

Tests:
- `execution/engine/federation_caching_analytics_test.go:505` — `TestCacheAnalyticsE2E / "subgraph fetch records HTTPStatusCode and ResponseBytes"`

### AC-ANA-03: Aggregate convenience methods
The `CacheAnalyticsSnapshot` provides pre-computed metrics: `L1HitRate()`, `L2HitRate()`,
`CachedBytesServed()`, `L1HitCount()`, `L2HitCount()`, `AvgCacheAgeMs()`, etc. These are
derived from the raw events at snapshot time.

Tests:
- `v2/pkg/engine/resolve/cache_analytics_test.go:239` — `TestCacheAnalyticsCollector_SnapshotDerivedMetrics`

### AC-ANA-04: Event deduplication in snapshots
When `Snapshot()` is called, duplicate events (same CacheKey + Kind combination) are
removed to prevent double-counting from retry or re-merge scenarios.

Tests:
- `v2/pkg/engine/resolve/cache_analytics_test.go:1679` — `TestSnapshotDeduplication`

### AC-ANA-05: Header impact analytics
When `IncludeSubgraphHeaderPrefix` is active, the system records `HeaderImpactEvent`s
containing the base key (without header hash) and the response hash. By comparing response
hashes across different header hash values, consumers can detect whether the header prefix
is actually necessary — if all responses are identical regardless of headers, the prefix
adds cache fragmentation without benefit.

Tests:
- `execution/engine/federation_caching_analytics_test.go:1791` — `TestCacheAnalyticsE2E / "shadow mode with header prefix - same response different headers"`
- `v2/pkg/engine/resolve/mutation_cache_test.go` — `TestBuildMutationEntityDisplayKey` (display key always without prefix)

### AC-ANA-06: Cache operation error tracking
When analytics is enabled, L2 cache operation errors (`Get`, `Set`, `Delete`) are recorded
as `CacheOperationError` events in the analytics snapshot. Each event contains the operation
type, cache name, entity type, data source, error message (truncated to 256 chars), and
the number of keys involved. This allows operators to detect cache infrastructure issues
(e.g., Redis timeouts, connection failures) without requiring a logger on the Loader.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_test.go` — `TestDetectMutationEntityImpact / "array response invalidates all entities in the list"`

### AC-ANA-07: Cache write event source tracking
Each `CacheWriteEvent` carries a `Source` field (`CacheOperationSource`) indicating what
triggered the write: `"query"`, `"mutation"`, or `"subscription"`. This enables the metrics
exporter to label cache operations by trigger source for dashboard attribution. Subscription
cache writes are reported via `OnSubscriptionCacheWrite` callback since subscriptions run
outside per-request analytics.

### AC-ANA-08: Cache write reason tracking
Each `CacheWriteEvent` carries a `WriteReason` field (`CacheWriteReason`) indicating why
the write occurred. For root field `EntityKeyMappings` writes, the reason is one of:
- `"refresh"` — existing cached key rewritten with fresh or merged data
- `"backfill"` — missing requested key proven by final entity data
- `"derived"` — new key derived from entity data that was not in the original request

For entity fetches and non-EntityKeyMappings root field writes, the reason is empty.
The reason is set on `CacheEntry.WriteReason` during `cacheKeysToExactRootFieldEntityEntries`
and propagated to `CacheWriteEvent.WriteReason` when `RecordWrite` is called with the event.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2397` — `TestCacheBackfill_SkipFetch_HappyPath` (backfill reason on emailKey write)
- `v2/pkg/engine/resolve/cache_load_test.go:2498` — `TestCacheBackfill_FetchPath_HappyPath` (refresh on idKey, backfill on emailKey)
- `v2/pkg/engine/resolve/cache_load_test.go:2608` — `TestCacheBackfill_FetchPath_ValueMismatch` (refresh on idKey, derived on actualEmailKey)
- `v2/pkg/engine/resolve/cache_load_test.go:2663` — `TestCacheBackfill_DerivedKeyExpansion` (refresh + backfill + derived across three keys)

Tests:
- `v2/pkg/engine/resolve/cache_analytics_test.go` — `TestCacheAnalyticsCollector_WriteEventSource / "write events preserve source field"`
- `v2/pkg/engine/resolve/cache_analytics_test.go` — `TestCacheAnalyticsCollector_WriteEventSource / "mutation event preserves source field"`
- `v2/pkg/engine/resolve/cache_analytics_test.go` — `TestCacheAnalyticsCollector_WriteEventSource / "mixed sources in single snapshot"`

### AC-NEG-05: Negative cache with mutation population
When a mutation with `EnableMutationL2CachePopulation=true` triggers an entity fetch that
returns null and `NegativeCacheTTL > 0`, the negative sentinel is stored with the
`NegativeCacheTTL`, not the entity's regular TTL.

Tests:
- `v2/pkg/engine/resolve/negative_cache_test.go` — `TestNegativeCaching / "negative cache with mutation population stores sentinel with NegativeCacheTTL"`

### AC-NEG-06: Negative cache entry replaced after TTL expiry
When a negative cache sentinel expires (TTL elapses) and the entity subsequently becomes
available, the next fetch retrieves real data from the subgraph and stores it with the
entity's regular TTL, replacing the expired negative sentinel.

Tests:
- `v2/pkg/engine/resolve/negative_cache_test.go` — `TestNegativeCaching / "negative cache entry overwritten by real data on subsequent fetch"`

## Cache Trace in Response Extensions

### AC-TRACE-01: Per-fetch cache trace in extensions.trace
When tracing is enabled (`TraceOptions.Enable = true`) and `ExcludeCacheStats` is false
(default), each fetch in `extensions.trace.fetches` includes a `cache_trace` object with
L1/L2 hit/miss counts, L2 Get/Set timing, cache name, TTL, and configuration flags.

Tests:
- `execution/engine/federation_caching_trace_test.go` — `TestFederationCaching_CacheTraceInExtensions / "L2 miss then hit shows cache_trace in extensions.trace"`
- `v2/pkg/engine/resolve/cache_trace_test.go` — `TestCacheTrace_JSON` (3 subtests: full serialization, omitempty, shadow mode)

### AC-TRACE-02: Zero overhead when disabled
When `TraceOptions.Enable` is false or `ExcludeCacheStats` is true, no cache trace data
is collected: no `time.Now()` calls, no counting, no allocations. The `tracingCache` guard
(`l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats`) short-circuits
all instrumentation.

Tests:
- `v2/pkg/engine/resolve/cache_trace_test.go` — `TestBuildCacheTrace / "returns nil when tracing disabled"`
- `v2/pkg/engine/resolve/cache_trace_test.go` — `TestBuildCacheTrace / "returns nil when ExcludeCacheStats true"`

### AC-TRACE-03: Cache-hit fetches still produce trace
When L1 or L2 provides a complete hit, `load*Fetch` is never called (so `fetch.Trace` is
not normally allocated). The `ensureFetchTrace` helper allocates `DataSourceLoadTrace` on
the cache-hit path so that `CacheTrace` can still be attached.

Tests:
- `v2/pkg/engine/resolve/cache_trace_test.go` — `TestBuildCacheTrace / "full L1 hit"` (verifies CacheTrace built even when cacheSkipFetch=true)

### AC-TRACE-04: Trace attached after final cache state
`CacheTrace` is built AFTER `mergeResult` + `populateCachesAfterFetch` complete, ensuring
L2 write timing, negative cache hits, and shadow comparison results are all captured.
Attachment happens in `resolveSingle` (after `callOnFinished`) and `resolveParallel`
Phase 4 (after merge loop).

Tests:
- `execution/engine/federation_caching_trace_test.go` — `TestFederationCaching_CacheTraceInExtensions` (verifies L2 Set timing present on miss, absent on hit)

### AC-TRACE-05: Predictable debug timings
When `EnablePredictableDebugTimings` is true, all L2 timing values in `CacheTrace` are
normalized to `1ns` for deterministic test assertions.

Tests:
- `v2/pkg/engine/resolve/cache_trace_test.go` — `TestBuildCacheTrace / "predictable debug timings"`

## Batch Entity Key Mode (Root Field with List Arguments)

### AC-BATCH-01: Per-element cache key construction
When `ArgumentIsEntityKey: true` is set on a `FieldMapping` and the root field argument
is a list (e.g., `ids: ["1","2","3"]`),
the engine constructs one cache key per list element using entity key format.
Each key is identical to what an `_entities` fetch would produce for the same entity,
enabling cache sharing between root fields and entity resolution.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:2175` — `TestRenderCacheKeys_BatchEntityKey` (batch key format, single and multi-element lists)
- `v2/pkg/engine/resolve/cache_key_test.go:2273` — `TestRenderCacheKeys_BatchEntityKey / "batch key format matches scalar key format"` (scalar and batch produce identical keys for the same ID)

### AC-BATCH-02: Positional correspondence via BatchIndex
Each cache key records its position in the original list argument via `CacheKey.BatchIndex`.
This is used during response reassembly to place cached and fresh entities in the correct
output positions.
For non-batch cache keys, `BatchIndex` is unused (default 0).

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:2175` — `TestRenderCacheKeys_BatchEntityKey` (verifies BatchIndex 0, 1, 2 for three-element list)

### AC-BATCH-03: Empty list short-circuit
When the list argument is `[]` or `null`,
the engine returns an empty response (`[]`) immediately without calling the resolver
or the cache.
This avoids unnecessary subgraph calls and cache operations for trivially empty queries.

Tests:
- `v2/pkg/engine/resolve/loader_skip_fetch_test.go:889` — `TestLoader_BatchEntityKeyEmptyListShortCircuit`
- `execution/engine/federation_caching_batch_test.go:330` — `TestBatchEntityCacheLookup_FullFetch_EmptyList`

### AC-BATCH-04: Full fetch mode (all-or-nothing)
When `PartialBatchLoad` is false (default),
any cache miss in a batch causes the full list argument to be sent to the subgraph.
All returned entities are cached individually with their entity keys.

Tests:
- `execution/engine/federation_caching_batch_test.go:60` — `TestBatchEntityCacheLookup_FullFetch_AllMiss` (no cache entries, full list fetched)
- `execution/engine/federation_caching_batch_test.go:141` — `TestBatchEntityCacheLookup_FullFetch_AllHit` (all cached, no subgraph call)
- `execution/engine/federation_caching_batch_test.go:237` — `TestBatchEntityCacheLookup_FullFetch_PartialMiss_FetchesAll` (partial hit, full list refetched)
- `execution/engine/federation_caching_batch_test.go:499` — `TestBatchEntityCacheLookup_FullFetch_SingleElement` (single-element list behaves like scalar)

### AC-BATCH-05: Partial fetch mode (fetch only missing)
When `PartialBatchLoad` is true,
only IDs with cache misses are sent to the subgraph.
The input list variable is filtered to exclude IDs that were cache hits.
Cached entities are merged with fresh results in the correct positional order.

Tests:
- `execution/engine/federation_caching_batch_test.go:579` — `TestBatchEntityCacheLookup_PartialFetch_SomeCached` (some hit, only missing IDs fetched)
- `execution/engine/federation_caching_batch_test.go:676` — `TestBatchEntityCacheLookup_PartialFetch_AllHit` (all cached, no subgraph call)
- `execution/engine/federation_caching_batch_test.go:769` — `TestBatchEntityCacheLookup_PartialFetch_AllMiss` (none cached, full list fetched)
- `execution/engine/federation_caching_batch_test.go:848` — `TestBatchEntityCacheLookup_PartialFetch_OrderPreservation` (response order matches input list order)

### AC-BATCH-06: Cache sharing between scalar and batch root fields
Batch entity keys use the same format as scalar `EntityKeyMappings`.
A scalar root field `product(id: "1")` and a batch root field `products(ids: ["1","2"])`
both produce `{"__typename":"Product","key":{"id":"1"}}` for ID `"1"`,
so they share the same L2 cache entry.

Tests:
- `execution/engine/federation_caching_batch_test.go:390` — `TestBatchEntityCacheLookup_CacheKeySharing_ScalarAndBatch` (scalar write, batch read hits same cache entry)
- `v2/pkg/engine/resolve/cache_key_test.go:2273` — `TestRenderCacheKeys_BatchEntityKey / "batch key format matches scalar key format"`

### AC-BATCH-07: Constructor precomputes batch metadata
`NewRootQueryCacheKeyTemplate` precomputes batch entity key information
(argument path, entity type, merge path) via `precomputeDerivedFields()`.
The precomputed values are exposed via `BatchEntityKeyArgumentPath()` and
`EntityMergePath()` on the `CacheKeyTemplate` interface.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:2395` — `TestRenderCacheKeys_BatchEntityKey / "constructor precomputes batch entity key metadata"`

## TypeName Fallback

### AC-TYPENAME-01: Plan-time TypeName used when __typename missing
When `__typename` is missing from the response data,
the plan-time `TypeName` field on `EntityQueryCacheKeyTemplate` is used as fallback
for the cache key's `__typename` value.
This ensures cache keys always reflect the correct entity type
rather than falling back to a hardcoded default.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:632` — `TestCachingRenderEntityQueryCacheKeyTemplate` (TypeName field set on template)

## Smart Cache Key Backfill (L2, Root Field EntityKeyMappings)

### AC-L2-BACKFILL-01: Requested missing key backfilled from cached sibling
When a root field with `EntityKeyMappings` produces multiple L2 keys on read,
and one key hits while another misses,
the missing key is backfilled during writeback if the final entity value proves
the mapped key field.
The existing key that already had a cache hit is not rewritten unless
`fromCacheNeedsWriteback` is true.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2397` — `TestCacheBackfill_SkipFetch_HappyPath` (idKey hits, emailKey misses, cached value contains email → emailKey backfilled, idKey not rewritten)

### AC-L2-BACKFILL-02: Backfill requires entity-field proof
A requested missing key is NOT backfilled when the final entity value does not contain
the mapped key field,
even if the original request arguments were sufficient to construct that key on the read path.
This prevents creating unvalidated cache associations from request arguments alone.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2448` — `TestCacheBackfill_SkipFetch_Counterexample_NotDerivable` (cached value lacks email field → zero L2 writes)

### AC-L2-BACKFILL-03: Value mismatch writes the actual key, not the requested key
When the final entity value contains a mapped key field with a different value than the
requested key (e.g., request asked for `email:"a@example.com"` but subgraph returned
`email:"b@example.com"`), the requested key is NOT written, but the actual key derived
from entity data IS written.
The subgraph returned this value as backend-proven data, so it is valid to cache under
the actual key.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2608` — `TestCacheBackfill_FetchPath_ValueMismatch` (requested `a@` not written, actual `b@` written as derived key)

### AC-L2-BACKFILL-04: Fetch-path refresh plus backfill
After a partial cache hit forces a subgraph fetch,
the existing key is refreshed with fresh data and the missing requested key is backfilled
when the final entity value proves it.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2498` — `TestCacheBackfill_FetchPath_HappyPath` (idKey refreshed, emailKey backfilled — two writes)
- `v2/pkg/engine/resolve/cache_load_test.go:2553` — `TestCacheBackfill_FetchPath_MissingField` (subgraph returns no email → only idKey refreshed — one write)

### AC-L2-BACKFILL-05: Derived key expansion from final entity data
Beyond refreshing existing keys and backfilling requested missing keys,
the write path also writes additional keys when final backend-proven entity data makes
those keys derivable via `EntityKeyMappings`,
even if those keys were not part of the original read request.
This is the mechanism that enables cross-lookup:
a query with `id` argument populates the `username` key too,
so a later query with `username` argument can hit L2.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2663` — `TestCacheBackfill_DerivedKeyExpansion` (three mappings: id+email requested, username derived — three writes)
- `execution/engine/federation_caching_test.go:2300` — `TestRootFieldCachingWithArgs_PartialKeyWrite / "entity key mapping - partial key write does not generate extra keys from response"` (E2E: id requested, username derived from response)

### AC-L2-BACKFILL-06: No double-accounting between regular and derived writes
The regular write path and derived-key expansion use a single `seen` map to prevent
the same key from being written twice.
A key that is already included in the regular write set is not re-added by the
derived-key path.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2498` — `TestCacheBackfill_FetchPath_HappyPath` (idKey appears in both regular and derived paths, written exactly once)

### AC-L2-BACKFILL-07: Reproducibility checked by rendering, not by guessing
Write eligibility is determined by rendering keys from final entity data using
`renderDerivedEntityKeyFromValue` (the same renderer used by `renderDerivedEntityKey` for
request-arg-based keys).
This uses the same L2 prefix and interceptor logic as normal cache-key generation.
When a rendered key matches a requested missing key, it is a backfill.
When it doesn't match any requested key, it is a derived expansion.
In both cases, the rendered key string is the cache key — never the requested key.

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:2608` — `TestCacheBackfill_FetchPath_ValueMismatch` (rendered key `b@` differs from requested `a@` → `b@` written as derived, `a@` not written)

## @requestScoped Coordinate L1 Cache

The coordinate L1 cache is a per-request `sync.Map` on the Loader (`requestScopedL1`),
separate from the entity L1 cache.
It stores field values keyed by subgraph-qualified strings (e.g., `"viewer.currentViewer"`).

### Directive

```graphql
directive @requestScoped(key: String!) on FIELD_DEFINITION
```

**Symmetric semantics**: every field annotated with `@requestScoped(key: "X")` in the
same subgraph shares the same L1 entry `{subgraphName}.X`. There is no
receiver/provider distinction. Every participating field is simultaneously:

- A **reader** — the planner emits a hint so the resolver can inject from L1 and
  potentially skip the subgraph fetch
- A **writer** — the planner emits an export so the resolver stores the value in L1
  after the fetch

The first field to resolve populates L1; subsequent fields with the same key inject
from L1 (subject to widening checks and alias-aware normalization).

**Composition validation**:
- `key` is mandatory
- When a key is declared on only one field in the subgraph, a warning is emitted —
  `@requestScoped` is meaningless unless ≥ 2 fields share the same key

### AC-RS-01: L1 storage uses schema-normalized values via the `ProvidesData` pipeline

The coordinate L1 cache uses the same `astjson.Transform` pipeline as entity L1 and L2
caches. Per-field `normalizeXform` / `denormalizeXform` Transforms are built from the
`RequestScopedField.ProvidesData` `*Object` tree. Writes DeepCopy onto `l.jsonArena`
via `astjson.DeepCopyWithTransform` (applying the normalize Transform). Reads DeepCopy
back onto `l.jsonArena` via `astjson.DeepCopyWithTransform` with the denormalize
Transform, re-applying aliases for the current query's selection set. The planner
populates `ProvidesData` in `populateRequestScopedFieldsProvidesData` in `visitor.go`.

Values in L1 are stored under schema field names (aliases normalized away on write),
and re-aliased on read per the current query's selection set.

Tests:
- `v2/pkg/engine/plan/request_scoped_provides_data_test.go` — `TestPopulateRequestScopedFieldsProvidesData`
- `v2/pkg/engine/resolve/request_scoped_test.go` — `TestRequestScopedProvidesDataShapes` (nested aliases, array of aliased items, arg-variant sub-fields, mixed depths, __typename, nullable)

### AC-RS-02: Export on fetch completion, inject before fetch

Every `@requestScoped` field participates in both:
- **Export** (after fetch): the field's value is read from the response, normalized
  via `ProvidesData`, and stored in L1 under its `L1Key`
- **Inject** (before fetch): the resolver checks L1 under the `L1Key`; if found and
  the cached value satisfies the widening check, the value is denormalized (aliases
  re-applied), injected onto items, and the fetch is skipped

Tests:
- `v2/pkg/engine/resolve/request_scoped_test.go` — `TestExportRequestScopedFields`, `TestTryRequestScopedInjection`, `TestRequestScopedRoundTrip`

### AC-RS-03: Field widening check prevents partial injection

When the coordinate L1 has a cached value but it lacks fields required by the current
query's selection set (e.g., L1 has `{id, name}` but the current fetch needs
`{id, name, email}`), injection is blocked and the fetch proceeds normally.

The check uses `validateItemHasRequiredData` against `hint.ProvidesData` — the same
validator used by entity L1 and L2.

Tests:
- `v2/pkg/engine/resolve/request_scoped_test.go` — `TestTryRequestScopedInjection / "field widening blocks injection when cached value missing required fields"`

### AC-RS-04: @interfaceObject type mapping

When `@requestScoped` is declared on a field of an `@interfaceObject` type (e.g.,
`Personalized.currentViewer`), the planner resolves the concrete entity type
(e.g., `Article`) to the interface type via `InterfaceObjects` and finds the
`@requestScoped` fields on the interface. This enables injection on entity batches
for concrete types even when the directive is declared on the interface.

### AC-RS-05: Collect-then-inject atomicity

When multiple hints exist on the same fetch, the injection is atomic: either ALL hints
are satisfied (and items are mutated with all injected values) or NONE are (items are
left untouched). The collect-then-inject pattern prevents partial mutations from
corrupting items when a later hint fails.

Tests:
- `v2/pkg/engine/resolve/request_scoped_test.go` — `TestTryRequestScopedInjection / "partial hints returns false but does not mutate items"`, `TestRequestScopedRoundTrip / "multiple hints one blocked by field widening other cached"`

### AC-RS-06: Trace reporting — L1 hit counters and LoadSkipped

When `tryRequestScopedInjection` returns true and the fetch is skipped:
- `ensureFetchTrace(f).LoadSkipped = true` is set so the ART trace reports the fetch as skipped
- `res.cacheTraceRequestScopedHits = res.cacheTraceEntityCount` is set so `buildCacheTrace`
  folds these into the `L1Hit` counter (subtracting from `L1Miss`). The playground renders
  the red L1 hit badge accordingly.

### AC-RS-07: Arena detach on export via StructuralCopy onto `l.jsonArena`

`exportRequestScopedFields` must store a value that is independent of any source
arena. It does this by StructuralCopying onto `l.jsonArena` before storing:
- With `ProvidesData.HasAliases == true`, `StructuralCopyWithTransform` copies
  via the per-field normalize Transform, stripping aliases and arg suffixes while
  producing a fresh value owned by `l.jsonArena`.
- With `HasAliases == false`, `StructuralCopy` copies verbatim onto `l.jsonArena`.

Merging an incoming export into an existing `requestScopedL1` entry uses the
working-copy-and-swap pattern: StructuralCopy the existing entry into a working
copy, run `astjson.MergeValues` against the working copy, and store the working
copy only on success. On merge failure the existing live entry is preserved
unchanged, so a partial `MergeValues` failure cannot corrupt sibling L1 keys.

Without this, if the source value pointed into a goroutine arena or response tree
that gets freed or mutated, subsequent reads would panic or resurrect stale data.

Tests:
- `v2/pkg/engine/resolve/request_scoped_test.go` — `TestExportedValuesAreIndependentCopies`
- `v2/pkg/engine/resolve/loader_cache_phase2_test.go:147` — `TestExportRequestScopedFields_MergeWorkingCopyOnFailure` (working-copy-and-swap isolates merge failure from live cache entry)

### AC-RS-08: L1 gating

`tryRequestScopedInjection` and `exportRequestScopedFields` must check
`l.ctx.ExecutionOptions.Caching.EnableL1Cache`. Per-request headers like
`X-WG-Disable-Entity-Cache-L1` disable L1 for the request and must also disable
the coordinate L1 since it's part of the L1 layer.

## Future Improvements

The following features are not yet implemented but are planned or under consideration:

- **Stale-While-Revalidate (SWR)**: Serve stale cached data immediately while revalidating
  asynchronously in the background. Would reduce tail latency for cache-miss scenarios
  by serving slightly stale data rather than waiting for the subgraph.

- **Tag-based invalidation**: Associate cache entries with tags (e.g., `team:123`) and
  invalidate all entries with a given tag in a single operation. Would simplify bulk
  invalidation for related entities.

- **Cache entry compression**: Compress cached entity data (e.g., with zstd or gzip) to
  reduce memory and network usage for large entities in external cache stores.
