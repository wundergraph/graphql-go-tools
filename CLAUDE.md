# Entity Caching Reference

GraphQL Federation entity caching system with L1 (per-request) and L2 (external) caches.

## Architecture Overview

| Cache | Storage | Scope | Key Fields | Thread Safety |
|-------|---------|-------|------------|---------------|
| **L1** | `sync.Map` in Loader | Single request | `@key` only (L1Keys) | sync.Map |
| **L2** | External (LoaderCache) | Cross-request | `@key` + `@requires` (Keys) | Atomic stats |

**Key Principle**: L1 uses only `@key` fields for stable entity identity. L2 uses full entity representation.

## Key Files

| File | Purpose |
|------|---------|
| `v2/pkg/engine/resolve/loader.go` | L1/L2 cache core: `prepareCacheKeys`, `tryL1CacheLoad`, `tryL2CacheLoad`, `populateL1Cache` |
| `v2/pkg/engine/resolve/loader_json_copy.go` | Shallow copy for self-referential entities |
| `v2/pkg/engine/resolve/caching.go` | `RenderL1CacheKeys`, `RenderL2CacheKeys`, `EntityQueryCacheKeyTemplate`, `RootQueryCacheKeyTemplate` |
| `v2/pkg/engine/resolve/context.go` | `CachingOptions`, `CacheStats`, tracking methods |
| `v2/pkg/engine/resolve/fetch.go` | `FetchCacheConfiguration`, `FetchInfo.ProvidesData` |
| `v2/pkg/engine/plan/visitor.go` | `configureFetchCaching()`, `isEntityBoundaryField` |
| `v2/pkg/engine/plan/federation_metadata.go` | `EntityCacheConfiguration`, `RootFieldCacheConfiguration` |
| `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` | `buildL1KeysVariable()`, cache key template building |
| `execution/engine/config_factory_federation.go` | `SubgraphCachingConfig`, per-subgraph configuration |
| `execution/engine/federation_caching_test.go` | E2E caching tests |
| `v2/pkg/engine/resolve/l1_cache_test.go` | L1 cache unit tests |

## Core Types

### Cache Key Templates
```go
// Entity caching - uses different keys for L1 vs L2
type EntityQueryCacheKeyTemplate struct {
    Keys   *ResolvableObjectVariable  // L2: @key + @requires fields
    L1Keys *ResolvableObjectVariable  // L1: @key fields only
}
func (e *EntityQueryCacheKeyTemplate) RenderL1CacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value) ([]*CacheKey, error)
func (e *EntityQueryCacheKeyTemplate) RenderL2CacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)

// Root field caching - same template for L1 and L2
type RootQueryCacheKeyTemplate struct {
    RootFields []QueryField  // TypeName + FieldName + Args
}
```

### Configuration Types
```go
// Per-subgraph caching config (explicit opt-in)
type SubgraphCachingConfig struct {
    SubgraphName     string
    EntityCaching    plan.EntityCacheConfigurations    // For _entities queries
    RootFieldCaching plan.RootFieldCacheConfigurations // For root queries
}

type EntityCacheConfiguration struct {
    TypeName                    string        // e.g., "User"
    CacheName                   string
    TTL                         time.Duration
    IncludeSubgraphHeaderPrefix bool
}

type RootFieldCacheConfiguration struct {
    TypeName                    string        // e.g., "Query"
    FieldName                   string        // e.g., "topProducts"
    CacheName                   string
    TTL                         time.Duration
    IncludeSubgraphHeaderPrefix bool
}
```

### Cache Stats (Thread Safety)
```go
type CacheStats struct {
    L1Hits   int64           // Main thread only (non-atomic)
    L1Misses int64           // Main thread only (non-atomic)
    L2Hits   *atomic.Int64   // Goroutine-safe (atomic)
    L2Misses *atomic.Int64   // Goroutine-safe (atomic)
}
```

## Enabling Caching

### Runtime Options
```go
ctx.ExecutionOptions.Caching = CachingOptions{
    EnableL1Cache: true,  // Per-request entity cache
    EnableL2Cache: true,  // External cache
}
```

### Per-Subgraph Configuration (L2 only)
```go
subgraphCachingConfigs := engine.SubgraphCachingConfigs{
    {
        SubgraphName: "products",
        RootFieldCaching: plan.RootFieldCacheConfigurations{
            {TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
        },
    },
    {
        SubgraphName: "accounts",
        EntityCaching: plan.EntityCacheConfigurations{
            {TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
        },
    },
}

opts := []engine.FederationEngineConfigFactoryOption{
    engine.WithSubgraphEntityCachingConfigs(subgraphCachingConfigs),
}
```

## Cache Flow

### Sequential Execution (`tryCacheLoad`)
1. `prepareCacheKeys()` - Generate L1 and L2 cache keys
2. `tryL1CacheLoad()` - Check L1 (main thread)
3. `tryL2CacheLoad()` - Check L2 (main thread)
4. Fetch if needed, then `populateL1Cache()` and `updateL2Cache()`

### Parallel Execution (`resolveParallel`)
1. **Main thread**: `prepareCacheKeys()` + `tryL1CacheLoad()` for all nodes
2. **Goroutines**: `tryL2CacheLoad()` + fetch via `loadFetchL2Only()`
3. **Main thread**: Merge results, populate L1 cache

**Rationale**: L1 is cheap (in-memory), check on main thread to skip goroutine work early. L2/fetch are expensive, run in parallel.

## L1Keys vs Keys

Built in `graphql_datasource.go:buildL1KeysVariable()`:
```go
for _, cfg := range p.dataSourcePlannerConfig.RequiredFields {
    // Only @key configs have empty FieldName
    // @requires/@provides have FieldName set
    if cfg.FieldName != "" {
        continue  // Skip @requires fields
    }
    // Include only @key fields for L1
}
```

## Self-Referential Entity Fix

**Problem**: When `User.friends` returns the same `User` entity, L1 cache causes pointer aliasing → stack overflow on merge.

**Solution**: `shallowCopyProvidedFields()` in `loader_json_copy.go` creates copies based on `ProvidesData` schema.

```go
// In tryL1CacheLoad:
ck.FromCache = l.shallowCopyProvidedFields(cachedValue, info.ProvidesData)
```

## ProvidesData and Validation

`FetchInfo.ProvidesData` describes what fields a fetch provides. Used by:
- `validateItemHasRequiredData()` - Check if cached entity is complete
- `shallowCopyProvidedFields()` - Copy only required fields

**Critical**: For nested entity fetches, `ProvidesData` must contain entity fields (`id`, `username`), NOT the parent field (`author`).

## configureFetchCaching Logic

```go
func configureFetchCaching(internal, external) FetchCacheConfiguration {
    // 1. Always preserve CacheKeyTemplate for L1
    result := FetchCacheConfiguration{CacheKeyTemplate: external.Caching.CacheKeyTemplate}

    // 2. Check global disable
    if v.Config.DisableEntityCaching { return result }

    // 3. Determine fetch type FIRST
    if external.RequiresEntityFetch || external.RequiresEntityBatchFetch {
        // Entity fetch: all rootFields same type, use first
        entityTypeName := internal.rootFields[0].TypeName
        cacheConfig := fedConfig.EntityCacheConfig(entityTypeName)
    } else {
        // Root field fetch: need exactly 1 rootField
        if len(internal.rootFields) != 1 { return result }
        cacheConfig := fedConfig.RootFieldCacheConfig(rootField.TypeName, rootField.FieldName)
    }
}
```

## Unit Testing

```go
// Standard test setup
ctrl := gomock.NewController(t)
defer ctrl.Finish()

ds := NewMockDataSource(ctrl)
ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
    DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
        return []byte(`{"data":{...}}`), nil
    }).Times(1)

loader := &Loader{caches: map[string]LoaderCache{"default": cache}}

// REQUIRED: Disable singleFlight for unit tests
ctx := NewContext(context.Background())
ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
ctx.ExecutionOptions.Caching = CachingOptions{EnableL1Cache: true, EnableL2Cache: true}

// REQUIRED: Always use arena
ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
resolvable := NewResolvable(ar, ResolvableOptions{})
resolvable.Init(ctx, nil, ast.OperationTypeQuery)

err := loader.LoadGraphQLResponseData(ctx, response, resolvable)
out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
```

### FakeLoaderCache
Test mock in `cache_load_test.go` with TTL support and operation logging.

### Assertions

**IMPORTANT**: Always use exact assertions in cache tests. Never use vague comparisons.

```go
// GOOD: Exact values - always preferred
assert.Equal(t, 3, hitCount, "should have exactly 3 L1 hits")
assert.Equal(t, int64(12), l1HitsInt, "should have exactly 12 L1 hits")
assert.Equal(t, 2, accountsCalls, "should call accounts subgraph exactly twice")

// BAD: Never use vague comparisons
assert.GreaterOrEqual(t, hitCount, 1)  // DON'T DO THIS
assert.Greater(t, l1HitsInt, int64(0)) // DON'T DO THIS
assert.LessOrEqual(t, calls, 5)        // DON'T DO THIS
```

Exact assertions catch regressions that vague assertions miss. If the expected value changes, update the test to reflect the new exact value.

## Federation Test Setup

Test services: `accounts`, `products`, `reviews` in `execution/federationtesting/`

### Testing Entity Caching vs @provides
```graphql
type Review {
    # @provides - gateway trusts subgraph, NO entity resolution
    author: User! @provides(fields: "username")

    # No @provides - gateway MUST resolve via _entities
    # Use for testing L1/L2 caching
    authorWithoutProvides: User!
}
```

### Run Tests
```bash
go test -run "TestL1Cache" ./v2/pkg/engine/resolve/... -v
go test -run "TestFederationCaching" ./execution/engine/... -v
go test -race ./execution/engine/... -v  # Race detector
```

## astjson API Reference

```go
// Create values on arena
astjson.ObjectValue(arena)
astjson.ArrayValue(arena)
astjson.StringValue(arena, string)
astjson.StringValueBytes(arena, []byte)
astjson.NumberValue(arena, string)
astjson.TrueValue(arena)
astjson.FalseValue(arena)
astjson.NullValue  // Global constant (not a function)

// Manipulate
value.Set(arena, key, val)
value.SetArrayItem(arena, idx, val)
value.Get(keys...)
value.GetArray()
value.GetStringBytes()
value.MarshalTo([]byte)
value.Type()  // TypeNull, TypeTrue, TypeObject, etc.
```

## LoaderCache Interface

```go
type LoaderCache interface {
    Get(ctx context.Context, keys []string) ([]*CacheEntry, error)
    Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error
    Delete(ctx context.Context, keys []string) error
}

type CacheEntry struct {
    Key   string
    Value []byte  // JSON-encoded entity
}
```
