# ADR-0003: @requires caching support

## Status

Proposed

## Context

`@requires(fields: _FieldSet!)` is a federation directive on `FIELD_DEFINITION`.
It declares external fields that a resolver needs as *input* before it can resolve its own field,
where those required inputs are owned by another subgraph and supplied per request.
A classic example: a `shippingEstimate` field that `@requires(fields: "weight size")`,
where `weight` and `size` come from a different subgraph and are passed into the resolving subgraph as part of the `_entities` representation.

For entity caching, `@requires` matters for one reason, and that reason is exclusionary.
The required fields are not part of the entity's stable identity, and they are not data the resolving subgraph *owns*.
They are inputs that vary per request, derived from whatever the upstream subgraph happened to return for *this* query.
If `@requires` data leaked into a cached entity shape, two failure modes follow.
First, a later request whose required inputs differ would read back stale, mismatched values from cache.
Second, the cache would fragment on data that has nothing to do with entity identity, so two requests for the same `User:1234` that supply different `@requires` inputs would store and read divergent entries for the same logical entity.

The architecture foundation (ADR-0001) already provides everything this directive needs as a *substrate*.
The foundation establishes:
- the two-level cache model and the per-fetch `FetchCacheConfiguration`,
- the `ProvidesData *Object` field tree that travels on every fetch and is the single source of truth for *what shape gets cached* (see ADR-0001 decision 6),
- the four StructuralCopy helpers in `loader_cache_transform.go` that perform the alias-rename and field-projection during every cache read and write,
  where L2 projection (`structuralCopyNormalized`, `Transform.Passthrough = false`) keeps only the fields listed in `ProvidesData` and drops everything else.

`@provides` (ADR-0004) defines and populates the `ProvidesData` shape; `@requires` is then expressed as the absence of those fields *from* that shape.
This is why the directive-inventory dependency ordering places `@provides` before `@requires`: once the cached shape is defined positively by `ProvidesData`, the `@requires` rule is just "do not add required fields to it."

The problem this ADR records is narrow and contract-shaped:
**how does the caching layer guarantee that `@requires` fields never enter the cached entity shape, without changing the loader hot path or the cache-key/projection primitives the foundation already froze?**

Detailed contract: [../directives/requires.md](../directives/requires.md).
Foundation: [0001-foundation.md](0001-foundation.md).
Directive taxonomy and PR mapping: [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).

## Decision

`@requires` plugs into the foundation seam as a **plan-time projection rule only**.
It adds no resolver hooks, no loader call sites, and no new cache interface.
It is, deliberately, the smallest possible directive PR: it constrains what the planner writes into `ProvidesData` and into the cache-key field set, and the already-frozen resolver primitives do the rest for free.

This directive's PR is **gqtools PR 5 / PR-CACHE-PROJECTION**, stacked on the foundation PR and co-resident with `@provides` (the two share the `ProvidesData` projection machinery, per [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)).

### What plugs in, and where

**1. Cache-key field set excludes `@requires`.**
The entity cache-key template (`EntityQueryCacheKeyTemplate`) is built from `@key` fields only.
The planner must never feed `@requires` field configurations into the key-field list.
The foundation already documents the key shape as `{"__typename":"User","key":{"id":"123"}}` built "from `@key` fields only, never `@requires`" (ADR-0001 decision 3).
This ADR pins that as a hard requirement of the `@requires` contract rather than an incidental property:
the key must be alias-independent *and* `@requires`-independent so entity identity is stable no matter what inputs a query supplies.

**2. `ProvidesData` omits `@requires` fields.**
The planner builds the per-fetch `ProvidesData *Object` (driven by `@provides` / the selection shape) so that it lists the fields the subgraph genuinely *provides* for the entity.
Fields the planner has marked with the requires dependency reason (the visitor's `IsRequires` flag on dependency origins) must not be emitted as `ProvidesData` entries on the cached shape.
Because the L2 write path projects strictly to `ProvidesData` (`structuralCopyNormalized`, `Passthrough = false`), excluding a field from `ProvidesData` is *sufficient* to keep it out of the L2 entry — no resolver change is required.

**3. The L1 passthrough nuance is handled by exclusion, not special-casing.**
L1 writes use passthrough (`structuralCopyNormalizedPassthrough`, `Passthrough = true`), which keeps source fields beyond `ProvidesData` (notably `@key` fields needed for later merges, per ADR-0001 decision 5).
This means L1 passthrough *would* retain a `@requires` field if that field were present in the merged response value at write time.
The decision here is that `@requires` correctness is enforced at the **L2 boundary** (the cross-request store, where staleness is the real risk) via `ProvidesData` projection, and that the within-request L1 tier is not a staleness vector because L1 lives and dies inside one request — the required inputs are constant for that single request by construction.
The clean rule: `@requires` exclusion is a property of the *cached-shape definition* (`ProvidesData` for L2, `@key`-only for keys), and the resolver's existing copy primitives enforce it without a `@requires`-aware branch anywhere in the loader.

### What it does NOT add

- **No loader/resolvable change.**
  The five foundation seams (pre-fetch lookup, post-fetch write, merge point, L1 read/write, invalidation) are untouched.
  `@requires` never appears as a runtime condition; it is fully resolved into `ProvidesData` and key-field shape at plan time.
- **No new interface.**
  `LoaderCache`, `CacheKeyTemplate`, and the analytics sink are unchanged.
- **No new per-fetch state.**
  `cacheSkipFetch` / `cacheMustBeUpdated` semantics are unchanged.
  `@requires` adds no boolean and no field to the per-fetch result.

### Plan-time metadata it relies on (existing, not new)

The planner already distinguishes dependency reasons via `fieldDependencyKindRequires` / the `IsRequires` flag on dependency origins (`visitor.go`), and exposes `@requires` configurations through `FederationMetaData.Requires` and `RequiredFieldsByRequires(typeName, fieldName)`.
The `@requires` caching work *consumes* this existing metadata to decide field membership of `ProvidesData` and the key-field set.
It introduces no new federation-metadata type; it only adds the projection rule that reads the existing requires metadata when the planner populates `FetchCacheConfiguration` / `FetchInfo.ProvidesData`.

### How the PR stacks

```text
astjson release (PR #0)
   └─ Foundation (ADR-0001): seam + interfaces + ProvidesData + StructuralCopy helpers
        └─ @key (PR-CACHE-KEYS): key-field identity, L1 passthrough keeps @key
             └─ @provides + @requires (gqtools PR 5 / PR-CACHE-PROJECTION):
                  @provides DEFINES ProvidesData; @requires EXCLUDES required fields from it
```

`@requires` cannot be reviewed before `@provides`, because the exclusion is meaningless until the positive `ProvidesData` shape exists to exclude *from*.
Both land in the same projection PR.

## Consequences

### Positive

- **Zero hot-path cost.**
  Exclusion happens once at plan time; the resolver never branches on `@requires`.
  The cache-disabled path and the cache-enabled path both run the same loader code.
- **Correctness by construction.**
  Because cached shape is *defined* by `ProvidesData` (L2) and `@key` (keys), keeping `@requires` out of those two structures is the entire fix — there is no second place a required field could sneak into a cross-request entry.
- **Stable entity identity.**
  Keys built from `@key` only mean two requests for the same entity collide on one cache entry regardless of which `@requires` inputs each supplied, which is exactly the dedup behavior L1 and L2 exist to provide.
- **Tiny, reviewable diff.**
  The PR touches the planner's `ProvidesData` / key-field population and adds tests; it does not touch the loader, the resolvable, or any interface.

### Negative / costs

- **The guarantee is implicit, carried by `ProvidesData`.**
  There is no runtime assertion that a `@requires` field is absent from a cached value; correctness depends on the planner never listing required fields in `ProvidesData` and never feeding them to the key template.
  This must be locked down by tests (a `@requires` field present in the merged response must not appear in the L2 entry, and the key must be byte-identical across two requests with differing required inputs).
- **L1 passthrough retains a `@requires` field if present at write time.**
  This is intentional and safe within one request, but it is a subtlety a re-implementer must understand:
  `@requires` exclusion is enforced at the L2 boundary, not by stripping the field from every in-memory value.
  A future change that promoted L1 entries across requests (which the design forbids) would break this assumption.
- **Coupling to the dependency-reason flag.**
  The exclusion reads the planner's existing `IsRequires` / requires-config metadata; if that metadata were ever miscomputed, the cache would silently inherit the error.
  The fix lives where the metadata is produced, not in the cache layer.

### Performance implications

- No additional copies, no additional allocations, no additional cache lookups.
- L2 entries are *smaller* than they would be if required fields were stored, because projection drops them, which marginally improves serialized payload size and backend bandwidth.

### What becomes possible for later directives

- Once `@requires` exclusion is established as "a property of `ProvidesData`," every later config concept (entity-cache config, root-field config, mutation, subscription) inherits correct projection for free — none of them needs to re-reason about required fields.
- The field-widening check (`validateItemHasRequiredData`, introduced by `@provides`) operates against `ProvidesData`, so it automatically does *not* demand `@requires` fields be present in a cached value — a cached entity that legitimately lacks request-derived inputs still satisfies the widening check.

## Alternatives considered

### A. Strip `@requires` fields at the L2 write boundary with a dedicated `@requires`-aware copy step

Add a runtime pass in `updateL2Cache` that walks the value and removes any field marked as `@requires` before serialization.
**Rejected.**
It duplicates work the `ProvidesData` projection already does (the L2 write path *already* drops every field not listed in `ProvidesData`), adds a per-write tree walk on the hot path, and introduces a second source of truth for "what is cached."
Defining the cached shape positively via `ProvidesData` and simply not listing required fields is strictly simpler and free.

### B. Include `@requires` fields in the cache key so entries with different required inputs never collide

Widen the entity key to incorporate the `@requires` field values, making each (entity, required-inputs) combination its own entry.
**Rejected.**
This destroys the entire point of entity caching: the same `User:1234` would shard into many entries keyed by incidental request inputs, hit rates would collapse, and L1 within-request dedup would stop working because two fetch paths for the same entity could carry different inputs.
Entity identity must be `@key`-only (ADR-0001 decision 3); `@requires` belongs nowhere near the key.

### C. Add a per-fetch `HasRequires` boolean and branch in the loader

Surface a flag on `FetchCacheConfiguration` and let the loader skip or alter caching when a fetch involves `@requires`.
**Rejected.**
It violates the foundation's central constraint of keeping the loader untouched (ADR-0001, "what stays untouched"), pushes a plan-time concern into the runtime hot path, and is unnecessary: nothing about caching needs to *behave* differently for a `@requires` fetch — the field just must not be in the cached shape, which is a plan-time projection decision, not a runtime branch.

## References

- [../directives/requires.md](../directives/requires.md) — full `@requires` caching contract
- [0001-foundation.md](0001-foundation.md) — foundation seam, `ProvidesData`, StructuralCopy strategy
- [0004-provides.md](0004-provides.md) — defines and populates the `ProvidesData` shape this directive excludes from
- [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) — directive taxonomy and PR mapping
- [../01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3.4 (L1/L2 projection switch), §4 (cache-key model)
- `docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md` AC-L1-03 (cache keys use only `@key` fields; `@requires` never included)
- `v2/pkg/engine/resolve/loader_cache_transform.go` (`structuralCopyNormalized` L2 projection), `v2/pkg/engine/plan/visitor.go` (`IsRequires` dependency reason), `v2/pkg/engine/plan/federation_metadata.go` (`Requires`, `RequiredFieldsByRequires`)
