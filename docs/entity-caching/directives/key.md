# Directive Specification: `@key`

> Part of the entity-caching re-implementation document set.
> Cross-references:
> [adr/0002-key.md](../adr/0002-key.md) (decision record for this directive),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) (the integration seam, L1/L2 model, StructuralCopy invariants, cache-key model),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) (the directive taxonomy and PR mapping this spec stays consistent with).
>
> Re-implementation PR: **gqtools PR 3 — cache-key templates** (logical name `PR-CACHE-KEYS`).

---

## 0. Who this section is for

This document specifies how the entity-caching layer consumes the federation `@key` directive.
It assumes you have never seen this feature before.
`@key` is **not introduced** by entity caching — it is an existing federation primitive.
Entity caching *reads* it to decide cache identity, and the re-implementation must re-consume it correctly.
The one truly new directive in this feature is `@requestScoped`,
specified separately in [request-scoped.md](./request-scoped.md).

---

## 1. Purpose & responsibility

`@key` declares the set of fields that uniquely identifies an entity within the federated graph.
For entity caching it is load-bearing in two distinct ways.
First, the `@key` field set is the **sole source of cache-key identity** for both L1 (per-request) and L2 (cross-request) entity caches,
so two requests asking for the same `User` by the same `id` collide on exactly the same cache entry,
regardless of what else either query selects.
Second, the entity L1 projection runs in **passthrough** mode that explicitly *retains* `@key` fields even when the current query did not select them and they are absent from the fetch's `ProvidesData` shape,
because a later, wider entity fetch needs the key present in the L1 entry to merge correctly.
The caching layer **consumes** `@key`; it never redefines it.

---

## 2. SDL / configuration definition

### 2.1 The federation SDL directive (unchanged, consumed only)

```graphql
directive @key(fields: _FieldSet!, resolvable: Boolean = true) repeatable on OBJECT | INTERFACE
```

It appears in subgraph SDL on entity types, for example:

```graphql
type User @key(fields: "id") { id: ID! }
type Product @key(fields: "upc") { upc: String! }
```

The directive itself is owned by the federation composition pipeline.
Entity caching does not parse SDL — it receives the already-resolved key fields as Go data structures described next.

### 2.2 The plan-time data shape: `KeyField`

The planner pre-extracts the `@key` field set into a recursive `KeyField` tree (defined in `node_object.go`).
This is the engine-internal representation of `@key` that every downstream cache concern reads:

```go
type KeyField struct {
    Name     string
    Children []KeyField // non-nil for nested object key fields
}
```

Mapping from SDL to `KeyField`:

- `@key(fields: "id")` → `[]KeyField{{Name: "id"}}`
- `@key(fields: "sku upc")` (composite key) → `[]KeyField{{Name: "sku"}, {Name: "upc"}}`
- `@key(fields: "id address { city }")` (nested key) → `[]KeyField{{Name: "id"}, {Name: "address", Children: []KeyField{{Name: "city"}}}}`

`KeyField` carries **only** key fields — `__typename` is excluded by construction (it is added back at key-render time, see §5).

### 2.3 The cache-key template: `EntityQueryCacheKeyTemplate`

For an entity (`_entities`) fetch the planner attaches an `EntityQueryCacheKeyTemplate` (defined in `caching.go`).
Tiny signature:

```go
type EntityQueryCacheKeyTemplate struct {
    Keys     *ResolvableObjectVariable // @key fields ONLY (no @requires, no selected fields)
    TypeName string                    // plan-time fallback when __typename missing from data
}

func (*EntityQueryCacheKeyTemplate) IsEntityFetch() bool { return true }
```

`Keys` is an `Object`-tree variable whose fields are exactly the `@key` fields.
The comment on the field is the contract: *"Keys contains only `@key` fields (without `@requires` fields). Used for both L1 and L2 cache keys to ensure stable entity identity."*
`KeyFields()` on the template converts the embedded `Object` tree back into a `[]KeyField` for analytics, skipping `__typename`.

### 2.4 Where the template fits on a fetch

Each cacheable fetch carries a `FetchCacheConfiguration` (see §6 and [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §11).
For entity fetches its `CacheKeyTemplate` is an `EntityQueryCacheKeyTemplate`.
The configuration also surfaces the key fields directly via a `KeyFields []KeyField` field used by analytics.

---

## 3. Composition rules & validation

These rules are enforced by the **federation composition** pipeline, not by the caching layer.
Entity caching depends on them holding.

- **Repeatable.**
  A type may declare multiple `@key` directives (multiple alternative identifying field sets).
  Each declared key set can independently seed cache identity for the entity-fetch path that resolves by that key.
- **`fields` is a mandatory `_FieldSet`.**
  Composition requires the argument and validates that every named field exists on the type and that nested selections are valid.
- **Resolvability.**
  An entity must be *resolvable* (the default, or `resolvable: true`) to participate in entity fetches.
  A `@key(fields: "...", resolvable: false)` type (for example `type Product @key(fields: "upc", resolvable: false)` in the accounts test subgraph) declares identity but is **not** independently fetchable as an `_entities` target from that subgraph;
  the caching layer will not build an entity-fetch cache key against a non-resolvable key in that subgraph.
- **Key stability across subgraphs.**
  The same entity must use a consistent identity across subgraphs so that an entry written by one subgraph's fetch and read by another keys identically.
  This is a federation composition guarantee the cache relies on; the cache does not re-validate it.

The caching layer adds **no new composition validation** for `@key`.
It only reads the composed key fields.

---

## 4. Runtime semantics

This section maps `@key` consumption onto the planner and the four-phase parallel resolve flow described in [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §5.
The governing rule from the architecture spec applies throughout: **the main thread parses, merges, and runs all cache logic; goroutines do subgraph HTTP only.**
Cache-key rendering, L1 reads/writes, and L2 reads/writes all run on the main thread on the per-request arena.

### 4.1 Plan time (compile-time, once per operation)

- The planner's key-fields visitor walks each entity type's `@key` selection set and produces the `KeyField` tree and the `EntityQueryCacheKeyTemplate.Keys` `Object`.
- The template is attached to every entity (`_entities`) fetch's `FetchCacheConfiguration.CacheKeyTemplate`.
- The planner does **not** touch any cache.
  It produces the template, the key fields, and the provides-shape only.
  L1 remains off (`UseL1Cache = false`) until the post-process L1 optimizer flips it (see [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §6).

### 4.2 Resolve time — where `@key` acts in each phase

- **Phase 1 — prepare + L1 check (main thread).**
  `prepareCacheKeys` invokes `CacheKeyTemplate.RenderCacheKeys(arena, ctx, items, prefix)`.
  For an `EntityQueryCacheKeyTemplate` this reads each item's `__typename` (falling back to the template's plan-time `TypeName` when absent) and extracts **only** the template's `@key` fields from the item data,
  producing one `CacheKey` per item.
  L1 is then probed with that key.
  On a complete entity hit the fetch is marked `cacheSkipFetch = true` and the stored value is copied out via a denormalizing passthrough StructuralCopy (§5.3).
- **Phase 2-L2 — bulk L2 lookup (main thread).**
  The same rendered entity keys (after the key transform pipeline of §5.4) are grouped by cache instance and issued as one bulk `Get` per instance.
  Returned bytes are parsed verbatim onto the Loader arena and distributed back to each `CacheKey.FromCache`; a per-fetch decision sets `cacheSkipFetch` when L2 hits cover all items.
- **Phase 2-HTTP — parallel HTTP (goroutines).**
  Only fetches not already satisfied run here. Goroutines return `[]byte`; they never render keys or touch the cache.
- **Phase 4 — merge + populate (main thread).**
  After merging the fetched (or cached) value into the response tree, `populateL1Cache` writes the entity into L1 keyed by its `@key`-derived key, and `updateL2Cache` writes the projected value into L2 under the same key shape.

### 4.3 Ordering & threading constraints rooted in `@key`

- **Key fields must be present in the data being keyed.**
  Entity-key rendering reads `@key` fields from response item data.
  An item whose `@key` fields are absent produces an **empty key object**, and that item is **skipped** for caching (see §5.2) — never given a degenerate key that would collide across all entities of the type.
- **`@key` rendering is alias-independent** (§5.1), so it is safe to render before or after alias normalization.
- **All key rendering allocates on the per-request arena on the main thread.**
  No goroutine renders a key.

---

## 5. Cache key & data shape

### 5.1 The entity key shape

`EntityQueryCacheKeyTemplate.RenderCacheKeys` produces, per item, a deterministic JSON key:

```json
{"__typename":"User","key":{"id":"123"}}
```

Construction rules (each verified by `cache_key_test.go`):

- `__typename` comes from the item's `__typename`, falling back to the template's plan-time `TypeName`.
- The `key` object contains **only** the `@key` fields named by the template, in template order.
- **Composite keys** render all key fields under `key`, for example `{"__typename":"Product","key":{"sku":"ABC123","upc":"DEF456"}}`.
- **Nested key fields** render as nested objects, for example a `store.id` mapping renders `{"key":{"store":{"id":"123"}}}`.
- **Array key fields** render the array verbatim, for example `{"key":{"tags":["electronics","sale"]}}`.
- **Number coercion.**
  Numeric key values are coerced to strings (`CoerceToString`) so that an integer `id` (`1`) and a string `id` (`"1"`) collide on the **same** entry.
  This applies to flat scalars, scalars inside composite keys, and scalars inside nested key objects — the contract is uniform.
- **Alias independence.**
  The key is computed from the `@key` field *schema names*, never from response aliases, so the same entity yields the same key no matter how a query aliases its fields.

### 5.2 The empty-key skip rule

If, after extraction, the `key` object is empty (the `@key` fields were not selected and are absent from the data),
the item is **omitted** from the returned cache keys entirely.
Caching such an item would produce `{"__typename":"User","key":{}}`, which collides for every `User`,
causing incorrect cross-entity cache sharing.
This is a hard correctness rule, not an optimization.

### 5.3 The projected/stored data shape — passthrough vs projection

`@key` interacts directly with the L1 vs L2 projection switch (`Transform.Passthrough`, see [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3.4):

- **L1 (passthrough = true).**
  L1 writes use `structuralCopyNormalizedPassthrough`: rename aliases to schema names but **keep all source fields**, including `@key` fields that are not in `ProvidesData` and fields accumulated by sibling fetches.
  This is the reason entity L1 uses passthrough rather than strict projection — **the `@key` fields must survive into the L1 entry** so a later, wider entity fetch can merge against an entry that still carries its identity.
  L1 reads use `structuralCopyDenormalizedPassthrough`: restore aliases while preserving every accumulated field.
- **L2 (passthrough = false).**
  L2 writes use the non-passthrough `structuralCopyNormalized` path (rendered to bytes via `MarshalToWithTransform`), projecting to `ProvidesData` fields only.
  Because `ProvidesData` for an entity fetch must include the entity's `@key` fields (they identify the row), the projected L2 payload still round-trips its identity.

### 5.4 The key transform pipeline

The rendered key is transformed identically on read, write, and delete:

```text
GlobalCacheKeyPrefix  →  subgraph header-hash prefix  →  L2CacheKeyInterceptor
```

The `prefix` argument to `RenderCacheKeys` carries the subgraph header-hash prefix when `IncludeSubgraphHeaderPrefix = true`; it is prepended as `prefix:{key}`.
Anyone performing manual invalidation must reproduce this exact pipeline or they target the wrong entry.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation (StructuralCopy / arena).**
  `@key`-keyed L1 entries obey the StructuralCopy invariant of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3 exactly:
  every L1 **write** StructuralCopies into the cache (passthrough, keeping `@key`),
  every L1 **read** StructuralCopies out before merging into the response tree,
  and every **merge-into-existing-L1-entry** uses working-copy-and-swap.
  The cache-key template itself is the engine-internal `CacheKeyTemplate` seam ([01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §7.5); the router never implements it.
- **`@provides` (`ProvidesData`).**
  `ProvidesData` defines the projected shape for L2 and the widening check; `@key` defines identity within that shape.
  For correctness `ProvidesData` on an entity fetch must include the `@key` fields.
  See [provides.md](./provides.md).
- **`@requires` (exclusion).**
  `@key` is the only field set that seeds the cache key.
  `@requires` fields are **never** part of the key and **never** written into the cached entity shape, because they are request-derived, not entity-owned.
  See [requires.md](./requires.md).
- **Root-field caching / `EntityKeyMapping`.**
  A root-field fetch can be mapped onto **entity-key shape** via `EntityKeyMappings` so a root query (`user(id: "1")`) and an `_entities` fetch for `User{id:"1"}` share one L2 entry.
  The derived key is rendered in the *same* `{"__typename":...,"key":{...}}` shape with the same number coercion, so identity matches the entity template byte-for-byte.
  See [root-field-cache-config.md](./root-field-cache-config.md).
- **`@requestScoped`.**
  Independent of `@key`; it keys on a directive-supplied string, not on entity identity.
  See [request-scoped.md](./request-scoped.md).

Dependency ordering ([02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) §3): `@key` is consumed by everything caching-related and must land first among the directive specs, right after the foundation.

---

## 7. End-to-end test plan

Tests live in two layers.
**Unit** key-rendering tests live under `v2/pkg/engine/resolve/` (extend `cache_key_test.go`).
**E2E** tests live under `execution/engine/` and run against the `accounts` / `products` / `reviews` federation services.
Before writing any test, re-read the relevant package `CLAUDE.md`.
All assertions follow the universal rules: `assert.Equal` on full values, **never** `Contains` / `GreaterOrEqual` / fuzzy comparisons, inline literal queries and keys, vertical multi-key struct literals, and every `ClearLog()` followed by `GetLog()` + full assertions.

### 7.1 Unit — single `@key` field (identity)

- **Template:** `EntityQueryCacheKeyTemplate{Keys: <Object with __typename + id>}`.
- **Data item:** `{"__typename":"User","id":"123"}`.
- **What is cached:** the rendered entity key for this `User`.
- **Assertion (exact, full value):**

```go
expected := []*CacheKey{
    {
        Item: data,
        Keys: []string{`{"__typename":"User","key":{"id":"123"}}`}, // identity = @key field only
    },
}
assert.Equal(t, expected, cacheKeys)
```

### 7.2 Unit — composite `@key`

- **Template fields:** `__typename`, `sku`, `upc`.
- **Data item:** `{"__typename":"Product","sku":"ABC123","upc":"DEF456","name":"Trilby"}`.
- **What is cached:** key includes **both** `@key` fields, drops the non-key `name`.
- **Assertion:**

```go
assert.Equal(t, []string{`{"__typename":"Product","key":{"sku":"ABC123","upc":"DEF456"}}`}, cacheKeys[0].Keys)
```

### 7.3 Unit — number coercion (integer vs string identity collision)

- **Two renders:** one item with integer `id` (`1`), one with string `id` (`"1"`).
- **What is cached:** both must produce the **same** key so they share one entry.
- **Assertions (one per line, with why):**

```go
assert.Equal(t, []string{`{"__typename":"User","key":{"id":"1"}}`}, keysFromInteger[0].Keys) // integer coerced to string
assert.Equal(t, []string{`{"__typename":"User","key":{"id":"1"}}`}, keysFromString[0].Keys)  // string stays string
assert.Equal(t, keysFromInteger[0].Keys, keysFromString[0].Keys)                              // collide on one entry
```

### 7.4 Unit — empty key is skipped

- **Data item:** `{"__typename":"User","name":"Me"}` (no `@key` field selected).
- **What is cached:** nothing — the item is omitted from the returned keys.
- **Assertion (exact count):**

```go
assert.Equal(t, 0, len(cacheKeys)) // @key field absent → item skipped, never keyed as {"key":{}}
```

### 7.5 Unit — subgraph header-hash prefix

- **Same `User{id:"123"}` template, `prefix = "h1"`.**
- **What is cached:** key carries the prefix exactly once.
- **Assertion:**

```go
assert.Equal(t, []string{`h1:{"__typename":"User","key":{"id":"123"}}`}, cacheKeys[0].Keys) // prefix isolates per subgraph header hash
```

### 7.6 E2E — entity L2 hit/miss across two requests (`accounts` / `reviews`)

- **Setup (inline in the subtest):** a single in-memory `LoaderCache` with a cache log, gateway over `accounts` + `reviews`, `EnableL2Cache: true`.
- **Query (inline):**

```graphql
{ me { id reviews { body product { upc } } } }
```

- **What is cached:** the `User` entity keyed `{"__typename":"User","key":{"id":"1234"}}` and the `Product` entity keyed `{"__typename":"Product","key":{"upc":"..."}}`, written by request 1.
- **Request 1** (cold): assert the cache log shows `get` (all miss) then `set` for the entity keys; clear log; assert; then re-clear.
- **Request 2** (warm): assert `get` returns hits for the same entity keys and the subgraph entity fetch is skipped.
- **Assertion style:** assert the **full** response JSON with `assert.Equal` on both requests (identical bytes), and assert the **full** cache log vertically, one key per line, with a trailing comment per event:

```go
wantLog := []CacheLogEntry{
    {
        Operation: "get",
        Keys: []string{
            `{"__typename":"User","key":{"id":"1234"}}`, // request 1: cold, L2 empty
        },
        Hits: []bool{false},
    },
    {
        Operation: "set",
        Keys: []string{
            `{"__typename":"User","key":{"id":"1234"}}`, // request 1 populates the User entity
        },
    },
}
assert.Equal(t, wantLog, defaultCache.GetLog())
```

(The exact key set, including `Product` entries and any header-hash prefix, must be enumerated fully and inline once the concrete fixture is wired.)

### 7.7 E2E — alias independence

- **Two queries** selecting the same `User` once plain and once aliased:

```graphql
{ me { id username } }
```

```graphql
{ me { ident: id handle: username } }
```

- **What is cached:** **one** L2 entry, because the key is derived from the `@key` *schema* name `id`, not the alias `ident`.
- **Assertion:** after request 1 (plain) populates, request 2 (aliased) must be a cache **hit** on the identical key:

```go
assert.Equal(t, []string{`{"__typename":"User","key":{"id":"1234"}}`}, hitKeys) // alias did not change identity
```

### 7.8 E2E — L1 dedup within one request (same entity twice)

- **Query** that resolves the same `User` along two fetch paths within a single request (for example `me` and a `reviews.author` that resolves to the same user id).
- **What is cached:** L1 holds `User{id:"1234"}` after the first fetch; the second entity fetch is skipped (`cacheSkipFetch`).
- **Assertion:** assert the **full** response JSON with `assert.Equal`, and assert the subgraph was called for the `User` entity **exactly once** (exact integer, no `GreaterOrEqual`):

```go
assert.Equal(t, 1, accountsEntityCalls) // L1 deduped the second User fetch within the request
```

---

## 8. Acceptance criteria

A reviewer can verify the `@key` consumption is correct against this checklist.

- [ ] The planner extracts `@key` fields into a `KeyField` tree that excludes `__typename` and preserves nesting (flat, composite, nested-object, array forms all represented).
- [ ] Entity fetches receive an `EntityQueryCacheKeyTemplate` whose `Keys` contains **only** `@key` fields — no `@requires` fields, no arbitrary selected fields.
- [ ] `RenderCacheKeys` produces the exact shape `{"__typename":<T>,"key":{<key fields>}}`, byte-identical across requests for the same entity.
- [ ] `__typename` falls back to the template's plan-time `TypeName` when absent from item data.
- [ ] Numeric key values are coerced to strings at flat, composite, and nested levels, so integer and string identities collide on one entry.
- [ ] Items whose `@key` fields are absent produce an empty key object and are **skipped** (never keyed as `{"key":{}}`).
- [ ] Keys are alias-independent — aliasing a key field does not change the rendered key.
- [ ] The subgraph header-hash prefix (and the full transform pipeline `GlobalCacheKeyPrefix → header-hash → L2CacheKeyInterceptor`) is applied identically on read, write, and delete.
- [ ] Both L1 and L2 key entities on `@key` fields **only**; query selection does not widen the key.
- [ ] Entity L1 uses **passthrough** projection so `@key` fields survive into the L1 entry even when not in `ProvidesData`; the StructuralCopy write/read/merge invariants of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3 hold.
- [ ] Non-resolvable `@key` types do not become independent entity-fetch cache targets in the declaring subgraph.
- [ ] L1 deduplicates repeated same-entity fetches within one request; L2 deduplicates the same entity across requests (verified by exact subgraph call counts and full-value response assertions).
- [ ] All tests use `assert.Equal` on full values, inline literal keys/queries, vertical multi-key log literals, and clear-then-assert cache logs.

---

## 9. Cross-links

- Decision record: [adr/0002-key.md](../adr/0002-key.md).
- Architecture seam, L1/L2 model, StructuralCopy invariants, cache-key model: [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
- Directive taxonomy and PR mapping: [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).
- Sibling directive specs: [provides.md](./provides.md), [requires.md](./requires.md), [request-scoped.md](./request-scoped.md), [root-field-cache-config.md](./root-field-cache-config.md).
