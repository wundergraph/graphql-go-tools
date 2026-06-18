# ADR-0004: @provides caching support

## Status

Proposed

## Context

The federation directive `@provides(fields: _FieldSet!)` applies to a `FIELD_DEFINITION` and declares that,
when a subgraph resolves that field,
it can return additional fields of the referenced entity inline,
without a separate `_entities` fetch.
The directive already exists in the schema and the planner reads it during query planning.
The caching layer does not redefine it.
It *consumes* it, and re-implementing caching cleanly means re-consuming it correctly.

For caching, `@provides` answers one question that the foundation deliberately left open:
**what is the exact field shape a fetch yields at a given location, and how does that shape relate to the cached entity shape?**

The foundation, recorded in [0001-foundation.md](0001-foundation.md), already established the machinery this directive needs but did not populate it:

- Each fetch carries a `ProvidesData *Object` field tree —
  an alias-aware description of the shape the query expects at the fetch location (foundation decision 6).
- The four StructuralCopy helpers (`structuralCopyNormalized` / `structuralCopyDenormalized` and their passthrough variants) already accept a `*Object` and drive their rename / project / passthrough behavior from it (foundation decision 5).
- The cache-key templates are alias-independent by construction;
  all alias awareness was explicitly deferred to `ProvidesData` (foundation decision 3).

So the foundation built the slot.
This directive fills it.
Without a populated `ProvidesData`,
the L2 projection has nothing to project to,
the field-widening check has no required-field list to validate against,
and cached values cannot be denormalized back into an aliased response tree.

Three concrete caching behaviors depend on a correctly populated `ProvidesData`:

1. **L2 projection.**
   An L2 entry must be minimal and self-contained so it round-trips cleanly across requests.
   The L2 write path (`structuralCopyNormalized`, `Passthrough = false`) keeps only the provided / listed fields and drops everything else.
   `ProvidesData` is the field list it projects to.

2. **Field-widening check.**
   A narrow query (`{ id name }`) must not poison the cache for a wider entity fetch (`{ id name email }`).
   Before serving a cached value, the resolver calls `validateItemHasRequiredData(cachedValue, ProvidesData)`
   to confirm the cached value contains *all* fields this fetch requires;
   if it does not, the hit is downgraded to a miss / partial hit and the fetch proceeds.
   `ProvidesData` is the required-field list.

3. **Alias round-tripping.**
   Cache entries are stored under schema field names (normalized);
   the active query may alias those fields.
   `ProvidesData` carries the response-side aliases (`Field.Name` vs `Field.OriginalName`, `Object.HasAliases`)
   so cached values can be denormalized back into the response tree under the names the current query expects.

`ProvidesData` is also the shape that distinguishes L1 from L2 behavior via the `Passthrough` switch,
and it is read by the L1-enable post-process pass (the union-of-providers coverage check),
but those are L1 and post-process concerns;
this ADR is scoped to the projection / widening / alias contract that `@provides` defines.

## Decision

Re-implement `@provides` caching support as the plan-time population of `ProvidesData`
and the resolver-side consumption of it for projection and widening,
**without touching the loader or resolvable hot path**.
This ships as **gqtools PR 5 / PR-CACHE-PROJECTION**, stacked on the foundation PR.

### What plugs in where

**Plan time (new work in this PR).**
A provides-fields visitor walks the planned response `Object` tree per fetch and produces the fetch's `ProvidesData *Object`.
The `*Object` it produces is the entity-shaped sub-tree the fetch yields,
with `Field.Name` set to the response key (alias when aliased),
`Field.OriginalName` set to the schema name,
and `Object.HasAliases` set when any field on the object is aliased.
A critical shape rule, carried over from the foundation contract:
for a nested entity fetch, `ProvidesData` must contain the *entity* fields (`id`, `username`),
not the parent field that wraps them (`author`).
The planner is responsible for navigating to the entity level before attaching the shape.

**Resolve time (no new hot-path code).**
The resolver already has the four StructuralCopy helpers and `validateItemHasRequiredData` from the foundation;
this PR is what makes them meaningful by giving them a populated `*Object` to operate on.
The consumption points are all *inside the existing cache collaborator*, not in the loader's resolution flow:

- L2 write projects through `structuralCopyNormalized(value, ProvidesData)` (rename + drop unlisted).
- L2 read denormalizes through `structuralCopyDenormalized(value, ProvidesData)` (restore aliases, projected).
- Cache hits are gated through `validateItemHasRequiredData(cachedValue, ProvidesData)` so a narrow value never satisfies a wider fetch.

When `ProvidesData` is nil or has no aliases,
every helper falls back to a plain `StructuralCopy` and `validateItemHasRequiredData` is a no-op accept —
so a fetch with no `@provides` shape degrades to today's behavior, never to corruption.

### Why this keeps the loader / resolvable untouched

The loader still calls the same five seams and honors the same two booleans (`cacheSkipFetch` / `cacheMustBeUpdated`) from the foundation.
This PR changes *what the collaborator computes when those seams fire*,
by threading a now-populated `ProvidesData` through the StructuralCopy helpers and the widening check.
No new seam, no new boolean, no change to `mergeResult`'s signature,
and zero change to the resolvable's two-pass walk.
The new control flow lives in the planner (the provides-fields visitor) and in the collaborator helpers,
which is exactly the property the foundation was designed to preserve.

### New metadata and annotations

- **Plan-time annotation:** `FetchInfo.ProvidesData *Object`, populated by the provides-fields visitor (the field already exists from the foundation; this PR fills it).
- **No new fetch-config fields.**
  Projection and widening are driven entirely off the existing `ProvidesData` and the existing helpers.
- **No new resolver hooks.**
  The consumption sites (`structuralCopyNormalized` / `structuralCopyDenormalized` / `validateItemHasRequiredData`) are foundation surface;
  this PR only supplies their input.

### How its PR stacks

PR-CACHE-PROJECTION stacks directly on the foundation PR and on PR-CACHE-KEYS (`@key`).
`@key` must land first because cache-key identity and the L1 passthrough that retains `@key` fields are prerequisites;
`@provides` then defines the projected shape *around* that stable identity.
Per the dependency ordering in [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md),
`@provides` lands before `@requires`,
because `@requires` is most cleanly specified as an *exclusion from* the `ProvidesData` shape that this PR introduces.
`@requestScoped` also consumes this PR's `ProvidesData` for its own widening check,
so it stacks after `@provides` even though it is otherwise independent of the L2 / mutation / subscription branch.

## Consequences

### Positive

- L2 entries are minimal and self-contained,
  because projection drops everything not in the provided shape,
  which keeps external storage small and makes entries safe to round-trip across requests.
- The field-widening check makes cache hits *correct under query variation*:
  a narrow root query can no longer serve stale-shaped data to a wider entity fetch.
- Alias handling becomes uniform:
  the same `ProvidesData`-driven normalize / denormalize pipeline serves L1, L2, and `@requestScoped`,
  so there is one alias contract, not three.
- The loader and resolvable remain untouched,
  so this PR is a small, additive diff against the planner plus the already-tested collaborator helpers.

### Negative / costs

- `ProvidesData` is now load-bearing for correctness, not just optimization.
  A planner bug that attaches the wrong shape (for example the parent field instead of the entity fields) silently corrupts projection and widening,
  so the shape rule (entity-level, not wrapper-level) must be tested directly.
- The shape is alias-aware and per-fetch,
  which adds planner complexity in the provides-fields visitor (response-key vs schema-name resolution, nested navigation).
- Building the ephemeral normalize / denormalize Transforms from `ProvidesData` costs a tree walk per cache operation,
  though it is amortized on the reusable transform slabs and is far cheaper than a byte round-trip.

### Performance implications

- L2 projection trims payload bytes before they cross the heap boundary (`MarshalTo`),
  reducing external store size and serialization cost.
- The widening check is a single structural walk of the cached value against `ProvidesData`;
  it runs only on candidate hits and short-circuits on the first missing field.
- When `ProvidesData` has no aliases the helpers fall back to plain `StructuralCopy` with no Transform build,
  so the alias machinery costs nothing for queries that do not alias.

### What this makes possible for later directives

- `@requires` (PR-CACHE-PROJECTION, same PR family) can be expressed purely as *exclusion from* this shape:
  request-derived `@requires` fields are simply never added to `ProvidesData`,
  so they never enter the cached projection.
  See [0003-requires.md](0003-requires.md).
- `@requestScoped` reuses this PR's `ProvidesData` and `validateItemHasRequiredData` for its coordinate-L1 widening guard,
  inheriting the same correctness property for free.
  See [0005-request-scoped.md](0005-request-scoped.md).
- Root-field L1 promotion derives entity-shaped sub-Objects from `ProvidesData`,
  so the whole-response caching and entity-key-sharing work depends on this shape being present.

## Alternatives Considered

### A. Project / widen off the cache-key template instead of a separate ProvidesData shape

Fold the field shape into the `CacheKeyTemplate` so one object both computes the key and describes the projection.

**Rejected.**
The foundation deliberately made templates alias-independent so the same entity produces the same key regardless of aliasing (foundation decision 3).
Mixing the alias-aware projection shape into the key template would either re-introduce alias dependence into keys (breaking cross-request key stability) or bloat the template interface with concerns it should not own.
Keeping `ProvidesData` as a separate, alias-aware `*Object` preserves the clean split: templates do identity, `ProvidesData` does shape.

### B. Cache the full subgraph response and skip projection entirely

Store whatever the subgraph returned, unprojected, and rely on the response-shape merge to pick out fields on read.

**Rejected.**
Unprojected entries leak `@requires` fields and request-derived data into the cache,
which is exactly the staleness hazard `@requires` exclusion exists to prevent (see [0003-requires.md](0003-requires.md)).
They also bloat external storage and make entries non-self-contained,
so two requests selecting different field subsets could not safely share an entry.
Projection to the provided shape is what makes an L2 entry a clean, reusable unit of entity data.

### C. Skip the field-widening check and trust that any cache hit is complete

Treat any present cache entry as a full hit without verifying it contains the fields the current fetch needs.

**Rejected.**
A narrow earlier query (`{ id name }`) would populate an entry that a later wider fetch (`{ id name email }`) would then read as a hit,
serving `email: undefined` and silently corrupting the response.
The widening check via `validateItemHasRequiredData(cachedValue, ProvidesData)` is the only thing that keeps cache hits correct under query-shape variation,
and `ProvidesData` is precisely the required-field list it needs.
Dropping it trades a single short-circuiting structural walk for silent data corruption.

## References

- Directive contract: [../directives/provides.md](../directives/provides.md)
- Foundation decision record: [0001-foundation.md](0001-foundation.md)
- Directive inventory and dependency ordering: [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)
- Related directives: [0002-key.md](0002-key.md), [0003-requires.md](0003-requires.md), [0005-request-scoped.md](0005-request-scoped.md)
- Architecture spec (L1/L2 projection, the Passthrough switch): [../01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)
