# Directive Specification: Root-field caching config

> Part of the entity-caching re-implementation document set.
> Cross-links: [adr/0007-root-field-cache-config.md](../adr/0007-root-field-cache-config.md),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).
> Re-implementation PR: gqtools PR 4 + PR 8 / PR-CACHE-CONFIG.

This document specifies the **Root-field caching config**.
It is written for a reader with no prior knowledge of the feature.
Read [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) first for the L1/L2 model,
the StructuralCopy invariants,
and the cache-key model that this spec builds on.

---

## 1. Purpose & responsibility

The root-field caching config caches the **whole response of a root field** — for example `Query.topProducts` or `Query.product` — in L2 so that a second identical request can be served from the external cache without calling the subgraph at all.
Its distinctive capability is `EntityKeyMapping`:
a declarative binding that rewrites the root-field cache key into **entity key shape** so that a root query such as `product(upc: "top-1")` and a nested `_entities` fetch for `Product{upc:"top-1"}` resolve to the **same** L2 cache entry and therefore share data.
When the mapped argument is a **list** marked `ArgumentIsEntityKey`, the config additionally unlocks three batch optimisations:
one cache key per list element,
an empty-list (or null) short-circuit that returns an empty response without ever calling the resolver,
and a partial-fetch mode that fetches only the missing entities and serves the rest from cache.
The config is a Go configuration concept supplied per subgraph, **not** a GraphQL wire directive,
and it is consumed by the planner caching state and the resolver root-field cache path.

---

## 2. Configuration definition

There is **no SDL syntax**.
The config is a Go struct attached per subgraph via `SubgraphCachingConfig.RootFieldCaching` and consumed by the planner.

The router-facing plan-level shape (in `v2/pkg/engine/plan/federation_metadata.go`):

```go
type RootFieldCacheConfiguration struct {
    TypeName                    string             // "Query" (the root type containing the field)
    FieldName                   string             // "topProducts", "product", "products"
    CacheName                   string             // names a registered LoaderCache instance
    TTL                         time.Duration      // entry lifetime
    IncludeSubgraphHeaderPrefix bool               // header-hash prefix on the key
    EntityKeyMappings           []EntityKeyMapping // optional: derive entity-shaped keys
    ShadowMode                  bool               // read/write L2 but never serve cached data
    PartialBatchLoad            bool               // batch list mode: fetch only missing IDs
}

type EntityKeyMapping struct {
    EntityTypeName string          // entity type the root field returns, e.g. "Product"
    FieldMappings  []FieldMapping  // one mapping per @key field
}

type FieldMapping struct {
    EntityKeyField      string   // the @key field name on the entity, e.g. "upc"
    ArgumentPath        []string // path into ctx.Variables for the argument value
    ArgumentIsEntityKey bool     // list element ↔ entity 1:1 correspondence (batch)
}
```

Lookup helpers travel with the collection type:
`RootFieldCacheConfigurations.FindByTypeAndField(typeName, fieldName)` returns `nil` when caching is not configured for a root field (caching is opt-in).

`ArgumentPath` notes the re-implementation must honor:
- It uses the same `[]string` format as `ContextVariable.Path`:
  object keys are `["id"]` or `["input","userId"]`,
  an array index is a decimal string segment `["ids","0"]`.
- It is subject to `ctx.RemapVariables` on its first segment only (top-level variable names are remapped, nested input-object field names are not).
  A path `["a","ids"]` with `RemapVariables["a"] == "input"` must become `["input","ids"]`, not be left unchanged.
- It is the **variable** path, not the schema argument name.
  A historical bug used the schema argument name (`"upc"`) instead of the remapped variable path, which broke cache sharing — the re-implementation must resolve through `RemapVariables`.

The internal resolver-side mirror of these structs lives in `v2/pkg/engine/resolve/caching.go` as `EntityKeyMappingConfig` / `EntityFieldMappingConfig`, carried on the `RootQueryCacheKeyTemplate` (a `CacheKeyTemplate` implementation, see §5).
The plan→resolve translation is a flat field copy; no behavior is added at the boundary.

---

## 3. Composition rules & validation

There are **no composition or SDL validation rules**, because this is not a wire directive.
The config is synthesised by the router from per-subgraph configuration and handed to the engine.
The invariants the engine *relies on* (and that the router-side config builder must therefore guarantee) are:

- **At most one `ArgumentIsEntityKey` mapping per root field.**
  The resolver's `batchEntityKeyMapping()` returns the first list-keyed mapping and assumes uniqueness;
  supplying two would silently use only the first.
- **`FieldMapping.ArgumentPath` must be resolvable from request variables.**
  If a mapping's argument is missing or `null`, that mapping renders no key and is skipped;
  with multiple mappings the remaining ones still produce keys,
  with a single mapping the request is simply not cached for that key.
- **`EntityKeyField` must be an actual `@key` field of `EntityTypeName`** so the derived key collides with the entity fetch's key (this is the whole point of sharing).
  See [directives/key.md](key.md) for the entity-identity contract.
- **`ArgumentIsEntityKey` requires a list argument** to engage batch mode;
  a scalar value with the flag set falls back to the single-entity-key path.

`ShadowMode` on a root field is currently implemented for **entity fetches only** — set it on the root config for forward compatibility, but do not rely on root-field shadow comparison behavior beyond read/write-without-serve.

---

## 4. Runtime semantics

### 4.1 Plan time (annotate, do not touch the cache)

The planner reads `RootFieldCacheConfiguration` for the resolved root field and attaches a `FetchCacheConfiguration` to the root `SingleFetch`:
`Enabled` (L2 on), `CacheName`, `TTL`, `IncludeSubgraphHeaderPrefix`, `ShadowMode`, `EnablePartialCacheLoad` (from `PartialBatchLoad`),
and a `CacheKeyTemplate` of concrete type `RootQueryCacheKeyTemplate` carrying the `RootFields` (coordinate + args + the response key, i.e. alias-or-field-name) and the `EntityKeyMappings`.
The planner leaves `UseL1Cache = false`;
the L1-optimizer post-process pass owns that decision (see [01-ARCHITECTURE-SPEC.md §6](../01-ARCHITECTURE-SPEC.md)),
and root-field fetches generally do not read L1 (a root field has no prior entity data to key on, per [01-ARCHITECTURE-SPEC.md §2](../01-ARCHITECTURE-SPEC.md)).

### 4.2 Resolve time — single fetch path (`resolveSingle`)

For a root `SingleFetch` the loader runs, in order:

1. **Empty-list / null short-circuit (pre-cache).**
   If the template has a batch entity-key argument path (`ArgumentIsEntityKey` + list) and the argument resolves to `null` or `[]`,
   the loader returns an empty response via `mergeBatchEmptyResponse` **without** calling the resolver or any cache.
   This is a fetch-level optimisation gated by the config, not a cache read.
2. **`prepareCacheKeys` → L1 → L2 (`tryCacheLoad`).**
   Root fields skip L1 reads in practice; L2 is consulted via `tryL2CacheLoad`,
   which renders keys from `RootQueryCacheKeyTemplate.RenderCacheKeys` and issues a `Get`.
3. **HTTP** if not satisfied by cache.
4. **`mergeResult`** honoring the two-boolean contract (`cacheSkipFetch`, `cacheMustBeUpdated`) from [01-ARCHITECTURE-SPEC.md §7.2](../01-ARCHITECTURE-SPEC.md), then `updateL2Cache`.

### 4.3 Resolve time — parallel path (`resolveParallel`)

The root-field config slots into the existing four-phase machinery with no rewrite:

- **Phase 1** generates L1/L2 keys via `prepareCacheKeys`.
- **Phase 2-L2** (`bulkL2Lookup`) groups L2-eligible root fetches by cache instance, issues one bulk `Get`, parses verbatim on the Loader arena, and runs `applyRootFetchL2Results` to decide `cacheSkipFetch` per fetch.
- **Phase 2-HTTP** runs only fetches L2 did not cover.
- **Phase 4** merges and writes L2 via `updateL2Cache`.

All cache work is main-thread; goroutines do HTTP only.

### 4.4 Batch key rendering

`RootQueryCacheKeyTemplate.RenderCacheKeys` branches:
- **`EntityKeyMappings` present, batch list argument** → `tryRenderBatchEntityKeys` produces **one `CacheKey` per list element**, each with `BatchIndex` set to its position in the original argument list, and each rendered in entity key shape `{"__typename":"Product","key":{"upc":"top-1"}}`.
- **`EntityKeyMappings` present, scalar/derived** → `renderDerivedEntityKey` renders one entity-shaped key per mapping from the request arguments; missing/null arguments yield an empty key (skip caching for that mapping).
- **No `EntityKeyMappings`** → `renderField` renders the root-field-shape key `{"__typename":"Query","field":"topProducts","args":{...}}` (args present only when the field has arguments).

### 4.5 Partial batch fetch

When `PartialBatchLoad` (→ `EnablePartialCacheLoad`) is true and some batch keys hit while others miss:
- Cached entities are spliced into the response array at their `BatchIndex` via `mergeBatchCacheHit` / `mergeBatchPartialResponse`,
  each StructuralCopied before `SetArrayItem` to preserve cache isolation (see [01-ARCHITECTURE-SPEC.md §3](../01-ARCHITECTURE-SPEC.md) and the Copy Budget).
- Only the **missing** indices are sent to the subgraph.
  `cloneVariablesWithBatchIndices` clones `ctx.Variables` and rewrites the batch argument array to contain only the missing elements, so the subgraph receives a filtered list.
- Fresh results are interleaved back at their original positions.

When `PartialBatchLoad` is false (default), any miss in the batch refetches the **whole** list (all-or-nothing).

### 4.6 Alias & normalization handling

The cache key is **alias-independent** by construction:
keys are derived from `@key` fields and arguments, never from response aliases (see [01-ARCHITECTURE-SPEC.md §4](../01-ARCHITECTURE-SPEC.md)).
The **merge path** (where cached data is spliced into the response tree) *is* alias-aware:
`EntityMergePath` uses the root field's `ResponseKey` (the alias if present, else the schema field name).
For `u: user(id: $id)` the merge path is `["u"]`, not `["user"]`;
an explicit `PostProcessing.MergePath` overrides the derived key.
For batch responses the array's response key is likewise captured as the alias (`p` for `p: products(...)`, not `products`).

### 4.7 Smart cache-key backfill (EntityKeyMappings, write side)

When `EntityKeyMappings` produce multiple L2 keys on read and some miss, `updateL2Cache` makes **per-key** write decisions (no blanket rewrite):
- **Requested key** (rendered from request arguments): written on backfill / refresh, and on a skip-fetch path only when `fromCacheNeedsWriteback`.
- **Rendered key** (rendered from the final entity data): on the fetch path always written (subgraph is source of truth); on skip-fetch only for genuinely new keys.
This means if a request asked for `email:a@` but the entity actually has `email:b@`, the engine writes the `b@`-derived key and correctly skips the unproven `a@` key.
`WriteReason` (`refresh` / `backfill` / `derived`) is set on the resulting L2 write events.

### 4.8 Ordering / threading

The empty-list short-circuit runs before any cache or resolver call.
All key rendering, L1/L2 reads, merging, and L2 writes run on the **main thread**;
goroutines run subgraph HTTP only.
Per-request `Transform`s (used for normalization/denormalization of the merged entity shape) are ephemeral and must never be cached across requests (see [01-ARCHITECTURE-SPEC.md §3](../01-ARCHITECTURE-SPEC.md) and the resolve-package `CLAUDE.md`).

---

## 5. Cache key & data shape

**Key shapes produced** (all subject to the §4 key transform pipeline: `GlobalCacheKeyPrefix → subgraph header-hash prefix → L2CacheKeyInterceptor`):

| Config | Rendered key | Notes |
|---|---|---|
| No `EntityKeyMappings` | `{"__typename":"Query","field":"topProducts","args":{"first":5}}` | `args` object omitted when the field has none; args are deterministic |
| `EntityKeyMappings` (scalar) | `{"__typename":"Product","key":{"upc":"top-1"}}` | Same shape an `_entities` fetch produces → entries are shared |
| `EntityKeyMappings` (list, `ArgumentIsEntityKey`) | one `{"__typename":"Product","key":{"upc":"top-1"}}` per element | Each `CacheKey` carries its `BatchIndex` |

Number coercion in keys: `@key` values that arrive as numbers are coerced to strings (`setNestedKey`), so `id: 1` and `id: "1"` collide on the same entry (consistent with [01-ARCHITECTURE-SPEC.md §4](../01-ARCHITECTURE-SPEC.md)).

**Stored shape:**
- **With `EntityKeyMappings`** the stored value is the **entity shape**, projected (L2 projection, `Passthrough = false`) to the provided fields so it round-trips and is interchangeable with an `_entities` fetch's L2 entry.
- **Without `EntityKeyMappings`** the stored value is the **root-field response** value for that field.
- L2 always projects (not passthrough); L1 (when used) would passthrough — see [01-ARCHITECTURE-SPEC.md §3.4](../01-ARCHITECTURE-SPEC.md).
  L2 writes serialize to heap bytes via `MarshalTo` before handing to the backend (the heap boundary).

**Root-field L1 promotion (optional):**
when a root-field fetch returns entities that carry `RootFieldL1EntityCacheKeyTemplates`, the loader promotes them into the per-request L1 under their entity keys so a later entity fetch in the same request can short-circuit.
Promotion derives the entity-shaped sub-`Object` from the fetch's `ProvidesData` and is **silently skipped** when `ProvidesData` is nil (defense-in-depth against test-constructed fetches).

---

## 6. Interaction with the foundation seam and other directives

- **Foundation seam** ([01-ARCHITECTURE-SPEC.md §7](../01-ARCHITECTURE-SPEC.md)):
  this config rides entirely on the existing seam — it sets fields on `FetchCacheConfiguration`, supplies a `CacheKeyTemplate` (`RootQueryCacheKeyTemplate`), and flows through the same `cacheSkipFetch` / `cacheMustBeUpdated` two-boolean merge contract.
  No new interface is introduced.
  Copy isolation for every cached-into-response splice obeys the §3 StructuralCopy invariants and the Copy Budget.
- **`@key`** ([directives/key.md](key.md)):
  hard dependency.
  `EntityKeyMapping.EntityKeyField` must be a real `@key` field, and the derived key must be byte-identical to the entity fetch's key for sharing to work.
- **Entity caching config** ([directives/entity-cache-config.md](entity-cache-config.md)):
  the two share L2 entries when `EntityKeyMappings` is set, so they should generally point at the **same `CacheName`** and use compatible TTLs.
  Entity config comes first in the dependency order (root-field config reuses entity cache keys), per [02-DIRECTIVE-INVENTORY.md §3](../02-DIRECTIVE-INVENTORY.md).
- **`@provides`** ([directives/provides.md](provides.md)):
  the fetch's `ProvidesData *Object` drives the L2 projection of the stored entity shape and the widening check on read, and is required for root-field L1 promotion.
- **`@requires`** ([directives/requires.md](requires.md)):
  `@requires` fields are request-derived and must never be written into the cached shape — the projection excludes them.
- **Mutation / subscription configs** ([directives/mutation-cache-config.md](mutation-cache-config.md), [directives/subscription-cache-config.md](subscription-cache-config.md)):
  downstream — they invalidate or populate the same entity-shaped entries this config can create via `EntityKeyMappings`.

---

## 7. End-to-end test plan

All tests use **exact** assertions: `assert.Equal` on the full value, never `Contains` / `GreaterOrEqual` / fuzzy comparisons.
Inline every query, cache key, and expected JSON at the assertion site.
Multi-key / multi-event struct literals are formatted **one item per line**.
Every `ClearLog()` is followed by `GetLog()` + full assertions before the next clear or end of test.
E2E subtests under `execution/engine/` are **self-contained** (inline setup, no shared helpers) per [execution/engine/CLAUDE.md](../../entity-caching-v2/execution/engine/CLAUDE.md).
The federation services are `accounts`, `products`, `reviews`;
the products subgraph exposes `topProducts(first: Int = 5): [Product]`, `product(upc: String!): Product`, `products(upcs: [String!]!): [Product]`, with `type Product @key(fields: "upc")`.

### Case 1 — root-field response cache, miss then hit (no EntityKeyMappings)

- **Config:** `RootFieldCacheConfiguration{TypeName:"Query", FieldName:"topProducts", CacheName:"default", TTL:1*time.Minute}`.
- **Query (both requests):** `query { topProducts(first: 2) { upc name price } }`.
- **What is cached:** the whole `topProducts` response under key `{"__typename":"Query","field":"topProducts","args":{"first":2}}`.
- **Assertions:**
  - Request 1 and Request 2 produce byte-identical responses:
    `assert.Equal(t, `{"data":{"topProducts":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22}]}}`, out1)` and `assert.Equal(t, out1, out2)`.
  - After Request 1, assert the cache log with one `get` (miss) then one `set`, keys inline, then `ClearLog()`.
  - After Request 2, assert the cache log is a single `get` with `Hits: []bool{true}` keyed by the inline root-field key, then assert the products subgraph was called **exactly once** total (`assert.Equal(t, 1, productsCalls)`).

### Case 2 — EntityKeyMappings: root field and entity fetch share one L2 entry

- **Config:** `RootFieldCacheConfiguration{TypeName:"Query", FieldName:"product", CacheName:"default", TTL:1*time.Minute, EntityKeyMappings: []EntityKeyMapping{{EntityTypeName:"Product", FieldMappings: []FieldMapping{{EntityKeyField:"upc", ArgumentPath:[]string{"upc"}}}}}}`, plus an `EntityCacheConfiguration` for `Product` on the same `CacheName`.
- **Query 1 (root):** `query { product(upc: "top-1") { upc name } }`.
- **Query 2 (entity path through reviews):** a query whose plan resolves `Product{upc:"top-1"}` via an `_entities` fetch.
- **What is cached:** one entry under `{"__typename":"Product","key":{"upc":"top-1"}}`.
- **Assertions:**
  - `assert.Equal(t, `{"data":{"product":{"upc":"top-1","name":"Trilby"}}}`, out1)`.
  - `assert.Equal(t, []byte(`{"upc":"top-1","name":"Trilby"}`), cache.GetValue(`{"__typename":"Product","key":{"upc":"top-1"}}`))` — entity-shaped, byte-exact.
  - Query 2's entity fetch hits the **same** key: assert its cache log `get` has `Hits: []bool{true}` for the inline `{"__typename":"Product","key":{"upc":"top-1"}}` key, and assert the products subgraph entity resolver was **not** called.

### Case 3 — batch list keys, all-miss then all-hit

- **Config:** `RootFieldCacheConfiguration{TypeName:"Query", FieldName:"products", CacheName:"default", TTL:1*time.Minute, EntityKeyMappings: []EntityKeyMapping{{EntityTypeName:"Product", FieldMappings: []FieldMapping{{EntityKeyField:"upc", ArgumentPath:[]string{"upcs"}, ArgumentIsEntityKey:true}}}}}`.
- **Query (both):** `query($upcs:[String!]!){ products(upcs:$upcs){ upc name price } }` with variables `{"upcs":["top-1","top-2","top-3"]}`.
- **What is cached:** three entries, one per element, in entity shape.
- **Assertions:**
  - `assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, out)` for both requests, and `assert.Equal(t, out1, out2)`.
  - Request 1 cache log, vertical literals:
    ```go
    log := cache.GetLog()
    assert.Equal(t, "get", log[0].Operation)
    assert.Equal(t, []CacheLogItem{
        {Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false}, // first request, all miss
        {Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
        {Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: false},
    }, log[0].Items)
    assert.Equal(t, "set", log[1].Operation)
    // assert the three set keys inline here as well
    cache.ClearLog()
    ```
  - After populating, assert each stored entity value byte-exact:
    `assert.Equal(t, `{"upc":"top-1","name":"Trilby","price":11}`, string(cache.GetValue(`{"__typename":"Product","key":{"upc":"top-1"}}`)))` (and likewise for `top-2`, `top-3`).
  - Request 2 cache log: a single `get` with `Hit: true` for all three inline keys; assert the products subgraph was called exactly once (only on Request 1).

### Case 4 — partial batch fetch (`PartialBatchLoad: true`)

- **Config:** as Case 3 but `PartialBatchLoad: true`; pre-seed the cache with only `top-1` and `top-3`.
- **Query:** `products(upcs:["top-1","top-2","top-3"])`.
- **What happens:** only `top-2` is fetched from the subgraph; `top-1` and `top-3` are served from cache and spliced at their original positions.
- **Assertions:**
  - `assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, out)`.
  - Assert the subgraph received the **filtered** list (only `top-2`) — e.g. capture the subgraph request and `assert.Equal(t, []string{"top-2"}, sentUpcs)`.
  - Cache log: one `get` (hits for `top-1`,`top-3`, miss for `top-2`, vertical literal) then one `set` for `top-2` only.

### Case 5 — empty-list / null short-circuit

- **Config:** as Case 3 (`ArgumentIsEntityKey: true`).
- **Queries:** `products(upcs: [])` and `products(upcs: $upcs)` with `{"upcs":null}`.
- **What happens:** the loader returns an empty response without calling the resolver or the cache.
- **Assertions:**
  - `assert.Equal(t, `{"data":{"products":[]}}`, out)` (or the schema-correct empty/null shape — assert the exact bytes).
  - Assert the products subgraph was called **exactly zero** times: `assert.Equal(t, 0, productsCalls)`.
  - Assert the cache log is **empty** (`assert.Equal(t, []CacheLogEntry{}, cache.GetLog())`) — no `get`, no `set`.

### Case 6 — aliased root field merge path (unit, `resolve` package)

- **Setup:** `RootQueryCacheKeyTemplate` with a single root field whose `ResponseKey` is the alias.
- **Assertions:**
  - `assert.Equal(t, []string{"u"}, template.EntityMergePath(pp))` for `u: user(id: $id)` (alias wins).
  - `assert.Equal(t, []string{"user"}, template.EntityMergePath(pp))` when no alias is present.
  - `assert.Equal(t, []string{"data","user"}, template.EntityMergePath(pp))` when an explicit `MergePath` is configured (explicit wins over derived).

### Case 7 — variable remap on ArgumentPath (unit, `resolve` package)

- **Assertions** for `resolveArgumentVariablePath`:
  - `assert.Equal(t, []string{"input"}, got)` for single-segment remap `["id"]` with `RemapVariables["id"]=="input"`.
  - `assert.Equal(t, []string{"input","ids"}, got)` for multi-segment remap on the first segment only.
  - `assert.Equal(t, []string{"a","ids"}, got)` when the first segment is not remapped (pass-through).

---

## 8. Acceptance criteria

A reviewer can verify the re-implementation against this checklist:

- [ ] `RootFieldCacheConfiguration` and its `EntityKeyMapping` / `FieldMapping` sub-structs exist with the fields in §2 and `FindByTypeAndField` returns `nil` when unconfigured (caching opt-in).
- [ ] A root field with caching enabled and **no** `EntityKeyMappings` caches its whole response under `{"__typename":"<Type>","field":"<field>","args":{...}}` (args omitted when none); a second identical request hits L2 and skips the subgraph (Case 1).
- [ ] With `EntityKeyMappings`, the L2 key is rendered in **entity shape** and is byte-identical to the `_entities` fetch key, so root query and entity fetch **share** the entry (Case 2).
- [ ] `ArgumentIsEntityKey` + list argument renders **one cache key per element** with `BatchIndex` set; all-miss then all-hit behaves as in Case 3.
- [ ] `PartialBatchLoad: true` fetches **only** missing elements (filtered variable list via `cloneVariablesWithBatchIndices`) and splices cached entities at their original positions; `false` is all-or-nothing (Case 4).
- [ ] Empty-list `[]` or `null` batch argument short-circuits to an empty response with **zero** subgraph calls and **zero** cache operations (Case 5).
- [ ] The merge path uses the response **alias** when present, the schema name otherwise, and an explicit `MergePath` overrides both (Case 6).
- [ ] `ArgumentPath` resolves through `ctx.RemapVariables` on its first segment only, and number `@key` values are coerced to strings so `1` and `"1"` collide (Case 7 + §5).
- [ ] Per-key smart backfill: the rendered (entity-derived) key is written and the unproven requested key is skipped on value mismatch; `WriteReason` is set on `EntityKeyMappings` writes (§4.7).
- [ ] All cache work runs on the main thread; goroutines do HTTP only; every cached-into-response splice StructuralCopies first (Copy Budget honored, no cache/response aliasing).
- [ ] Stored shape is L2-projected (no `@requires`, no aliases) and round-trips identically with the entity-fetch entry; L2 writes go through `MarshalTo` to heap bytes.
- [ ] Tests follow the §7 assertion discipline: full-value `assert.Equal`, inline literals, vertical multi-key literals, `ClearLog()` always paired with a verified `GetLog()`.
