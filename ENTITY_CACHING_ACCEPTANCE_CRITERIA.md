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

### AC-L1-07: Shallow copy on L1 read
Every L1 cache read returns a shallow copy of the cached value (via `shallowCopyProvidedFields`),
not a direct pointer. This prevents pointer aliasing that would cause stack overflow during
JSON merge when an entity type references itself (e.g., `User.friends` returns `[User]`).
The copy is unconditional — it always happens, even for non-self-referential entities —
because the overhead is minimal and the safety guarantee is universal. The copy includes
only the fields specified in `ProvidesData`, not the entire entity.

_Future optimization_: for entities known to never self-reference, the copy could be skipped.

Tests:
- `execution/engine/federation_caching_l1_test.go:344` — `TestL1CacheSelfReferentialEntity`
- `v2/pkg/engine/resolve/l1_cache_test.go:1993` — `TestShallowCopyWithAliases` (reads original name, writes alias)

### AC-L1-08: Root field entity population
When a root field query (e.g., `topProducts`) returns entities, those entities are
extracted and stored in L1 using their `@key`-based cache keys. This means a subsequent
entity fetch for the same entity within the same request can hit L1 instead of making
another subgraph call. Requires `RootFieldL1EntityCacheKeyTemplates` to be configured.

If the client's query doesn't select the `@key` fields (e.g., omits `id`), the cache key
is produced with an empty key object (`{"__typename":"Product","key":{}}`) and the entity
is silently stored under this degraded key. It will never match a real entity fetch, so the
behavior is benign but wasteful.

Tests:
- `execution/engine/federation_caching_l1_test.go:667` — `TestL1CacheRootFieldEntityListPopulation`
- `v2/pkg/engine/resolve/l1_cache_test.go:1813` — `TestPopulateL1CacheForRootFieldEntities_MissingKeyFields`

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

### AC-L2-02: L2 operations run in goroutines
L2 `Get` (cache read) and the fallback subgraph HTTP call happen in parallel goroutines
during Phase 2. This means `LoaderCache` implementations must be safe for concurrent
access from multiple goroutines.

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
schema field names, and fields with arguments get an xxhash suffix appended. This
ensures cached data is query-independent and can be reused across different GraphQL
operations that request the same entity.

Tests:
- `v2/pkg/engine/resolve/l1_cache_test.go:1535` — `TestNormalizeForCache` (7 subtests: fast path, aliases, mixed, nested, __typename, CacheArgs suffix, alias+CacheArgs)
- `v2/pkg/engine/resolve/l1_cache_test.go:1693` — `TestNormalizeDenormalizeRoundTrip` (7 subtests: round-trip with CacheArgs, alias+CacheArgs, nested, arrays, __typename preservation)
- `v2/pkg/engine/resolve/l1_cache_test.go:1858` — `TestDenormalizeFromCache` (4 subtests: fast path, aliases, CacheArgs suffixed lookup, alias+CacheArgs)

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

Tests:
- `v2/pkg/engine/resolve/cache_load_test.go:605` — `TestCacheLoadSequential / "single entity fetch with cache miss"`

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

### AC-KEY-02: Root field key format
Root field cache keys use `{"__typename":"Query","field":"fieldName","args":{...}}`.
Arguments are included when present. Root field keys can optionally map to entity keys
via `EntityKeyMappings` so that a root field query and an entity query share the same
cache entry.

Tests:
- `v2/pkg/engine/resolve/cache_key_test.go:13` — `TestCachingRenderRootQueryCacheKeyTemplate`

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
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:21` — `TestNavigateProvidesDataToField` (4 subtests: valid field, missing field, nil providesData, non-Object field)
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:71` — `TestBuildEntityKeyValue` (4 subtests: simple key, composite key, nested key, missing field)
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:128` — `TestBuildMutationEntityCacheKey` (3 subtests: basic key, with header prefix, with interceptor)
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:249` — `TestDetectMutationEntityImpact` (includes array response invalidation and non-object item skipping)

### AC-MUT-05: Pre-delete cache read for analytics
When both cache invalidation and analytics are enabled, the cached value is read BEFORE
the delete operation. This allows the analytics system to compare the stale cached value
against the fresh mutation response to measure staleness.

_Known limitation_: `LoaderCache.Delete()` returns only an error, not a success/existence
indicator. The analytics system cannot distinguish "key did not exist" from "key was
successfully deleted". This would require extending the `LoaderCache` interface.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:378` — `TestDetectMutationEntityImpact / "analytics enabled, no cached value records MutationEvent with HadCachedValue=false"`

### AC-MUT-06: Staleness detection via hash comparison
Mutation impact analytics computes xxhash of both the cached entity (pre-delete) and the
fresh mutation response (both filtered to `ProvidesData` fields only). If hashes differ,
the entity is marked as stale. This measures how often mutations actually change cached
data.

_Note_: This mechanism (xxhash of `ProvidesData`-filtered fields) is shared with
shadow mode staleness detection (AC-SHADOW-03). The trigger differs (mutation response
vs shadow mode) but the comparison logic is identical.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:416` — `TestDetectMutationEntityImpact / "analytics enabled, stale cached value records MutationEvent with IsStale=true"`

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

### AC-SUB-03: Full key pipeline for subscription cache operations
Subscription cache operations (both populate and invalidate) apply the full cache key
pipeline: template rendering → global prefix → header hash prefix → `L2CacheKeyInterceptor`.
This ensures subscription-written keys match query-read keys even when custom key
transforms (e.g., tenant prefix via interceptor) are in use.

Tests:
- `v2/pkg/engine/resolve/trigger_cache_test.go:51` — `TestHandleTriggerEntityCache / "populate single entity"` (verifies key pipeline for populate)
- `v2/pkg/engine/resolve/trigger_cache_test.go:224` — `TestHandleTriggerEntityCache / "invalidate mode deletes cache entry"` (verifies key pipeline for invalidate)

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
L2 `LoaderCache.Get()`, `Set()`, and `Delete()` are called from goroutines during Phase 2
parallel execution. Implementers must ensure thread-safe access (e.g., connection pooling
for Redis).

Tests:
- `execution/engine/federation_caching_test.go:1435` — `TestFederationCaching / "concurrency with different IDs"`

### AC-THREAD-03: Per-result analytics accumulation
During Phase 2, each goroutine accumulates analytics events (L2 key events, fetch timings,
errors) on its own per-result slice. After all goroutines complete (`g.Wait()`), the main
thread merges all per-result events into the single analytics collector via
`MergeL2Events`/`MergeL2FetchTimings`/`MergeL2Errors`.

Tests:
- `v2/pkg/engine/resolve/cache_analytics_test.go:65` — `TestCacheAnalyticsCollector_MergeL2Events`

### AC-THREAD-04: Per-goroutine arenas for thread-safe allocation
The JSON arena (`jsonArena`) uses a `MonotonicArena` which is NOT thread-safe. Phase 2
goroutines that run `tryL2CacheLoad` allocate JSON values (in `extractCacheKeysStrings`,
`populateFromCache`, `EntityMergePath` wrapping, and `denormalizeFromCache`).

To avoid data races, each Phase 2 goroutine receives its own arena from `l2ArenaPool`
(a `sync.Pool` of `MonotonicArena` instances). The per-goroutine arenas are stored in
`Loader.goroutineArenas` and released in `Loader.Free()` — NOT inside the goroutine —
because `astjson.MergeValues` is shallow (it links `*Value` pointers from the source into
the target tree without deep-copying). After merge, the response tree holds cross-arena
references into the goroutine arenas, which must remain valid until response rendering
completes.

Tests:
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:21` — `TestCrossArenaMergeValuesCreatesShallowReferences`
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:83` — `TestGoroutineArenaLifetimeWithDeferredRelease`
- `v2/pkg/engine/resolve/arena_thread_safety_gc_test.go:137` — `Benchmark_CrossArenaGCSafety`
- `v2/pkg/engine/resolve/arena_thread_safety_bench_test.go:40` — `BenchmarkConcurrentArena`
- `v2/pkg/engine/resolve/arena_thread_safety_bench_test.go:61` — `BenchmarkPerGoroutineArena`
- `v2/pkg/engine/resolve/loader_arena_gc_test.go:102` — `Benchmark_ArenaGCSafety`

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
`CachedBytesServed()`, `SubgraphCallsAvoided()`, `AvgCacheAgeMs()`, etc. These are
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
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:216` — `TestBuildMutationEntityDisplayKey` (display key always without prefix)

### AC-ANA-06: Cache operation error tracking
When analytics is enabled, L2 cache operation errors (`Get`, `Set`, `Delete`) are recorded
as `CacheOperationError` events in the analytics snapshot. Each event contains the operation
type, cache name, entity type, data source, error message (truncated to 256 chars), and
the number of keys involved. This allows operators to detect cache infrastructure issues
(e.g., Redis timeouts, connection failures) without requiring a logger on the Loader.

Tests:
- `v2/pkg/engine/resolve/mutation_cache_impact_test.go:625` — `TestDetectMutationEntityImpact / "array response invalidates all entities in the list"`

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
