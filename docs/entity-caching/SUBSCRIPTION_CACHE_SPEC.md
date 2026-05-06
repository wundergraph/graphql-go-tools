# Subscription Entity Cache — Behavioral Specification

This document specifies the behavior of trigger-level entity caching for GraphQL subscriptions on the `feat/add-caching-support` branch.

It is the contract the `resolve` package must satisfy regardless of how subscription event handling is internally implemented (event-loop, direct dispatch, goroutine fanout, etc.).

The corresponding integration tests live in `execution/engine/federation_subscription_caching_test.go` and `execution/engine/federation_caching_source_test.go`.

## Purpose

Subscriptions emit entity data on every event.
Without caching support, every downstream subscriber that triggers a child entity fetch (`@key`-resolved field) round-trips to the entity-owning subgraph for each event.

This spec describes how the resolver populates and invalidates the L2 entity cache **on every trigger event**, so that:

- Subsequent subscription events (and queries) can resolve child entity fetches from cache.
- The cache stays consistent with the latest entity state observed via the subscription stream.

## Configuration Surface

The router supplies, per subscription, a `SubscriptionEntityCachePopulation` struct (carried on `GraphQLSubscription.EntityCachePopulation`).
Relevant fields:

| Field | Purpose |
|-------|---------|
| `Mode` | `SubscriptionCacheModePopulate` or `SubscriptionCacheModeInvalidate`. |
| `CacheName` | Logical name of the L2 cache backend (looked up in `Resolver.options.Caches`). |
| `CacheKeyTemplate` | An `*EntityQueryCacheKeyTemplate` that renders cache keys from entity data. |
| `EntityTypeName` | The entity type whose data is being cached (used for `__typename` filtering / injection). |
| `SubscriptionFieldName` | If set, navigate into this field of the response data before treating items as entities. |
| `DataSourceName` | Subgraph name (used to retrieve subgraph-scoped header hash). |
| `IncludeSubgraphHeaderPrefix` | Whether to prepend a header-hash segment to the cache key. |
| `TTL` | TTL for `Set` operations. |

When this struct is `nil` (or its `CacheKeyTemplate` is `nil`), the subscription has no cache integration and nothing in this spec applies.

## Behavioral Requirements

### R1 — Populate mode writes entity data to L2 on every trigger event

When `Mode == SubscriptionCacheModePopulate`:

- For each entity item in the subscription response data, the resolver MUST `Set` an L2 cache entry whose:
  - **Key** is rendered from `CacheKeyTemplate` against the item, using the request's variables.
  - **Value** is the JSON-encoded entity item (only the fields present in the response).
  - **TTL** is `pop.TTL`.
- All entries from a single trigger event are submitted to the cache backend (callers may batch them or issue them sequentially — both satisfy the spec).

E2E coverage: `subscription entity populates L2 - verified via cache`, `subscription entity list populates L2 - multiple entities cached`, `subscription populates L2 - cached data has only selected fields`.

### R2 — Invalidate mode deletes the entity's L2 entry

When `Mode == SubscriptionCacheModeInvalidate`:

- For each entity item, the resolver MUST `Delete` the L2 entry under the rendered cache key.
- No `Set` is performed in this mode.

This mode is intended for "key-only" subscriptions (subscription returns just `@key` fields and signals "this entity changed, drop your copy").

E2E coverage: `key-only subscription invalidates L2 cache`, `invalidation on every event`.

### R3 — Cache operation runs once per trigger event, not once per subscriber

When N subscribers share the same trigger (same data source, same input), a trigger event MUST cause exactly ONE cache operation per affected entity, not N.

The order is:

1. Trigger emits an event with bytes `data`.
2. The resolver performs the configured cache operation (populate or invalidate) ONCE for the entire `data` payload.
3. After the cache operation completes, the resolver fans the same `data` out to all N subscribers.

E2E coverage: `entity population happens once per trigger event with multiple subscriptions`, `entity invalidation happens once per trigger event with multiple subscriptions`, `three clients - cache operations still happen once`.

### R4 — Cache populate completes before subscriber fanout

A subscriber that runs a child entity fetch as part of resolving the same event (or the next event) MUST be able to read the just-populated entity from L2.

In practice this means:

- Populate (`Set`) MUST complete before the resolver invokes the per-subscriber resolution path that may trigger a child entity fetch.
- Equivalently: a child entity fetch issued during the same event's fanout sees an L2 hit.

E2E coverage: `cache populate completes before child entity fetch`, `L2 pre-populated - subscription child fetch hits L2`, `multiple subscription events share L2 - second event skips fetch`.

(Note: the implementation may achieve this via synchronous-before-fanout, or via an async-then-fanout pattern that waits for completion before scheduling subscriber work, or via event-loop coordination. Any approach that guarantees the ordering satisfies R4.)

### R5 — Filter items by EntityTypeName for union/interface return types

When a subscription returns a union or interface type, the response may contain a mix of entity types.

For each item:

- If the item has a `__typename` field equal to `pop.EntityTypeName`, include it in the cache operation.
- If the item has a `__typename` field NOT equal to `pop.EntityTypeName`, SKIP it (it belongs to a different concrete type and is not subject to this configuration).

E2E coverage: `subscription union return type - entity population works`, `subscription interface return type - entity population works`, `subscription union return type - unconfigured type not cached`, `subscription interface return type - unconfigured type not cached`.

### R6 — Inject EntityTypeName when __typename is missing

For items that lack `__typename` (common for narrowly-typed root fields), the resolver MUST inject `pop.EntityTypeName` as `__typename` on the item before rendering the cache key.

This injection is required so that `EntityQueryCacheKeyTemplate.RenderCacheKeys` produces correctly typed cache keys.

The injection MUST be done on a parsed copy of the response data — it MUST NOT mutate the bytes the resolver hands to subscribers.
(Equivalently: write to an arena-allocated parsed value, not to the wire bytes.)

Implicit e2e coverage: `subscription entity populates L2 - verified via cache` (queries don't always select `__typename`).

### R7 — Cache key construction matches non-subscription cache flow

The cache key construction pipeline MUST match what `prepareCacheKeys()` and `processExtensionsCacheInvalidation()` use in the standard query flow:

1. **Global prefix** (`ExecutionOptions.Caching.GlobalCacheKeyPrefix`)
2. **Subgraph header hash prefix** (when `pop.IncludeSubgraphHeaderPrefix == true` and `SubgraphHeadersBuilder` is set)
3. **Template-rendered key** (`CacheKeyTemplate.RenderCacheKeys`)
4. **L2CacheKeyInterceptor** transform (when configured on the request context)

Steps 1–3 are concatenated into the prefix passed to `RenderCacheKeys`.
Step 4 is applied to the rendered keys after.

E2E coverage: `subscription entity population with header prefix`.

### R8 — Per-request L2 disable bypasses the cache operation

When the request context has `ExecutionOptions.Caching.EnableL2Cache == false`, the cache operation MUST be skipped.

(For subscriptions, the request context here is the captured-at-subscription-creation context, since trigger-level operations run independently of any one subscriber's context.)

### R9 — Missing cache name is a silent no-op

If `pop.CacheName` is not present in `Resolver.options.Caches`, the resolver MUST return without performing any cache operation and without raising an error.

This is a defensive guard against misconfiguration; subscription delivery MUST NOT be blocked.

### R10 — Cache backend errors do not block subscription delivery

`Set` and `Delete` errors from the L2 backend MUST NOT propagate to the subscription event flow.
The event MUST be delivered to subscribers regardless of cache backend health.

(Errors MAY be reported via callbacks or logs; they MUST NOT abort the trigger.)

### R11 — OnSubscriptionCacheWrite callback fires on populate

When `ResolverOptions.OnSubscriptionCacheWrite` is set, after a successful populate:

- The callback MUST be invoked once per cached entry, with a `CacheWriteEvent` containing:
  - `CacheKey` — the final key (post-interceptor)
  - `EntityType` — `pop.EntityTypeName`
  - `ByteSize` — size of the JSON payload written
  - `DataSource` — `pop.DataSourceName`
  - `CacheLevel` — `CacheLevelL2`
  - `TTL` — `pop.TTL`
  - `Source` — `CacheSourceSubscription`

E2E coverage: `OnSubscriptionCacheWrite fires on subscription entity population`.

### R12 — OnSubscriptionCacheInvalidate callback fires on invalidate

When `ResolverOptions.OnSubscriptionCacheInvalidate` is set, after a successful invalidate:

- The callback MUST be invoked once with `(entityType string, keys []string)` where:
  - `entityType` — `pop.EntityTypeName`
  - `keys` — the slice of finalized keys (post-interceptor) that were deleted

E2E coverage: `OnSubscriptionCacheInvalidate fires on invalidation-only subscription`.

### R13 — Cache key strings are independent of the trigger arena

Cache keys passed to backend `Set`/`Delete` calls MUST remain valid after the trigger's resolve arena is released.

In practice: keys rendered onto an arena MUST be `strings.Clone`d (or similarly copied) before being handed to the cache backend, since the backend may retain them past the trigger's lifecycle.

(This is an implementation requirement, not an externally observable behavior — but it's load-bearing because in-process arena-based caches will otherwise return corrupted keys.)

### R14 — Field-name disambiguation for shared entity types

A subgraph may emit two different subscriptions on the same entity type with different `SubscriptionFieldName`s and different TTLs.
The resolver MUST select the correct `SubscriptionEntityCachePopulation` config (typically by both `TypeName` AND `FieldName`) for each subscription.

E2E coverage: `subscription field-name disambiguation - updateProductPrice uses 30s TTL`, `subscription field-name disambiguation - updatedPrice uses 60s TTL`.

(The router-side lookup helper is `SubscriptionEntityCachePopulationConfigs.FindByTypeAndFieldName(typename, fieldname)`.)

## Non-Requirements (explicitly out of scope)

The following are NOT part of this spec; their behavior is governed by other specs / unit tests:

- L1 cache hit/miss accounting on subscription events.
- `@requestScoped` coordinate L1 within a subscription event (covered by request-scoped L1 spec).
- Cache circuit breaker behavior in subscription context.
- Cache analytics snapshot recording during subscription events.
- Per-request `X-WG-Disable-Entity-Cache*` headers in subscription context (these are router-side concerns).

## Implementation Latitude

The following are implementation details and MAY change without changing this spec:

- Whether the trigger-level cache operation runs synchronously on the trigger goroutine or asynchronously on a separate goroutine, **provided** R4 (ordering) is satisfied.
- Whether the cache operation uses an event-loop event kind (`subscriptionEventKindTriggerCacheDone`) or a direct call/wait pattern.
- The internal name and shape of the per-trigger cache config holder (`triggerEntityCacheConfig` is a current implementation detail, not a contract).
- The exact lifecycle hook used to install the per-trigger cache config (`buildTriggerCacheConfig` at trigger creation is a current implementation detail).
- The choice of `errgroup`, `sync.WaitGroup`, channel, or worker pool for fanning the event out to subscribers, **provided** R3 (single cache op per event) and R4 (ordering) hold.
