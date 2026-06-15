# Directive Specification: Mutation Cache Config

> Part of the entity-caching re-implementation document set.
> Cross-links:
> [adr/0008-mutation-cache-config.md](../adr/0008-mutation-cache-config.md) (rationale),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) (integration seam, L1/L2 model, copy invariants, cache-key model),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) (directive taxonomy + PR mapping).
> Re-implementation PR: **gqtools PR 13 / PR-CACHE-INVALIDATION**.
> Depends on: `@key` (entity identity / cache keys),
> `@provides` (`ProvidesData` projection),
> the entity cache config and root-field cache config concepts,
> and the foundation StructuralCopy / arena invariants.

---

## 0. Scope note: this is a configuration concept, not a wire directive

"Mutation cache config" is **not** a GraphQL schema directive.
There is no SDL syntax a subgraph author writes.
It is a pair of Go configuration structs that the router synthesises from composition output and per-subgraph config,
then hands to the engine.
The two structs are surfaced to the router as `plan.MutationFieldCacheConfiguration` and `plan.MutationCacheInvalidationConfiguration`,
collapsed by the planner into a single per-fetch runtime struct, `MutationEntityImpactConfig`,
which the resolver consumes.
Throughout this document, "the mutation config" means that pair plus its runtime projection.

---

## 1. Purpose & responsibility

Mutations are the write side of caching, and their job is to keep the cache **correct after a change**, never to serve stale data.
The mutation cache config gives per-mutation control over two distinct behaviours.
First, **opt-in L2 population**: by default, entity fetches triggered by a mutation are forbidden from writing to L2,
so a mutation cannot accidentally repopulate a cache with data that may already be obsolete;
a single flag re-enables those writes (with an optional TTL override) for mutations whose returned entities are known-good.
Second, **invalidation on success**: after a mutation returns, the L2 entries for the entities it touched can be deleted (or, in the single-subgraph case, directly overwritten from the mutation payload),
so the next query reads fresh data.
A hard, non-negotiable rule underpins both: **mutations always skip L2 *reads*** — a mutation never serves a cached response, it always hits the subgraph for fresh truth.

---

## 2. Configuration definition (Go config shapes)

### 2.1 Router-facing config (what the router supplies, per subgraph)

Supplied through `SubgraphCachingConfig` as two collections.

L2 write control, per mutation field:

```go
// plan.MutationFieldCacheConfiguration
type MutationFieldCacheConfiguration struct {
    FieldName                     string        // e.g. "addReview"
    EnableEntityL2CachePopulation bool          // false (default) = entity fetches under this mutation skip L2 writes
    TTL                           time.Duration // 0 = use entity default TTL; otherwise overrides it for these writes
}
```

Invalidation control, per mutation field:

```go
// plan.MutationCacheInvalidationConfiguration
type MutationCacheInvalidationConfiguration struct {
    FieldName      string // e.g. "updateUser"
    EntityTypeName string // optional — inferred from the mutation return type when omitted
}
```

### 2.2 Runtime config (what the resolver sees, per fetch)

The planner attaches one `MutationEntityImpactConfig` to the fetch's `FetchCacheConfiguration.MutationEntityImpactConfig` when a mutation field is configured for population and/or invalidation.
This is the only mutation shape the resolver code reads:

```go
// resolve.MutationEntityImpactConfig (engine-internal)
type MutationEntityImpactConfig struct {
    EntityTypeName              string        // "User"
    KeyFields                   []KeyField    // [{Name:"id"}] — the @key field set, supports composite + nested
    CacheName                   string        // named L2 cache instance
    IncludeSubgraphHeaderPrefix bool          // prefix the invalidation/populate key with the subgraph header hash
    InvalidateCache             bool          // delete the L2 entry after the mutation
    PopulateCache               bool          // write the L2 entry directly from the mutation payload
    PopulateTTL                 time.Duration // TTL for the PopulateCache write (0 = backend default)
}
```

Two additional booleans live directly on `FetchCacheConfiguration` and drive the **propagated** L2-write gate (distinct from the direct populate path above):

```go
// fields on resolve.FetchCacheConfiguration
EnableMutationL2CachePopulation bool          // mutation root fetch: allow follow-up entity fetches to write L2
MutationCacheTTLOverride        time.Duration // TTL applied to those propagated writes (0 = entity default)
```

`InvalidateCache` and `PopulateCache` are mutually exclusive in practice — composition annotates a single mutation field with one or the other, never both.

---

## 3. Composition rules & validation

There is **no schema syntax**, so there is nothing for the federation composer to parse or validate at the SDL level.
The rules are entirely structural, enforced where the router builds the config and where the resolver consumes it.

- **`FieldName` is mandatory.**
  Both router-facing structs key on the mutation root field name.
  A config with an empty `FieldName` can never be matched against a fetch's root field and silently does nothing.
- **`EntityTypeName` may be omitted** on the invalidation config.
  When absent, it is inferred from the mutation's GraphQL return type.
  Once resolved, the runtime `MutationEntityImpactConfig.EntityTypeName` is always populated.
- **`KeyFields` must be the entity's `@key` set**, not arbitrary selected fields.
  Mutation keys are built from `@key` only, exactly like entity L1/L2 keys, so a deletion targets the same entry a query would read.
  See [directives/key.md](key.md).
- **Default is conservative.**
  With neither flag set, a mutation populates nothing and invalidates nothing;
  it merely fetches fresh and skips L2 reads.
  Re-implementers must preserve this default — silence is the safe state.

---

## 4. Runtime semantics

### 4.1 Where it acts in plan

At plan time, when the planner walks a mutation operation and finds a configured mutation field, it:

- sets `EnableMutationL2CachePopulation` and `MutationCacheTTLOverride` on the **root mutation fetch's** `FetchCacheConfiguration`, and
- attaches a `MutationEntityImpactConfig` (carrying `InvalidateCache` and/or `PopulateCache`, plus key fields and cache name) to the fetch whose response yields the entity.

The planner does **not** touch any cache.
It only annotates fetches, consistent with §1 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).

### 4.2 The L2-read skip (always, no opt-out)

Before any fetch dispatch, the resolver checks the operation type.
For `OperationType == Mutation`, **L2 read is suppressed**: no `Get` is issued for the mutation root fetch.
This is unconditional and not configurable.
It is why every test in §7 asserts the cache log contains **no `get` entry** for a mutation.

### 4.3 The L2-write gate (two paths)

There are two ways a mutation can end up writing to L2, and they are deliberately separate.

**Path A — propagated write (multi-subgraph mutation).**
A mutation often returns an entity stub (`{__typename, id}`) and a follow-up `_entities` fetch hydrates the remaining fields.
That follow-up fetch is a normal cacheable entity fetch.
The gate works by propagation:

1. The root mutation fetch carries `EnableMutationL2CachePopulation` and `MutationCacheTTLOverride`.
2. When the resolver processes the mutation root fetch, it records these onto the Loader (`enableMutationL2CachePopulation` and the TTL override).
3. The follow-up entity fetch inherits those flags during `resolveSingle` propagation.
4. In `updateL2Cache`, the entity fetch writes to L2 **only if** the propagated flag is true and per-request `EnableL2Cache` is on.
   - If `EnableMutationL2CachePopulation == false`: **no L2 write at all** (the entity fetch behaves as if uncached for writes).
   - TTL for the write is `MutationCacheTTLOverride` when non-zero, else the entity's default TTL from its entity cache config.

**Path B — direct populate (single-subgraph mutation).**
A single-subgraph mutation that returns the full entity has no follow-up fetch to inherit anything.
For these, composition sets `PopulateCache = true` (with `PopulateTTL`) on the `MutationEntityImpactConfig`.
After the mutation merges, `detectMutationEntityImpact` writes the entity payload **directly** to L2 under the entity cache key:

- Gated on per-request `EnableL2Cache`; a disabled L2 means no write.
- The stored payload is the entity projected through `ProvidesData` (schema field names, provided fields only), not the raw response object.
- TTL is `PopulateTTL` (0 → backend default).

### 4.4 Invalidation on success

After the mutation response is merged into the response tree (Phase 4 of the parallel flow, or the equivalent point in `resolveSingle`), the resolver calls `detectMutationEntityImpact(result, info, responseData)`.
Step by step:

1. **Guard conditions** (any failure returns `nil`, a no-op):
   - the operation is not a mutation,
   - `info` is nil,
   - no `MutationEntityImpactConfig` is attached,
   - there is no `caches` map,
   - `ProvidesData` is nil,
   - the response payload at the root field is not an object (or array of objects).
2. **Locate the entity object.**
   `navigateProvidesDataToField(providesData, rootFieldName)` descends the mutation `ProvidesData` (`{updateUsername: {id, username}}`) to the inner entity `*Object`.
   This is alias-aware: it uses the `ProvidesData` field shape, which carries the query's aliases.
3. **Build the entity key.**
   `buildEntityKeyValue(entityData, keyFields)` extracts `@key` fields from the response into a key value;
   `buildMutationEntityCacheKey(cfg, entityData, info)` renders the canonical key string and applies the full key transform pipeline (see §5).
4. **Act per flag:**
   - `InvalidateCache == true` → issue `cache.Delete([key])`, and collect the key into the returned `deletedKeys` set.
   - `PopulateCache == true` → write the entity payload to L2 (the Path B write of §4.3).
   - Neither set, analytics off → **touch nothing** (the cache log stays empty).
5. **Arrays.**
   When the mutation returns a list (`{deleteUsers: [{id},{id}]}`), every object item produces its own key, and all of them are invalidated/recorded.
   Non-object items (null, scalars) in the list are skipped.
6. **Returned value.**
   `detectMutationEntityImpact` returns the set of deleted keys (`map[string]struct{}`), so the surrounding flow can dedupe extension-driven deletes against mutation deletes (see §6).

### 4.5 No mutation-time cache reads (even with analytics on)

A subtle but load-bearing rule: even when analytics is enabled, mutation impact detection **must not read from L2**.
The `MutationEvent` it records always has `HadCachedValue = false`, `IsStale = false`, `CachedHash = 0`, and `CachedBytes = 0`,
because the resolver deliberately skips the cached-vs-fresh comparison rather than paying a read.
Only `FreshHash` / `FreshBytes` (derived from the mutation response) are populated.
The cache log for an analytics-enabled invalidation shows exactly one `delete` and no `get`.

### 4.6 Ordering & threading constraints

- All mutation cache work runs on the **main thread**, after merge, consistent with the seam in §5/§7 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
  No goroutine performs deletes, populates, or key construction.
- Invalidation runs **after** the response is merged, so the key is built from final, post-merge entity data.
- The delete-before-set dedupe (§6) runs against the keys `updateL2Cache` is about to write, so ordering between invalidation and L2 write is coordinated on the main thread.

---

## 5. Cache key & data shape

### 5.1 Key shape

Mutation invalidation/populate keys are **entity keys**, identical in shape to those a query uses:

```text
{"__typename":"User","key":{"id":"1234"}}
```

Built from `@key` fields only (`KeyFields` on the config), via `buildEntityKeyValue` + `buildMutationEntityCacheKey`.
This identity-stability is the whole point: the key a mutation deletes is byte-for-byte the key a prior query wrote.

### 5.2 Key transform pipeline (must match reads/writes exactly)

`buildMutationEntityCacheKey` applies the same pipeline as every other cache operation, in the same order (see §4 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)):

```text
GlobalCacheKeyPrefix  →  subgraph header-hash prefix  →  L2CacheKeyInterceptor
```

- With `IncludeSubgraphHeaderPrefix = true` and a header hash of `99887766` for subgraph `accounts`, the key becomes `99887766:{"__typename":"User","key":{"id":"1234"}}`.
- An `L2CacheKeyInterceptor` returning `"tenant-42:" + key` yields `tenant-42:{"__typename":"User","key":{"id":"1234"}}`.

If a mutation built its key through a different pipeline than the original write, it would delete the wrong entry and leave stale data — so re-implementers must route mutation keys through the identical transform path.

### 5.3 Data shape for populate writes

The Path B populate write stores the **`ProvidesData` projection** of the entity, not the raw mutation payload.
That means schema field names and provided fields only — `structuralCopyNormalized` semantics, `Passthrough = false` (projection, not passthrough), per §3.4 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
A mutation returning `{updateUsername:{id:"u-pop", username:"PopMe"}}` stores exactly `{"id":"u-pop","username":"PopMe"}` under the entity key.
For propagated writes (Path A), the follow-up entity fetch's normal L2 write rules apply (projection through its own `ProvidesData`), with only the TTL altered.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation (StructuralCopy / arena).**
  Populate writes serialise the projected entity to heap bytes (`MarshalToWithTransform`) before handing to the backend — the heap boundary of §3.4 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
  Deletes carry only key strings, no Value copies, so they are copy-free.
- **`@key`.**
  Mutation keys are derived from `@key` fields exactly as entity keys are.
  Composite keys (`{id, orgId}`) and nested keys (`{key:{subId}}`) are supported by `buildEntityKeyValue`.
  See [directives/key.md](key.md).
- **`@provides`.**
  `navigateProvidesDataToField` and the populate projection both consume the fetch's `ProvidesData *Object`, the alias-aware shape introduced by `@provides`.
  A nil `ProvidesData` short-circuits the whole impact path.
  See [directives/provides.md](provides.md).
- **Entity cache config.**
  The default TTL used by Path A propagated writes (when `MutationCacheTTLOverride == 0`) comes from the entity's cache config.
  See [directives/entity-cache-config.md](entity-cache-config.md).
- **Root-field cache config.**
  Mutations may also invalidate root-field-mapped entries; the key still goes through the same pipeline.
  See [directives/root-field-cache-config.md](root-field-cache-config.md).
- **Extension-based invalidation (sibling mechanism).**
  Subgraphs can also return `extensions.cacheInvalidation.keys`.
  This shares the dedupe logic: if a key being invalidated is **the same key `updateL2Cache` is about to write**, the delete is **skipped** (redundant).
  The dedupe is per-key, not all-or-nothing, and operates on the post-transform key, so it holds under header prefixes and interceptors.
  Mutation `detectMutationEntityImpact` returns its deleted-keys set precisely so the two invalidation sources can coordinate.
- **Seam compliance.**
  The entire feature rides the two-boolean merge contract (`cacheSkipFetch`, `cacheMustBeUpdated`) and the additive `MutationEntityImpactConfig` on the per-fetch result, per §7 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
  `mergeResult` keeps its shape; mutation logic slots in as a post-merge call.

---

## 7. End-to-end test plan

These are the behaviours a reviewer must see proven.
Unit-level cases (under `v2/pkg/engine/resolve/`) belong in `mutation_cache_test.go` and `extensions_cache_invalidation_test.go`;
E2E cases (under `execution/engine/`) use the federation services `accounts` / `products` / `reviews` and must follow the self-contained-subtest rule.

**Assertion style is mandatory for every case below:**
- `assert.Equal` on the **full** value (response string, full struct, full cache log) — never `Contains`, `GreaterOrEqual`, `Greater`, or any fuzzy comparison.
- Inline every literal (query, cache key, TTL, expected JSON) at the assertion site.
- Cache-log struct literals: **one item per line**, vertical, with a trailing comment per entry explaining *why* it occurred.
- Every `ClearLog()` must be followed by `GetLog()` + full assertions before the next clear or end of test.
- `MutationEvent` slices: assert the **entire** slice with every field of every event populated inline.

### Case 1 — mutation always skips L2 reads (AC-MUT-01)

- **Query:** `mutation { updateUser(id:"u1", name:"Alice") { __typename id } }` followed by a hydrating `_entities` fetch returning `{name:"Alice"}`.
- **What is cached:** nothing read; the follow-up entity write depends on the population flag (see Case 2/4).
- **Assertion:** the cache log contains **no `get`** entry; assert the full log slice, e.g. for the disabled-population variant:
  ```go
  assert.Equal(t, []CacheLogEntry{}, cache.GetLog()) // mutation skips L2 reads; population disabled → no writes either
  ```
  and the merged response: `assert.Equal(t, `{"data":{"updateUser":{"__typename":"User","id":"u1","name":"Carol"}}}`, out)`.

### Case 2 — propagated L2 write uses TTL override

- **Query:** mutation as in Case 1, with `EnableMutationL2CachePopulation = true`, `MutationCacheTTLOverride = 60s`, entity default `300s`.
- **What is cached:** the hydrated entity, written by the follow-up fetch under `{"__typename":"User","key":{"id":"u1"}}`.
- **Assertion (full log, vertical, commented):**
  ```go
  assert.Equal(t, []CacheLogEntry{
      {
          Operation: "set",
          Items: []CacheLogItem{
              {Key: `{"__typename":"User","key":{"id":"u1"}}`, TTL: 60 * time.Second}, // mutation TTL override (60s) beats entity default (300s); no prior get
          },
      },
  }, cache.GetLog())
  ```

### Case 3 — propagated L2 write falls back to entity default TTL when override is 0

- **Query:** as Case 2 but `MutationCacheTTLOverride = 0`.
- **Assertion:** identical log shape, but `TTL: 300 * time.Second` with a comment noting the entity-default fallback.

### Case 4 — population disabled writes nothing

- **Query:** as Case 2 but `EnableMutationL2CachePopulation = false` (override irrelevant).
- **Assertion:** `assert.Equal(t, []CacheLogEntry{}, cache.GetLog())` with a comment: mutation skips both reads and writes when population is off.

### Case 5 — invalidate deletes the entity entry and returns the deleted key

- **Setup:** pre-populate `{"__typename":"User","key":{"id":"1234"}}` → `{"id":"1234","username":"OldMe"}`, then `ClearLog()`.
- **Query:** `mutation { updateUsername(id:"1234", username:"NewMe") { id username } }`, config `InvalidateCache = true`.
- **Assertions:**
  ```go
  assert.Equal(t, map[string]struct{}{`{"__typename":"User","key":{"id":"1234"}}`: {}}, deletedKeys)
  entries, _ := cache.Get(ctx, []string{`{"__typename":"User","key":{"id":"1234"}}`})
  assert.Nil(t, entries[0]) // entry deleted by mutation invalidation
  ```

### Case 6 — direct populate write (single-subgraph mutation, `PopulateCache`)

- **Query:** `mutation { updateUsername(id:"u-pop", username:"PopMe") { id username } }`, config `PopulateCache = true`, `PopulateTTL = 60s`, `EnableL2Cache = true`.
- **Assertion:** the projected payload is written under the entity key:
  ```go
  entries, _ := cache.Get(ctx, []string{`{"__typename":"User","key":{"id":"u-pop"}}`})
  assert.Equal(t, `{"id":"u-pop","username":"PopMe"}`, string(entries[0].Value)) // ProvidesData projection, not raw payload
  ```
  Variant with `EnableL2Cache = false` must leave `entries[0]` nil.

### Case 7 — array mutation invalidates every entity in the list

- **Setup:** pre-populate `User:1` and `User:2`.
- **Query:** `mutation { deleteUsers { id username } }` returning `[{id:"1"},{id:"2"}]`, `InvalidateCache = true`.
- **Assertions:**
  ```go
  assert.Equal(t, map[string]struct{}{
      `{"__typename":"User","key":{"id":"1"}}`: {},
      `{"__typename":"User","key":{"id":"2"}}`: {},
  }, deletedKeys)
  ```
  Non-object items (`null`, `"invalid"`) in the list are skipped — assert only the valid keys appear.

### Case 8 — composite & nested keys, and key transforms

- **Composite:** `KeyFields = [{id},{orgId}]` → key `{"__typename":"User","key":{"id":"1","orgId":"acme"}}` (assert the exact string).
- **Header prefix:** `IncludeSubgraphHeaderPrefix = true`, hash `99887766` → `99887766:{"__typename":"User","key":{"id":"1234"}}`.
- **Interceptor:** returning `"tenant-42:" + key` → `tenant-42:{"__typename":"User","key":{"id":"1234"}}`.
- Assert each rendered key string exactly via `buildMutationEntityCacheKey`.

### Case 9 — analytics records a mutation event without reading the cache

- **Setup:** pre-populate `User:1234` (stale and fresh variants), `ClearLog()`, analytics on.
- **Assertion (full event + full log):**
  ```go
  stats := ctx.GetCacheStats()
  assert.Equal(t, []MutationEvent{
      {
          MutationRootField: "updateUsername",
          EntityType:        "User",
          EntityCacheKey:    `{"__typename":"User","key":{"id":"1234"}}`, // display key, no prefix
          HadCachedValue:    false,  // mutation impact never issues an L2 get
          IsStale:           false,  // no cached-vs-fresh comparison performed
          CachedHash:        0,      // no cached value read
          CachedBytes:       0,
          // FreshHash / FreshBytes are non-zero (derived from the mutation payload)
      },
  }, stats.MutationEvents)
  assert.Equal(t, []CacheLogEntry{
      {Operation: "delete", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`}}}, // exactly one delete, no get
  }, cache.GetLog())
  ```
  (Where `FreshHash`/`FreshBytes` are non-deterministic, assert them as non-zero via a separate `assert.NotEqual(t, uint64(0), ...)` line, never folded into a `Contains`.)

### Case 10 — extension/mutation delete dedupe against `updateL2Cache`

- **Same-key skip:** an entity fetched and invalidated in the same response (`User:1`) → the delete is **skipped** because `updateL2Cache` will set the same key.
  Assert `cache.GetLog()` contains no `delete`.
- **Different-key delete:** invalidate `User:1` (skipped) and `User:2` (different) → exactly one delete for `User:2`.
  Assert the full delete-key slice equals `[]string{`{"__typename":"User","key":{"id":"2"}}`}`.
- **Holds under transforms:** repeat the same-key skip with header prefix and with an interceptor — the dedupe must still skip, because both the invalidation key and the set key pass through the identical transform pipeline.
- **Interceptor metadata:** assert the interceptor is invoked with the exact `L2CacheKeyInterceptorInfo{SubgraphName:"accounts", CacheName:"default"}` for both the L2 set and the invalidation key construction.

### Case 11 — E2E mutation invalidation (federation services)

- **Service:** `accounts` (`User` entity, `@key(fields:"id")`), `reviews` (`addReview` mutation).
- **Flow (self-contained subtest, inline setup, inline queries via `QueryStringWithHeaders`):**
  1. Query `{ me { id username } }` → populates L2 for `User:1`.
  2. Mutation `mutation { updateUsername(username:"Renamed") { id username } }` configured with `InvalidateCache = true`.
  3. Query `{ me { id username } }` again → L2 miss for `User:1` (it was invalidated), subgraph hit, fresh value.
- **Assertions:** full response strings for all three operations (`assert.Equal`), plus the full cache log per phase with `ClearLog()` + `GetLog()` between phases and a `why` comment on every entry.

---

## 8. Acceptance criteria

A reviewer can check off the following:

- [ ] Mutations **never** issue an L2 `Get` — verified by an empty/`get`-free cache log on every mutation path (AC-MUT-01).
- [ ] With `EnableEntityL2CachePopulation = false` (default), no follow-up entity fetch writes to L2 during a mutation.
- [ ] With `EnableEntityL2CachePopulation = true`, the follow-up entity fetch writes to L2 under the correct entity key.
- [ ] `MutationCacheTTLOverride` (non-zero) is used for propagated writes; a zero override falls back to the entity's default TTL.
- [ ] `PopulateCache = true` writes the **`ProvidesData`-projected** entity payload directly to L2 under the entity key, gated on per-request `EnableL2Cache`.
- [ ] `PopulateCache = true` with `EnableL2Cache = false` writes nothing.
- [ ] `InvalidateCache = true` deletes the L2 entry and returns the deleted key set; `InvalidateCache = false` deletes nothing.
- [ ] Array mutation responses invalidate/populate **every** object item; non-object items are skipped.
- [ ] Composite and nested `@key` fields produce correct, exact key strings.
- [ ] The key transform pipeline (`GlobalCacheKeyPrefix → header-hash prefix → L2CacheKeyInterceptor`) is applied identically to mutation keys as to read/write keys.
- [ ] Mutation impact detection **never reads** the cache, even with analytics enabled: `MutationEvent.HadCachedValue == false`, `CachedHash == 0`, `CachedBytes == 0`.
- [ ] A `MutationEvent` is recorded for each impacted entity when analytics is on; none is recorded when the delete is deduped away.
- [ ] Delete-before-set dedupe skips a delete when `updateL2Cache` is about to write the same post-transform key; it deletes different keys; it holds under header prefix and interceptor.
- [ ] All guard conditions (non-mutation op, nil info, no impact config, no caches map, nil `ProvidesData`, non-object payload) return a no-op `nil` and touch nothing.
- [ ] The default state (neither flag set, analytics off) touches the cache **zero** times.
- [ ] All cache work runs on the main thread, after merge; no goroutine performs deletes, populates, or key construction.
- [ ] Existing `loader` / `resolvable` flow is unchanged in shape — mutation logic is a post-merge call honoring the two-boolean seam (per [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §10).
- [ ] Tests use `assert.Equal` on full values, inline literals, vertical multi-item cache-log literals with per-entry `why` comments, and `ClearLog()` always paired with `GetLog()` assertions.
