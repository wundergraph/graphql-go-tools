# Directive Specification: `@requires`

> Part of the entity-caching re-implementation document set.
> Re-implementation PR: **gqtools PR 5 / PR-CACHE-PROJECTION**.
> Cross-links: [adr/0003-requires.md](../adr/0003-requires.md), [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md), [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).
> Sibling directive specs: [provides.md](provides.md) (defines the shape `@requires` is excluded from), [key.md](key.md) (entity identity).

This spec assumes no prior knowledge of the feature.
It describes the contract for `@requires` *as the caching layer consumes it* — the caching layer never redefines the directive, it only reads it and reacts to it.
The single most important fact in this document:
caching's relationship to `@requires` is **exclusionary**.
The directive does not turn caching on or add fields to a cached entity.
It marks fields that must be *kept out* of every cached shape.

---

## 1. Purpose & responsibility

`@requires(fields: _FieldSet!)` is a federation directive that declares the external fields a subgraph resolver needs as **input** before it can resolve its own field.
Those input fields are owned by, and resolved from, a *different* subgraph, then fed back into the requiring subgraph's fetch as variables.
Because the required values are supplied per request and are not owned by the entity the resolver belongs to, they are **request-derived, not entity-owned**.
The caching layer's sole responsibility toward `@requires` is therefore negative:
when it builds the projected shape that gets written into L1 or L2, it must **exclude** any field that arrived only because it was required as an input.
A `@requires` value must never be persisted as part of the cached entity, because a later request that supplies different required inputs would otherwise read stale, mismatched data attributed to the wrong inputs.

---

## 2. SDL / configuration definition

`@requires` is a wire directive that lives in subgraph SDL.
The caching layer does not introduce it; it is consumed exactly as composition emits it.

```graphql
directive @requires(fields: _FieldSet!) on FIELD_DEFINITION
```

Concrete fixture from the `reviews` subgraph (`execution/federationtesting/reviews/graph/schema.graphqls`):

```graphql
type User @key(fields: "id") {
    id: ID!
    username: String! @external
    coReviewers: [User!]! @requires(fields: "username")
    sameUserReviewers: [User!]! @requires(fields: "username")
}
```

Here `username` is `@external` (owned by `accounts`).
`coReviewers` and `sameUserReviewers` are resolved by `reviews`, but only after the gateway resolves `username` from `accounts` and feeds it back in.

Inside the planner, the parsed directive is carried on `FederationMetaData` as a field-configuration list, not as a bespoke caching struct:

```go
type FederationMetaData struct {
    Keys     FederationFieldConfigurations
    Requires FederationFieldConfigurations // ← parsed @requires field sets
    Provides FederationFieldConfigurations
    // ...
}
```

Lookups go through:

```go
func (d *FederationMetaData) RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool)
```

There is **no caching-specific configuration object** for `@requires`.
The caching layer never reads `Requires` directly to *enable* anything.
The exclusion is achieved indirectly: the projected cache shape is driven entirely by `ProvidesData` (see [provides.md](provides.md)), and required-only fields simply never appear in `ProvidesData`, so they are never copied into the cache.

---

## 3. Composition rules & validation

These rules are enforced by composition, *before* the caching layer ever runs.
The caching layer assumes a well-composed schema and does not re-validate them.

- `fields` is a **mandatory** `_FieldSet`.
  It is parsed into a selection set against the field's enclosing type.
- Every field referenced in the `_FieldSet` must be marked `@external` on the same type in the same subgraph.
  `@requires` declares a dependency on values the requiring subgraph does **not** own; `@external` is what marks those borrowed fields.
- The referenced external fields must be resolvable from another subgraph (typically via that subgraph's `@key` resolution), or composition fails — the gateway has to have a place to fetch them from.
- `@requires` applies to `FIELD_DEFINITION` only.
  It annotates the field whose resolver needs the inputs, not a type.

Planner-side consequence (not a caching rule, but the caching layer depends on it):
a field with `@requires` forces the planner's required-fields visitor to emit a prior fetch for the required field set, producing **sequential** execution — the required field must be resolved before the requiring field's fetch is dispatched.
This sequencing is why `@requires` fields commonly appear in `Sequence` fetch-tree nodes, which is the exact shape the L1 read path benefits from (a prior fetch can populate L1 for a later one).

---

## 4. Runtime semantics

`@requires` has no run-time hook of its own.
It acts purely by **shaping what the planner records**, and that recorded shape is then honored by the resolver's existing cache-write path.
Walk it through the two stages where caching attaches (see [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §1).

### 4.1 Plan time (compile-time, once per operation)

1. The planner's **required-fields visitor** reads `Requires` from `FederationMetaData` for each selected field.
   For a requiring field it injects the required field set as a prior selection so the gateway resolves those inputs first, and it threads them into the requiring fetch's input template as variables.
2. The planner's **provides-fields visitor** builds the `FetchInfo.ProvidesData *Object` — the alias-aware description of the exact entity shape this fetch yields (see [provides.md](provides.md)).
   **Required-only fields are deliberately not part of `ProvidesData`.**
   They are inputs, not outputs of the entity; the fetch does not *provide* them, it *consumes* them.
3. The fetch's `FetchCacheConfiguration` (key template, TTL, cache name) is attached using `@key` fields only.
   `@requires` fields never enter the cache key (see §5).

### 4.2 Resolve time (run-time, once per request) — the 4-phase parallel flow

The resolver (`loader.go`, see resolve [CLAUDE.md](../../entity-caching-v2/v2/pkg/engine/resolve/CLAUDE.md)) runs the fetch tree.
`@requires` participates only by virtue of the shape recorded at plan time; there is no `@requires`-specific branch in the loader.

- **Phase 1 — prepare + L1 check (main thread).**
  `prepareCacheKeys` renders keys from `@key` fields only.
  A `@requires` value present on an item is never part of the key, so two requests with different required inputs collide on the same entity key — correct, because the *entity* is the same; only the request-derived input differs.
- **Phase 2-L2 — bulk L2 lookup (main thread).**
  When the cached entity is read back, it was projected to `ProvidesData` at write time, so it never contained the required-only field.
  The denormalized read therefore cannot reintroduce a stale `@requires` value into the response tree.
- **Phase 2-HTTP — parallel HTTP (goroutines).**
  The requiring subgraph receives the required values as inputs (variables) in its request body, not from cache.
  This is by design: the requiring fetch always sees the *current* request's required inputs.
- **Phase 4 — merge + populate caches (main thread).**
  `populateL1Cache` writes via `structuralCopyNormalizedPassthrough` and `updateL2Cache` writes via the non-passthrough projecting transform.
  Both are driven by `ProvidesData`.
  Because the required-only field is absent from `ProvidesData`:
  - **L2 projection (`Passthrough = false`)** drops the required field outright — it is not in the listed fields, so projection excludes it.
    This is the load-bearing exclusion.
  - **L1 passthrough (`Passthrough = true`)** keeps unlisted source fields verbatim, so it does *not* by itself strip a required-only field that happens to be sitting on the response item.
    The exclusion at L1 relies on the merged item not carrying the required input as a persisted entity field in the first place, and on `@key`-only keying so a `@requires`-shaped result cannot be served back to a different-input request.
    Re-implementers must verify with the adversarial test in §7 that a required input value never round-trips through L1 to a sibling fetch.

### 4.3 Alias / normalization handling

Alias handling is inherited from the `ProvidesData` pipeline (see [provides.md](provides.md)); `@requires` adds nothing.
Because required-only fields are excluded from `ProvidesData`, they have no entry in the normalize/denormalize `Transform` and are simply never renamed, projected, or restored.

### 4.4 Ordering / threading constraints

- All cache reads, writes, projections, and the `ProvidesData`-driven transforms run on the **main thread**.
  Goroutines do subgraph HTTP only and never touch the projected shape.
- `@requires` reinforces **sequential ordering**: the required field's fetch must complete before the requiring fetch dispatches.
  This is a planner property; the caching layer does not enforce or relax it.

---

## 5. Cache key & data shape

**Effect on the cache key: none, deliberately.**
Both L1 and L2 key entities on `@key` fields only (see [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §2, §4 and [key.md](key.md)).
`@requires` fields are **never** included in the key.
Including them would fragment the cache by request-supplied input and break entity identity — the same `User:1234` would land in different entries depending on which required input a given request happened to supply.
Canonical key shape is unchanged by `@requires`:

```json
{"__typename":"User","key":{"id":"1234"}}
```

**Effect on the stored data shape: exclusion via projection.**

| Tier | Transform mode | What happens to a `@requires`-only field |
|------|----------------|------------------------------------------|
| **L2** | `structuralCopyNormalized`, `Passthrough = false` (project) | Dropped. Only `ProvidesData`-listed fields survive; the required input is not listed, so it is excluded from the bytes written to the backend. |
| **L1** | `structuralCopyNormalizedPassthrough`, `Passthrough = true` (keep + rename) | Not added by the cache. L1 accumulates entity-owned fields across fetches; it must not accumulate a required input as if it were an entity field. The `@key`-only key plus exclusion from `ProvidesData` keep required inputs out of the served L1 shape. |

The rule restated: **passthrough vs projection both converge on excluding `@requires`** — L2 by projecting it away, L1 by never treating it as an entity-owned field eligible for cross-fetch reuse.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation ([01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3, [adr/0001-foundation.md](../adr/0001-foundation.md)).**
  `@requires` exclusion is implemented entirely through the `Transform.Passthrough` switch and the `ProvidesData`-driven `StructuralCopyWithTransform` copy primitives.
  It introduces no new primitive and no new copy site; it is a property of *which fields appear in `ProvidesData`*, which the transform builders consume.
- **`@provides` ([provides.md](provides.md)) — hard dependency.**
  `@requires` is the dual of `@provides`: `@provides` defines the inclusive `ProvidesData` shape, and `@requires` is expressed as the *absence* from that shape.
  This is why both are rebuilt in the same PR (**PR-CACHE-PROJECTION**) and why `@provides` must be specified first (see dependency ordering in [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) §3).
- **`@key` ([key.md](key.md)) — dependency.**
  Entity identity comes from `@key`; `@requires` must never widen the key.
  The interplay matters most for the L1 widening check (see [provides.md](provides.md) / `validateItemHasRequiredData`): a fetch's required fields are inputs and must not be confused with the provided fields the widening check validates.
- **`@external`.**
  Every `@requires` field references `@external` fields.
  The caching layer does not consume `@external` directly, but the composition guarantee (required ⇒ external) is what assures the required value originated in another subgraph and is therefore request-derived.
- **`@requestScoped` ([request-scoped.md](request-scoped.md)) — orthogonal.**
  `@requestScoped` coordinate L1 also uses `validateItemHasRequiredData` for its widening check, but its `ProvidesData` is the *provided* shape, never the required inputs.
  There is no overlap in field handling.

---

## 7. End-to-end test plan

All tests follow the mandatory conventions: `assert.Equal` on full values only (never `Contains` / `GreaterOrEqual` / fuzzy), inline literal queries and expected JSON at the assertion site, vertical one-item-per-line for multi-key cache-log / snapshot literals, and every `ClearLog()` paired with a `GetLog()` + full assertion before the next clear.
E2E cases live under `execution/engine/` and **must be self-contained per `t.Run`** (inline setup, no shared `newXxxEnv` helpers) — re-read [execution/engine/CLAUDE.md](../../entity-caching-v2/execution/engine/CLAUDE.md) before writing.
The `reviews` subgraph already ships the fixtures these cases need (`coReviewers`, `sameUserReviewers`, both `@requires(fields: "username")`; `CacheEntity.nested @requires(fields: "a")`).

### Case A — `@requires` value is excluded from the L2 cached shape (unit, `v2/pkg/engine/resolve/`)

- **Setup.** Plan an entity fetch whose `ProvidesData` is `{id, name}` and whose input item carries an extra request-derived `requiredInput` field that is NOT in `ProvidesData`.
  Enable L2, use a logging `LoaderCache`.
- **Query / fetch.** A `User` entity fetch keyed on `id`, item `{"__typename":"User","id":"1234","name":"Me","requiredInput":"from-accounts"}`.
- **What is cached.** Exactly the projected shape, with the required input dropped.
- **Assertion.** After resolve, assert the L2 `Set` payload bytes exactly:

  ```go
  wantLog := []CacheLogEntry{
      {
          Operation: "set",
          Keys: []string{
              `{"__typename":"User","key":{"id":"1234"}}`, // key uses @key only — no requiredInput
          },
          Values: []string{
              `{"id":"1234","name":"Me"}`, // projected: requiredInput excluded from cached bytes
          },
      },
  }
  assert.Equal(t, wantLog, defaultCache.GetLog())
  ```

### Case B — `@requires` field never enters the cache key (unit, `v2/pkg/engine/resolve/cache_key_test.go`)

- **Setup.** Render the entity cache key for a `User` item that carries both `id` (`@key`) and `requiredInput` (`@requires`).
- **Assertion.** The rendered key contains only the `@key` field — exact match:

  ```go
  assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, renderedKey)
  ```

  Two items differing only in `requiredInput` must render the **same** key:

  ```go
  assert.Equal(t, keyForInputA, keyForInputB) // request-derived input must not fragment identity
  ```

### Case C — different required inputs hit the same entity entry, response stays request-correct (E2E, `execution/engine/`)

- **Setup (inline in the subtest).** Federation gateway over `accounts` + `reviews`, L2 enabled, a logging cache, two sequential requests differing only in the value `accounts` resolves for `username`.
- **Query (inline).**

  ```graphql
  { me { coReviewers { id } } }
  ```

  `coReviewers @requires(fields: "username")` forces `accounts` (resolve `username`) → `reviews` (resolve `coReviewers` with that username as input).
- **What is cached.** The `User` entity entry keyed on `id`, projected to entity-owned fields; `username` (the required input) is NOT persisted.
- **Assertions.**
  - Request 1 full response asserted exactly with `assert.Equal` on the response string.
  - Cache log after request 1: a single `set` for the `User` key whose value bytes exclude `username` — full `assert.Equal` on the `[]CacheLogEntry`, one key per line, with a trailing comment per entry.
  - `ClearLog()` then request 2: assert the `coReviewers` resolver still received the *current* request's `username` as input (the requiring fetch must not be served the prior request's required input from cache) and the response is request-correct — full `assert.Equal` on the response string and on the cache log.

### Case D — L1 does not leak a required input across sibling fetches in one request (adversarial unit, `v2/pkg/engine/resolve/`)

- **Setup.** One request, two fetches for the same `User` entity in a `Sequence`: fetch 1 provides `{id, name}` and carries a transient `requiredInput`; fetch 2 reads the same `User` from L1.
- **Assertion.** After fetch 2's L1 read, assert the merged response item for fetch 2 equals exactly the provided shape with no `requiredInput`:

  ```go
  assert.Equal(t, `{"id":"1234","name":"Me"}`, string(out)) // requiredInput must not round-trip via L1
  ```

  This is the mutation-isolation guard analogue for `@requires`: it proves the exclusion holds on the L1 read path, not only on L2 write.

### Case E — deep sequential chain via `CacheEntity.nested @requires(fields: "a")` (E2E, `execution/engine/`)

- **Setup (inline).** Gateway over `accounts` + `reviews`, L1 enabled, count subgraph HTTP calls.
- **Query (inline).**

  ```graphql
  { cacheEntity(id: "1") { nested { nested { id } } } }
  ```

  `nested @requires(fields: "a")` chains sequential entity fetches; each level resolves `a` from `accounts` first.
- **What is cached.** Each `CacheEntity` is L1-cached by `id`; the required `a` is excluded from the cached shape.
- **Assertion.** Exact HTTP call count (`assert.Equal`, never `GreaterOrEqual`) showing repeated identical entities are served from L1, plus the full response string asserted with `assert.Equal`.

---

## 8. Acceptance criteria

A reviewer can verify the re-implementation of `@requires` consumption by checking each item.
These mirror and extend the canonical criteria in [ENTITY_CACHING_ACCEPTANCE_CRITERIA.md](../../entity-caching-v2/docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md) (notably AC-L1-03).

- [ ] **No key contribution.** `@requires` fields never appear in any L1 or L2 cache key; keys are built from `@key` fields only (AC-L1-03). Two items differing only in a required input render byte-identical keys.
- [ ] **L2 projection excludes required fields.** The L2 `Set` payload contains only `ProvidesData`-listed (entity-owned) fields; required-only inputs are absent from the stored bytes (Case A).
- [ ] **L1 does not serve required inputs.** A required input present on a response item does not round-trip through L1 to a sibling fetch in the same request (Case D).
- [ ] **Request-correct inputs.** A requiring fetch always receives the *current* request's required inputs (as variables), never a value reconstructed from cache (Case C).
- [ ] **No `@requires` caching config.** The re-implementation adds no caching-specific struct for `@requires`; exclusion is achieved solely by absence from `ProvidesData`.
- [ ] **No new copy site.** `@requires` introduces no new StructuralCopy and does not change the Copy Budget; it is a `ProvidesData`-shape property consumed by existing transforms.
- [ ] **Sequential ordering respected.** The required field's fetch completes before the requiring fetch dispatches; cache logic does not relax this.
- [ ] **Tests use exact assertions.** Every test asserts full values with `assert.Equal`, inline literals, vertical multi-key literals, and clear-then-assert cache-log discipline. No `Contains` / `GreaterOrEqual` / fuzzy comparisons.
- [ ] **Acceptance criteria doc updated.** Any new or changed `@requires` test is linked from [ENTITY_CACHING_ACCEPTANCE_CRITERIA.md](../../entity-caching-v2/docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md) with path, line number, and test name.

---

## Cross-references

- Rationale and decision record: [adr/0003-requires.md](../adr/0003-requires.md).
- Architecture, seam, copy invariants, cache-key model: [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).
- Directive taxonomy and PR mapping: [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md).
- The shape `@requires` is excluded from: [provides.md](provides.md).
- Entity identity / cache-key seed: [key.md](key.md).
