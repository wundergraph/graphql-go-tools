# Directive Specification: Entity Caching Config

> Part of the entity-caching re-implementation document set.
> Cross-links:
> [adr/0006-entity-cache-config.md](../adr/0006-entity-cache-config.md) (rationale),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) (integration seam, L1/L2 model, StructuralCopy invariants, cache-key model),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) (directive taxonomy and PR mapping).
>
> Re-implementation PR: **gqtools PR 4 / PR-CACHE-CONFIG**.

This document assumes no prior knowledge of the feature.
It describes *what* the entity caching config is, *how* it is supplied, and *how the resolver acts on it* — without copying implementation code.

---

## 1. Purpose & responsibility

The "entity caching config" is the unit of declaration that turns L2 (cross-request) caching **on** for one entity type in one subgraph.
It is **not a GraphQL wire directive** — there is no `@entityCache` in any subgraph SDL.
It is a Go configuration struct (`plan.EntityCacheConfiguration`), one per `(subgraph, entity type)`, that binds an entity type name to a named cache instance, a TTL, and a small set of per-type behavior flags (negative caching, partial loads, shadow mode, header-aware keys, analytics key hashing).
Caching is strictly **opt-in**: an entity type with no config is never written to or read from L2 (its L1 behavior is independent — see §6).
The config is consumed in two stages: the **planner** copies the matched config onto each entity fetch's `FetchCacheConfiguration`, and the **resolver** reads that per-fetch config to decide whether to consult L2, what cache instance and TTL to use, and how to shape the stored value.

---

## 2. Configuration definition (Go struct, not SDL)

There is no SDL.
The authoritative shape is `plan.EntityCacheConfiguration` (in `v2/pkg/engine/plan/federation_metadata.go`).
Fields and their meaning:

```go
type EntityCacheConfiguration struct {
    TypeName                    string        // GraphQL entity type, must match the __typename returned by _entities
    CacheName                   string        // which registered LoaderCache instance backs this type
    TTL                         time.Duration // lifetime of a positive (real-data) cache entry; 0 = never expire
    IncludeSubgraphHeaderPrefix bool          // when true, prefix the L2 key with a hash of forwarded headers
    EnablePartialCacheLoad      bool          // when true, fetch only cache-missed entities in a batch
    HashAnalyticsKeys           bool          // when true, analytics record a hashed key instead of the raw key
    ShadowMode                  bool          // when true, read+write L2 but never serve cached data (staleness probe)
    NegativeCacheTTL            time.Duration // when > 0, cache "entity not found" (null) results for this duration
}
```

A collection type wraps it:

```go
type EntityCacheConfigurations []EntityCacheConfiguration
func (c EntityCacheConfigurations) FindByTypeName(typeName string) *EntityCacheConfiguration // nil = not configured
```

It is supplied per subgraph via the execution-engine factory option `WithSubgraphEntityCachingConfigs`, which carries `SubgraphCachingConfig.EntityCaching`:

```go
type SubgraphCachingConfig struct {
    SubgraphName  string
    EntityCaching plan.EntityCacheConfigurations // one entry per entity type cached in this subgraph
    // ... RootFieldCaching, MutationFieldCaching, SubscriptionEntityPopulation, MutationCacheInvalidation
}
```

The factory copies `subgraphCachingConfig.EntityCaching` onto the matching subgraph's `FederationMetaData.EntityCaching`.
At plan time the planner reads it back through `FederationMetaData.EntityCacheConfig(typeName) *EntityCacheConfiguration`.

**Note on the inventory's "serve-stale window".**
The directive inventory describes this config as binding "a named cache, a TTL, optional negative-cache TTL, and a serve-stale window."
The first three map directly to `CacheName`, `TTL`, and `NegativeCacheTTL`.
There is **no dedicated serve-stale-window field** in the current struct.
The two adjacent behaviors that exist instead are `ShadowMode` (always refetch and compare, never serve cached data) and `EnablePartialCacheLoad` (serve cached batch members directly while refetching only the missing ones, accepting in-TTL staleness).
The re-implementation should treat "serve-stale window" as **not yet a field**; if a real stale-serving window is wanted, it is a new field and must be specified in [adr/0006-entity-cache-config.md](../adr/0006-entity-cache-config.md) before it is added.

---

## 3. Composition rules & validation

There is no schema syntax, so there is **no composition-side directive validation** for this config — it never appears in subgraph SDL and composition neither emits nor checks it.
The rules are structural, enforced by construction and by the lookup contract rather than by a validator:

- **One config per `(subgraph, type)`.**
  `EntityCacheConfigurations` is a flat slice and `FindByTypeName` returns the **first** match.
  Two entries with the same `TypeName` in one subgraph's `EntityCaching` are a configuration error: the second is silently shadowed.
  The re-implementation should treat duplicate `TypeName` within one subgraph's list as invalid input (the router/composition layer that builds these lists is responsible for de-duplicating).

- **`TypeName` must match the subgraph's `__typename`.**
  The lookup key is the entity type name as returned in `_entities` responses.
  A typo means the config never matches and the entity is silently uncached.

- **`CacheName` must be registered at runtime.**
  The name is resolved against the resolver's `Caches map[string]LoaderCache`.
  An unregistered name means the fetch finds no backing cache and falls through to the subgraph (no panic; the L2 path is simply skipped).
  Multiple types may share one `CacheName` — sharing a backing cache instance is explicitly allowed and common.

- **`@key` must exist for the type.**
  This config has no key information of its own; the entity cache key is built from the type's `@key` fields by the planner's key-fields machinery (see [directives/key.md](key.md)).
  A type with no resolvable `@key` cannot be entity-cached even if a config is present, because no stable key shape can be produced.

- **Opt-in default.**
  Absence of a config = L2 disabled for that type.
  There is no "cache everything" mode at this layer.

---

## 4. Runtime semantics (plan + four-phase resolve)

### 4.1 Plan time (once per operation)

For each entity fetch the planner (`configureFetchCaching` in `caching_planner_state.go`) does the following:

1. Always attach the `CacheKeyTemplate` (an `EntityQueryCacheKeyTemplate`) regardless of whether L2 is configured — L1 needs the template even when L2 is off.
2. Look up `FederationMetaData.EntityCacheConfig(entityTypeName)`.
   - If nil: leave `Enabled = false` (L2 off), but still record `KeyFields` for analytics and keep the template for L1.
   - If present: produce a `FetchCacheConfiguration` with `Enabled = true` and copy `CacheName`, `TTL`, `IncludeSubgraphHeaderPrefix`, `EnablePartialCacheLoad`, `HashAnalyticsKeys`, `ShadowMode`, and `NegativeCacheTTL` onto it.
3. Leave `UseL1Cache = false`.
   The L1-enable decision is made later by the post-process optimizer (see §6 and [01-ARCHITECTURE-SPEC.md §6](../01-ARCHITECTURE-SPEC.md)), independent of this config.

The planner does **not** touch any cache; it only annotates the fetch.

### 4.2 Resolve time — where this config acts in the four phases

The resolver path is the one described in [01-ARCHITECTURE-SPEC.md §5](../01-ARCHITECTURE-SPEC.md) and `resolve/CLAUDE.md`.
The governing rule holds throughout: **the main thread runs all cache logic and the arena; goroutines do subgraph HTTP only.**
This config influences these touchpoints:

- **Phase 1 — prepare keys + L1 check (main thread).**
  `prepareCacheKeys` renders the entity key from `@key` fields via the template.
  The entity-cache config's `IncludeSubgraphHeaderPrefix` selects whether the rendered L2 key is prefixed with the subgraph header hash.
  L1 is consulted here, but L1 eligibility is governed by `UseL1Cache`, not by this config's `Enabled` flag.

- **Phase 2-L2 — bulk L2 lookup (main thread).**
  Only fetches with `Enabled = true` (set from this config) participate.
  Fetches are grouped by `LoaderCache` instance (resolved from `CacheName`); one `Get` is issued per instance.
  Returned bytes are parsed verbatim onto the Loader arena.
  `applyEntityFetchL2Results` then validates each returned value against the fetch's `ProvidesData` (the field-widening check from [directives/provides.md](provides.md)) and sets `cacheSkipFetch` for entities whose L2 entry covers all required fields.
  `ShadowMode` short-circuits the *serve* decision here: the read still happens and a `ShadowComparisonEvent` is recorded, but `cacheSkipFetch` is never set — the subgraph is always fetched and compared.
  `EnablePartialCacheLoad` decides batch behavior: when false, any miss in the batch forces a full refetch; when true, only missing entities are fetched and cached members are spliced in directly.
  A **negative-cache sentinel** (a stored literal `null`, written under `NegativeCacheTTL`) is a *hit*: it satisfies the fetch as a known-absent entity and skips the subgraph.

- **Phase 2-HTTP — parallel HTTP (goroutines).**
  Entity fetches not satisfied by L1 or L2 fetch from the subgraph; goroutines return raw bytes only.

- **Phase 4 — merge + populate (main thread).**
  `mergeResult` merges the body (or the cached value when `cacheSkipFetch`) into the response tree.
  `updateL2Cache` then writes back when `Enabled = true` and the fetch was a miss/partial/refresh:
  - A real entity → stored with `TTL`, projected to `ProvidesData` fields only (see §5).
  - A null entity (`_entities` returned `null` with no errors) → when `NegativeCacheTTL > 0`, stored as a negative sentinel under `NegativeCacheTTL` (not `TTL`); when `NegativeCacheTTL == 0`, not cached at all and refetched next request.
  `populateL1Cache` runs independently and uses passthrough copying (§5, §6).

### 4.3 Alias / normalization handling

The cache key is **alias-independent** by construction: it is built from `@key` fields through the template, never from response aliases.
The stored L2 *value* is normalized to schema field names (aliases stripped) and projected to `ProvidesData` via `structuralCopyNormalized` before serialization, so two queries that alias the same entity produce byte-identical keys and compatible stored shapes.
On read, the cached value is denormalized (aliases re-applied for the current query) via `structuralCopyDenormalized` before merging into the response tree.

### 4.4 Ordering / threading constraints

- All key rendering, L2 Get/Set, parsing, merging, and cache population happen on the **main thread**; this config never crosses into a goroutine.
- L2 writes serialize to heap bytes (`MarshalTo*`) before handing them to the backend — the heap boundary that makes external storage safe across requests.
- The `LoaderCache` backend named by `CacheName` must be concurrency-safe (forward-compatible contract), even though the engine currently issues bulk reads on the main thread.

---

## 5. Cache key & data shape

**Key shape (entity key).**
This config produces the entity key shape, built by `EntityQueryCacheKeyTemplate` from `@key` fields only:

```text
{"__typename":"User","key":{"id":"123"}}
```

Numbers in keys are coerced to strings (an integer `id` and a string `id` collide).
The key transform pipeline (applied identically on read, write, delete) is:

```text
GlobalCacheKeyPrefix  →  subgraph header-hash prefix (when IncludeSubgraphHeaderPrefix)  →  L2CacheKeyInterceptor
```

`TTL`, `ShadowMode`, `NegativeCacheTTL`, `EnablePartialCacheLoad`, and `HashAnalyticsKeys` do **not** change the key — they change behavior and storage, not identity.
`IncludeSubgraphHeaderPrefix` is the only field here that affects the final key bytes.

**Stored data shape — projection, not passthrough (L2).**
L2 entries are minimal and self-contained: `updateL2Cache` stores the value **projected to `ProvidesData` fields only** (`structuralCopyNormalized`, `Transform.Passthrough = false`), dropping non-provided fields and excluding `@requires`-derived fields (see [directives/requires.md](requires.md)).
This is the L1/L2 split from [01-ARCHITECTURE-SPEC.md §3.4](../01-ARCHITECTURE-SPEC.md): L2 projects, L1 passes through.

**Negative sentinel shape.**
When `NegativeCacheTTL > 0` and the entity came back null:
- with no prior positive entity data for that key → the stored value is the literal `null` sentinel.
- with prior positive non-key data already present for that key → the stored value preserves the existing object and materializes newly requested nullable fields as explicit `null`, so the same selection validates from cache.

**L1 shape (for contrast).**
L1 writes use passthrough (`structuralCopyNormalizedPassthrough`): rename aliases but **keep all source fields**, including `@key` fields not in `ProvidesData`, so L1 entries accumulate across sibling fetches within a request.
L1 is governed by `UseL1Cache`, not by this config's `Enabled` flag.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation seam ([01-ARCHITECTURE-SPEC.md §7](../01-ARCHITECTURE-SPEC.md)).**
  This config feeds the additive `FetchCacheConfiguration` struct on each entity fetch; it adds no new interface.
  It rides the existing hook points (`prepareCacheKeys`, `bulkL2Lookup`/`tryL2CacheLoad`, `mergeResult`, `updateL2Cache`) and the two-boolean merge contract (`cacheSkipFetch`, `cacheMustBeUpdated`).
  It depends on the `LoaderCache` backend interface (resolved by `CacheName`) and the engine-internal `CacheKeyTemplate` seam, and it depends on the StructuralCopy isolation discipline of [01-ARCHITECTURE-SPEC.md §3](../01-ARCHITECTURE-SPEC.md) for correctness of both writes and reads.

- **`@key` ([directives/key.md](key.md)) — hard dependency.**
  The entity cache key is derived from `@key` fields.
  Without a resolvable `@key`, this config cannot produce a key and the type cannot be entity-cached.

- **`@provides` ([directives/provides.md](provides.md)) — shapes the stored value.**
  `ProvidesData` drives the L2 projection, the field-widening validation on read, and the alias denormalization on serve.
  This config does not define the shape; it relies on `@provides`/selection-derived `ProvidesData`.

- **`@requires` ([directives/requires.md](requires.md)) — exclusion rule.**
  Request-derived `@requires` inputs must never be written into the cached entity shape; the projection inherits this exclusion.

- **Per-request toggles ([01-ARCHITECTURE-SPEC.md §9](../01-ARCHITECTURE-SPEC.md)).**
  L2 participation also requires `Context.ExecutionOptions.Caching.EnableL2Cache`; this config is necessary but not sufficient.
  L1 is gated by `EnableL1Cache` and by `UseL1Cache` (set by the post-process L1 optimizer of [01-ARCHITECTURE-SPEC.md §6](../01-ARCHITECTURE-SPEC.md)), independently of this config.

- **Downstream configs that build on this one.**
  Root-field config ([directives/root-field-cache-config.md](root-field-cache-config.md)) shares L2 entries with entity fetches via `EntityKeyMapping` — it needs the entity cache config to populate the shared keys.
  Mutation ([directives/mutation-cache-config.md](mutation-cache-config.md)) and subscription ([directives/subscription-cache-config.md](subscription-cache-config.md)) configs read and invalidate exactly what this config populates; mutation L2 writes may override `TTL` via the mutation field config.

---

## 7. End-to-end test plan

These cases target the federation services `accounts`, `products`, `reviews` (under `execution/federationtesting/`) through the gateway, and the resolve-package unit harness for the negative-cache lifecycle.
**Mandatory assertion style for every case** (from the package CLAUDE.md files):

- Exact assertions only — `assert.Equal` on the full value.
  Never `assert.Contains`, `assert.GreaterOrEqual`, `assert.Greater`, or any fuzzy comparison.
- Assert entire structs — full `CacheAnalyticsSnapshot` / full `[]CacheLogEntry`, not selected fields.
- Inline literals — queries, cache keys, byte sizes, TTLs, and expected JSON inline at the assertion/setup site; no shared `const`/var for expected values; configs inline in the setup call.
- Vertical multi-key literals — one `Keys`/`Hits`/event entry per line.
- Snapshot comments — every event line carries a trailing `// why` comment.
- Cache-log discipline — every `ClearLog()` is immediately followed by `GetLog()` + full assertions before the next `ClearLog()` or end of test.
- Self-contained subtests under `execution/engine/` — duplicate setup per `t.Run`; no shared `newXxxEnv` helpers.

### Case 1 — L2 miss then hit (single entity type)

- **Setup:** subgraph `reviews` with `EntityCaching: plan.EntityCacheConfigurations{{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second}}` inline; `CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}`.
- **Query (inline):** `query { topProducts { name reviews { body } } }`.
- **What is cached:** each `Product` entity fetched from `reviews` under key `{"__typename":"Product","key":{"upc":"..."}}`, projected to provided fields, with `TTL: 30 * time.Second`.
- **Assertions:**
  - Request 1: `assert.Equal` on the full response body.
    `assert.Equal` on the full `CacheAnalyticsSnapshot` — `L2Reads` all `CacheKeyMiss` (// first request, cache empty), `L2Writes` for each Product key with exact `ByteSize`, `TTL: 30 * time.Second`, `CacheLevel: CacheLevelL2`, `Source: CacheSourceQuery` (// written after subgraph fetch on miss).
  - Request 2: same response body; full snapshot with `L2Reads` all `CacheKeyHit` carrying exact `ByteSize` (// populated by request 1), no `L2Writes` (// all served from cache).

### Case 2 — multiple entity types share one cache

- **Setup (inline):** `products` with `RootFieldCaching` for `Query.topProducts`; `reviews` with `EntityCaching {TypeName:"Product", CacheName:"default", TTL:30s}`; `accounts` with `EntityCaching {TypeName:"User", CacheName:"default", TTL:30s}` — same `CacheName: "default"`.
- **Query (inline):** `query { topProducts { name reviews { body author { username } } } }`.
- **What is cached:** `Product` and `User` entities plus the root field, all in the one `default` cache.
- **Assertions:** full `CacheAnalyticsSnapshot` over both requests, one `L2Reads`/`L2Writes` entry **per line** with a `// why` comment per line (e.g. `// User 1234 deduplicated in batch`), exact `ByteSize` and `TTL` literals.
  Verifies a shared `CacheName` keys distinct entity types into distinct entries within the same instance.

### Case 3 — opt-in: no config = no L2

- **Setup (inline):** caching enabled (`EnableL2Cache: true`) but `EntityCaching` empty for `reviews`.
- **Query (inline):** `query { topProducts { name reviews { body } } }`.
- **What is cached:** nothing in L2.
- **Assertions:** full `[]CacheLogEntry` is empty (no `get`, no `set`) for the Product key; full snapshot has empty `L2Reads`/`L2Writes`.
  Run twice and assert both subgraph fetches happen (exact tracker count, e.g. `assert.Equal(t, 2, productCalls)`).

### Case 4 — header-aware keys (`IncludeSubgraphHeaderPrefix`)

- **Setup (inline):** `reviews` with `{TypeName:"Product", CacheName:"default", TTL:30s, IncludeSubgraphHeaderPrefix: true}`; mock header source returns a deterministic, cloned `http.Header` (use `.Clone()`).
- **What is cached:** the Product entry under a header-hash-prefixed key.
- **Assertions:** full `[]CacheLogEntry` with the exact prefixed key string inline (e.g. `11945571715631340836:{"__typename":"Product","key":{"upc":"top-1"}}`).
  A second request with a different header value produces a different prefixed key (assert both exact keys, separate entries) — proving header isolation.

### Case 5 — negative caching lifecycle (resolve-package unit, mirrors `negative_cache_test.go`)

- **Setup:** entity fetch with `Caching: FetchCacheConfiguration{Enabled: true, CacheName: "default", TTL: 30 * time.Second, NegativeCacheTTL: 10 * time.Second, CacheKeyTemplate: ...}`; subgraph returns `{"data":{"_entities":[null]}}`.
- **What is cached:** a `null` sentinel under the entity key, written with `NegativeCacheTTL`, not `TTL`.
- **Assertions:**
  - First execution: `assert.Equal(t, "null", string(cache.GetValue(key)))`.
    `assert.Equal` on the full `[]CacheLogEntry`: a `get` miss then a `set` with `Items: [{Key: key, TTL: 10 * time.Second}]` (// negative sentinel uses NegativeCacheTTL, not TTL).
  - Second execution (sentinel still live): subgraph mock `Times(1)` — not called again; `get` returns `Hit: true`.
  - `NegativeCacheTTL: 0` variant: subgraph mock `Times(2)` — not cached, refetched.
  - Eviction-then-real-data variant: after `cache.Delete(key)`, second request stores real data under `TTL: 30 * time.Second`; assert the full stored value JSON inline and the full two-entry log (`get` miss, `set` 30s).

### Case 6 — shadow mode never serves

- **Setup (inline):** `reviews` with `{TypeName:"Product", CacheName:"default", TTL:30s, ShadowMode: true}`.
- **What is cached:** L2 is read and written normally, but cached data is never served.
- **Assertions:** across two requests, the subgraph is fetched **both** times (exact tracker count, `assert.Equal(t, 2, productCalls)`); full snapshot shows a populated `ShadowComparisons` slice (assert the full `ShadowComparisonEvent` value inline) and `L2Reads`/`L2Writes` present.
  Response body identical to the non-shadow case.

---

## 8. Acceptance criteria (reviewer checklist)

- [ ] `plan.EntityCacheConfiguration` exposes exactly: `TypeName`, `CacheName`, `TTL`, `IncludeSubgraphHeaderPrefix`, `EnablePartialCacheLoad`, `HashAnalyticsKeys`, `ShadowMode`, `NegativeCacheTTL`.
  No undocumented fields; any "serve-stale window" addition is recorded in the ADR first.
- [ ] Supplied per subgraph via `SubgraphCachingConfig.EntityCaching` and registered through `WithSubgraphEntityCachingConfigs`; surfaced to the planner through `FederationMetaData.EntityCacheConfig(typeName)`.
- [ ] Opt-in: absence of a config means the entity type is never read from or written to L2; presence with `EnableL2Cache: true` turns L2 on.
- [ ] `FindByTypeName` returns the first match; duplicate `TypeName` within one subgraph is treated as invalid input (de-duplicated upstream).
- [ ] Multiple entity types may share one `CacheName`, keying into distinct entries in the same instance (Case 2).
- [ ] An unregistered `CacheName` skips L2 gracefully (subgraph fetch), no panic.
- [ ] Entity cache key is alias-independent, built from `@key` fields only, in the shape `{"__typename":T,"key":{...}}` with numeric keys coerced to strings.
- [ ] Key transform pipeline applied identically on read/write/delete: `GlobalCacheKeyPrefix → header-hash prefix (when IncludeSubgraphHeaderPrefix) → L2CacheKeyInterceptor`.
- [ ] L2 stored value is projected to `ProvidesData` (`structuralCopyNormalized`, no passthrough), excludes `@requires`-derived fields, and is denormalized on read.
- [ ] All cache key rendering, L2 Get/Set, parsing, merging, and population run on the main thread; goroutines do HTTP only.
- [ ] `TTL` governs positive entries; `NegativeCacheTTL > 0` caches null results as sentinels under `NegativeCacheTTL` (not `TTL`); `NegativeCacheTTL == 0` disables negative caching.
- [ ] `ShadowMode` reads and writes L2 but never serves cached data and records `ShadowComparisons` (Case 6).
- [ ] `EnablePartialCacheLoad` controls batch all-or-nothing vs missing-only fetch behavior.
- [ ] L1 behavior is independent of this config and gated by `UseL1Cache` (post-process optimizer) plus `EnableL1Cache`.
- [ ] All E2E/unit tests follow the mandatory assertion style of §7 (exact `assert.Equal` on full values, inline literals, vertical multi-key literals, snapshot `// why` comments, cache-log clear+assert discipline, self-contained subtests).
