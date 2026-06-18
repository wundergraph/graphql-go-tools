# ADR-0006: Entity caching config caching support

## Status

Proposed

## Context

The foundation ([0001-foundation.md](0001-foundation.md)) ships the *machinery* for entity caching but deliberately ships **no policy**.
It establishes the integration seam into the loader,
the `LoaderCache` (L2) and `CacheKeyTemplate` (key rendering) interfaces,
the per-fetch `FetchCacheConfiguration` data shape,
the per-request `CachingOptions` toggles,
and the StructuralCopy discipline that keeps cache entries and the live response tree from corrupting each other.
What the foundation does **not** answer is the most basic operational question: *for a given entity type, should it be cached at all,
in which named cache,
and for how long?*

Without that binding, every entity fetch carries a key template but no L2 destination,
so L2 is effectively off and only the per-request L1 dedup runs.

Entity caching is **opt-in per entity type per subgraph**.
A subgraph that owns `User` and `Product` may want `Product` cached in a long-lived shared cache,
`User` cached briefly in a tenant-isolated cache,
and a third type not cached at all.
The engine cannot infer this from the schema —
`@key` tells it *how to identify* an entity (see [0002-key.md](0002-key.md)) but says nothing about TTL,
cache backend choice,
or whether a not-found result should be remembered.
That intent has to arrive as configuration.

This is the problem the **entity caching config** solves.
It is not a wire directive — there is no SDL syntax for it.
It is a Go configuration concept, `EntityCacheConfiguration`,
supplied per subgraph by the router (which typically synthesises it from composition output plus operator config).
It binds one entity type to one named cache,
a TTL,
an optional negative-cache TTL (how long a not-found / null result is remembered),
and the controls that govern serving data that may be stale within bounds (partial cache load and shadow mode).
The full field-by-field contract lives in the directive spec at [../directives/entity-cache-config.md](../directives/entity-cache-config.md);
this ADR records *why and how* it plugs into the foundation seam.

Per the directive inventory ([02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)),
this config is consumed by the planner caching state,
the resolver entity L1/L2 read/write paths,
and the cache-key construction.
It depends on `@key` being correct (keys are derived from key fields) and is itself the prerequisite for the root-field,
mutation,
and subscription config concepts that read and invalidate what it populates.

## Decision

Re-implement the entity caching config as **gqtools PR 4 / PR-CACHE-CONFIG**,
stacked directly on the foundation PR.
It adds **plan-time policy lookup** and the **per-fetch annotations** that turn the foundation's dormant L2 path on for configured entity types.
It changes the loader/resolvable hot path in **zero** new places —
it only populates the `FetchCacheConfiguration` fields the foundation already defined and the resolver already reads.

### Where the config lives and how it is supplied

The router supplies one `EntityCacheConfiguration` per (subgraph, entity type) through the execution-engine factory option `WithSubgraphEntityCachingConfigs`,
which takes `SubgraphCachingConfigs` —
a list of `SubgraphCachingConfig`, each carrying its subgraph name plus an `EntityCaching EntityCacheConfigurations` slice (and the sibling root-field / mutation / subscription configs covered by their own ADRs).
The factory copies the `EntityCaching` slice onto that subgraph's `FederationMetaData.EntityCaching`,
so by plan time each datasource carries its own entity-cache policy.

The minimal binding contract:

- One config per (subgraph, type) — keyed by `TypeName`.
- Multiple types may share a backing store by reusing the same `CacheName`.
- A `CacheName` is a logical handle; the actual `LoaderCache` instance is registered separately by the router in `ResolverOptions.Caches map[string]LoaderCache` and resolved by name at resolve time.
- Absence of a config for a type means **L2 disabled for that type** (opt-in model).
  L1 is unaffected — it rides on the key template the foundation already preserves.

Lookup is a single method on the federation metadata,
`FederationMetaData.EntityCacheConfig(typeName) *EntityCacheConfiguration`,
returning `nil` when the type is not configured.

### How it plugs into the foundation seam (no hot-path changes)

All new behavior lands in the **planner caching state** (`configureFetchCaching`),
not in the loader.
The planner already builds a `FetchCacheConfiguration` for every fetch and always preserves the `CacheKeyTemplate` (so L1 works regardless of L2 policy).
This PR adds one branch:
for an entity fetch (`RequiresEntityFetch` or `RequiresEntityBatchFetch`),
it reads the entity type name from the fetch's root field,
calls `EntityCacheConfig(entityTypeName)`,
and — when a config exists — fills in the L2-bearing fields of `FetchCacheConfiguration`:

- `Enabled = true` (this is the single boolean the foundation's loader checks to run the L2 read/write path),
- `CacheName`, `TTL`, `IncludeSubgraphHeaderPrefix`,
- `NegativeCacheTTL` (negative-cache window for null results),
- `EnablePartialCacheLoad` (serve cached entities, fetch only the missing ones — the bounded-staleness serve path),
- `ShadowMode` (read and write L2 but never serve cached data; always fetch fresh and compare — the staleness-detection serve mode),
- `HashAnalyticsKeys`,
- and `KeyFields` (the `@key` fields, extracted from the entity key template for analytics).

When no config exists,
the planner still attaches `KeyFields` for analytics but leaves `Enabled = false`,
so L1 keeps working and L2 stays off.
`UseL1Cache` is intentionally **not** set here —
it remains the responsibility of the L1-optimizer post-process pass described in the foundation,
keeping the two policies (L2 destination vs. L1 enablement) cleanly separated.

At resolve time **nothing new is added to the loader**.
The foundation's existing hooks already do the work:
`prepareCacheKeys` renders keys from the preserved template,
`bulkL2Lookup` / `tryL2CacheLoad` consult L2 only when `Enabled` is set and the named cache resolves,
and `updateL2Cache` writes back honoring per-entry `TTL` and the negative-cache TTL via the `CacheEntry.TTL` field (recall `Set` takes per-entry TTL, not a `Set` argument).
The negative-cache behavior — storing a sentinel for a null `_entities` result and serving it on the next request until `NegativeCacheTTL` elapses — is exercised end to end by the resolver in `v2/pkg/engine/resolve/negative_cache_test.go`,
which proves the config-to-behavior path without any loader signature change.

### How the PR stacks

PR-CACHE-CONFIG depends on three things already in flight or merged below it:

1. the **foundation** seam and `FetchCacheConfiguration` shape ([0001-foundation.md](0001-foundation.md)),
2. **`@key`** for entity identity and the entity key template ([0002-key.md](0002-key.md)),
3. **`@provides`** / **`@requires`** for the projected `ProvidesData` shape that the L2 read/write copies project against ([0004-provides.md](0004-provides.md), [0003-requires.md](0003-requires.md)).

It is the first *config concept* in the stack.
Root-field caching ([0007-root-field-cache-config.md](0007-root-field-cache-config.md)) builds on it (root fields share entity cache entries via key mappings),
and mutation ([0008-mutation-cache-config.md](0008-mutation-cache-config.md)) and subscription ([0009-subscription-cache-config.md](0009-subscription-cache-config.md)) configs read and invalidate what it populates.
Because the diff is confined to the planner caching state plus the config struct and its lookup,
it is small,
additive,
and reviewable in isolation.

## Consequences

### Positive

- The first directive PR that makes L2 actually do something — it activates the dormant foundation path for configured entity types.
- Zero new loader/resolvable touch points; the change is a planner branch plus a config struct and one lookup method.
- Opt-in by type keeps the blast radius small: an unconfigured type behaves exactly as today (L1-only dedup, no external I/O).
- Per-(subgraph, type) granularity with shared `CacheName` lets operators mix policies (long-lived shared cache for one type, short TTL isolated cache for another) without engine changes.
- Negative caching, partial load, and shadow mode are all expressed as plain fields on the same struct, so later behaviors compose without new seams.
- Establishes the config-supply pattern (`SubgraphCachingConfig` → `FederationMetaData`) that the root-field, mutation, and subscription ADRs reuse verbatim.

### Negative / costs

- The config is opaque to the schema: a type can be `@key`-correct yet silently uncached because no `EntityCacheConfiguration` was supplied. This is intentional (opt-in) but means "why isn't this cached?" is a config question, not a schema one.
- `CacheName` is a string handle decoupled from the actual backend; a typo or an unregistered name resolves to no cache at resolve time and silently disables L2 for that type. The router must keep `EntityCaching` names and `ResolverOptions.Caches` keys in sync.
- A zero `TTL` means entries never expire — convenient for tests, a footgun in production. The contract documents it; it cannot be enforced at the engine boundary.
- More config surface for operators to get right (TTL, negative TTL, partial load, shadow mode, header-prefix), all per type.

### Performance implications

- For configured types: one extra L2 round trip on a miss (already part of the foundation's bulk-Get path), amortised across all keys routed to the same cache instance via `bulkL2Lookup`.
- For unconfigured types: zero added cost — `Enabled = false` short-circuits before any key transform or cache call; the path is identical to the cache-disabled foundation behavior.
- Negative caching trades a small write (a sentinel) for avoiding repeated subgraph lookups of non-existent entities — a net win whenever not-found is hot.
- Partial cache load reduces subgraph fan-out (fetch only missing batch members) at the cost of serving entities that may be stale within their TTL window — a deliberate, configured trade.
- Shadow mode always pays for both the subgraph fetch and the cache read/write/compare; it is a diagnostic mode, not a performance mode, and never serves cached data.

### What becomes possible for later directives

- **Root-field caching** can map a root query onto the *same* entity cache entries this config defines, so `Query.user(id:"1")` and an `_entities` fetch for `User{id:"1"}` share L2 storage.
- **Mutation caching** can locate the entity cache config for a mutation's return type to decide what to invalidate or repopulate after a successful mutation.
- **Subscription population** can reuse the same per-type cache binding (cache name, TTL, header prefix) to populate or invalidate L2 on each event.
- All three inherit the `CacheName` → `LoaderCache` resolution and the per-entry-TTL write contract established here.

## Alternatives considered

### A. Express caching as a real SDL directive (e.g. `@cache(ttl: ..., name: ...)` on the type)

Make caching a wire directive so policy travels with the schema and is validated at composition time,
like `@requestScoped`.
**Rejected.**
Caching policy is an *operational* concern (which Redis, what TTL, tenant isolation) that legitimately differs per environment and per subgraph deployment,
not a *semantic* property of the graph.
Baking it into SDL would force schema changes for ops tuning,
couple composition to backend topology,
and lose the ability to vary policy per environment from the same supergraph.
Keeping it as router-supplied Go config (`EntityCacheConfiguration`) keeps the schema clean and the policy where it belongs —
in the router's runtime configuration.
The one genuinely semantic caching concept, `@requestScoped`, *is* a directive, which confirms the split.

### B. A single global cache policy instead of per-(subgraph, type) config

Apply one TTL and one cache backend to every entity, configured once on the resolver.
**Rejected.**
Real deployments need different lifetimes and isolation for different entity types (a slowly-changing `Product` vs. a session-sensitive `User`),
and federation means the *same* type can be owned by different subgraphs with different freshness guarantees.
A global knob cannot express "cache `Product` for an hour in the shared store, don't cache `User` at all."
The per-(subgraph, type) model with a shared-by-name backend gives that flexibility while still letting operators collapse to a single cache by reusing one `CacheName`.

### C. Make caching opt-out (cache every `@key` entity by default)

Default every entity to cached and let configs disable specific types.
**Rejected.**
Opt-out is unsafe: it would silently cache entities whose freshness or correctness assumptions the operator never reviewed,
and it makes the "is this safe to cache?" decision implicit.
Opt-in (absence of config = L2 off) makes caching a deliberate, auditable choice per type,
keeps the unconfigured path identical to today's behavior,
and means a mistake fails *closed* (no caching) rather than *open* (stale data served).

## References

- Directive contract: [../directives/entity-cache-config.md](../directives/entity-cache-config.md)
- Foundation: [0001-foundation.md](0001-foundation.md)
- Directive inventory: [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)
- Architecture spec (seam, L1/L2 model, cache-key model): [../01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)
- Related config ADRs: [0007-root-field-cache-config.md](0007-root-field-cache-config.md), [0008-mutation-cache-config.md](0008-mutation-cache-config.md), [0009-subscription-cache-config.md](0009-subscription-cache-config.md)
- Prerequisite directive ADRs: [0002-key.md](0002-key.md), [0003-requires.md](0003-requires.md), [0004-provides.md](0004-provides.md)
