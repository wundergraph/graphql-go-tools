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
```text
Resolver.ResolveGraphQLResponse(ctx, response, writer)
  1. Inbound singleflight check — followers reuse leader's bytes verbatim
  2. Acquire concurrency semaphore
  3. Create Loader + Resolvable from arena pool
  4. Resolvable.Init(ctx, nil, operationType)
  4. Loader.LoadGraphQLResponseData(ctx, response, resolvable)
     └─ Walk fetch tree: sequence/parallel/single
        └─ For each fetch: cache check → subgraph request → merge result
  5. Resolvable.Resolve(ctx, response.Data, response.Fetches, responseBuf)
     └─ Two-pass walk: validate+collect errors, then render JSON
  6. Release resolve arena, then writer.Write(responseBuf.Bytes())
     └─ Releasing first frees ~50KB during the slow client I/O
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
func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, writer io.Writer) (*GraphQLResolveInfo, error)
```
Uses two separate arenas (resolve + response buffer). The resolve arena is freed early before I/O. Inbound deduplication: leader executes, followers wait and reuse buffered response. Followers receive the leader's shared state (e.g. propagated headers) via `Context.SetDeduplicationData` if configured.

Inbound dedup requires `ctx.Request.ID` and `ctx.VariablesHash` to be populated by the caller. The execution engine populates them via `WithInboundRequestDeduplication()`.

**ArenaResolveGraphQLResponse** — Deprecated. Thin wrapper that delegates to `ResolveGraphQLResponse`. Kept for backwards compatibility.

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
    l1Cache      map[string]*astjson.Value  // Per-request entity cache (key → *astjson.Value on jsonArena). Main-thread only; plain map, not sync.Map.
    jsonArena    arena.Arena                // NOT thread-safe, main thread only
    parser       astjson.Parser             // Reusable, main thread only; scratch slabs amortize across requests
    singleFlight *SubgraphRequestSingleFlight
    enableMutationL2CachePopulation bool   // Set per-mutation, inherited by entity fetches
    entityCacheConfigs map[string]map[string]*EntityCacheInvalidationConfig
}
```

- `l1Cache` stores `*astjson.Value` pointing into `l.jsonArena` directly.
  Both writes and reads StructuralCopy (see "Entity L1 Representation" below),
  so there is no separate byte-backed entry type.
- `transformEntries` and `transforms` are reusable slabs for ephemeral Transforms,
  resliced to `[:0]` before each use to amortize allocation.
- `parser` is a Loader-owned `astjson.Parser` used exclusively from the main thread
  to parse bulk L2 responses onto `l.jsonArena`.
  Its scratch slabs are retained across requests to amortize cost.
- There is no `goroutineArenas` field anymore —
  L2 parsing is now serialized on the main thread via `bulkL2Lookup`,
  so goroutines do not allocate JSON at all.

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

### Parallel Execution (resolveParallel) — Phases

All cache logic runs on the main thread.
Goroutines exist only for subgraph HTTP fetches.
The model is:

**Phase 1: Prepare + L1 Check (Main Thread)**
- `prepareCacheKeys()` — generate L1 and L2 cache keys for each fetch
- `tryL1CacheLoad()` — check `l1Cache` (plain map) for entity hits;
  every hit StructuralCopies the stored `*astjson.Value` onto `l.jsonArena`,
  applying a passthrough denormalize Transform when aliases are present
- If L1 complete hit → set `cacheSkipFetch = true`,
  skip L2 and goroutine

**Phase 1.5: @requestScoped Injection (Main Thread)**
- `tryRequestScopedInjection()` for each not-yet-skipped fetch
- When injection satisfies the fetch → set `fetchSkipped = true`,
  skip L2 and goroutine,
  record LoadSkipped and `cacheTraceRequestScopedHits`

**Phase 2L2: Bulk L2 Lookup (Main Thread)** — see "Bulk L2 Lookup" below
- `bulkL2Lookup()` — group L2-eligible fetches by cache instance,
  one bulk `cache.Get` per instance,
  parse results verbatim on `l.parser` → `l.jsonArena`,
  distribute parsed values back to per-fetch `l2CacheKeys[].FromCache`,
  run `applyEntityFetchL2Results` / `applyRootFetchL2Results`
  to decide `cacheSkipFetch` per fetch,
  accumulate analytics and cache trace attachments

**Phase 2HTTP: Parallel HTTP Fetches (Goroutines via errgroup)**
- `loadFetchHTTP()` for fetches not already skipped by L1, request-scoped, or L2
- Goroutines do HTTP only —
  no cache Gets, no parsing, no arena allocation.
  The byte body is returned to the main thread for parsing in Phase 4.

**Phase 3: Merge Analytics (Main Thread)**
- Merge per-result `l2AnalyticsEvents`, `l2EntitySources`, `l2FetchTimings`,
  `l2ErrorEvents`, `l2CacheOpErrors` into the collector.
  These slices now only contain write-side / HTTP events;
  L2 reads are already accumulated by `bulkL2Lookup` in Phase 2L2.

**Phase 3.5: Retry @requestScoped Injection (Main Thread)**
- Rerun `tryRequestScopedInjection()` for hints that became satisfiable after
  sibling fetches produced the hinted data.

**Phase 4: Merge Results (Main Thread)**
- `mergeResult()` — parse response JSON on `l.jsonArena`,
  merge into Resolvable data tree
- `callOnFinished()` — invoke LoaderHooks
- `populateL1Cache()` / `updateL2Cache()` — write caches using StructuralCopy
  (L1) / `MarshalToWithTransform` (L2)
- `exportRequestScopedFields()` — populate request-scoped L1 for sibling fetches

**Why main-thread cache work?**
L1 is a plain map read and written only on the main thread —
check on the main thread to skip goroutine work early.
L2 parsing is now also main-thread:
a single bulk Get per cache instance replaces N parallel per-fetch Gets,
the parser and arena are reused,
and the goroutine-arena pool (formerly needed to avoid racing on `l.jsonArena`)
is gone entirely.
Goroutines shrink to what actually benefits from parallelism — subgraph HTTP.

### Bulk L2 Lookup

`bulkL2Lookup(ctx, nodes, results)` is the main-thread entry point that replaced
per-fetch goroutine L2 reads.
It runs between Phase 1.5 and the HTTP-fetch goroutine launch.

Flow:

1. **Group by cache instance.** Walk `results`, collect each fetch's
   `l2CacheKeys[].Keys` into a `planEntry{cache, keys, owners}` keyed by
   `LoaderCache` identity.
   Fetches that are already skipped (L1 complete, @requestScoped) are excluded.
2. **One bulk `cache.Get` per plan.** For each `planEntry`,
   issue a single `plan.cache.Get(ctx, plan.keys)`.
   Timing is measured once per bulk Get and attributed to every fetch in the plan
   (via `l2FetchTimings` with the bulk duration).
3. **Parse verbatim on `l.parser` / `l.jsonArena`.**
   Each returned `*CacheEntry` is parsed into an `*astjson.Value` on the
   Loader's own arena via `l.parseL2Bytes`.
   No denormalize Transform is applied at parse time —
   the denormalize Transform is applied later at the materialization site (`applyEntityFetchL2Results` /
   `applyRootFetchL2Results`) using `StructuralCopyWithTransform`,
   so that the cache-shape value remains available for the writeback merge in `updateL2Cache`.
4. **Distribute results back.** `populateFromCacheBulk` walks each fetch's
   `l2CacheKeys[]` and attaches the parsed values to `FromCache` (and
   candidate slices for multi-candidate resolution).
5. **Decide `cacheSkipFetch`.** `applyEntityFetchL2Results` /
   `applyRootFetchL2Results` run validation against `ProvidesData` and
   set `cacheSkipFetch` for fetches whose L2 hits cover all items.

**Failure semantics — documented behavior change.**
The old per-fetch goroutine path isolated cache errors: a `Get` failure on one
fetch affected only that fetch.
Under `bulkL2Lookup`, a single `plan.cache.Get` now serves every fetch
whose `l2CacheKeys` route to the same cache instance —
if that bulk Get returns an error,
**all fetches in the batch fall back to subgraph**.
Each affected fetch is marked `cacheMustBeUpdated = true`,
its `cacheTraceL2GetError` is set,
and a `CacheOperationError` is recorded per fetch in `l2CacheOpErrors`.
This is considered acceptable because production cache backends rarely fail partially;
the win is removing a goroutine per fetch and a per-goroutine arena per batch.

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

```text
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
| **L1** | Plain `map[string]*astjson.Value` in Loader | Single request | `@key` only | Main-thread only — no locking required |
| **L2** | External (`LoaderCache`) | Cross-request | `@key` only | Main-thread bulk Get + per-result write-side accumulation |

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
```text
prepareCacheKeys() → tryL1CacheLoad() → tryL2CacheLoad() → fetch → populateL1Cache() + updateL2Cache()
```
The sequential path still uses `tryL2CacheLoad` because there is no batch to bulk over.
It parses on `l.parser` / `l.jsonArena` just like `bulkL2Lookup`.

**Parallel (resolveParallel):**
```text
Phase 1   (main):       prepareCacheKeys + tryL1CacheLoad for all fetches
Phase 1.5 (main):       tryRequestScopedInjection (skip fetches whose data is already in requestScopedL1)
Phase 2L2 (main):       bulkL2Lookup — one cache.Get per cache instance,
                        parse verbatim on l.parser / l.jsonArena,
                        distribute results, decide cacheSkipFetch, attach cache trace
Phase 2HTTP (goroutines): loadFetchHTTP for remaining fetches — HTTP only,
                          no cache work, no JSON parsing
Phase 3   (main):       merge per-result analytics (write-side + HTTP) into the collector
Phase 3.5 (main):       retry tryRequestScopedInjection for late-satisfied hints
Phase 4   (main):       mergeResult + populateL1Cache + updateL2Cache + exportRequestScopedFields
```

### Entity L1 Representation

Entity L1 is pointer-backed via `*astjson.Value`.
Storage is always on `l.jsonArena`,
all reads and writes happen on the main thread,
and isolation from the response tree is guaranteed by always StructuralCopying on both sides of the cache.

**StructuralCopy semantics**: `l.parser.StructuralCopy` clones container nodes (objects, arrays)
on the arena while aliasing leaf nodes (strings, numbers, bools, nulls) from the source.
This is safe because all values within a request share the same arena lifetime.
Strings are always eagerly decoded during parsing (no lazy mutation),
making aliased leaf values safe for concurrent reads.

**Writes** (`populateL1Cache` + root-field promotion):
- L1 writes use `l.structuralCopyNormalizedPassthrough(value, fetchInfo)` —
  renames aliases to schema names but keeps ALL source fields
  (including @key fields not in ProvidesData).
  The passthrough behavior preserves field accumulation across fetches.
- With no alias / arg normalization → `l.parser.StructuralCopy(l.jsonArena, value)`
- With normalization needed → `l.parser.StructuralCopyWithTransform(l.jsonArena, value, xform)`
  where `xform` is built ephemeral with `Transform.Passthrough = true`
- Merging an incoming value into an existing L1 entry uses the
  **working-copy-and-swap** pattern:
  StructuralCopy the existing entry into a working copy,
  run `astjson.MergeValues(l.jsonArena, working, freshIncoming)` against the working copy,
  and `l1Cache.Store(key, working)` on success or `l1Cache.Store(key, freshIncoming)` on failure.
  The live entry pointer is never mutated in place,
  so a partial-mutation failure inside `MergeValues` cannot corrupt sibling L1 keys.

**Reads** (`tryL1CacheLoad` + `populateFromCache`):
- L1 reads use `l.structuralCopyDenormalizedPassthrough(stored, fetchInfo)` —
  restores aliases but keeps all accumulated fields from prior fetches.
- With no aliases → `l.parser.StructuralCopy(l.jsonArena, stored)` returns a fresh,
  mutable value owned by the current request arena.
- With aliases → `l.parser.StructuralCopyWithTransform(l.jsonArena, stored, xform)`
  re-applies aliases via an ephemeral passthrough Transform while producing an independent copy.
- Readers can freely mutate the returned value (merge into items, re-wrap, etc.)
  without affecting the cached entry.

**L2 writes** still use non-passthrough `l.structuralCopyNormalized` (projects to ProvidesData
fields only) since L2 entries must be minimal and self-contained.

StructuralCopy on the same arena is cheap —
a single tree walk with leaf aliasing, no byte round-trip, no parser invocation.
It gives a stronger isolation guarantee than the former byte-backed design
(which parsed on every read) and removes an entire class of arena-lifetime bugs
that used to require the goroutine-arena pool to paper over.

### Copy Budget

The minimum StructuralCopy count for each data flow,
verified by adversarial mutation tests in `loader_cache_copy_invariant_test.go`
and baseline benchmarks in `loader_cache_copy_bench_test.go` / `loader_noncaching_bench_test.go`.
Any PR that changes this budget must update both the tests and this table.

| Flow | Writes | Reads | Merge-into-response |
|------|--------|-------|---------------------|
| L1 write (`populateL1Cache`) | 1 (`structuralCopyNormalizedPassthrough`) | — | — |
| L1 read + merge (`tryL1CacheLoad` + `populateFromCache`) | — | 1 (`structuralCopyDenormalizedPassthrough`) | — |
| L2 write (`updateL2Cache`) | 1 (`MarshalToWithTransform` — byte-level, no Value copy) | — | — |
| L2 read + merge (`bulkL2Lookup` + `applyEntityFetchL2Results`) | — | 1 parse + 1 `structuralCopyDenormalized` per entity | — |
| Full L1 cache hit merge (`mergeResult` cacheSkipFetch, loader.go:1472) | — | (1 above) | 1 `StructuralCopy` per entity before `MergeValues` into response item |
| Partial-cache L1 merge (`mergeResult` partialCache, loader.go:1491) | — | (1 above) | 1 `StructuralCopy` per cached item before `MergeValues` |
| Batch L2 cache hit splice (`mergeBatchCacheHit`, loader.go:1220) | — | (L2 above) | 1 `StructuralCopy` per entity before `SetArrayItem` |
| Partial batch response interleave (`mergeBatchPartialResponse`, loader.go:1372) | — | (L2 above) | 1 `StructuralCopy` per cached entity before `SetArrayItem` |
| Entity L1 merge-into-existing working-copy-and-swap (`loader_cache.go:1647`, `:3110`) | 1 `StructuralCopy` of existing entry before in-place `MergeValues` | — | — |
| @requestScoped coordinate L1 inject/export | 1 per hint via `structuralCopyNormalized` / `structuralCopyDenormalized` | — | — |
| Non-caching fetch | — | — | **0** — one `ParseBytesWithArena` + `MergeValuesWithPath`, no copy |

**Why the response-tree merge copies are load-bearing**:
`astjson.MergeValues(dst, src)` aliases nested container nodes from `src` into `dst`.
Without a StructuralCopy isolating `src`, mutating a nested field under `dst`
(e.g., a subsequent fetch merging into the same response tree,
or the L1 merge-into-existing path writing back) corrupts the underlying cache entry.
Adversarial tests in `loader_cache_copy_invariant_test.go` verify each site by
mutating `mergedValue.Get("profile")` and asserting `FromCache` remains intact —
with any of the 4 copies removed, the `profile` nested container gets corrupted.

**Why working-copy-and-swap is load-bearing**:
`MergeValues` is non-atomic on failure. A partial mutation of a live L1 entry
would corrupt every sibling L1 key pointing at the same `*Value`.
Copy-merge-store is the only safe pattern.

**Absolute floor**: isolation between cache and response tree requires at least
one copy at the write boundary + one at the read boundary + one at the merge
boundary (because the read copy must survive `MergeValues` aliasing into the
response tree, which is a longer-lived writable structure than the cache entry).

**Root-field L1 promotion** (`populateL1CacheForRootFieldEntities`):
When a root-field fetch returns entities that have `RootFieldL1EntityCacheKeyTemplates`,
the loader promotes the entities into `l1Cache` under their entity cache keys
so a later entity fetch can short-circuit.
Promotion derives the entity-shaped sub-`Object` from `singleFetch.Info.ProvidesData`
via `batchEntityValidationObject(providesData, fieldPath)`,
builds a normalize Transform once per path group,
and stores a `StructuralCopyWithTransform`-ed entity on `l.jsonArena`.
If `singleFetch.Info.ProvidesData` is nil — typically because the planner ran with
`DisableFetchProvidesData = true` — promotion is silently skipped rather than
storing response-shape (aliased) values that would corrupt subsequent entity L1
reads. Production planners always populate `ProvidesData`, so this guard is
defense-in-depth against test/programmatic fetch construction.

### ProvidesData and Validation

`FetchInfo.ProvidesData` describes what fields a fetch provides. Used by:
- `validateItemHasRequiredData()` — check if cached entity has all required fields
- `buildNormalizeTransformForFetch()` / `buildDenormalizeTransformForFetch()` —
  derive per-fetch `astjson.Transform` descriptors from the `*Object` tree.
  The normalize Transform strips aliases and appends CacheArgs hash suffixes;
  the denormalize Transform is the inverse.
  Transforms are now ephemeral — built and consumed inline at each cache operation
  site via `l.structuralCopyNormalized()` / `l.structuralCopyDenormalized()`
  (and their passthrough variants for L1).
  The Loader has reusable `transformEntries []astjson.TransformEntry` and
  `transforms []astjson.Transform` slabs that are resliced to `[:0]` before each use.
  Driven by the astjson APIs
  (`StructuralCopyWithTransform`, `MarshalToWithTransform`, `ParseBytesWithTransform`).
  `Transform.Passthrough` — when true, source fields not listed in Entries or Forced
  are copied verbatim (no rename, no projection).
  Used by L1 writes/reads to preserve all entity fields while still renaming aliased fields.
- `shallowCopyProvidedFields()` — copy only required fields for shadow comparisons and request-scoped injection

**Critical**: For nested entity fetches, `ProvidesData` must contain entity fields (`id`, `username`), NOT the parent field (`author`).

**Union-based L1 optimization**: The postprocessor (`optimize_l1_cache.go`) computes the
UNION of ancestor providers' ProvidesData fields when checking if a fetch can read from L1.
If no single provider covers the consumer,
the union of all prior providers (same entity type, in dependency chain) is checked.
This enables L1 for fetches whose required fields are spread across multiple prior fetches.

**Request-scoped Transforms**: a Transform's OutputKey for any field with `CacheArgs`
depends on `l.ctx.Variables` and `l.ctx.RemapVariables`,
both of which are per-request state.
The same `*Field` on the same shared planner `*Object` therefore produces
different OutputKey suffixes in different requests.
Transforms are valid only for the request that built them
and MUST be ephemeral —
never cached on `*Object`, the plan tree, the `Resolver`, or anywhere else outliving a request.
Within one fetch, `cacheFieldName(field)` is deterministic,
so building Transforms once at the top of `prepareCacheKeys` and reusing for the rest of
the cache flow is sound.

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

### Smart Cache Key Backfill (Root Field EntityKeyMappings)

When `EntityKeyMappings` produces multiple L2 keys on read and some miss,
`updateL2Cache` makes precise per-key write decisions via `cacheKeysToExactRootFieldEntityEntries`.

Two independent write decisions per mapping:

1. **Requested key** (`shouldWriteRequestedKey`): the key rendered from request arguments.
   Written when it matches the rendered key (backfill) or on the fetch path (refresh).
   On skip-fetch, only written when `fromCacheNeedsWriteback`.
2. **Rendered key** (`shouldWriteRenderedKey`): the key rendered from final entity data.
   On the fetch path, always written — the subgraph is the source of truth.
   On the skip-fetch path, only written for genuinely new keys (missing or derived),
   not existing cached keys that would be redundantly rewritten.

This means a value mismatch (request asked for `email:a@` but entity has `email:b@`) writes
the `b@` key as a derived entry while correctly skipping the unproven `a@` key.

`hasMissingRequestedKeys` replaces the old `needsKeyBackfill` boolean with per-entity precision.
`cacheMustBeUpdated` is set optimistically before merge; exact filtering happens in `updateL2Cache`.

### Partial Cache Loading

- **Default** (`EnablePartialCacheLoad = false`): any cache miss → refetch ALL entities in batch
- **Enabled** (`EnablePartialCacheLoad = true`): only fetch missing entities, serve cached ones directly

### Shadow Mode

L2 reads and writes happen normally, but cached data is **never served**. Fresh data is always fetched from the subgraph and compared against the cached value. Used for staleness detection via `ShadowComparisonEvent`. L1 cache works normally (not affected by shadow mode).

### Cache Analytics

Enable via `ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true`. After execution, call `ctx.GetCacheStats()` to get `CacheAnalyticsSnapshot`.

**CacheAnalyticsSnapshot** contains:
- `L1Reads`, `L2Reads` — `[]CacheKeyEvent` (hit/miss/partial-hit per key)
- `L1Writes`, `L2Writes` — `[]CacheWriteEvent` (key, size, TTL, WriteReason for EntityKeyMappings writes)
- `FetchTimings` — `[]FetchTimingEvent` (duration, HTTP status, response size, TTFB)
- `ErrorEvents` — `[]SubgraphErrorEvent`
- `FieldHashes` — `[]EntityFieldHash` (xxhash of field values for staleness)
- `EntityTypes` — `[]EntityTypeInfo` (count and unique keys per type)
- `ShadowComparisons` — `[]ShadowComparisonEvent` (cached vs fresh comparison)
- `MutationEvents` — `[]MutationEvent` (mutation impact on cached entities)

**Convenience methods**: `L1HitRate()`, `L2HitRate()`, `L1HitCount()`, `L2HitCount()`, `CachedBytesServed()`, `EventsByEntityType()`.

**Thread safety**: L2 read events are accumulated by `bulkL2Lookup` on the main thread.
Write-side and HTTP events (`l2AnalyticsEvents`, `l2FetchTimings`, `l2ErrorEvents`,
`l2CacheOpErrors`, `l2EntitySources`) are accumulated per-result and merged into the
collector on the main thread after `g.Wait()` via `MergeL2Events()`,
`MergeL2FetchTimings()`, `MergeL2Errors()`, `MergeL2CacheOpErrors()`, and
`MergeEntitySources()`.

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

The model is intentionally simple:
**main thread parses, merges, and runs all cache logic;
goroutines do HTTP only.**

| Context | Operations | Safety Mechanism |
|---------|-----------|-----------------|
| Main thread | Arena allocation, parsing, L1 cache ops, bulk L2 Get + parse + distribute, result merging, two-pass rendering | Single-threaded |
| Goroutines (Phase 2HTTP) | Subgraph HTTP calls (byte body only) | No shared arena state; each goroutine returns a `[]byte` to its `*result` for main-thread parsing in Phase 4 |
| Analytics merge | Per-result write-side slices → collector | Main thread merge after `g.Wait()` (L2 read events are already accumulated on the main thread in Phase 2L2) |
| L1 cache | Read/write entity values | Plain map, main-thread only; values are pointer-stable because every write StructuralCopies first |

**Rule**: Never allocate on `jsonArena` from a goroutine.
HTTP goroutines must hand their response body back as `[]byte` for main-thread parsing.

## Arena Allocation

- Resolver owns `resolveArenaPool` and `responseBufferPool`
- All `*astjson.Value` nodes live on the shared arena (no GC pressure)
- Arena is NOT thread-safe → only main thread allocates
- **Early release pattern** (`ResolveGraphQLResponse`): resolve arena freed before I/O, response arena freed after write
- Never store heap-allocated `*Value` in arena-owned containers (GC can't trace into arena noscan memory)
- All parsed L2 values now live on `l.jsonArena` directly.
  There are no goroutine arenas and no cross-arena references in the response tree,
  so the old "MergeValues creates cross-arena references, arenas must outlive rendering"
  lifetime caveat no longer applies.

## Key Files

| File | Purpose |
|------|---------|
| `resolve.go` | Resolver: orchestration, concurrency, subscriptions |
| `loader.go` | Loader: fetch execution, parallel phases, result merging |
| `resolvable.go` | Resolvable: two-pass walk, JSON rendering |
| `loader_cache.go` | L1/L2 cache operations, LoaderCache interface, prepareCacheKeys, tryL1/L2CacheLoad, populateL1Cache, updateL2Cache |
| `loader_cache_transform.go` | StructuralCopy helpers: structuralCopyNormalized/Denormalized (+ passthrough variants), structuralCopyProjected, normalize/denormalize/project Transform builders |
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

### Caching Test / AC Sync Rule

**When modifying or adding caching-related tests**, you MUST also update `docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md` (from the repo root). Every AC must link to its covering tests with relative paths, line numbers, and test names. This applies to:
- New caching tests (add test links to the relevant AC)
- Changes to existing caching tests that affect which ACs are covered
- New ACs (must have at least one test link)

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

// Copy (methods on astjson.Parser)
parser.StructuralCopy(arena, value)                    // Clone containers, alias leaves
parser.StructuralCopyWithTransform(arena, value, xform) // Clone + rename/project fields

// Transform
astjson.Transform{
    Entries []TransformEntry  // Field rename/project rules
    Forced  []TransformEntry  // Always-included fields
    Passthrough bool          // true = copy unlisted fields verbatim (L1);
                              //        false = project to listed fields only (L2)
}
```

**String handling**: `Value.stringRaw` and `Value.stringHasEscapes` are removed.
Strings are always eagerly decoded during parsing.
`ensureDecodedString()` and the public `EnsureDecoded()` are removed.
`Value.stringNeedsEscape` is kept for `MarshalTo` optimization.
