# ADR-0009: Subscription population config caching support

## Status

Proposed

## Context

The foundation ([ADR-0001](0001-foundation.md)) establishes the entity-caching seam: a thin `entityCache` collaborator on the loader,
the `LoaderCache` (L2) interface, the `CacheKeyTemplate` key seam, the `ProvidesData *Object` alias-aware field shape,
the per-fetch `cacheSkipFetch` / `cacheMustBeUpdated` booleans, the StructuralCopy isolation discipline,
and the load-bearing key-transform pipeline (`GlobalCacheKeyPrefix` → subgraph header-hash prefix → `L2CacheKeyInterceptor`).
Entity cache config (ADR-0006) sits on top: it binds an entity type to a named cache, a TTL, and the entity key template,
and drives the query-path read/write of L1 and L2.

Subscriptions are the third GraphQL operation type, and they have a cache behavior that neither queries nor mutations cover.
A subscription emits entity data on **every event**, and that stream is the freshest possible view of an entity's state.
The problems this configuration concept solves:

1. **Subscription streams are a free source of fresh entity data.**
A subscription such as `updateProductPrice(upc:)` pushes a new `Product` on every change.
If that data is dropped after fanout, then every downstream query (and every subscriber that runs a child `@key`-resolved entity fetch) round-trips to the entity-owning subgraph,
even though the gateway just observed the entity's current state on the wire.
Writing each event's entity into L2 lets those later reads hit cache instead.

2. **Key-only subscriptions are an invalidation signal, not a population signal.**
Some subscriptions return only the entity's `@key` fields and mean "this entity changed, drop your copy."
For those, the correct action is to **delete** the L2 entry, not to write a stub that has no payload fields.
The two behaviors live behind one config: populate when the event carries fields beyond `@key`, invalidate when it carries only `@key` and `EnableInvalidationOnKeyOnly` is set.

3. **The cache operation must happen once per event, ordered before fanout, and never block delivery.**
N subscribers sharing one trigger must cause exactly one cache operation per affected entity, not N.
A subscriber that runs a child entity fetch for the same event must see the just-written value, so populate must complete before fanout.
And the subscription stream is the contract with the client, so a cache backend error must never abort an event.

None of this is a wire directive.
It arrives as Go configuration, supplied per subgraph through `SubgraphCachingConfig` as a `SubscriptionEntityPopulationConfiguration`
(carried to the resolver on the subscription, with an `*EntityQueryCacheKeyTemplate` for key rendering).
The config has a sharp footgun the foundation already flags: the lookup `FindByTypeAndFieldName` matches on **both** `TypeName` and `FieldName`,
so a config with an empty `FieldName` silently never fires.
The full behavioral contract — populate/invalidate semantics, once-per-event ordering, union/interface filtering, `__typename` injection, the key pipeline, and the callbacks —
lives in [../directives/subscription-cache-config.md](../directives/subscription-cache-config.md).

## Decision

Subscription population config is implemented as a **trigger-level cache operation on the subscription resolution path**, with **no new loader hot-path code**.
The query/mutation seams of ADR-0001 are reused conceptually (key rendering, projection, the transform pipeline, StructuralCopy isolation),
but the subscription path is a distinct code path in the resolver, so the operation hangs off it rather than off `mergeResult`.

### How it plugs into the foundation seam

**Plan-time / config metadata (additive, no schema syntax).**
The router supplies a `SubscriptionEntityPopulationConfiguration` per (subgraph, type, field), selected by `FindByTypeAndFieldName(typeName, fieldName)`.
Its load-bearing fields:

- `TypeName` and `FieldName` — **both mandatory**; the lookup matches on both, and an empty `FieldName` is a silent no-op.
  This pair is also what disambiguates two subscriptions on the same entity type with different TTLs (one config per field name).
- `CacheName` — selects the named L2 backend from `ResolverOptions.Caches`; a missing name is a silent no-op (defensive guard).
- An `*EntityQueryCacheKeyTemplate` (`CacheKeyTemplate`) — renders entity keys from the event's items, exactly the entity key shape of ADR-0001 §7.5.
- `EntityTypeName` — the concrete entity type, used to filter union/interface events and to inject `__typename` when absent.
- `SubscriptionFieldName` — if set, the resolver navigates into this field of the event data before treating items as entities.
- `EnableInvalidationOnKeyOnly` — selects invalidate-on-key-only behavior over populate.
- `TTL`, `DataSourceName`, `IncludeSubgraphHeaderPrefix` — the same TTL and key-prefix inputs every other L2 write uses.

`SubscriptionEntityPopulationConfiguration` is plain config, not a new `plan` dependency pulled into `resolve`,
consistent with the `EntityCacheInvalidationConfig` split in ADR-0001 §7.4.

**Resolver hook (the subscription path, not the merge funnel).**
On every trigger event the resolver runs one cache operation over the event payload, before fanning the event out to subscribers:

1. **Guards first.**
   Return early (no cache op, no error) when the config or its key template is nil,
   when `CacheName` is not present in `Caches`,
   or when the captured request context has `ExecutionOptions.Caching.EnableL2Cache == false`.
   These guards keep the disabled path free and ensure misconfiguration never blocks delivery.
2. **Parse a working copy.**
   The event bytes are parsed onto an arena value; the resolver navigates into `SubscriptionFieldName` if set, then treats the result as one entity item or a list of items.
   `__typename` injection and any normalization happen on this parsed copy — **never** on the bytes handed to subscribers (R6).
3. **Per-item type filtering.**
   For union/interface return types, items whose `__typename` differs from `EntityTypeName` are skipped;
   items with no `__typename` get `EntityTypeName` injected so the entity key template renders a correctly-typed key (R5, R6).
4. **Key construction reuses the exact query-path pipeline.**
   `EntityQueryCacheKeyTemplate.RenderCacheKeys` builds the entity key from the item;
   the key then passes through the **same transform pipeline** as every other key —
   global prefix concatenated with the subgraph header-hash prefix (when `IncludeSubgraphHeaderPrefix` is set) into the rendered prefix, then `L2CacheKeyInterceptor` applied after.
   Byte-identity with the read path is mandatory or the entry is targeted at the wrong key (R7).
5. **Populate vs invalidate.**
   In populate mode the resolver projects the item to its present fields and `Set`s a `CacheEntry{Key, Value, TTL}` per entity (R1).
   In invalidate mode (key-only event with `EnableInvalidationOnKeyOnly`) the resolver `Delete`s the rendered key and performs no `Set` (R2).
6. **Once per event, ordered before fanout, non-blocking.**
   The cache operation runs exactly once for the whole event payload regardless of subscriber count (R3),
   completes before the per-subscriber resolution path that may trigger a child entity fetch (R4),
   and swallows backend `Set`/`Delete` errors so the event is always delivered (R10).
7. **Key lifetime.**
   Rendered keys are cloned off the trigger arena before being handed to the backend, because the arena is released when the trigger event's resolve completes and the backend may retain the key (R13).

**No loader hot-path changes.**
The walker dispatch, the four-phase parallel machinery, and `mergeResult`'s signature are untouched.
Subscription caching lives on the subscription resolution path inside the `entityCache` collaborator and the subscription event handling,
not inline in the query/mutation merge funnel.
Whether the operation runs synchronously on the trigger goroutine or asynchronously with a completion barrier before fanout is implementation latitude,
provided the once-per-event (R3) and ordered-before-fanout (R4) guarantees hold.

### Event-driven callbacks (the deliberate push exception)

Analytics in the foundation is pull-based (`GetCacheStats()`), but subscriptions are inherently event-driven and outlive any single request's snapshot point,
so subscription cache effects are reported through dedicated push callbacks instead — the exception ADR-0001 already records:

- `ResolverOptions.OnSubscriptionCacheWrite` fires once per cached entry after a successful populate,
  with a `CacheWriteEvent{CacheKey, EntityType, ByteSize, DataSource, CacheLevel: CacheLevelL2, TTL, Source: CacheSourceSubscription}` (R11).
- `ResolverOptions.OnSubscriptionCacheInvalidate` fires once after a successful invalidate,
  with `(entityType string, keys []string)` listing the finalized post-interceptor keys deleted (R12).

These are router-set, opt-in, and zero-cost when unset.

### How the PR stacks

This work ships as **gqtools PR 15 / PR-CACHE-SUBSCRIPTION**, stacked on the foundation PR and on the entity cache config PR (ADR-0006).
It depends on them because subscription population builds entity-shaped keys (needs `@key` and the `EntityQueryCacheKeyTemplate`)
and writes through the same L2 projection and transform pipeline (needs the entity cache config and the key-prefix contract).
It reuses the post-effect cache-write/delete pattern that mutation invalidation (ADR-0008) established, rather than inventing a new one.
Because it adds only a trigger-level operation on the already-distinct subscription path plus two opt-in callbacks, it is independently reviewable against the now-frozen seam.

## Consequences

### Positive

- **Subscriptions warm and refresh the cache for free.** Every event keeps L2 consistent with the latest observed entity state, so downstream queries and child entity fetches hit cache instead of round-tripping to the owning subgraph.
- **One config, two correct behaviors.** Populate and key-only invalidate are selected by `EnableInvalidationOnKeyOnly` plus the event's field set, so a "this changed, drop it" stream and a "here's the new value" stream are both handled without separate machinery.
- **Delivery is never coupled to cache health.** Backend errors are swallowed, a missing cache name or disabled L2 is a silent no-op, and the cache op is ordered before fanout — so caching can be enabled on a live subscription with no risk to event delivery.
- **Zero new loader surface.** The query/mutation hot path is untouched; the operation lives entirely on the subscription path and the `entityCache` collaborator.

### Negative / costs

- **Key-pipeline duplication risk.** The subscription path must reproduce the full key-transform pipeline (header prefix → global prefix → interceptor) byte-for-byte, or populated/invalidated keys miss the read-path entry. Mitigated by routing through the same key-building helpers `prepareCacheKeys` uses, but the two must stay in lockstep.
- **The mandatory-pair footgun.** `FindByTypeAndFieldName` matches on both `TypeName` and `FieldName`; an empty `FieldName` makes the whole config a silent no-op with no error. The router integration must set `FieldName` on **both** populate and invalidate configs, and this must be tested, because the failure mode is invisible.
- **No L1 participation.** Subscription caching is L2-only; L1 hit/miss accounting and `@requestScoped` coordinate L1 within an event are explicitly out of scope, so the per-event win is cross-request, not within a single event's fanout.
- **Ordering constraint is load-bearing.** The before-fanout completion barrier (R4) is required for a same-event child entity fetch to see the populated value; any refactor of the subscription event loop must preserve it.

### Performance implications

- One cache operation per **event**, not per subscriber: N subscribers on one trigger amortize to a single `Set`/`Delete` batch per affected entity (R3), so high fan-out subscriptions do not multiply backend load.
- Populate writes are projected to present fields only and serialized to heap bytes once per entity, exactly like the query-path L2 write; there is no extra round-trip and no mutation-style staleness read.
- With L2 disabled per request, a missing cache name, or no config, the path is a guard-clause early return — effectively free.

### What becomes possible for later directives

- The trigger-level populate/invalidate pattern, plus the push callbacks, give a template for any future event-driven cache effect (for example, change-data-capture style invalidation feeds) without touching the request-scoped pull-analytics model.
- Sharing the key pipeline and L2 projection across query, mutation, and subscription writes means a single place governs cache-entry shape and identity, so later directives that add new write sources inherit byte-identical keys for free.

## Alternatives considered

### A. Reuse the query/mutation merge funnel for subscription caching

Route subscription events through `mergeResult` and the post-merge invalidation seam, so all three operation types share one write/delete site.
**Rejected.**
The subscription path is structurally different: it is trigger-driven, fans one event out to many subscribers, must run the cache op exactly once before fanout, and outlives any single subscriber's request context.
Forcing it through the per-request merge funnel would either run the op once per subscriber (violating R3) or contort the funnel with subscription-only branching.
A dedicated trigger-level operation that reuses the key pipeline and projection helpers (but not the merge funnel) keeps both paths clean.

### B. Pull-based analytics for subscription cache effects instead of callbacks

Record subscription writes/invalidations into the same pooled collector that `GetCacheStats()` snapshots.
**Rejected.**
A subscription is long-lived and emits events indefinitely, so there is no single "after resolve" point at which to snapshot, and the pooled collector is released per request.
Event-driven `OnSubscriptionCacheWrite` / `OnSubscriptionCacheInvalidate` callbacks fit the lifetime, which is exactly the deliberate exception ADR-0001 carves out for subscriptions.

### C. Make `FieldName` optional and match on `TypeName` alone

Drop the mandatory-pair rule and let one config apply to all subscription fields of an entity type.
**Rejected.**
Two subscriptions on the same entity type can legitimately need different TTLs or different populate/invalidate behavior (the field-name disambiguation case, R14).
Matching on `TypeName` alone would make those ambiguous and force the first config to win.
Keeping both fields mandatory is the explicit, testable contract; the cost is the silent-no-op footgun, which is documented and guarded by router-side tests rather than designed away.

### D. Always invalidate on every event (never populate)

Treat every subscription event purely as a "this entity changed, drop your copy" signal and only ever `Delete`.
**Rejected.**
That discards the freshest data the gateway will ever see and forces a subgraph round-trip on the next read, when the event already carried the new value.
Populate-when-fields-present plus invalidate-on-key-only captures both real-world stream shapes; a blanket invalidate would waste the populate opportunity entirely.

## References

- [ADR-0001: Foundation](0001-foundation.md) — the integration seam, L1/L2 model, key-transform pipeline, StructuralCopy invariants, and the subscription-callback exception to pull-based analytics.
- [ADR-0006: Entity cache config](0006-entity-cache-config.md) — binds the entity type to a named cache, TTL, and the entity key template the subscription path renders against.
- [ADR-0008: Mutation cache config](0008-mutation-cache-config.md) — the post-effect populate/invalidate pattern this directive mirrors on the subscription path.
- [../directives/subscription-cache-config.md](../directives/subscription-cache-config.md) — the detailed subscription population config contract.
- [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) — directive taxonomy and PR mapping.
- `docs/entity-caching/SUBSCRIPTION_CACHE_SPEC.md` — the behavioral requirements R1–R14 this ADR summarizes.
- `execution/engine/federation_subscription_caching_test.go` — populate, key-only invalidate, once-per-event, before-fanout ordering, header-prefix, union/interface filtering, field-name disambiguation, and callback coverage.
