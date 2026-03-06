# Resolve Package Reference

The `resolve` package is the execution core of the GraphQL engine. It takes a planned `GraphQLResponse` (response plan tree + fetch tree), executes subgraph fetches, and renders the final JSON response. Entity caching (L1/L2) is integrated directly into the fetch execution flow.

## Architecture Overview

Three components work together:

| Component | File | Responsibility |
|-----------|------|---------------|
| **Resolver** | `resolve.go` | Orchestration, concurrency, arena pools, subscriptions |
| **Loader** | `loader.go` | Fetch execution, caching, result merging |
| **Resolvable** | `resolvable.go` | Response data, two-pass rendering, error handling |

**End-to-end flow:**
```
Resolver.ResolveGraphQLResponse(ctx, response, data, writer)
  1. Acquire concurrency semaphore
  2. Create Loader + Resolvable from arena pool
  3. Resolvable.Init(ctx, data, operationType)
  4. Loader.LoadGraphQLResponseData(ctx, response, resolvable)
     └─ Walk fetch tree: sequence/parallel/single
        └─ For each fetch: cache check → subgraph request → merge result
  5. Resolvable.Resolve(ctx, response.Data, response.Fetches, writer)
     └─ Two-pass walk: validate+collect errors, then render JSON
```

## Resolver (resolve.go)

Resolver is a single-threaded event loop for subscriptions and an orchestrator for query/mutation resolution.

### Key Fields
```go
type Resolver struct {
    ctx                context.Context
    options            ResolverOptions
    maxConcurrency     chan struct{}           // Semaphore (buffered channel, default 32)
    resolveArenaPool   *arena.Pool            // Arena for Loader & Resolvable
    responseBufferPool *arena.Pool            // Arena for response buffering
    subgraphRequestSingleFlight *SubgraphRequestSingleFlight
    inboundRequestSingleFlight  *InboundRequestSingleFlight
    triggers           map[uint64]*trigger    // Subscription triggers
    events             chan subscriptionEvent  // Subscription event loop
}
```

### Entry Points

**ResolveGraphQLResponse** — standard resolution:
```go
func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (*GraphQLResolveInfo, error)
```

**ArenaResolveGraphQLResponse** — optimized with inbound request deduplication:
```go
func (r *Resolver) ArenaResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, writer io.Writer) (*GraphQLResolveInfo, error)
```
Uses two separate arenas (resolve + response buffer). The resolve arena is freed early before I/O. Inbound deduplication: leader executes, followers wait and reuse buffered response.

**ResolveGraphQLSubscription** — long-lived subscription:
```go
func (r *Resolver) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer SubscriptionResponseWriter) error
```

### ResolverOptions

Key fields on `ResolverOptions`:
- `MaxConcurrency` — semaphore size (default 32, ~50KB per concurrent resolve)
- `Caches map[string]LoaderCache` — named L2 cache instances
- `EntityCacheConfigs` — subgraph → entity type → invalidation config (for extension-based invalidation)
- `PropagateSubgraphErrors`, `SubgraphErrorPropagationMode` — error handling
- `ResolvableOptions` — Apollo compatibility flags
- `SubscriptionHeartbeatInterval` — heartbeat interval (default 5s)

## Loader (loader.go)

The Loader executes fetches and merges results into the Resolvable's data. Caching is embedded in the fetch execution flow.

### Key Fields
```go
type Loader struct {
    resolvable   *Resolvable
    ctx          *Context
    caches       map[string]LoaderCache     // Named L2 cache instances
    l1Cache      *sync.Map                  // Per-request entity cache (key→*astjson.Value)
    jsonArena    arena.Arena                // NOT thread-safe, main thread only
    singleFlight *SubgraphRequestSingleFlight
    enableMutationL2CachePopulation bool   // Set per-mutation, inherited by entity fetches
    entityCacheConfigs map[string]map[string]*EntityCacheInvalidationConfig
}
```

### Fetch Tree Execution

`LoadGraphQLResponseData` is the entry point. It dispatches on the fetch tree:

```go
func (l *Loader) resolveFetchNode(node *FetchTreeNode) error {
    switch node.Kind {
    case FetchTreeNodeKindSingle:    return l.resolveSingle(node.Item)
    case FetchTreeNodeKindSequence:  return l.resolveSerial(node.ChildNodes)
    case FetchTreeNodeKindParallel:  return l.resolveParallel(node.ChildNodes)
    }
}
```

### Sequential Execution (resolveSerial)

Each fetch waits for the previous one to complete:
```go
for i := range nodes {
    err := l.resolveFetchNode(nodes[i])
}
```

### Parallel Execution (resolveParallel) — 4-Phase Model

The most sophisticated part. Handles L1/L2 cache with thread-safe analytics:

**Phase 1: Prepare + L1 Check (Main Thread)**
- `prepareCacheKeys()` — generate L1 and L2 cache keys for each fetch
- `tryL1CacheLoad()` — check sync.Map for entity hits
- If L1 complete hit → set `cacheSkipFetch = true`, skip goroutine

**Phase 2: L2 + Fetch (Goroutines via errgroup)**
- `loadFetchL2Only()` for fetches not cached in L1
- Checks L2 cache (thread-safe), fetches from subgraph if needed
- Accumulates analytics in per-result slices (goroutine-safe)

**Phase 3: Merge Analytics (Main Thread)**
- Merge L2 analytics events from per-result slices into collector
- Merge entity sources, fetch timings, error events

**Phase 4: Merge Results (Main Thread)**
- `mergeResult()` — parse response JSON, merge into Resolvable data
- `callOnFinished()` — invoke LoaderHooks
- Populate L1 and L2 caches

**Why this design?** L1 is cheap (in-memory sync.Map) — check on main thread to skip goroutine work early. L2/fetch are expensive — run in parallel goroutines.

### Result Merging

After a fetch completes, `mergeResult` does:
1. Check for errors in subgraph response
2. Handle auth/rate-limit rejections
3. Parse response JSON into arena-allocated values
4. Merge into items using `astjson.MergeValuesWithPath`
5. For batch entities: map response items back to original items via `batchStats`
6. Run cache invalidation (mutations, extensions)
7. Populate L1 and L2 caches

### LoaderHooks

```go
type LoaderHooks interface {
    OnLoad(ctx context.Context, ds DataSourceInfo) context.Context
    OnFinished(ctx context.Context, ds DataSourceInfo, info *ResponseInfo)
}
```
Called before/after each fetch. `OnLoad` returns a context passed to `OnFinished`. Not called when fetch is skipped (null parent, auth rejection).

### DataSource Interface

```go
type DataSource interface {
    Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error)
    LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error)
}
```

## Resolvable (resolvable.go)

Holds the response data and renders it to JSON using a two-pass tree walk.

### Key Fields
```go
type Resolvable struct {
    data         *astjson.Value    // Root response object (arena-allocated)
    errors       *astjson.Value    // Errors array (lazily initialized)
    astjsonArena arena.Arena       // Shared with Loader, NOT thread-safe
    print        bool              // false=pre-walk, true=print-walk
    out          io.Writer         // Output for print pass
    path         []fastjsonext.PathElement  // Current JSON path
    depth        int
    operationType ast.OperationType

    // Entity cache analytics (set during print phase)
    currentEntityAnalytics *ObjectCacheAnalytics
    currentEntityTypeName  string
    currentEntitySource    FieldSource
}
```

### Two-Pass Walk

**Pass 1 (pre-walk)**: `print = false`
- Traverse response plan tree, validate types
- Check field authorization
- Collect errors (null bubbling for non-nullable fields)
- Do NOT write output

**Pass 2 (print-walk)**: `print = true`
- Traverse again, write JSON to output
- Record entity cache analytics during rendering
- Hash field values for staleness detection

### walkObject (core method)

```
1. Navigate to object in JSON: value = parent.Get(obj.Path...)
2. Null check: if nil and non-nullable → error with null bubbling
3. Type validation: check __typename against PossibleTypes
4. Entity analytics: extract key fields, record entity source (print phase only)
5. Walk all fields recursively: walkNode(field.Value, value)
6. Field authorization: skip unauthorized fields
```

### Error Handling Modes

- **ErrorBehaviorPropagate** (default): null bubbles up to nearest nullable parent
- **ErrorBehaviorNull**: field becomes null even if non-nullable
- **ErrorBehaviorHalt**: stop all execution on first error

## Response Plan Tree (Node Types)

The planner produces a tree of Node types describing the expected response shape.

### GraphQLResponse

```go
type GraphQLResponse struct {
    Data       *Object          // Response plan tree root
    Fetches    *FetchTreeNode   // Fetch execution tree
    Info       *GraphQLResponseInfo
    DataSources []DataSourceInfo
}
```

### Node Types

| Type | Fields | Purpose |
|------|--------|---------|
| `Object` | Path, Fields, Nullable, PossibleTypes, CacheAnalytics | Object with named fields |
| `Field` | Name, OriginalName, Value (Node), CacheArgs, OnTypeNames, Info | Named field in an object |
| `Array` | Path, Nullable, Item (Node), SkipItem | List of items |
| `String` | Path, Nullable, IsObjectID | String scalar |
| `Scalar` | Path, Nullable | Custom scalar (raw JSON) |
| `Boolean`, `Integer`, `Float`, `BigInt` | Path, Nullable | Typed scalars |
| `Enum` | Path, Nullable, TypeName, Values | Enumeration |
| `Null`, `EmptyObject`, `EmptyArray` | — | Constant nodes |
| `StaticString` | Path, Value | Constant string value |

### Field
```go
type Field struct {
    Name         []byte           // Output name (may be alias)
    OriginalName []byte           // Schema name (nil if Name IS original)
    Value        Node             // Nested response node
    CacheArgs    []CacheFieldArg  // Field arguments for cache key suffix (xxhash)
    OnTypeNames  [][]byte         // Fragment type conditions
    Info         *FieldInfo       // Metadata (type names, authorization, source tracking)
}
```

## Fetch Tree

The planner produces a separate tree for fetch execution.

### FetchTreeNode
```go
type FetchTreeNode struct {
    Kind       FetchTreeNodeKind  // Single | Sequence | Parallel
    Item       *FetchItem         // For Single nodes
    ChildNodes []*FetchTreeNode   // For Sequence/Parallel nodes
    Trigger    *FetchTreeNode     // For subscription triggers
}
```

### Fetch Types

| Type | Use Case | Key Fields |
|------|----------|------------|
| `SingleFetch` | Root fields, standalone queries | InputTemplate, DataSource, Caching |
| `EntityFetch` | Nested entity (single object) | EntityInput (Header, Item, Footer) |
| `BatchEntityFetch` | Nested entity (array) | BatchInput (Header, Items[], Separator, Footer) |

All fetch types carry `FetchCacheConfiguration` and `FetchInfo` (data source name, provides data, root fields).

### FetchCacheConfiguration
```go
type FetchCacheConfiguration struct {
    Enabled                          bool              // L2 enabled for this fetch
    CacheName                        string            // Cache instance name
    TTL                              time.Duration     // Cache entry lifetime
    CacheKeyTemplate                 CacheKeyTemplate  // Key generation template
    IncludeSubgraphHeaderPrefix      bool              // Prefix with header hash
    RootFieldL1EntityCacheKeyTemplates map[string]CacheKeyTemplate // Entity L1 keys for root fields
    EnablePartialCacheLoad           bool              // Fetch only missing entities
    UseL1Cache                       bool              // L1 enabled (set by postprocessor)
    ShadowMode                       bool              // Never serve cached data
    MutationEntityImpactConfig       *MutationEntityImpactConfig
    EnableMutationL2CachePopulation  bool              // Mutations populate L2
    HashAnalyticsKeys                bool              // Hash vs raw in analytics
    KeyFields                        []KeyField        // @key fields for analytics
}
```

## Entity Caching

### Architecture

| Cache | Storage | Scope | Key Fields | Thread Safety |
|-------|---------|-------|------------|---------------|
| **L1** | `sync.Map` in Loader | Single request | `@key` only | sync.Map |
| **L2** | External (`LoaderCache`) | Cross-request | `@key` only | Per-result accumulation |

**Key principle**: Both L1 and L2 use only `@key` fields for stable entity identity.

### LoaderCache Interface
```go
type LoaderCache interface {
    Get(ctx context.Context, keys []string) ([]*CacheEntry, error)
    Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error
    Delete(ctx context.Context, keys []string) error
}

type CacheEntry struct {
    Key          string
    Value        []byte          // JSON-encoded entity
    RemainingTTL time.Duration   // TTL from cache (0 = unknown)
}
```

### Cache Key Generation

**Entity keys** (via `EntityQueryCacheKeyTemplate`):
```json
{"__typename":"User","key":{"id":"123"}}
```

**Root field keys** (via `RootQueryCacheKeyTemplate`):
```json
{"__typename":"Query","field":"topProducts","args":{"first":5}}
```

**Key transformations** (applied in order):
1. Subgraph header hash prefix: `{headerHash}:{key}` (when `IncludeSubgraphHeaderPrefix = true`)
2. `L2CacheKeyInterceptor`: custom transform (e.g., tenant isolation)

**Entity field argument-aware keys**: Fields with arguments get xxhash suffix appended, so different argument values produce different cache entries.

### Cache Flow (Integrated into Loader Phases)

**Sequential (tryCacheLoad):**
```
prepareCacheKeys() → tryL1CacheLoad() → tryL2CacheLoad() → fetch → populateL1Cache() + updateL2Cache()
```

**Parallel (resolveParallel):**
```
Phase 1 (main): prepareCacheKeys + tryL1CacheLoad for all fetches
Phase 2 (goroutines): tryL2CacheLoad + fetch via loadFetchL2Only
Phase 3 (main): merge analytics from goroutines
Phase 4 (main): mergeResult + populateL1Cache + updateL2Cache
```

### Self-Referential Entity Fix

**Problem**: When `User.friends` returns `User` entities, L1 cache returns pointers to the same object → aliasing on merge → stack overflow.

**Solution**: `shallowCopyProvidedFields()` in `loader_json_copy.go` creates copies based on `ProvidesData` schema. Only fields required by the fetch are copied (shallow, not deep).

### ProvidesData and Validation

`FetchInfo.ProvidesData` describes what fields a fetch provides. Used by:
- `validateItemHasRequiredData()` — check if cached entity has all required fields
- `shallowCopyProvidedFields()` — copy only required fields for self-referential entities

**Critical**: For nested entity fetches, `ProvidesData` must contain entity fields (`id`, `username`), NOT the parent field (`author`).

### Cache Invalidation

**Extension-based** (`processExtensionsCacheInvalidation`):
Subgraphs return invalidation keys in response extensions:
```json
{"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}
```
Optimization: skips delete if the same key is being written by `updateL2Cache`.

**Mutation-based** (`MutationCacheInvalidationConfiguration`):
After mutation completes, delete L2 entry for the returned entity.

**Subscription-based** (`SubscriptionEntityPopulationConfiguration`):
- Populate mode: write entity data to L2 on each subscription event
- Invalidate mode (`EnableInvalidationOnKeyOnly`): delete L2 entry when subscription provides only @key fields

### Partial Cache Loading

- **Default** (`EnablePartialCacheLoad = false`): any cache miss → refetch ALL entities in batch
- **Enabled** (`EnablePartialCacheLoad = true`): only fetch missing entities, serve cached ones directly

### Shadow Mode

L2 reads and writes happen normally, but cached data is **never served**. Fresh data is always fetched from the subgraph and compared against the cached value. Used for staleness detection via `ShadowComparisonEvent`. L1 cache works normally (not affected by shadow mode).

### Cache Analytics

Enable via `ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true`. After execution, call `ctx.GetCacheStats()` to get `CacheAnalyticsSnapshot`.

**CacheAnalyticsSnapshot** contains:
- `L1Reads`, `L2Reads` — `[]CacheKeyEvent` (hit/miss/partial-hit per key)
- `L1Writes`, `L2Writes` — `[]CacheWriteEvent` (key, size, TTL)
- `FetchTimings` — `[]FetchTimingEvent` (duration, HTTP status, response size, TTFB)
- `ErrorEvents` — `[]SubgraphErrorEvent`
- `FieldHashes` — `[]EntityFieldHash` (xxhash of field values for staleness)
- `EntityTypes` — `[]EntityTypeInfo` (count and unique keys per type)
- `ShadowComparisons` — `[]ShadowComparisonEvent` (cached vs fresh comparison)
- `MutationEvents` — `[]MutationEvent` (mutation impact on cached entities)

**Convenience methods**: `L1HitRate()`, `L2HitRate()`, `CachedBytesServed()`, `EventsByEntityType()`.

**Thread safety**: Analytics are accumulated per-result in goroutines (`l2AnalyticsEvents`, `l2FetchTimings`, `l2ErrorEvents`), then merged on the main thread via `MergeL2Events()`, `MergeL2FetchTimings()`, `MergeL2Errors()`.

## Configuration Types

### Runtime Options (set per-request on Context)
```go
type CachingOptions struct {
    EnableL1Cache         bool                  // Per-request entity cache
    EnableL2Cache         bool                  // External cross-request cache
    EnableCacheAnalytics  bool                  // Detailed event tracking
    L2CacheKeyInterceptor L2CacheKeyInterceptor // Custom key transform
}

type L2CacheKeyInterceptor func(ctx context.Context, key string, info L2CacheKeyInterceptorInfo) string
type L2CacheKeyInterceptorInfo struct {
    SubgraphName string
    CacheName    string
}
```

### Plan-Time Configuration (in `plan/federation_metadata.go`)

Set per-subgraph via `SubgraphCachingConfig`:

| Type | Controls |
|------|----------|
| `EntityCacheConfiguration` | L2 caching for entity types (TypeName, CacheName, TTL, etc.) |
| `RootFieldCacheConfiguration` | L2 caching for root fields (TypeName, FieldName, EntityKeyMappings) |
| `MutationFieldCacheConfiguration` | Whether mutations populate L2 |
| `MutationCacheInvalidationConfiguration` | Which mutations delete L2 entries |
| `SubscriptionEntityPopulationConfiguration` | How subscriptions populate/invalidate L2 |

## Thread Safety Model

| Context | Operations | Safety Mechanism |
|---------|-----------|-----------------|
| Main thread | Arena allocation, L1 cache ops, result merging, two-pass rendering | Single-threaded |
| Goroutines (Phase 2) | L2 cache Get/Set/Delete, subgraph HTTP calls | Per-result accumulation slices |
| Analytics merge | Goroutine events → collector | Main thread merge after g.Wait() |
| L1 cache | Read/write entity values | sync.Map |

**Rule**: Never allocate on `jsonArena` from a goroutine. All arena-allocated JSON is created on the main thread.

## Arena Allocation

- Resolver owns `resolveArenaPool` and `responseBufferPool`
- All `*astjson.Value` nodes live on the shared arena (no GC pressure)
- Arena is NOT thread-safe → only main thread allocates
- **Early release pattern** (ArenaResolveGraphQLResponse): resolve arena freed before I/O, response arena freed after write
- Never store heap-allocated `*Value` in arena-owned containers (GC can't trace into arena noscan memory)

## Key Files

| File | Purpose |
|------|---------|
| `resolve.go` | Resolver: orchestration, concurrency, subscriptions |
| `loader.go` | Loader: fetch execution, parallel phases, result merging |
| `resolvable.go` | Resolvable: two-pass walk, JSON rendering |
| `loader_cache.go` | L1/L2 cache operations, LoaderCache interface, prepareCacheKeys, tryL1/L2CacheLoad, populateL1Cache, updateL2Cache |
| `loader_json_copy.go` | shallowCopyProvidedFields for self-referential entities |
| `caching.go` | CacheKeyTemplate, EntityQueryCacheKeyTemplate, RootQueryCacheKeyTemplate |
| `cache_analytics.go` | CacheAnalyticsCollector, CacheAnalyticsSnapshot, all event types |
| `extensions_cache_invalidation.go` | processExtensionsCacheInvalidation |
| `fetch.go` | Fetch types (SingleFetch, EntityFetch, BatchEntityFetch), FetchCacheConfiguration |
| `fetchtree.go` | FetchTreeNode tree structure |
| `node_object.go` | Object, Field node types |
| `node_array.go` | Array node type |
| `node.go` | Node interface, NodeKind constants |
| `context.go` | Context, CachingOptions, ExecutionOptions |
| `datasource.go` | DataSource, SubscriptionDataSource interfaces |
| `response.go` | GraphQLResponse, GraphQLResponseInfo |

## Testing Patterns

### Unit Test Setup
```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

ds := NewMockDataSource(ctrl)
ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
    DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
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

### Exact Assertions

**IMPORTANT**: Always use exact assertions. Never use vague comparisons.

```go
// GOOD: exact values
assert.Equal(t, 3, hitCount, "should have exactly 3 L1 hits")
assert.Equal(t, int64(12), stats.L1Hits, "should have exactly 12 L1 hits")
assert.Equal(t, 2, accountsCalls, "should call accounts subgraph exactly twice")

// BAD: hides regressions
assert.GreaterOrEqual(t, hitCount, 1)  // DON'T DO THIS
assert.Greater(t, stats.L1Hits, int64(0)) // DON'T DO THIS
```

### Snapshot Comments

Every event line in a `CacheAnalyticsSnapshot` assertion MUST have a brief comment explaining **why** that event occurred:

```go
// GOOD: explains the "why"
L2Reads: []resolve.CacheKeyEvent{
    {CacheKey: keyUser, Kind: resolve.CacheKeyMiss, ...}, // First request, L2 empty
    {CacheKey: keyUser, Kind: resolve.CacheKeyHit, ...},  // Populated by Request 1
},

// BAD: restates the field value
{CacheKey: keyUser, Kind: resolve.CacheKeyMiss, ...}, // this is a miss
```

### Cache Log Rule

Every `defaultCache.ClearLog()` MUST be followed by `defaultCache.GetLog()` with full assertions BEFORE the next `ClearLog()` or end of test. Never clear a log without verifying its contents.

### Run Tests
```bash
go test -run "TestL1Cache" ./v2/pkg/engine/resolve/... -v
go test -run "TestFederationCaching" ./execution/engine/... -v
go test -race ./v2/pkg/engine/resolve/... -v
```

## astjson Quick Reference

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

// Navigate
value.Get(keys...)       // Navigate nested path
value.GetArray()         // Get array items
value.GetStringBytes()   // Get string as []byte
value.Type()             // TypeNull, TypeTrue, TypeObject, etc.

// Mutate
value.Set(arena, key, val)          // Set object field
value.SetArrayItem(arena, idx, val) // Set array item

// Serialize
value.MarshalTo([]byte)  // Append JSON to buffer
```
