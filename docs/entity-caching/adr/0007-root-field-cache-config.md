# ADR-0007: Root-field caching config caching support

## Status

Proposed

## Context

The foundation ([0001-foundation.md](0001-foundation.md)) ships the caching *machinery* but no policy:
the loader seam (the five hook points plus the `cacheSkipFetch` / `cacheMustBeUpdated` booleans),
the `LoaderCache` (L2) and `CacheKeyTemplate` (key rendering) interfaces,
the per-fetch `FetchCacheConfiguration` data shape,
the per-request `CachingOptions` toggles,
and the StructuralCopy discipline that keeps cache entries and the live response tree from corrupting each other.

The entity caching config ([0006-entity-cache-config.md](0006-entity-cache-config.md)) added the first slice of policy:
it binds an *entity type* (resolved through an `_entities` fetch) to a named cache and a TTL,
turning on the foundation's dormant L2 path for that type.

What neither answers is the other half of the cacheable surface: the **root field**.
A federated request does not only resolve entities by `@key` through `_entities`.
It begins with a root-field fetch — `Query.user(id: "1")`, `Query.topProducts(first: 10)`, `Query.products(ids: ["1","2"])` — whose response a subgraph computes from the field name plus its arguments.
Root fields differ from entity fetches in two ways that matter for caching:

- A root field has **no prior entity data to key on**,
  so its cache identity must come from the field name and the per-request argument values, not from a `@key` already present in the data tree.
  This is why L1 (which deduplicates by entity identity within a request) does not apply to root fields, and L2 does ([01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §2).
- A root field's arguments very often *are* entity keys in disguise.
  `Query.user(id: "1")` returns exactly the `User` whose `@key` is `{id:"1"}`.
  If the root-field response and the later `_entities` fetch for that same `User` are cached under unrelated keys,
  the same data is stored twice and a root query never warms the entity cache (and vice versa).

This is the problem the **root-field caching config** solves.
Like entity caching config, it is **not a wire directive** — there is no SDL syntax.
It is a Go configuration concept, `RootFieldCacheConfiguration`,
supplied per subgraph by the router (typically synthesised from composition output plus operator config).
It does two distinct jobs:

1. Cache a **whole root-field response** under a root-field-shaped key
   (`{"__typename":"Query","field":"topProducts","args":{...}}`, with args sorted alphabetically for byte-identity).
2. Optionally, via `EntityKeyMapping`,
   rewrite that root-field key into **entity key shape**
   (`{"__typename":"User","key":{"id":"1"}}`)
   so a root query and an `_entities` fetch for the same entity share one L2 entry.

The `EntityKeyMapping` binding is expressed as a list of `FieldMapping{EntityKeyField, ArgumentPath, ArgumentIsEntityKey}`:
`EntityKeyField` names the `@key` field (dot-notation for nested keys),
`ArgumentPath` names the root-field argument that supplies it (multi-element for structured argument navigation),
and `ArgumentIsEntityKey` marks an argument whose **list elements** map 1:1 and positionally to the response entities.
When `ArgumentIsEntityKey` is set, the engine can build one entity key per list element,
short-circuit an empty (`[]` or `null`) list before touching the resolver or cache,
and — in partial-fetch mode — send only the cache-missed elements to the subgraph while serving the hits directly.

Per the directive inventory ([02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)),
this config is consumed by the planner caching state,
the resolver root-field cache path,
and the batch / partial-fetch optimisation.
It depends on `@key` (entity-mapped keys must match the `@key` field set byte-for-byte) and on entity caching config (so the two share the same L2 entries),
which is why it sits directly above [0006-entity-cache-config.md](0006-entity-cache-config.md) in the dependency chain.
The full field-by-field contract lives in the directive spec at [../directives/root-field-cache-config.md](../directives/root-field-cache-config.md);
this ADR records *why and how* it plugs into the foundation seam.

## Decision

Re-implement the root-field caching config as **gqtools PR 4 + PR 8 / PR-CACHE-CONFIG**,
stacked on the foundation PR and on entity caching config.
It adds **plan-time policy and a second key-template implementation**,
plus a small **batch / partial-fetch optimisation in the resolver's existing root-field cache path** —
and it changes the loader/resolvable *hot* path in **zero** new places.
Like entity caching config, it only populates `FetchCacheConfiguration` fields the foundation already defined and rides the hooks the foundation already invokes.

### Where the config lives and how it is supplied

The router supplies `RootFieldCacheConfiguration` per (subgraph, root field) through the same factory option entity config uses,
`WithSubgraphEntityCachingConfigs`,
which takes `SubgraphCachingConfigs` — a list of `SubgraphCachingConfig`, each carrying its subgraph name plus a `RootFieldCaching RootFieldCacheConfigurations` slice (alongside the entity / mutation / subscription siblings covered by their own ADRs).
The factory copies the `RootFieldCaching` slice onto that subgraph's federation metadata,
so by plan time each datasource carries its own root-field policy.
This reuses the config-supply pattern (`SubgraphCachingConfig` → `FederationMetaData`) established verbatim by entity caching config.

The minimal binding contract:

- One config per (subgraph, `TypeName` + `FieldName`) — e.g. `Query` + `topProducts`.
- `CacheName` is a logical handle resolved to a `LoaderCache` instance by the router via `ResolverOptions.Caches map[string]LoaderCache`; a root field and an entity type may share a backing store by reusing the same name (and *must*, for entity-key sharing to land in the same store).
- Absence of a config for a root field means **L2 disabled for that root field** — the opt-in model, fail-closed.

### The second key template: `RootQueryCacheKeyTemplate`

The foundation's `CacheKeyTemplate` interface has two implementations.
Entity caching config uses the entity template; root-field caching config uses **`RootQueryCacheKeyTemplate`**,
constructed from the root field set plus the `EntityKeyMappings`.
It produces one of two key shapes depending on configuration:

- **No mapping** — root-field shape `{"__typename":"Query","field":"topProducts","args":{...}}`, args sorted alphabetically.
  `EntityMergePath()` returns nil, signalling the template stores the **complete** root-field response payload.
- **With `EntityKeyMapping`** — entity shape `{"__typename":"User","key":{"id":"1"}}`,
  byte-identical to what the entity template produces for the same `User`,
  so the two cache entries coincide.
  `EntityMergePath()` returns the path at which entities live inside the root-field response, so cached entity values can be spliced back into the right place.

The template stays **alias-independent** by construction, exactly like the entity template:
keys are computed from field name + argument values (or the mapped `@key` fields), never from response aliases.
Alias awareness, where needed, lives in the separate `ProvidesData` field tree the foundation already carries.

A few rendering rules the re-implementation must honour, all confined to the template (not the loader):

- **Per-mapping independence**: an entity with multiple `@key` directives can carry multiple `EntityKeyMapping` entries.
  Each renders a key only when all its argument paths are present in the request variables; a mapping with missing arguments is skipped, the others still produce keys.
- **One key per list element** when `ArgumentIsEntityKey` is set on a list argument, with `BatchIndex` recording each key's position for positional response reassembly.
- **TypeName fallback**: when `__typename` is absent from the response, the plan-time `TypeName` on the template is the fallback for the entity type, never a hardcoded default.
- **Argument hash suffix** for arguments that are *not* entity keys, consistent with the foundation's argument-aware key rule (computed at resolve time from per-request variables, captured at plan time).

### How it plugs into the foundation seam (no hot-path changes)

All plan-time behavior lands in the **planner caching state** (`configureFetchCaching`),
mirroring entity caching config.
For a root-field fetch, the planner reads the root field's `TypeName` + `FieldName`,
looks up the `RootFieldCacheConfiguration`,
and — when one exists — attaches a `RootQueryCacheKeyTemplate` (built with the `EntityKeyMappings`) and fills the L2-bearing `FetchCacheConfiguration` fields:
`Enabled = true` (the one boolean the foundation's loader checks to run L2),
`CacheName`, `TTL`, `IncludeSubgraphHeaderPrefix`,
`ShadowMode`,
and the batch flag (`PartialBatchLoad`) plus the precomputed batch-argument metadata.
When no config exists, nothing is attached and the root field behaves exactly as today (no L2, and L1 does not apply to root fields anyway).

At resolve time the foundation's existing hooks do the read/write:
`prepareCacheKeys` renders keys from the template (root-field shape or, with mapping, entity shape);
`bulkL2Lookup` / `tryL2CacheLoad` consult L2 only when `Enabled`;
`mergeResult` honors `cacheSkipFetch` to splice cached values in (at `EntityMergePath` for entity-mapped templates, or the whole payload otherwise);
`updateL2Cache` writes back honoring per-entry `TTL`.

The **one genuinely new piece of resolver behavior** — and the reason this stacks as a small *additional* PR (PR 8) on top of the config PR (PR 4) — is the **batch / partial-fetch optimisation** for `ArgumentIsEntityKey` list arguments.
It is deliberately *not* a change to the four-phase machinery or to `mergeResult`'s shape.
It is logic inside the foundation's existing root-field cache path:

- **Empty-list short-circuit**: when the mapped list argument is `[]` or `null`, return an empty response immediately, before the resolver or any cache call.
- **Full-fetch mode** (`PartialBatchLoad = false`, default): any miss in the batch sends the full list to the subgraph; all returned entities are written back, all-or-nothing on the read side.
- **Partial-fetch mode** (`PartialBatchLoad = true`): filter the input list variable to only the cache-missed elements, send that reduced list to the subgraph, and merge the fresh entities with the cache hits in correct positional order using each key's `BatchIndex`.
- **Smart write-back**: on write, existing keys that hit on read are refreshed only when the data changed or a subgraph returned fresh data; *requested-but-missing* keys are backfilled only when the final entity value proves them (the mapped key field is present and renders to the exact same key string — request arguments alone never prove a write association); and *derived* keys for other `EntityKeyMapping` entries are written when the final entity contains those mapped fields (so a query by `id` can warm the `username` key for later cross-lookup).
  These distinctions are surfaced through the foundation's `CacheEntry.WriteReason` (refresh / backfill / derived).

This optimisation touches only the cache collaborator and the template, never the loader's dispatch or rendering, which is what keeps the seam intact.

### How the PR stacks

PR-CACHE-CONFIG (root-field portion) depends on, in order below it:

1. the **foundation** seam, `FetchCacheConfiguration`, and the `CacheKeyTemplate` interface ([0001-foundation.md](0001-foundation.md)),
2. **`@key`** for the entity identity that entity-mapped keys must reproduce byte-for-byte ([0002-key.md](0002-key.md)),
3. **`@provides`** / **`@requires`** for the projected `ProvidesData` shape used by the L2 read/write copies ([0004-provides.md](0004-provides.md), [0003-requires.md](0003-requires.md)),
4. **entity caching config** ([0006-entity-cache-config.md](0006-entity-cache-config.md)), because entity-key sharing only pays off when the same entity type is *also* cached under the same `CacheName`.

The work splits cleanly into two stacked diffs:
**PR 4** lands the plan-time policy plus the `RootQueryCacheKeyTemplate` (whole-response and entity-mapped scalar keys),
and **PR 8** layers on the batch / partial-fetch optimisation for `ArgumentIsEntityKey` lists.
Each is confined to the planner caching state, the template, and the cache collaborator — small, additive, and reviewable in isolation.
Behavioral coverage already exists for the target shape:
`v2/pkg/engine/resolve/aliased_root_field_caching_test.go` proves alias-independent root-field keys,
`v2/pkg/engine/resolve/batch_entity_cache_test.go` proves per-element batch keys and partial fetch,
and `execution/engine/federation_caching_root_*.go` exercise the config end to end (whole-response, entity-mapped, and split-batch behavior).

## Consequences

### Positive

- Caches the *entry point* of a federated query — the root field — which is otherwise un-cacheable by the entity-only path, because L1 cannot key a root field and only L2 applies.
- **Entity-key sharing** is the headline win: `Query.user(id:"1")` and an `_entities` fetch for `User{id:"1"}` collide on one L2 entry, so a root query warms the entity cache and an entity fetch warms the root query — no double storage, no cold-start for the other access path.
- Zero new loader/resolvable *hot-path* touch points; the change is a planner branch, a second key template, and optimisation logic in the existing root-field cache path.
- Opt-in per root field keeps the blast radius small: an unconfigured root field behaves exactly as today.
- Reuses the entire config-supply and `CacheName` → `LoaderCache` resolution pattern from entity caching config, so operators learn one model.

### Negative / costs

- `EntityKeyMapping` is the most error-prone config surface in the feature: the `EntityKeyField` / `ArgumentPath` bindings must match the `@key` field set *and* the actual argument names, and a wrong binding silently produces keys that never coincide with the entity cache (cache sharing quietly fails open to double storage rather than erroring).
- `ArgumentIsEntityKey` carries a hard, unchecked assumption: the response array is the **same length and order** as the input list argument (`ids[i]` ↔ `data.products[i]`).
  A subgraph that reorders, dedups, or drops elements breaks positional reassembly; the engine cannot detect this, so it is a documented contract the subgraph must satisfy.
- Two key shapes from one config (root-field vs entity-mapped) means "which key did this write?" is answered by configuration, not by the schema — harder to reason about than a single shape.
- A typo in `CacheName`, or a name not registered in `ResolverOptions.Caches`, silently disables L2 for the root field, and — worse — a *mismatched* name between the root field and the entity type silently defeats entity-key sharing even though both are "cached."

### Performance implications

- For configured root fields: one extra L2 round trip on a miss, amortised through the foundation's `bulkL2Lookup` (root-field and entity keys batch together when they share a cache instance).
- **Empty-list short-circuit** is a pure win — zero resolver and zero cache work for a trivially empty query.
- **Partial-fetch mode** cuts subgraph fan-out to only the missing batch members at the cost of serving cache hits that may be stale within their TTL window — a deliberate, configured trade, identical in spirit to entity partial cache load.
- Entity-key sharing roughly *halves* effective cache footprint for entities reachable both by root query and by `_entities`, and lifts hit rate because either access path warms the shared entry.
- For unconfigured root fields: zero added cost — `Enabled = false` short-circuits before any key transform or cache call; the path is identical to the cache-disabled foundation behavior.

### What becomes possible for later directives

- **Mutation caching** ([0008-mutation-cache-config.md](0008-mutation-cache-config.md)) can invalidate root-field cache entries (whole-response and entity-mapped) after a successful mutation, reusing the same template-driven key shapes and the `WriteReason` write semantics established here.
- **Subscription population** ([0009-subscription-cache-config.md](0009-subscription-cache-config.md)) inherits the entity-key sharing model: an event that carries entity fields can populate the *same* L2 entry a root query reads, keeping all access paths coherent.
- The batch / partial-fetch path and `BatchIndex` reassembly become a reusable mechanism for any future list-of-entities caching beyond root fields.

## Alternatives considered

### A. Cache root fields only as whole responses; never share with entity cache

Drop `EntityKeyMapping` entirely and store every root-field response under its own root-field key.
**Rejected.**
It is simpler, but it stores entity data twice (once under the root key, once under the entity key) and means a root query never warms the entity cache and an entity fetch never warms the root query — exactly the cross-path cold-start problem this config exists to solve.
The whole-response mode is still supported (it is what an unmapped config does), but making it the *only* mode would forgo the headline win.

### B. Auto-derive `EntityKeyMapping` from the schema instead of explicit config

Infer the argument-to-`@key` binding by matching argument names against `@key` field names at composition time, so operators never write `FieldMapping`.
**Rejected.**
The binding is genuinely ambiguous from the schema: an argument named `id` need not be the entity `@key` `id` (it could be an unrelated filter), list arguments may or may not be positional entity keys, and nested keys need explicit dot-notation / multi-element path navigation that the schema does not encode.
Guessing would produce keys that *look* shared but silently diverge, the worst failure mode for a cache.
Explicit `FieldMapping` makes the operator's intent auditable and keeps a wrong binding a config review item rather than a silent inference bug.

### C. Treat a list argument as one opaque cache key (no batch / per-element keys)

Cache `products(ids: ["1","2","3"])` under a single key derived from the whole list, rather than three per-element entity keys.
**Rejected.**
A single opaque list key cannot share entries with `_entities` fetches or with scalar root fields for the same entity, never benefits from partial reuse (`["1","2"]` and `["1","2","3"]` would be unrelated entries), and forfeits empty-list short-circuit and partial-fetch.
Per-element entity keys (`ArgumentIsEntityKey`) are strictly more capable: they share with entity caching, reuse across overlapping lists, and enable fetching only the missing members — at the modest cost of the positional-correspondence contract.

## References

- Directive contract: [../directives/root-field-cache-config.md](../directives/root-field-cache-config.md)
- Foundation: [0001-foundation.md](0001-foundation.md)
- Directive inventory: [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)
- Architecture spec (seam, L1/L2 model, cache-key model): [../01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)
- Prerequisite config ADR: [0006-entity-cache-config.md](0006-entity-cache-config.md)
- Prerequisite directive ADRs: [0002-key.md](0002-key.md), [0003-requires.md](0003-requires.md), [0004-provides.md](0004-provides.md)
- Downstream config ADRs: [0008-mutation-cache-config.md](0008-mutation-cache-config.md), [0009-subscription-cache-config.md](0009-subscription-cache-config.md)
- Behavioral coverage: `v2/pkg/engine/resolve/aliased_root_field_caching_test.go`, `v2/pkg/engine/resolve/batch_entity_cache_test.go`, `execution/engine/federation_caching_root_*.go`
