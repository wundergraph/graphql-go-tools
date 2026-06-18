# Directive Specification: Subscription Population Config

> Part of the entity-caching re-implementation document set.
> Cross-links: [adr/0009-subscription-cache-config.md](../adr/0009-subscription-cache-config.md),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).
>
> Re-implementation PR: gqtools PR 15 / PR-CACHE-SUBSCRIPTION.

---

## 1. Purpose & responsibility

Subscriptions emit fresh entity data on every event,
and without cache integration every subscriber that triggers a child entity fetch round-trips to the entity-owning subgraph for each event.
The subscription population config is a **configuration concept, not a wire directive**.
It binds a subscription root field to one of two per-event behaviors against the L2 cache.
On each trigger event, the resolver either **populates** L2 with the entity data carried by the event,
so later queries and subscription events can resolve that entity from cache,
or it **invalidates** the L2 entry,
deleting it so downstream readers do not serve a value the subscription has just superseded.
Population happens when the event carries entity fields beyond `@key`.
Invalidation happens when the event carries only `@key` fields and `EnableInvalidationOnKeyOnly` is set.
The cache operation runs exactly once per trigger event regardless of how many subscribers share the trigger,
and it completes before subscriber fanout so a child entity fetch on the same event sees the just-written value.

---

## 2. SDL / configuration definition

There is **no schema syntax**.
The concept arrives as a Go configuration struct on the federation metadata produced for a subgraph.

Planner-side config (one entry per `(subgraph, entity type, subscription field)`):

```go
type SubscriptionEntityPopulationConfiguration struct {
    TypeName                    string        // entity type, e.g. "Product"
    FieldName                   string        // subscription root field, e.g. "updateProductPrice"
    CacheName                   string        // which LoaderCache instance
    TTL                         time.Duration // write TTL (populate mode only)
    IncludeSubgraphHeaderPrefix bool          // prepend header-hash key segment
    EnableInvalidationOnKeyOnly bool          // key-only event => delete instead of no-op
}

type SubscriptionEntityPopulationConfigurations []SubscriptionEntityPopulationConfiguration
```

The lookup helper, consumed at plan time, matches on **both** fields:

```go
func (c SubscriptionEntityPopulationConfigurations) FindByTypeAndFieldName(typeName, fieldName string) *SubscriptionEntityPopulationConfiguration
```

The planner translates a matched config into the resolver-facing struct carried on the subscription plan node:

```go
type SubscriptionEntityCachePopulation struct {
    Mode                        SubscriptionCacheMode       // Populate | Invalidate
    CacheKeyTemplate            *EntityQueryCacheKeyTemplate // renders entity-shape keys
    CacheName                   string
    TTL                         time.Duration
    IncludeSubgraphHeaderPrefix bool
    DataSourceName              string // subgraph name, for header-hash lookup
    SubscriptionFieldName       string // navigate into this response field before treating items as entities
    EntityTypeName              string // for __typename filtering / injection in cache keys
}
```

`SubscriptionEntityCachePopulation` is attached to the plan as `GraphQLSubscription.EntityCachePopulation`.
When it is `nil`, or its `CacheKeyTemplate` is `nil`, the subscription has no cache integration and nothing in this spec applies.

The two callback hooks live on `ResolverOptions`:

```go
OnSubscriptionCacheWrite      func(CacheWriteEvent)
OnSubscriptionCacheInvalidate func(entityType string, keys []string)
```

---

## 3. Composition rules & validation

There is no SDL and therefore no composition-time schema validation of new directive syntax.
The single mandatory-shape invariant lives in the config struct and its lookup:

- **Both `TypeName` AND `FieldName` must be set.**
  `FindByTypeAndFieldName` matches on `c[i].TypeName == typeName && c[i].FieldName == fieldName`.
  An empty `FieldName` therefore makes the lookup silently fail,
  the planner never attaches a `SubscriptionEntityCachePopulation`,
  and populate/invalidate becomes a silent no-op.
  This is the load-bearing rule the router integration must honor:
  the router must set `FieldName` on **both** the populate path and the invalidate path.

- **`FieldName` disambiguates entries that share an entity type.**
  A single subgraph may emit two subscriptions on the same entity type with different field names and different TTLs.
  Because the lookup keys on both `TypeName` and `FieldName`,
  each subscription selects its own config independently,
  for example `updateProductPrice` selecting a 30s-TTL entry while `updatedPrice` selects a 60s-TTL entry.

- **Mode is derived at runtime, not configured directly.**
  Populate vs Invalidate is not a configured enum on the planner struct;
  it is decided from the event payload (fields beyond `@key`) combined with `EnableInvalidationOnKeyOnly`.
  The planner sets `SubscriptionEntityCachePopulation.Mode` accordingly.

---

## 4. Runtime semantics

### 4.1 Where it acts

This config acts **outside** the four-phase parallel resolver of [01-ARCHITECTURE-SPEC.md §5](../01-ARCHITECTURE-SPEC.md).
The four-phase machinery applies to a single query/mutation resolve pass.
Subscription population is a **trigger-level** operation:
it runs once per upstream trigger event, before the per-subscriber resolve passes that fan the event out.
Each per-subscriber resolve pass that runs a child entity fetch (a nested `_entities` fetch) still goes through the normal four-phase L1/L2 read/write path,
and benefits from whatever the trigger-level operation just populated or invalidated.

### 4.2 Step-by-step on each trigger event

1. The trigger emits an event with raw bytes `data`.
2. If `EntityCachePopulation` is `nil` or its `CacheKeyTemplate` is `nil`, skip — no cache integration.
3. If the request context captured at subscription creation has `ExecutionOptions.Caching.EnableL2Cache == false`, skip.
4. If `CacheName` is not present in `Resolver.options.Caches`, return without any cache operation and without error — defensive guard against misconfiguration; subscription delivery must not be blocked.
5. Parse `data` onto an arena-allocated value (never mutate the wire bytes handed to subscribers).
6. If `SubscriptionFieldName` is set, navigate into that response field before treating items as entities.
7. For each entity item:
   - If the item has a `__typename` and it does **not** equal `EntityTypeName`, skip the item (union/interface member of a different concrete type — see §4.4).
   - If the item lacks `__typename`, inject `EntityTypeName` as `__typename` on the parsed copy so the key template produces a correctly typed key.
   - Render the cache key via the key pipeline of §4.3.
8. Determine the mode and perform the operation **once for the whole payload**:
   - **Populate** (`SubscriptionCacheModePopulate`): `Set` one L2 entry per item — key from the template, value the JSON-encoded item (only the fields present in the response), TTL = config TTL.
   - **Invalidate** (`SubscriptionCacheModeInvalidate`): `Delete` the L2 entry per item; no `Set`.
9. Backend `Set`/`Delete` errors must **not** propagate to the event flow; deliver the event regardless of cache health.
10. After the cache operation completes, fan the same `data` out to all N subscribers.

### 4.3 Cache key construction (must match the standard flow)

The pipeline is identical to the query-side `prepareCacheKeys` / `processExtensionsCacheInvalidation` pipeline of [01-ARCHITECTURE-SPEC.md §4](../01-ARCHITECTURE-SPEC.md):

```text
GlobalCacheKeyPrefix → subgraph header-hash prefix → template-rendered key → L2CacheKeyInterceptor
```

- Steps 1–3 (global prefix, header-hash prefix when `IncludeSubgraphHeaderPrefix == true` and a `SubgraphHeadersBuilder` is set, then the template-rendered key) are concatenated into the prefix passed to `RenderCacheKeys`.
- Step 4 (`L2CacheKeyInterceptor`) is applied to the rendered keys afterwards.
- The header-hash prefix is looked up by `DataSourceName`.

### 4.4 Alias and abstract-type handling

- **Root-field alias.**
  The subscription may alias its root field, for example `priceUpdate: updateProductPrice(...)`.
  This does not break entity population:
  the key is computed from `@key` fields on the entity, never from the response alias,
  consistent with the alias-independence rule of [01-ARCHITECTURE-SPEC.md §4](../01-ARCHITECTURE-SPEC.md).
- **Union / interface return types.**
  When the subscription returns a union or interface,
  the planner resolves the configured concrete `TypeName` (for example `Product`) and attaches that config.
  At runtime, `EntityTypeName` filtering (§4.2 step 7) keeps only items whose `__typename` matches the configured type,
  and skips items of other concrete types — so an unconfigured member (for example `DigitalProduct`) is never cached.
- **`__typename` injection** is done on the parsed arena copy only, never on the bytes handed to subscribers.

### 4.5 Ordering and threading constraints

- **Once per trigger event, not once per subscriber.**
  N subscribers sharing a trigger cause exactly ONE cache operation per affected entity, then a single fanout.
- **Populate completes before fanout.**
  A child entity fetch issued during the same event's fanout must observe the just-populated L2 entry.
  The implementation may achieve this synchronously, or async-then-wait, or via event-loop coordination — any approach that guarantees the ordering is acceptable.
- **Key strings must outlive the trigger arena.**
  Keys rendered onto the trigger's arena must be cloned (e.g. `strings.Clone`) before being handed to the backend,
  because the backend may retain them past the trigger's arena release — load-bearing for in-process arena-backed caches.

### 4.6 Observability callbacks

- **`OnSubscriptionCacheWrite`** (populate): invoked once per cached entry after a successful `Set`, with a `CacheWriteEvent` carrying `CacheKey` (final, post-interceptor), `EntityType` = `EntityTypeName`, `ByteSize` = bytes written, `DataSource` = `DataSourceName`, `CacheLevel` = `CacheLevelL2`, `TTL`, and `Source` = `CacheSourceSubscription`.
- **`OnSubscriptionCacheInvalidate`** (invalidate): invoked once with `(entityType, keys)` where `entityType` = `EntityTypeName` and `keys` is the slice of finalized (post-interceptor) deleted keys.

---

## 5. Cache key & data shape

- **Key shape** is the **entity** shape of [01-ARCHITECTURE-SPEC.md §4](../01-ARCHITECTURE-SPEC.md),
  rendered by `EntityQueryCacheKeyTemplate` from `@key` fields only:
  `{"__typename":"Product","key":{"upc":"top-4"}}`.
  Numbers in keys coerce to strings; the key is alias-independent.

- **Stored value shape (populate).**
  The value is the JSON-encoded entity item containing **only the fields present in the response event**.
  A subscription selecting `{upc, name, price}` writes exactly those fields plus `__typename`,
  never an unselected field such as `inStock`.
  This is **projection**, consistent with the L2 `Passthrough = false` rule of [01-ARCHITECTURE-SPEC.md §3.4](../01-ARCHITECTURE-SPEC.md):
  L2 entries are minimal and self-contained so they round-trip across requests.
  Because the value comes straight from the event payload, the projection is effectively "what the event carried".

- **Invalidate shape.**
  No value is stored; the rendered key is deleted.

- **Key prefixing** follows §4.3.
  With a header prefix configured, a stored key looks like `11111:{"__typename":"Product","key":{"upc":"top-4"}}`.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation seam ([01-ARCHITECTURE-SPEC.md §7](../01-ARCHITECTURE-SPEC.md)).**
  This config uses only the router-facing `LoaderCache` interface (`Set`, `Delete`) and the engine-internal `CacheKeyTemplate` (`EntityQueryCacheKeyTemplate.RenderCacheKeys`).
  It does **not** touch the four-phase hooks (`prepareCacheKeys`, `bulkL2Lookup`, `mergeResult`, the two-boolean merge contract);
  it is a sibling trigger-level path.
  The arena/StructuralCopy isolation discipline of [§3](../01-ARCHITECTURE-SPEC.md) still applies:
  parse onto the arena, never mutate wire bytes, clone keys before they cross the arena boundary.

- **`@key`** ([directives/key.md](key.md)) is the identity source — the key template renders from `@key` fields,
  and a key-only event (only `@key` selected) is what triggers invalidate mode.

- **`@provides`** ([directives/provides.md](provides.md)) can make a child entity fetch unnecessary:
  when reviews are resolved with `author { username }` via `@provides`,
  no `User` entity fetch occurs and there are no cache operations at all for that child.

- **Entity cache config** ([directives/entity-cache-config.md](entity-cache-config.md)) governs the **child** entity fetches that run during per-subscriber fanout.
  Subscription population (root entity) and entity caching (child entities) compose:
  a subscription can populate `Product` at the root while child `User` fetches independently read/write their own L2 entries.

- **Root-field cache config** ([directives/root-field-cache-config.md](root-field-cache-config.md)) does **not** apply to subscription root fields.
  Even if a `Subscription.<field>` root-field cache entry is configured, it must be ignored — subscriptions are never cached as root fields.

- **Mutation cache config** ([directives/mutation-cache-config.md](mutation-cache-config.md)) is the sibling write/invalidate concern for mutations; the two share the L2 write/delete primitives but operate on different operation types.

---

## 7. End-to-end test plan

All tests live under `execution/engine/` and **must** follow [execution/engine/CLAUDE.md](../../entity-caching-v2/execution/engine/CLAUDE.md):
self-contained subtests, inline config and inline GraphQL, no shared helpers, full `assert.Equal` snapshots, vertical multi-item cache-log literals, and a `GetLog()` assertion after every `ClearLog()`.
Use the `products`, `accounts`, `reviews` federation services.
Assertion style is **mandatory**: `assert.Equal` on full values only — never `Contains`, `GreaterOrEqual`, `Greater`, or any fuzzy comparison.

### Case 1 — Populate: event carries fields beyond `@key`

- Config: inline `SubgraphCachingConfigs` for `products` with `SubscriptionEntityPopulation: {{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second}}`.
- Query (inline):
  `subscription UpdatePrice($upc: String!) { updateProductPrice(upc: $upc) { upc name price } }`, vars `{"upc":"top-4"}`, 1 event.
- Cached: one `Product` L2 entry under `{"__typename":"Product","key":{"upc":"top-4"}}`.
- Assertions:
  - Full event message:
    `assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])`.
  - After `ClearLog()`, the full cache log (vertical, one item per line):
    one `Set` entry, key `{"__typename":"Product","key":{"upc":"top-4"}}`, `TTL: 30 * time.Second`.
  - Direct `Get` of the stored value, exact:
    `assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`, string(entries[0].Value))`.

### Case 2 — Projection: only selected fields are stored

- Same config as Case 1.
- Query selects `{upc, name, price}` but NOT `inStock`.
- Assertion: stored value is exactly `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}` — `inStock` absent.
  Full cache-log `Set` assertion as in Case 1.

### Case 3 — List populate: multiple entities cached

- Config for `products` with `FieldName: "updatedPrices"`, 30s TTL.
- Query (inline): `subscription { updatedPrices { upc name price reviews { body authorWithoutProvides { username } } } }`, 1 event.
- Cached: three `Product` L2 entries (`top-1`, `top-2`, `top-3`).
- Assertions: full event message; full cache log with one `Set` entry containing three items (vertical, one key per line, each `TTL: 30 * time.Second`); then exact `Get` value per product, e.g. `{"upc":"top-1","name":"Trilby","price":1,"__typename":"Product"}`.

### Case 4 — Invalidate: key-only event with `EnableInvalidationOnKeyOnly`

- Config for `products` with `EnableInvalidationOnKeyOnly: true`, plus `accounts` entity caching for `User`.
- Pre-populate L2 with `{"__typename":"Product","key":{"upc":"top-4"}}` → `{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`; assert the seed log (`set`).
- `ClearLog()`. Query (inline) selecting only `upc` plus `reviews { body authorWithoutProvides { username } }`, 1 event.
- Assertions:
  - Full event message.
  - Full cache log (vertical): one `Delete` of the Product key, then a `Get` (miss) for both `User` keys, then a `Set` of both `User` keys at 30s.
  - `Get` of the Product key returns `nil` (deleted).
  - `Get` of both `User` keys returns exact values, e.g. `{"__typename":"User","id":"5678","username":"User 5678"}`.

### Case 5 — Key-only event WITHOUT the invalidation flag is a no-op

- Same as Case 4 but `EnableInvalidationOnKeyOnly: false`.
- Assertions: no `Delete` in the cache log (only the child `User` `Get`/`Set`); the Product entry is unchanged — exact `Get` value still `{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`.

### Case 6 — Not configured: no cache operations from subscription

- `SubscriptionEntityPopulation` absent.
- Assertions: after `ClearLog()`, `GetLog()` equals the empty log (`sortCacheLogEntries([]CacheLogEntry(nil))`); a follow-up `Query` for the same product misses and calls the subgraph once (assert subgraph call count `== 1`).

### Case 7 — Header prefix

- Config with `IncludeSubgraphHeaderPrefix: true`; install a `SubgraphHeadersBuilder` whose `products` hash is `11111`.
- Assertions: full cache log shows a `Set` under `11111:{"__typename":"Product","key":{"upc":"top-4"}}`; direct `Get` with the prefixed key returns the exact value.

### Case 8 — Once-per-trigger dedup (multiple subscribers)

- Config populate for `Product`.
- Start 2 (and a separate subtest with 3) subscriptions on the same query/vars (shared trigger); warm up, drain, `ClearLog()`, emit one measured event.
- Assertions: both/all clients receive the identical event message; the cache log contains exactly ONE `Set` (not 2 / not 3); exact stored value asserted.
  Mirror the dedup subtest for invalidate mode (exactly one `Delete`).

### Case 9 — Union / interface return types

- Configure the concrete `TypeName: "Product"` with the union field (`updateProductPriceUnion`) and, in a sibling subtest, the interface field (`updateProductPriceInterface`).
- Assertions: full event message; full cache log with one `Product` `Set`; exact stored value `{"__typename":"Product","upc":"top-4","name":"Bowler","price":1}`.
- Unconfigured-member subtests: configure `Product` but subscribe to a field that returns `DigitalProduct` — assert empty cache log and that both `Product` and `DigitalProduct` keys are `nil` in L2.

### Case 10 — Root alias

- Config populate for `Product`.
- Query aliases the root: `priceUpdate: updateProductPrice(...)`.
- Assertions: full event message under the alias; full cache log with one `Product` `Set` keyed by `@key` (alias-independent); exact stored value.

### Case 11 — Field-name disambiguation (different TTLs)

- One config block with two entries: `{TypeName:"Product", FieldName:"updateProductPrice", TTL: 30s}` and `{TypeName:"Product", FieldName:"updatedPrice", TTL: 60s}`.
- Two subtests: subscribing to `updateProductPrice` produces a `Set` at `TTL: 30 * time.Second`; subscribing to `updatedPrice` produces a `Set` at `TTL: 60 * time.Second`.
- Assertion: full cache log per subtest, each with a trailing comment naming which config was selected and why.

### Case 12 — Callbacks fire

- Populate subtest with `OnSubscriptionCacheWrite` set: assert it is invoked once per entry with the full `CacheWriteEvent` (exact `CacheKey`, `EntityType: "Product"`, exact `ByteSize`, `DataSource`, `CacheLevel: resolve.CacheLevelL2`, `TTL`, `Source: resolve.CacheSourceSubscription`).
- Invalidate subtest with `OnSubscriptionCacheInvalidate` set: assert it is invoked once with `entityType == "Product"` and the full finalized `keys` slice.

### Case 13 — Per-request L2 disable

- `CachingOptions{EnableL2Cache: false}` on the captured subscription context.
- Assertion: no cache operations occur (empty log) even though a populate config is present.

---

## 8. Acceptance criteria

A reviewer can verify each item below against the implementation and the tests above.

- [ ] **No SDL.** No new wire directive is introduced; the concept is the Go config struct only.
- [ ] **Mandatory pair.** `FindByTypeAndFieldName` matches on both `TypeName` and `FieldName`; an empty `FieldName` silently no-ops, and the router sets `FieldName` on both populate and invalidate paths.
- [ ] **Populate writes projected data.** On an event carrying fields beyond `@key`, one L2 `Set` per entity item with the configured TTL; the stored value contains only the fields present in the response plus `__typename`.
- [ ] **Invalidate deletes.** On a key-only event with `EnableInvalidationOnKeyOnly`, one L2 `Delete` per item and no `Set`.
- [ ] **Key-only without the flag is a no-op** for the root entity (no `Delete`).
- [ ] **Entity key shape.** Keys are rendered by `EntityQueryCacheKeyTemplate` from `@key` fields only, alias-independent, numbers coerced to strings.
- [ ] **Key pipeline parity.** Global prefix → header-hash prefix (gated on `IncludeSubgraphHeaderPrefix` + `SubgraphHeadersBuilder`) → template key → `L2CacheKeyInterceptor`, matching the query-side flow.
- [ ] **Once per trigger.** Exactly one cache operation per affected entity per event, regardless of subscriber count; then a single fanout.
- [ ] **Ordering.** Populate completes before fanout; a child entity fetch on the same event observes the L2 hit.
- [ ] **`__typename` filtering & injection.** Union/interface members of unconfigured types are skipped; missing `__typename` is injected (on the parsed copy, never on wire bytes) before key rendering.
- [ ] **Alias safe.** Root-field aliases do not change the cache key or break population.
- [ ] **Root-field cache excluded.** A configured `Subscription.<field>` root-field cache entry never applies.
- [ ] **Defensive guards.** Missing `CacheName` and per-request `EnableL2Cache == false` both skip cleanly; backend `Set`/`Delete` errors never block delivery.
- [ ] **Key lifetime.** Rendered keys are cloned before crossing the trigger arena boundary into the backend.
- [ ] **Callbacks.** `OnSubscriptionCacheWrite` fires once per entry on populate with the full `CacheWriteEvent`; `OnSubscriptionCacheInvalidate` fires once with `(entityType, keys)` on invalidate.
- [ ] **Tests conform.** All E2E tests use `assert.Equal` on full values, inline literals, vertical multi-item cache-log literals, and a `GetLog()` assertion after every `ClearLog()`.
