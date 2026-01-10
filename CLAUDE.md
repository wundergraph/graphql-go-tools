# Claude Code Project Context

> **IMPORTANT**: In every future session, learnings and user feedback should automatically be added to this file to continuously improve collaboration. When discovering new patterns, important code structures, or receiving user corrections/preferences, update this document accordingly.

## Project Overview

This is the `graphql-go-tools` repository - a GraphQL engine implementation in Go that supports GraphQL Federation. The codebase is organized into two main versions:
- `v2/` - The current/modern implementation
- Legacy code at the root level

## Key Architecture

### Plan Building (`v2/pkg/engine/plan/`)
- `SynchronousResponsePlan` wraps a `*resolve.GraphQLResponse` for query/mutation execution
- The `Planner` orchestrates plan creation through AST walking
- `Visitor` builds the response structure during the AST walk
- DataSource planners (like GraphQL datasource) implement `ConfigureFetch()` to create fetch configurations

### Resolution (`v2/pkg/engine/resolve/`)
- **Resolver**: Event loop orchestrating GraphQL resolution
- **Loader**: Executes fetch operations, manages caching, handles entity resolution
- **Resolvable**: Holds response data being built

### Caching System
- `LoaderCache` interface: `Get`, `Set`, `Delete` methods
- `CacheKeyTemplate` interface with implementations:
  - `RootQueryCacheKeyTemplate` - for root query fields
  - `EntityQueryCacheKeyTemplate` - for federation entity queries
- `FetchCacheConfiguration` on fetches controls caching behavior
- Cache keys are JSON strings like `{"__typename":"Product","key":{"id":"prod-1"}}`

## Testing Patterns

### Unit Testing in `resolve` Package
```go
// Standard test setup
ctrl := gomock.NewController(t)
defer ctrl.Finish()

// Create mock datasource
ds := NewMockDataSource(ctrl)
ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
    DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
        return []byte(`{"data":{...}}`), nil
    }).Times(1)

// Create loader
loader := &Loader{
    caches: map[string]LoaderCache{"default": cache},
}

// Create context - disable singleFlight for unit tests
ctx := NewContext(context.Background())
ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true

// Create resolvable with arena (ALWAYS use arena in tests)
ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
resolvable := NewResolvable(ar, ResolvableOptions{})
err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)

// Execute
err = loader.LoadGraphQLResponseData(ctx, response, resolvable)

// Get output
out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
```

### Important: Disable SingleFlight for Unit Tests
When unit testing the Loader directly, set `ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true` to avoid nil pointer issues with uninitialized `singleFlight`.

### Important: Always Use Arena When Creating Resolvable
Always provide an arena when creating a new Resolvable in tests:
```go
ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
resolvable := NewResolvable(ar, ResolvableOptions{})
```
The arena is used for memory allocation optimization. Never pass `nil` as the first argument to `NewResolvable`.

### FakeLoaderCache for Testing
A test mock cache implementation is available in `cache_load_test.go` that:
- Stores entries in memory with TTL support
- Logs all operations (get/set/delete) with hit/miss tracking
- Useful for verifying cache behavior in tests

### File Naming Conventions for Tests
- `*_test.go` - Standard Go test files
- `cache_key_test.go` - Tests for cache key generation
- `cache_load_test.go` - Tests for cache loading behavior
- `resolve_federation_test.go` - Federation-specific resolution tests

## Code Organization Preferences

### Test File Structure
1. Package declaration and imports at top
2. Test functions in the middle
3. Testing utilities (mocks, helpers) at the bottom

### GraphQL Response Structure
```go
response := &GraphQLResponse{
    Info: &GraphQLResponseInfo{
        OperationType: ast.OperationTypeQuery,
    },
    Fetches: Sequence(
        SingleWithPath(&SingleFetch{...}, "query"),
        SingleWithPath(&BatchEntityFetch{...}, "query.field", ArrayPath("field")),
    ),
    Data: &Object{
        Fields: []*Field{...},
    },
}
```

## Git Workflow
- Main branch: `master`
- Feature branches like `feat/add-caching-support`
- Use `git mv` for file renames to preserve history

## Key Files Reference

| File | Purpose |
|------|---------|
| `v2/pkg/engine/resolve/loader.go` | Main execution engine, caching integration |
| `v2/pkg/engine/resolve/caching.go` | Cache key templates |
| `v2/pkg/engine/resolve/fetch.go` | Fetch types and configurations |
| `v2/pkg/engine/resolve/resolvable.go` | Response data container |
| `v2/pkg/engine/plan/planner.go` | Query plan building |
| `v2/pkg/engine/plan/visitor.go` | AST walking for plan construction |
| `execution/engine/federation_caching_test.go` | E2E caching tests (reference) |

## Common Patterns

### Entity Fetch with Caching
```go
&SingleFetch{
    FetchConfiguration: FetchConfiguration{
        DataSource: ds,
        Caching: FetchCacheConfiguration{
            Enabled:          true,
            CacheName:        "default",
            TTL:              30 * time.Second,
            CacheKeyTemplate: &EntityQueryCacheKeyTemplate{
                Keys: NewResolvableObjectVariable(&Object{
                    Fields: []*Field{
                        {Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
                        {Name: []byte("id"), Value: &String{Path: []string{"id"}}},
                    },
                }),
            },
        },
    },
    Info: &FetchInfo{
        OperationType: ast.OperationTypeQuery,
        ProvidesData:  providesDataObject, // Required for cache skip validation
    },
}
```

### BatchEntityFetch Structure
```go
&BatchEntityFetch{
    Input: BatchInput{
        Header: InputTemplate{...},
        Items:  []InputTemplate{...},
        Separator: InputTemplate{...},
        Footer: InputTemplate{...},
    },
    DataSource: ds,
    Caching: FetchCacheConfiguration{...}, // Direct field, not nested
}
```

## Session History

### 2024-01-10: Entity Caching Unit Tests
- Created `cache_load_test.go` for unit testing GraphQL Federation entity caching
- Renamed `caching_test.go` to `cache_key_test.go` for clarity
- Implemented `FakeLoaderCache` mock for cache testing
- Key learnings:
  - `BatchEntityFetch.Caching` is a direct field, not nested in `FetchConfiguration`
  - Must disable `SubgraphRequestDeduplication` for unit tests without full Resolver setup
  - `resolvable.Init()` takes `(ctx, initialData []byte, operationType)` - initialData can be nil
  - **Always use arena when creating Resolvable**: Use `NewResolvable(arena, ResolvableOptions{})` not `NewResolvable(nil, ...)`
