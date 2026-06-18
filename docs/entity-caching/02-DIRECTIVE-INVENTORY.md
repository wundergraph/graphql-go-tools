# 02 — Directive & Caching-Concept Inventory

Reference catalogue of every GraphQL directive and caching configuration concept that the entity-caching feature depends on, reads, or introduces.
Read this document to learn *what each directive means*, *who consumes it*, and *in which re-implementation PR it is rebuilt*.
No prior knowledge of the feature is assumed.

Sibling documents:
- Overview and navigation: [00-OVERVIEW.md](00-OVERVIEW.md)
- Architecture and the integration seam: [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md)
- Foundation decision record: [adr/0001-foundation.md](adr/0001-foundation.md)
- graphql-go-tools PR plan: [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md)
- Router PR plan: [04-PR-PLAN-router.md](04-PR-PLAN-router.md)
- astjson primitives: [05-ASTJSON-PRIMITIVES.md](05-ASTJSON-PRIMITIVES.md)
- Test and bench plan: [06-TEST-AND-BENCH-PLAN.md](06-TEST-AND-BENCH-PLAN.md)

---

## How to read this inventory

The entity-caching feature interacts with two families of concept.

1. **Federation directives.**
These already exist in the schema and the planner.
The feature does not redefine them, but it *reads* them to know how to build cache keys, project cached shapes, and validate field widening.
They are listed here because re-implementing the caching layer means re-consuming them correctly.

2. **Caching configuration concepts.**
These are *not* schema directives in the wire sense.
They arrive as Go configuration structs (for example `EntityCacheConfiguration`),
typically synthesised by the router from composition output and per-subgraph config.
The one true *new directive* in this family is `@requestScoped`, which is declared in subgraph SDL and validated at composition time.

Each row links to a per-directive spec under `directives/<name>.md` and a decision record under `adr/00NN-<name>.md`.
The specs describe the contract in detail.
The ADRs record why the contract is shaped the way it is.

---

## 1. Summary table

| Name | Applies To | Responsibility | Composition Rules (short) | Consumed By | Re-impl PR |
|---|---|---|---|---|---|
| [`@key`](directives/key.md) · [ADR](adr/0002-key.md) | `OBJECT`, `INTERFACE` | Declares the field set that uniquely identifies an entity. Source of the cache-key identity for entity L1/L2. | Repeatable. `fields` is a mandatory `_FieldSet`. Must be resolvable to participate in entity fetches. | Planner key-fields visitor; cache-key builder; entity L1 passthrough (keeps `@key` fields even when not in `ProvidesData`). | PR-CACHE-KEYS |
| [`@requires`](directives/requires.md) · [ADR](adr/0003-requires.md) | `FIELD_DEFINITION` | Declares external fields a resolver needs as input. Excluded from cache projection because it is request-derived, not entity-owned. | `fields` is a mandatory `_FieldSet` referencing `@external` fields on the same type. | Planner required-fields visitor; L1 population logic that explicitly *excludes* `@requires` from the cached shape. | PR-CACHE-PROJECTION |
| [`@provides`](directives/provides.md) · [ADR](adr/0004-provides.md) | `FIELD_DEFINITION` | Declares extra entity fields a subgraph can return inline. Defines the `ProvidesData` shape used for cache projection and field-widening checks. | `fields` is a mandatory `_FieldSet` over the returned entity type. | Planner provides-fields visitor; `ProvidesData *Object` on fetches; L2 projection (`structuralCopyNormalized`); widening validation. | PR-CACHE-PROJECTION |
| [`@requestScoped`](directives/request-scoped.md) · [ADR](adr/0005-request-scoped.md) | `FIELD_DEFINITION` | NEW. Marks fields whose value is identical for the whole request inside one subgraph. Enables a per-request coordinate L1 that lets the first resolved field populate and later fields skip their fetch. | `key: String!` is mandatory. Composition warns when a key appears on only one field in a subgraph (a lone reader is meaningless). | Datasource `ConfigureFetch` (emits one `RequestScopedField` per annotated field); planner `configureFetchCaching`; resolver `requestScopedL1`. | PR-REQUEST-SCOPED |
| [Entity caching config](directives/entity-cache-config.md) · [ADR](adr/0006-entity-cache-config.md) | Entity type (`TypeName`) | Configuration concept, not a wire directive. Binds an entity type to a named cache, a TTL, optional negative-cache TTL, and a serve-stale window. | No schema syntax. Supplied via `SubgraphCachingConfig.EntityCaching`; one config per (subgraph, type). | Planner caching state; resolver entity L1/L2 read/write paths; cache-key construction. | PR-CACHE-CONFIG |
| [Root-field caching config](directives/root-field-cache-config.md) · [ADR](adr/0007-root-field-cache-config.md) | Root field (`Query.field`) | Configuration concept. Caches a whole root-field response and optionally maps it onto entity cache keys via `EntityKeyMapping` so root queries and entity fetches share L2 entries. | No schema syntax. Supplied via `SubgraphCachingConfig.RootFieldCaching`; `EntityKeyMapping` lists the `@key` field ↔ argument-path bindings. | Planner caching state; resolver root-field cache path; batch and partial-fetch optimisation. | PR-CACHE-CONFIG |
| [Mutation cache config](directives/mutation-cache-config.md) · [ADR](adr/0008-mutation-cache-config.md) | Mutation root field | Configuration concept. Per-mutation control: opt-in L2 population for entity fetches triggered by the mutation, plus entity/root-field invalidation on success. | No schema syntax. Supplied via `SubgraphCachingConfig.MutationFieldCaching` and `MutationCacheInvalidation`. Mutations always skip L2 *reads*. | Resolver mutation path; L2 write gate; post-mutation invalidation. | PR-CACHE-INVALIDATION |
| [Subscription population config](directives/subscription-cache-config.md) · [ADR](adr/0009-subscription-cache-config.md) | Subscription root field | Configuration concept. On each event, either populates L2 (event carries fields beyond `@key`) or invalidates the L2 entry (event carries only `@key`). | No schema syntax. Requires BOTH `TypeName` AND `FieldName` set, or the lookup silently no-ops. | Resolver subscription path; L2 populate/invalidate on event. | PR-CACHE-SUBSCRIPTION |

Re-impl PR identifiers above are logical names.
Their concrete stack ordering lives in [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md) and [04-PR-PLAN-router.md](04-PR-PLAN-router.md).

---

## 2. Per-directive notes

### `@key` — entity identity
`@key(fields: _FieldSet!)` is the federation primitive that names which fields uniquely identify an entity, for example `@key(fields: "id")` on `type User`.
For caching it is load-bearing twice over.
First, the key field set is the seed for every entity cache key, so two requests asking for the same `User` by the same `id` collide on the same cache entry.
Second, the L1 passthrough projection must *retain* `@key` fields even when the current query did not ask for them, because a later, wider entity fetch needs the key present to merge correctly.
This is why entity L1 uses passthrough rather than a strict projection.
The directive is consumed, never redefined, by the caching layer.
Full contract: [directives/key.md](directives/key.md).
Rationale: [adr/0002-key.md](adr/0002-key.md).

### `@requires` — request-derived inputs, excluded from cache
`@requires(fields: _FieldSet!)` declares fields a resolver needs as *input* before it can resolve its own field, where those inputs come from another subgraph.
The critical caching rule is exclusionary.
`@requires` data is supplied per request and is not owned by the entity,
so it must never be written into the cached entity shape.
If it leaked into the cache, a later request with different required inputs would read stale, mismatched data.
The re-implementation must reproduce this exclusion when it builds the L1/L2 projected shape.
Full contract: [directives/requires.md](directives/requires.md).
Rationale: [adr/0003-requires.md](adr/0003-requires.md).

### `@provides` — the projected cache shape
`@provides(fields: _FieldSet!)` lets a subgraph declare that it can return additional entity fields inline on a given field.
For caching, `@provides` is the source of the `ProvidesData *Object` that travels on a fetch.
`ProvidesData` is the alias-aware description of the exact shape the query expects at a fetch location, and it drives three things.
It scopes the L2 projection, which keeps only the provided/listed fields via `structuralCopyNormalized`.
It defines the field-widening check, so a narrow query cannot poison the cache for a wider one.
And it carries the response-side aliases needed to denormalise cached values back into the active response tree.
Full contract: [directives/provides.md](directives/provides.md).
Rationale: [adr/0004-provides.md](adr/0004-provides.md).

### `@requestScoped` — the one new directive
`directive @requestScoped(key: String!) on FIELD_DEFINITION` is the only directive this feature *introduces*.
Its model is purely symmetric.
Every field annotated with `@requestScoped(key: "X")` in the same subgraph shares one per-request L1 entry keyed `{subgraphName}.X`.
There is no provider/receiver distinction: whichever annotated field resolves first writes the value, and every later field with the same key reads it and may skip its own fetch.
A tiny on-the-wire surface — one struct, `RequestScopedField{FieldName, FieldPath, ProvidesData}` — carries it from planner to resolver.
The resolver keeps a main-thread-only `requestScopedL1 map[string]*astjson.Value`, gated behind the per-request `EnableL1Cache` flag, and injects only after a full field-widening check via `validateItemHasRequiredData`.
Composition makes `key` mandatory and warns when a key is declared on just one field, because a lone reader can never coordinate with a second.
Full contract: [directives/request-scoped.md](directives/request-scoped.md).
Rationale: [adr/0005-request-scoped.md](adr/0005-request-scoped.md).

### Entity caching config — bind a type to a cache and a TTL
Not a wire directive, this is the `EntityCacheConfiguration` struct: `{TypeName, CacheName, TTL, NegativeCacheTTL, ...}`.
It tells the resolver which named cache backs a given entity type, how long entries live, how long a null result (entity not found) stays negatively cached, and whether stale data may be served within bounds.
Multiple entity types can share a backing cache by reusing the same `CacheName`.
It is supplied per subgraph through `SubgraphCachingConfig.EntityCaching`.
Full contract: [directives/entity-cache-config.md](directives/entity-cache-config.md).
Rationale: [adr/0006-entity-cache-config.md](adr/0006-entity-cache-config.md).

### Root-field caching config — cache whole root responses, share with entities
`RootFieldCacheConfiguration` caches the response of a root field such as `Query.user`.
Its distinctive piece is `EntityKeyMapping`, which maps the entity's `@key` fields onto the root field's arguments (`FieldMapping{EntityKeyField, ArgumentPath, ArgumentIsEntityKey}`).
With that mapping, a root query `user(id: "1")` and an entity fetch for `User{id:"1"}` resolve to the *same* L2 cache key, so they share data.
When the key argument is a list and marked `ArgumentIsEntityKey`, the engine can build one cache key per element, short-circuit empty lists, and fetch only the missing entities in partial-fetch mode.
Full contract: [directives/root-field-cache-config.md](directives/root-field-cache-config.md).
Rationale: [adr/0007-root-field-cache-config.md](adr/0007-root-field-cache-config.md).

### Mutation cache config — write-on-mutation and invalidation
Two related structs cover mutations.
`MutationFieldCacheConfiguration{FieldName, EnableEntityL2CachePopulation, TTL}` decides whether entity fetches *triggered by* a mutation are allowed to write to L2; by default they are not, and mutations always skip L2 reads regardless.
`MutationCacheInvalidationConfiguration` describes what to evict from L2 after a mutation succeeds, so downstream queries see fresh data.
The TTL on the mutation config overrides the entity default for those mutation-triggered writes.
Full contract: [directives/mutation-cache-config.md](directives/mutation-cache-config.md).
Rationale: [adr/0008-mutation-cache-config.md](adr/0008-mutation-cache-config.md).

### Subscription population config — populate or invalidate per event
`SubscriptionEntityPopulationConfiguration` runs on every subscription event.
If the event carries entity fields beyond `@key`, it writes them to L2 so later queries hit cache.
If the event carries only `@key` fields and `EnableInvalidationOnKeyOnly` is set, it deletes the L2 entry instead, evicting stale data.
The mandatory-pair invariant matters: the lookup `FindByTypeAndFieldName` matches on BOTH `TypeName` and `FieldName`, so if `FieldName` is empty the config silently never fires.
The router integration must set `FieldName` on both populate and invalidate paths.
Full contract: [directives/subscription-cache-config.md](directives/subscription-cache-config.md).
Rationale: [adr/0009-subscription-cache-config.md](adr/0009-subscription-cache-config.md).

---

## 3. Dependency ordering

The specs build on each other.
The chain below is the order in which the directive contracts can be re-implemented and reviewed, and it mirrors the stacked-PR ordering.
Each level assumes everything above it is already correct.

```text
adr/0001-foundation.md            (astjson StructuralCopy + Transform; arena lifetime; L1/L2 layering)
        │
        ├─ @key            ──────────────► cache-key identity + L1 passthrough (keeps key fields)
        │       │
        │       ▼
        ├─ @provides       ──────────────► ProvidesData shape + projection + widening check
        │       │
        │       ▼
        ├─ @requires       ──────────────► exclusion rule (request-derived, never cached)
        │
        ▼
Entity cache config         ──────────────► bind type → cache + TTL (needs @key for keys)
        │
        ▼
Root-field cache config     ──────────────► whole-response cache + EntityKeyMapping
                                            (needs @key + entity cache config to share entries)
        │
        ├─► Mutation cache config          (write-on-mutation + invalidation; needs entity + root config)
        │
        └─► Subscription population config  (per-event populate/invalidate; needs entity cache config)

@requestScoped              ──────────────► coordinate L1
                                            (needs @provides ProvidesData + foundation StructuralCopy;
                                             independent of L2 / mutation / subscription specs)
```

Reading guide for the ordering:

- **Foundation first.**
Everything depends on the StructuralCopy / Transform primitives and the arena lifetime rules in [adr/0001-foundation.md](adr/0001-foundation.md).
Do not start any directive spec until the foundation contract is settled.

- **`@key` before everything caching-related.**
Cache keys are derived from key fields, and L1 passthrough must preserve them, so `@key` is the first directive every other spec leans on.

- **`@provides` before `@requires`.**
`@provides` defines the `ProvidesData` shape; `@requires` is then expressed as the exclusion *from* that shape, so it is easier to specify second.

- **Config concepts after the projection directives.**
Entity, root-field, mutation, and subscription configs all assume the projected shape and cache-key rules are nailed down.
Entity config comes before root-field config (root-field reuses entity cache keys), and both come before mutation/subscription (which read and invalidate what the other two populate).

- **`@requestScoped` is a side-branch.**
It needs the foundation and the `@provides` `ProvidesData` machinery for its widening check, but it does not depend on L2, mutation, or subscription specs.
It can be implemented in parallel with the config-concept branch once `@provides` lands.
