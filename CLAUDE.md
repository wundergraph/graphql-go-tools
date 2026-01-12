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

### Assertion Best Practices
**Always use precise assertions over vague ones:**

```go
// BAD - vague, doesn't catch regressions
assert.GreaterOrEqual(t, callCount, 1, "should call subgraph")
assert.GreaterOrEqual(t, len(log), 1, "should have operations")
assert.True(t, hasHit, "should have cache hit")

// GOOD - precise, catches regressions immediately
assert.Equal(t, 2, callCount, "should call subgraph exactly twice")
assert.Equal(t, 6, len(log), "should have exactly 6 cache operations")
assert.Equal(t, 3, hitCount, "should have exactly 3 cache hits")
```

**Why this matters:**
- Vague assertions like `GreaterOrEqual(x, 1)` pass whether x is 1, 2, or 100
- If a refactor accidentally doubles subgraph calls, vague assertions won't catch it
- Precise assertions document expected behavior and catch unintended changes
- When tests fail, precise assertions make debugging easier

**Document the reasoning for expected values:**
```go
// Verify exact subgraph call counts:
// - Products: 1 call for topProducts query
// - Reviews: 2 calls (Product.reviews + User.coReviewers after @requires)
// - Accounts: 2 calls (authorWithoutProvides entity + coReviewers entities)
assert.Equal(t, 1, productsCallsL1Enabled, "Products subgraph called exactly once")
assert.Equal(t, 2, reviewsCallsL1Enabled, "Reviews subgraph called twice")
assert.Equal(t, 2, accountsCallsL1Enabled, "Accounts subgraph called twice")
```

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
| `v2/pkg/engine/resolve/loader.go` | Main execution engine, L1/L2 caching integration |
| `v2/pkg/engine/resolve/loader_json_copy.go` | Shallow copy functions for L1 cache (prevents self-reference stack overflow) |
| `v2/pkg/engine/resolve/caching.go` | Cache key templates (RenderL1CacheKeys, RenderL2CacheKeys) |
| `v2/pkg/engine/resolve/context.go` | Context with CachingOptions and CacheStats |
| `v2/pkg/engine/resolve/fetch.go` | Fetch types and configurations |
| `v2/pkg/engine/resolve/resolvable.go` | Response data container |
| `v2/pkg/engine/plan/planner.go` | Query plan building |
| `v2/pkg/engine/plan/visitor.go` | AST walking, ProvidesData generation, entity boundary detection |
| `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` | Federation planner, L1Keys building |
| `execution/engine/federation_caching_test.go` | E2E L1/L2 caching tests |
| `v2/pkg/engine/resolve/l1_cache_test.go` | L1 cache unit tests |
| `v2/pkg/engine/resolve/cache_key_test.go` | Cache key generation tests |

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

### 2025-01-12: L1/L2 Caching Implementation

#### L1/L2 Cache Architecture
- **L1 Cache**: Per-request, in-memory cache using `sync.Map` in `Loader.l1Cache`
  - Prevents redundant fetches for same entity within a single request
  - Only applies to entity fetches (not root fetches)
  - Uses L1Keys (only @key fields) for stable entity identity
  - No prefix needed (same request = same context)
- **L2 Cache**: External cache (e.g., Redis) via `LoaderCache` interface
  - Shares entity data across requests
  - Uses Keys (includes @key and @requires fields)
  - Uses optional prefix for subgraph header isolation

#### Cache Key Template Refactoring
`EntityQueryCacheKeyTemplate` now has explicit methods:
```go
// L1 cache - uses L1Keys template (only @key fields), no prefix
func (e *EntityQueryCacheKeyTemplate) RenderL1CacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value) ([]*CacheKey, error)

// L2 cache - uses Keys template (all fields), with prefix
func (e *EntityQueryCacheKeyTemplate) RenderL2CacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)

// Internal shared implementation
func (e *EntityQueryCacheKeyTemplate) renderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, keysTemplate *ResolvableObjectVariable, prefix string) ([]*CacheKey, error)
```

#### L1Keys vs Keys in EntityQueryCacheKeyTemplate
- **Keys**: Full entity representation (`@key` + `@requires` fields) - used for L2 cache
- **L1Keys**: Only `@key` fields (no `@requires`) - used for L1 cache for stable identity
- L1Keys are built in `graphql_datasource.go:buildL1KeysVariable()` by filtering RequiredFields where `FieldName == ""`

#### ProvidesData and Entity Boundary Fields
`FetchInfo.ProvidesData` describes what fields a fetch provides - used for cache validation.

**Critical**: For nested entity fetches, `ProvidesData` must contain entity fields (like `id`, `username`), NOT the parent field (like `author`).

The `isEntityBoundaryField` function in `visitor.go` detects entity boundaries by:
1. Normalizing response paths: `strings.ReplaceAll(responsePath, ".@", "")` removes array markers
2. Comparing current field path to normalized response path
3. When at boundary, creates new object for entity fields instead of adding parent field

#### Array Markers in Paths
Response paths use `.@` to mark array positions:
- `query.topProducts.@.reviews.@.author` = path through two arrays
- Must normalize for comparison: `query.topProducts.reviews.author`

#### resolveFieldValue Array Support
`resolveFieldValue` in `caching.go` now handles `*Array`:
```go
case *Array:
    arrayValue := data.Get(node.Path...)
    if arrayValue == nil || arrayValue.Type() != astjson.TypeArray {
        return nil
    }
    items := arrayValue.GetArray()
    resultArray := astjson.ArrayValue(a)
    resultIndex := 0
    for _, itemData := range items {
        resolvedItem := e.resolveFieldValue(a, node.Item, itemData)
        if resolvedItem != nil {
            resultArray.SetArrayItem(a, resultIndex, resolvedItem)
            resultIndex++
        }
    }
    return resultArray
```

#### Cache Stats Tracking
`Context` now tracks per-entity cache hits/misses:
```go
type CacheStats struct {
    L1Hits   int64
    L1Misses int64
    L2Hits   int64
    L2Misses int64
}

// Track in loader
l.ctx.trackL1Hit()
l.ctx.trackL1Miss()
l.ctx.trackL2Hit()
l.ctx.trackL2Miss()

// Retrieve after execution
stats := ctx.GetCacheStats()
```

#### Enabling L1/L2 Caching
```go
ctx.ExecutionOptions.Caching = CachingOptions{
    EnableL1Cache: true,  // Per-request entity cache
    EnableL2Cache: true,  // External cache
}
```

#### Key Files Modified
| File | Changes |
|------|---------|
| `v2/pkg/engine/resolve/context.go` | `CachingOptions`, `CacheStats`, tracking methods |
| `v2/pkg/engine/resolve/loader.go` | L1 cache (`sync.Map`), `tryCacheLoad`, `tryL1CacheLoadWithTracking`, `tryL2CacheLoad`, `populateL1Cache` |
| `v2/pkg/engine/resolve/caching.go` | `RenderL1CacheKeys`, `RenderL2CacheKeys`, `renderCacheKeys`, array support |
| `v2/pkg/engine/plan/visitor.go` | `isEntityBoundaryField` path normalization, `isEntityRootField` |
| `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` | `buildL1KeysVariable` |
| `execution/engine/execution_engine.go` | `WithCachingOptions`, `WithCacheStatsOutput` |

### Federation Testing Infrastructure

#### @provides Directive Behavior
The `@provides` directive tells the gateway that a subgraph CAN provide certain fields, so the gateway skips entity resolution for those fields. For `@provides` to work correctly:
1. The schema must declare `@provides(fields: "fieldName")` on the field
2. The resolver data must actually include the provided field values
3. Without data, the response will have empty values for provided fields

#### Testing Entity Resolution vs @provides
The reviews service schema has two approaches for the `author` field:
```graphql
type Review {
    # Uses @provides - gateway trusts reviews service to provide username
    # Does NOT trigger entity resolution from accounts
    author: User! @provides(fields: "username")

    # No @provides - gateway MUST fetch username via entity resolution from accounts
    # Use this for testing L1/L2 entity caching behavior
    authorWithoutProvides: User!
}
```

**Test file mapping:**
- `multiple_upstream.query` - Uses `author` field (tests `@provides` behavior)
- `multiple_upstream_without_provides.query` - Uses `authorWithoutProvides` (tests entity caching)

#### Reviews Service Data Setup
For `@provides` to work, reviews data must include usernames:
```go
// reviews/graph/reviews.go
var reviews = []*model.Review{
    {
        Body:    "A highly effective form of birth control.",
        Product: &model.Product{Upc: "top-1"},
        Author:  &model.User{ID: "1234", Username: "Me"},  // Include Username for @provides
    },
}
```

The `AddReview` mutation must also generate usernames to match accounts service patterns:
```go
// Generate username matching accounts service pattern for @provides
username := fmt.Sprintf("User %s", authorID)
if authorID == "1234" {
    username = "Me"
}
```

#### Key Federation Test Files
| File | Purpose |
|------|---------|
| `execution/engine/federation_integration_test.go` | Tests `@provides` behavior via `author` field |
| `execution/engine/federation_caching_test.go` | Tests L1/L2 caching via `authorWithoutProvides` |
| `execution/federationtesting/reviews/graph/schema.graphqls` | Review schema with both field variants |
| `execution/federationtesting/reviews/graph/reviews.go` | Static review data with usernames |
| `execution/federationtesting/testdata/queries/` | Query files for different test scenarios |

### Updating the Federation Test Environment

The federation test environment consists of three subgraph services:
- **accounts** - User entities with id, username, history
- **products** - Product entities with upc, name, price
- **reviews** - Review data linking users and products

#### Directory Structure
```
execution/federationtesting/
├── accounts/
│   ├── gqlgen.yml              # gqlgen configuration
│   ├── handler.go              # go:generate directive
│   └── graph/
│       ├── schema.graphqls     # GraphQL schema (edit this)
│       ├── schema.resolvers.go # Query/Mutation resolvers (implement here)
│       ├── entity.resolvers.go # Entity resolvers for federation
│       ├── model/
│       │   ├── models.go       # Custom model definitions (edit for complex types)
│       │   └── models_gen.go   # Auto-generated models (don't edit)
│       └── generated/          # Auto-generated code (don't edit)
├── products/                   # Same structure as accounts
├── reviews/                    # Same structure as accounts
└── testdata/queries/           # Query files for tests
```

#### Step-by-Step: Adding a New Field

1. **Edit the schema** (`graph/schema.graphqls`):
   ```graphql
   type Review {
       body: String!
       author: User! @provides(fields: "username")
       newField: String!  # Add your field
   }
   ```

2. **Regenerate gqlgen code** from the service directory:
   ```bash
   cd execution/federationtesting/reviews
   go generate ./...
   ```
   Or from repo root:
   ```bash
   go generate ./execution/federationtesting/reviews/...
   ```

3. **Implement the resolver** in `graph/schema.resolvers.go`:
   ```go
   // NewField is the resolver for the newField field.
   func (r *reviewResolver) NewField(ctx context.Context, obj *model.Review) (string, error) {
       return "value", nil
   }
   ```
   Note: gqlgen creates a stub; you fill in the implementation.

4. **Update static data** if needed (e.g., `graph/reviews.go`):
   ```go
   var reviews = []*model.Review{
       {
           Body:     "Review text",
           Author:   &model.User{ID: "1234", Username: "Me"},
           NewField: "static value",  // Add if stored in model
       },
   }
   ```

5. **Update models** if the field needs custom types (`graph/model/models.go`):
   ```go
   type Review struct {
       Body     string
       Author   *User
       NewField string  // Add to struct if not auto-generated
   }
   ```

#### Step-by-Step: Adding a New Entity Type

1. **Define the entity in schema** with `@key` directive:
   ```graphql
   type Order @key(fields: "id") {
       id: ID!
       items: [Product!]!
   }
   ```

2. **Regenerate code**: `go generate ./...`

3. **Implement entity resolver** in `graph/entity.resolvers.go`:
   ```go
   func (r *entityResolver) FindOrderByID(ctx context.Context, id string) (*model.Order, error) {
       return &model.Order{ID: id}, nil
   }
   ```

4. **Create model** in `graph/model/models.go`:
   ```go
   type Order struct {
       ID    string `json:"id"`
       Items []*Product
   }

   func (Order) IsEntity() {}  // Required for federation entities
   ```

#### Regenerating All Services
```bash
# From repo root - regenerate all federation test services
go generate ./execution/federationtesting/...
```

#### Common Issues

1. **"missing method" compiler error after generate**: Usually a false positive from IDE. Run `go build ./...` to verify.

2. **Entity not resolving**: Ensure model has `IsEntity()` method:
   ```go
   func (MyType) IsEntity() {}
   ```

3. **@provides not working**: Data must include the provided field values:
   ```go
   // Wrong - username will be empty
   Author: &model.User{ID: "1234"}
   // Correct - username provided
   Author: &model.User{ID: "1234", Username: "Me"}
   ```

4. **@external fields**: Fields marked `@external` come from other subgraphs. Don't try to resolve them locally unless using `@provides` or `@requires`.

#### Testing Changes
```bash
# Run federation integration tests
go test -run "TestFederationIntegration" ./execution/engine/... -v

# Run all federation tests
go test ./execution/engine/... -v

# Run with race detector
go test -race ./execution/engine/... -v
```

### Self-Referential Entity Stack Overflow Fix

#### The Problem
When L1 cache stores a pointer to an entity and a self-referential field (e.g., `User.sameUserReviewers` returning `[User]`) returns the same entity, both `key.Item` and `key.FromCache` can point to the same memory location. Calling `astjson.MergeValues(ptr, ptr)` causes infinite recursion → stack overflow.

**Trigger query:**
```graphql
query {
    topProducts {
        reviews {
            authorWithoutProvides {
                id
                username
                sameUserReviewers {  # Returns same User entity
                    id
                    username
                }
            }
        }
    }
}
```

#### The Solution: Shallow Copy
Create a shallow copy of cached values instead of using direct pointer assignment. The copy only includes fields specified in `ProvidesData`, breaking pointer aliasing.

**File: `v2/pkg/engine/resolve/loader_json_copy.go`**

Key functions:
- `shallowCopyProvidedFields(cached, providesData)` - Entry point
- `shallowCopyObject(cached, obj)` - Copies object fields recursively per schema
- `shallowCopyArray(cached, arr)` - Copies array elements per item schema
- `shallowCopyNode(cached, node)` - Dispatches based on Node type (Object/Array/Scalar)
- `shallowCopyScalar(cached)` - Creates actual copies of scalar values

**Usage in `loader.go:tryL1CacheLoad`:**
```go
// Before (caused stack overflow):
ck.FromCache = cachedValue

// After (creates shallow copy):
ck.FromCache = l.shallowCopyProvidedFields(cachedValue, info.ProvidesData)
```

#### Important: Copy Scalars, Not References
When copying astjson values, scalars must be actual copies, not references:
```go
func (l *Loader) shallowCopyScalar(cached *astjson.Value) *astjson.Value {
    switch cached.Type() {
    case astjson.TypeNull:
        return astjson.NullValue  // Global constant, safe
    case astjson.TypeTrue:
        return astjson.TrueValue(l.jsonArena)  // New value on arena
    case astjson.TypeFalse:
        return astjson.FalseValue(l.jsonArena)
    case astjson.TypeNumber:
        raw := cached.MarshalTo(nil)  // Get raw number string
        return astjson.NumberValue(l.jsonArena, string(raw))
    case astjson.TypeString:
        str := cached.GetStringBytes()
        return astjson.StringValueBytes(l.jsonArena, str)
    // ... handle Object/Array recursively
    }
}
```

#### astjson API Reference
```go
// Create values on arena
astjson.ObjectValue(arena)              // Empty object
astjson.ArrayValue(arena)               // Empty array
astjson.StringValue(arena, string)      // String from string
astjson.StringValueBytes(arena, []byte) // String from bytes
astjson.NumberValue(arena, string)      // Number from string representation
astjson.IntValue(arena, int)            // Number from int
astjson.FloatValue(arena, float64)      // Number from float
astjson.TrueValue(arena)                // Boolean true
astjson.FalseValue(arena)               // Boolean false
astjson.NullValue                       // Global null constant (not a function!)

// Manipulate values
value.Set(arena, key, val)              // Set object field
value.SetArrayItem(arena, idx, val)     // Set array item at index
value.Get(keys...)                      // Get nested value
value.GetArray()                        // Get array items as []*Value
value.GetStringBytes()                  // Get string as []byte
value.MarshalTo([]byte)                 // Serialize to bytes
value.Type()                            // Get TypeNull/TypeTrue/TypeObject/etc.
value.Object()                          // Get *Object for iteration
obj.Visit(func(key []byte, v *Value))   // Iterate object fields
```

#### Test: `TestL1CacheSelfReferentialEntity`
Located in `execution/engine/federation_caching_test.go`. Tests that self-referential entities don't cause stack overflow when L1 cache is enabled.

### Pending: L1/L2 Cache Refactoring Plan

A plan exists at `.claude/plans/radiant-gathering-scroll.md` for refactoring the cache lookup flow:

#### Current Issues
1. **Performance**: L1 (in-memory) and L2 (external) cache lookups happen together in `tryCacheLoad`. In parallel execution, L1 should be checked on main thread (cheap, can skip parallel work early) while L2 is checked in parallel goroutines.

2. **Race Condition**: `resolveParallel()` spawns goroutines that call cache stat tracking methods (`trackL1Hit`, `trackL2Miss`, etc.) using plain `int64++` which is NOT thread-safe.

#### Proposed Solution
Split `tryCacheLoad` into 3 functions:
- `prepareCacheKeys()` - Generate cache keys (main thread)
- `tryL1CacheLoad()` - Check L1 cache (main thread only, non-atomic stats)
- `tryL2CacheLoad()` - Check L2 cache (thread-safe with atomic stats)

Make L2 stats use `go.uber.org/atomic` (already in codebase):
```go
type CacheStats struct {
    L1Hits   int64           // Safe: main thread only
    L1Misses int64           // Safe: main thread only
    L2Hits   *atomic.Int64   // Thread-safe for parallel goroutines
    L2Misses *atomic.Int64   // Thread-safe for parallel goroutines
}
```

#### Verification
Run tests with race detector:
```bash
go test -race ./v2/pkg/engine/resolve/... -run "TestCacheStats" -v
```
