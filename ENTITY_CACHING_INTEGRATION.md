# Entity Caching Integration Guide

This guide covers everything needed to integrate the entity caching system into a GraphQL Federation router. After reading this, you should be able to fully configure L1/L2 caching, implement a cache backend, set up invalidation, and collect analytics.

## Overview

The caching system has two levels:

| Level | Storage | Scope | Applies To | Default |
|-------|---------|-------|-----------|---------|
| **L1** | In-memory `sync.Map` per request | Single request | Entity fetches only | Disabled |
| **L2** | External cache (Redis, etc.) | Cross-request with TTL | Entity + root field fetches | Disabled |

Both levels are opt-in and disabled by default. L1 prevents redundant fetches for the same entity within a single request. L2 shares entity data across requests.

**Key principle**: Cache keys use only `@key` fields for stable entity identity (never `@requires`).

## 1. Implement the LoaderCache Interface

To use L2 caching, implement the `LoaderCache` interface from `v2/pkg/engine/resolve`:

```go
import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

type LoaderCache interface {
    // Get retrieves cache entries by keys.
    // Returns a slice of the same length as keys. Use nil for cache misses.
    // Called from goroutines during parallel resolution — must be thread-safe.
    Get(ctx context.Context, keys []string) ([]*resolve.CacheEntry, error)

    // Set stores cache entries with a TTL.
    // Called from goroutines during parallel resolution — must be thread-safe.
    Set(ctx context.Context, entries []*resolve.CacheEntry, ttl time.Duration) error

    // Delete removes cache entries by keys.
    // Called during cache invalidation (extension-based, mutation-based).
    Delete(ctx context.Context, keys []string) error
}

type CacheEntry struct {
    Key          string        // Cache key string (JSON format)
    Value        []byte        // JSON-encoded entity data
    RemainingTTL time.Duration // Remaining TTL from cache (0 = unknown/not supported)
}
```

**Thread safety requirement**: `Get`, `Set`, and `Delete` may be called from multiple goroutines during parallel fetch execution. Your implementation must be safe for concurrent use.

**RemainingTTL**: If your cache backend supports it, return the remaining TTL in `CacheEntry.RemainingTTL`. This is used for cache analytics (cache age tracking) and shadow mode staleness detection. Return 0 if not supported.

## 2. Configure Per-Subgraph Caching

### SubgraphCachingConfig

Each subgraph can have independent caching configuration. Pass these via the factory option:

```go
import (
    "github.com/wundergraph/graphql-go-tools/execution/engine"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

subgraphCachingConfigs := engine.SubgraphCachingConfigs{
    {
        SubgraphName: "accounts",  // Must match SubgraphConfiguration.Name
        EntityCaching: plan.EntityCacheConfigurations{...},
        RootFieldCaching: plan.RootFieldCacheConfigurations{...},
        MutationFieldCaching: plan.MutationFieldCacheConfigurations{...},
        MutationCacheInvalidation: plan.MutationCacheInvalidationConfigurations{...},
        SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{...},
    },
}

factory := engine.NewFederationEngineConfigFactory(
    ctx,
    subgraphsConfigs,
    engine.WithSubgraphEntityCachingConfigs(subgraphCachingConfigs),
)
config, err := factory.BuildEngineConfiguration()
```

### Entity Cache Configuration

Controls L2 caching for entity types resolved via `_entities` queries:

```go
plan.EntityCacheConfiguration{
    // TypeName is the entity type to cache (must match __typename from subgraph).
    TypeName: "User",

    // CacheName identifies which LoaderCache instance to use.
    // Multiple entity types can share a cache by using the same name.
    CacheName: "default",

    // TTL specifies how long cached entities remain valid.
    // Zero TTL means entries never expire (not recommended for production).
    TTL: 60 * time.Second,

    // IncludeSubgraphHeaderPrefix controls whether forwarded headers affect cache keys.
    // When true, cache keys include a hash of headers sent to the subgraph,
    // ensuring different header configurations (e.g., different auth tokens)
    // use separate cache entries.
    IncludeSubgraphHeaderPrefix: true,

    // EnablePartialCacheLoad enables fetching only cache-missed entities.
    // Default (false): any miss in a batch refetches ALL entities.
    // When true: only missing entities are fetched, cached ones served directly.
    EnablePartialCacheLoad: false,

    // HashAnalyticsKeys controls whether entity keys are hashed or stored raw
    // in cache analytics. When true, KeyHash is populated instead of KeyRaw.
    HashAnalyticsKeys: false,

    // ShadowMode enables shadow caching: L2 reads/writes happen but cached data
    // is never served. Fresh data is always fetched and compared against cache
    // for staleness detection. L1 cache is unaffected.
    ShadowMode: false,
}
```

### Root Field Cache Configuration

Controls L2 caching for root query fields (e.g., `Query.topProducts`):

```go
plan.RootFieldCacheConfiguration{
    TypeName:  "Query",
    FieldName: "topProducts",
    CacheName: "default",
    TTL:       30 * time.Second,
    IncludeSubgraphHeaderPrefix: true,

    // EntityKeyMappings enables cache sharing between root fields and entity fetches.
    // When set, the L2 cache key uses entity key format instead of root field format.
    // Example: Query.user(id: "123") shares cache with User entity key {"id":"123"}.
    EntityKeyMappings: []plan.EntityKeyMapping{
        {
            EntityTypeName: "User",
            FieldMappings: []plan.FieldMapping{
                {
                    EntityKeyField: "id",        // @key field on User
                    ArgumentPath:   []string{"id"}, // Root field argument name
                },
            },
        },
    },

    ShadowMode: false,
}
```

### Mutation Field Cache Configuration

Controls whether entity fetches triggered by a mutation populate L2:

```go
plan.MutationFieldCacheConfiguration{
    // Mutation field name
    FieldName: "addReview",

    // By default, mutations skip L2 reads AND L2 writes.
    // Set to true to allow entity fetches during this mutation to write to L2.
    EnableEntityL2CachePopulation: true,
}
```

**Mutation caching behavior**:
- Mutations **always skip L2 reads** (always fetch fresh from subgraph)
- Mutations **skip L2 writes by default**
- With `EnableEntityL2CachePopulation: true`, entity fetches triggered by this mutation **will write to L2**

### Mutation Cache Invalidation Configuration

Configures automatic L2 cache deletion after a mutation completes:

```go
plan.MutationCacheInvalidationConfiguration{
    FieldName:      "updateUser",
    // EntityTypeName can be omitted — it's inferred from the mutation return type.
    EntityTypeName: "User",
}
```

When the mutation returns an entity with `@key` fields, the corresponding L2 cache entry is deleted.

### Subscription Entity Population Configuration

Controls how subscription events update the L2 cache:

```go
plan.SubscriptionEntityPopulationConfiguration{
    TypeName:  "Product",
    CacheName: "default",
    TTL:       30 * time.Second,
    IncludeSubgraphHeaderPrefix: true,

    // When true and the subscription only provides @key fields (no additional
    // entity fields), DELETE the L2 cache entry on each event.
    // When false (default), populate L2 with entity data from the event.
    EnableInvalidationOnKeyOnly: false,
}
```

**Two modes**:
- **Populate** (default): subscription provides entity fields beyond `@key` → write to L2
- **Invalidate** (`EnableInvalidationOnKeyOnly: true`): subscription provides only `@key` → delete from L2

## 3. Wire Caches into the Resolver

Register your `LoaderCache` implementations in the `ResolverOptions`:

```go
resolver := resolve.New(ctx, resolve.ResolverOptions{
    MaxConcurrency: 32,

    // Register named cache instances (referenced by CacheName in configs)
    Caches: map[string]resolve.LoaderCache{
        "default": myRedisCache,
        "fast":    myInMemoryCache,
    },

    // Required for extension-based cache invalidation
    // Maps subgraphName → entityTypeName → invalidation config
    EntityCacheConfigs: map[string]map[string]*resolve.EntityCacheInvalidationConfig{
        "accounts": {
            "User": {
                CacheName:                   "default",
                IncludeSubgraphHeaderPrefix: true,
            },
        },
    },

    // ... other options
})
```

## 4. Enable Caching at Runtime

Set caching options per-request on the execution context:

```go
ctx := resolve.NewContext(context.Background())
ctx.ExecutionOptions.Caching = resolve.CachingOptions{
    // Enable per-request in-memory entity cache
    EnableL1Cache: true,

    // Enable external cross-request cache
    EnableL2Cache: true,

    // Enable detailed cache analytics collection
    EnableCacheAnalytics: true,

    // Optional: transform L2 cache keys (e.g., for tenant isolation)
    L2CacheKeyInterceptor: func(ctx context.Context, key string, info resolve.L2CacheKeyInterceptorInfo) string {
        tenantID := ctx.Value("tenant-id").(string)
        return tenantID + ":" + key
    },
}
```

**L2CacheKeyInterceptor** receives:
```go
type L2CacheKeyInterceptorInfo struct {
    SubgraphName string  // e.g., "accounts"
    CacheName    string  // e.g., "default"
}
```

The interceptor is applied **after** subgraph header prefix. It does NOT affect L1 keys.

## 5. Cache Key Format

### Entity Keys

Generated by `EntityQueryCacheKeyTemplate` from `@key` fields:
```json
{"__typename":"User","key":{"id":"123"}}
{"__typename":"Product","key":{"upc":"top-1"}}
{"__typename":"Order","key":{"id":"1","orgId":"acme"}}
```

### Root Field Keys

Generated by `RootQueryCacheKeyTemplate` from field name and arguments:
```json
{"__typename":"Query","field":"topProducts"}
{"__typename":"Query","field":"user","args":{"id":"123"}}
{"__typename":"Query","field":"search","args":{"max":10,"term":"C3PO"}}
```

Arguments are sorted alphabetically for stable key generation.

### Key Transformations (applied in order)

1. **Subgraph header hash prefix** (when `IncludeSubgraphHeaderPrefix = true`):
   ```
   {headerHash}:{"__typename":"User","key":{"id":"123"}}
   ```

2. **L2CacheKeyInterceptor** (when set):
   ```
   tenant-X:{headerHash}:{"__typename":"User","key":{"id":"123"}}
   ```

### Entity Field Argument-Aware Keys

When entity fields have arguments (e.g., `greeting(style: "formal")`), the field argument values are hashed via xxhash and appended as a suffix to the cache key. Different argument values produce different cache entries.

### EntityKeyMappings (Cache Sharing)

When `EntityKeyMappings` is configured on a root field, the L2 cache key uses entity key format instead of root field format. This means:
- `Query.user(id: "123")` → cache key `{"__typename":"User","key":{"id":"123"}}`
- A subsequent `_entities` fetch for `User(id: "123")` hits the same cache entry

## 6. Cache Behavior by Operation Type

### Queries

```
L1 check (main thread, entity fetches only)
  ↓ miss
L2 check (goroutine, entity + root fetches)
  ↓ miss
Subgraph fetch (goroutine)
  ↓ response
Populate L1 + L2 (main thread for L1, goroutine for L2)
```

L1 is checked first on the main thread. If it's a complete hit, the goroutine is not spawned (saves overhead). L2 and fetch happen in parallel goroutines.

### Mutations

- **Always skip L2 reads** — fetch fresh data from subgraph
- **Skip L2 writes by default** — unless `EnableEntityL2CachePopulation: true` on the mutation field
- **Optional invalidation** — with `MutationCacheInvalidationConfiguration`, delete L2 entry after mutation
- **Mutation impact detection** — when analytics enabled, compare mutation response against cached value

### Subscriptions

Based on `SubscriptionEntityPopulationConfiguration`:
- **Populate mode** (default): on each subscription event, write entity data to L2
- **Invalidate mode** (`EnableInvalidationOnKeyOnly: true`): on each event with only `@key` fields, delete L2 entry

## 7. Cache Invalidation

### Mutation-Triggered Invalidation

Configure via `MutationCacheInvalidationConfiguration`. After a mutation completes and returns an entity, the L2 cache entry for that entity is deleted.

### Subgraph Response Extension Invalidation

Subgraphs can signal cache invalidation through GraphQL response extensions:

```json
{
  "data": { "updateUser": { "id": "1", "name": "Updated" } },
  "extensions": {
    "cacheInvalidation": {
      "keys": [
        { "typename": "User", "key": { "id": "1" } },
        { "typename": "User", "key": { "id": "2" } }
      ]
    }
  }
}
```

The engine automatically:
1. Parses `extensions.cacheInvalidation.keys` from each subgraph response
2. Builds L2 cache keys matching entity type and key fields
3. Applies subgraph header prefix and `L2CacheKeyInterceptor` transformations
4. Calls `LoaderCache.Delete()` for each key
5. **Optimization**: skips delete if the same key is being written in the same fetch (no unnecessary round-trip)

**Requirements for extension-based invalidation**:
- `EntityCacheConfigs` must be set on `ResolverOptions` (maps subgraph name → entity type → cache config)
- `EnableL2Cache` must be true on the request context

### Subscription-Based Invalidation

With `EnableInvalidationOnKeyOnly: true`, subscription events that only contain `@key` fields trigger L2 deletion.

### Manual Invalidation

Call `LoaderCache.Delete()` directly with cache keys. The key format is:
```
[optional-interceptor-prefix:][optional-header-hash:]{"__typename":"TypeName","key":{...}}
```

## 8. Partial Cache Loading

Controls what happens when some entities in a batch are cached and others are not.

**Default (`EnablePartialCacheLoad: false`)**:
Any cache miss in a batch → refetch ALL entities from the subgraph. This keeps the cache maximally fresh because every entity gets a fresh value on each batch miss.

**Enabled (`EnablePartialCacheLoad: true`)**:
Only missing entities are fetched from the subgraph. Cached entities are served directly within their TTL window. This reduces subgraph load but cached entities may be slightly stale (within TTL).

Choose based on your freshness vs. performance tradeoff.

## 9. Shadow Mode

Shadow mode lets you test caching in production without serving cached data to clients.

**Behavior**:
- L2 cache reads and writes happen normally
- Cached data is **never served** — fresh data is always fetched from the subgraph
- Fresh and cached data are compared for staleness detection
- L1 cache works normally (not affected by shadow mode)

**Configuration**: Set `ShadowMode: true` on `EntityCacheConfiguration` or `RootFieldCacheConfiguration`.

**Staleness results** are available in `CacheAnalyticsSnapshot.ShadowComparisons`:
```go
type ShadowComparisonEvent struct {
    CacheKey      string        // Cache key for correlation
    EntityType    string        // Entity type name
    IsFresh       bool          // true if cached data matches fresh data
    CachedHash    uint64        // xxhash of cached ProvidesData fields
    FreshHash     uint64        // xxhash of fresh ProvidesData fields
    CachedBytes   int           // Size of cached ProvidesData
    FreshBytes    int           // Size of fresh ProvidesData
    DataSource    string        // Subgraph name
    CacheAgeMs    int64         // Age of cached entry (ms, 0 = unknown)
    ConfiguredTTL time.Duration // TTL configured for this entity
}
```

## 10. Cache Analytics

Enable via `EnableCacheAnalytics: true` in `CachingOptions`. After execution, collect stats:

```go
snapshot := ctx.GetCacheStats()
```

### CacheAnalyticsSnapshot

```go
type CacheAnalyticsSnapshot struct {
    L1Reads           []CacheKeyEvent          // L1 read events (hit/miss)
    L2Reads           []CacheKeyEvent          // L2 read events (hit/miss/partial-hit)
    L1Writes          []CacheWriteEvent        // L1 write events
    L2Writes          []CacheWriteEvent        // L2 write events
    FetchTimings      []FetchTimingEvent       // Per-fetch timing with HTTP status
    ErrorEvents       []SubgraphErrorEvent     // Subgraph errors
    FieldHashes       []EntityFieldHash        // Field value hashes for staleness
    EntityTypes       []EntityTypeInfo         // Entity counts by type
    ShadowComparisons []ShadowComparisonEvent  // Shadow mode results
    MutationEvents    []MutationEvent          // Mutation impact on cache
}
```

### Convenience Methods

```go
snapshot.L1HitRate()           // float64 [0, 1]
snapshot.L2HitRate()           // float64 [0, 1]
snapshot.CachedBytesServed()   // int64
snapshot.EventsByEntityType()  // map[string]EntityTypeCacheStats
```

### Key Event Types

**CacheKeyEvent** — per-key cache lookup:
```go
type CacheKeyEvent struct {
    CacheKey   string            // Cache key
    EntityType string            // Entity type name
    Kind       CacheKeyEventKind // CacheKeyHit, CacheKeyMiss, CacheKeyPartialHit
    DataSource string            // Subgraph name
    ByteSize   int               // Cached entry size
    CacheAgeMs int64             // Age in ms (L2 only, 0 = unknown)
    Shadow     bool              // Shadow mode event
}
```

**FetchTimingEvent** — per-fetch timing:
```go
type FetchTimingEvent struct {
    DataSource     string      // Subgraph name
    EntityType     string      // Entity type (empty for root fields)
    DurationMs     int64       // Fetch/lookup duration
    Source         FieldSource // FieldSourceSubgraph, FieldSourceL1, FieldSourceL2
    ItemCount      int         // Number of entities
    IsEntityFetch  bool        // true for _entities queries
    HTTPStatusCode int         // HTTP status (0 for cache hits)
    ResponseBytes  int         // Response body size (0 for cache hits)
    TTFBMs         int64       // Time to first byte
}
```

**MutationEvent** — mutation impact on cached entities:
```go
type MutationEvent struct {
    MutationRootField string // e.g., "updateUser"
    EntityType        string // e.g., "User"
    EntityCacheKey    string // Display key JSON
    HadCachedValue    bool   // true if L2 had an entry
    IsStale           bool   // true if cached differs from mutation response
    CachedHash        uint64 // Hash of cached ProvidesData
    FreshHash         uint64 // Hash of mutation response ProvidesData
    CachedBytes       int    // 0 when HadCachedValue=false
    FreshBytes        int
}
```

### Integration Pattern

```go
// After each request:
snapshot := ctx.GetCacheStats()

// Export to observability
metrics.RecordL1HitRate(snapshot.L1HitRate())
metrics.RecordL2HitRate(snapshot.L2HitRate())
metrics.RecordCachedBytesServed(snapshot.CachedBytesServed())

for _, timing := range snapshot.FetchTimings {
    metrics.RecordFetchDuration(timing.DataSource, timing.DurationMs, timing.Source)
}

for _, shadow := range snapshot.ShadowComparisons {
    if !shadow.IsFresh {
        log.Warn("stale cache entry", "entity", shadow.EntityType, "key", shadow.CacheKey, "age_ms", shadow.CacheAgeMs)
    }
}

for _, mutation := range snapshot.MutationEvents {
    if mutation.IsStale {
        log.Info("mutation updated stale cache", "field", mutation.MutationRootField, "entity", mutation.EntityType)
    }
}
```

## 11. Complete Integration Example

```go
package main

import (
    "context"
    "time"

    "github.com/wundergraph/graphql-go-tools/execution/engine"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func setupCaching() {
    // 1. Define subgraph caching configurations
    cachingConfigs := engine.SubgraphCachingConfigs{
        {
            SubgraphName: "accounts",
            EntityCaching: plan.EntityCacheConfigurations{
                {
                    TypeName:                    "User",
                    CacheName:                   "default",
                    TTL:                         5 * time.Minute,
                    IncludeSubgraphHeaderPrefix: true,
                },
            },
            RootFieldCaching: plan.RootFieldCacheConfigurations{
                {
                    TypeName:                    "Query",
                    FieldName:                   "me",
                    CacheName:                   "default",
                    TTL:                         1 * time.Minute,
                    IncludeSubgraphHeaderPrefix: true,
                },
            },
            MutationFieldCaching: plan.MutationFieldCacheConfigurations{
                {
                    FieldName:                     "updateUser",
                    EnableEntityL2CachePopulation:  true,
                },
            },
            MutationCacheInvalidation: plan.MutationCacheInvalidationConfigurations{
                {
                    FieldName:      "deleteUser",
                    EntityTypeName: "User",
                },
            },
        },
        {
            SubgraphName: "products",
            EntityCaching: plan.EntityCacheConfigurations{
                {
                    TypeName:  "Product",
                    CacheName: "default",
                    TTL:       10 * time.Minute,
                },
            },
            RootFieldCaching: plan.RootFieldCacheConfigurations{
                {
                    TypeName:  "Query",
                    FieldName: "topProducts",
                    CacheName: "default",
                    TTL:       30 * time.Second,
                },
            },
            SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
                {
                    TypeName:                    "Product",
                    CacheName:                   "default",
                    TTL:                         10 * time.Minute,
                    EnableInvalidationOnKeyOnly: true,
                },
            },
        },
    }

    // 2. Create engine configuration
    factory := engine.NewFederationEngineConfigFactory(
        context.Background(),
        subgraphConfigs, // []engine.SubgraphConfiguration
        engine.WithSubgraphEntityCachingConfigs(cachingConfigs),
    )
    config, _ := factory.BuildEngineConfiguration()

    // 3. Create resolver with cache instances
    resolver := resolve.New(context.Background(), resolve.ResolverOptions{
        MaxConcurrency: 64,
        Caches: map[string]resolve.LoaderCache{
            "default": NewRedisCache("redis://localhost:6379"),
        },
        EntityCacheConfigs: map[string]map[string]*resolve.EntityCacheInvalidationConfig{
            "accounts": {
                "User": {CacheName: "default", IncludeSubgraphHeaderPrefix: true},
            },
            "products": {
                "Product": {CacheName: "default"},
            },
        },
    })

    // 4. Per-request: enable caching
    execCtx := resolve.NewContext(context.Background())
    execCtx.ExecutionOptions.Caching = resolve.CachingOptions{
        EnableL1Cache:        true,
        EnableL2Cache:        true,
        EnableCacheAnalytics: true,
        L2CacheKeyInterceptor: func(ctx context.Context, key string, info resolve.L2CacheKeyInterceptorInfo) string {
            // Optional: add tenant isolation
            if tenantID, ok := ctx.Value("tenant-id").(string); ok {
                return tenantID + ":" + key
            }
            return key
        },
    }

    // 5. Resolve (uses config from step 2)
    resolveInfo, _ := resolver.ResolveGraphQLResponse(execCtx, response, initialData, writer)

    // 6. Collect cache analytics
    snapshot := execCtx.GetCacheStats()
    _ = snapshot.L1HitRate()
    _ = snapshot.L2HitRate()
    _ = snapshot.CachedBytesServed()
    _ = config
    _ = resolveInfo
}
```

## 12. Configuration Reference Summary

| Configuration | Package | Purpose |
|--------------|---------|---------|
| `SubgraphCachingConfig` | `execution/engine` | Top-level per-subgraph config container |
| `EntityCacheConfiguration` | `v2/pkg/engine/plan` | L2 entity caching (TypeName, TTL, etc.) |
| `RootFieldCacheConfiguration` | `v2/pkg/engine/plan` | L2 root field caching (FieldName, EntityKeyMappings) |
| `MutationFieldCacheConfiguration` | `v2/pkg/engine/plan` | Mutation L2 write control |
| `MutationCacheInvalidationConfiguration` | `v2/pkg/engine/plan` | Mutation-triggered L2 deletion |
| `SubscriptionEntityPopulationConfiguration` | `v2/pkg/engine/plan` | Subscription L2 populate/invalidate |
| `CachingOptions` | `v2/pkg/engine/resolve` | Per-request L1/L2/analytics enable |
| `L2CacheKeyInterceptor` | `v2/pkg/engine/resolve` | Custom key transform (tenant isolation) |
| `LoaderCache` | `v2/pkg/engine/resolve` | Cache backend interface |
| `EntityCacheInvalidationConfig` | `v2/pkg/engine/resolve` | Extension-based invalidation lookup |
| `ResolverOptions.Caches` | `v2/pkg/engine/resolve` | Named cache instance registry |
