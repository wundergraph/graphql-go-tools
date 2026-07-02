# Defer `__typename` scope alignment

## Context

When a client selects `__typename` inside an `@defer` on a federated entity, the
current planner can produce a query that the resolver cannot execute. Example:

```graphql
{ article { id ... @defer { __typename title } reviews { id ... } } }
```

The initial subgraph fetch comes out as `{article {id __internal___typename: __typename}}`
— the `__typename` is **aliased**. The resolver determines every object's type by
reading the *literal* `__typename` key (`resolvable.go`:
`typeName := value.GetStringBytes("__typename")`), which then gates `OnTypeNames`
fields, entity representations, and abstract-type resolution. With only
`__internal___typename` present, the type read returns nil, the `Article`
representation for the `reviews` entity jump is empty, that fetch is skipped, and
the non-nullable `reviews` resolves to null → the whole response errors.

### Root cause

`__typename` has two roles that collide only here:

1. **Structural/meta** — read literally as `__typename` to discriminate object
   type. Must be a plain `__typename`, present in the scope where its object is
   materialized (for `article`, the initial response — where the `reviews` jump
   also happens).
2. **User-selectable** — can appear anywhere, including inside `@defer`.

A field's `@__defer_internal(id)` is the **single source of truth for both
planning** (which fetch the field lands in) **and resolving** (which payload it
renders into). When a user defers `__typename` on an object that is materialized
*earlier* than that defer scope, the planner tries to keep both a deferred copy
and the structurally-required copy, and aliases one — destroying the literal
`__typename` the resolver needs.

Key property: **`__typename` is free and constant** — its value is known the
instant its object materializes. Deferring it past its object's scope buys no
latency.

## Decision

Adopt a single invariant:

> **A `__typename`'s defer id equals the defer id of its enclosing object field**
> (the object it describes) — never the innermost `@defer` it was textually
> written in.

Because `__typename` is available exactly when its object is, this keeps one
defer id per field (no planning/resolving decoupling) and resolves both shapes:

- **Object materialized earlier than the textual defer** (the bug): `article` is
  non-deferred → its `__typename` becomes non-deferred and is fetched/delivered
  in the initial response. The planner then finds a plain, non-deferred
  `__typename`, reuses it without aliasing → literal `__typename` in the initial
  fetch → type discrimination and the `reviews` jump work. `title` still defers.
- **Object materialized inside the defer** (nested entity jump):
  `user { ... @defer { articleFromOtherSubgraph { __typename id anotherSubgraphField } } }`
  — `articleFromOtherSubgraph` lives in defer-1, so its `__typename` stays in
  defer-1 (unchanged, still correct; it is the representation source for the
  in-scope `anotherSubgraphField` jump).

Client-visible consequence: a `__typename` written inside `@defer` arrives in the
scope where its object first appears, which may be earlier than written. This is
accepted (Option A from brainstorming). The alternative — preserving the exact
textual defer frame for `__typename` delivery while fetching it earlier (Option
B) — requires splitting the single defer id into separate planning and delivery
ids, an architectural change not justified for a free, constant field.

This generalizes existing behavior: `deferEnsureTypename` already stamps its
injected placeholder `__typename` with the **parent** defer id
(`AddDeferInternalDirectiveToField(fieldRef, parentDeferID, …)`). We extend the
same "typename takes its enclosing object's scope" rule to user-written
`__typename`.

## Mechanism

A normalization adjustment that runs in the defer pipeline: for every
`@__defer_internal`-stamped `__typename` field, set its defer id (and parent) to
match its **enclosing object field's** defer scope; if the enclosing object field
carries no defer (or there is no enclosing field — root level), remove the defer
directive from the `__typename` entirely.

- **Locus**: a small dedicated defer normalization rule (working name
  `deferAlignTypenameScope`), or folded into the existing defer finalization.
  It walks fields; on a `__typename` field it reads the nearest ancestor field's
  `FieldInternalDeferID` and rewrites the `__typename`'s directive accordingly.
- **Ordering**: must run after field merging (`deduplicateFields`, so the
  enclosing object's *final* defer id is known) and **before**
  `deferPopulateParentIds` (so parent ids are computed from the aligned value).
  Reuse the existing AST helpers (`FieldInternalDeferID`,
  `AddDeferInternalDirectiveToField`, `RemoveDirectiveByName` /
  `ArgumentList.RemoveArgumentByName`) rather than new ones where possible.
- The change is normalization-only: the planner (`required_fields_visitor.go`)
  and resolver (`resolvable.go`) are untouched. With a plain non-deferred
  `__typename` present, the planner's existing key/representation handling does
  the right thing and never aliases it.

## Edge cases

- **`__typename`-only defer fragment** (e.g. `... @defer { __typename }` on an
  initially-materialized object): after alignment the defer group is empty.
  Must collapse to no defer (no `pending` entry, nothing in `incremental`),
  consistent with how empty defers are already dropped.
- **Root-level `__typename`** (`{ ... @defer { __typename } }`): no enclosing
  object field → treat as initial (remove defer).
- **Deeper nesting**: `__typename` aligns to its *immediate* enclosing object's
  scope, which may itself be a nested defer id — correct by construction.
- **Multiple `__typename` at one level** post-merge: after alignment they share
  the enclosing object's id and dedup to one.

## Testing strategy

- **Rule-level unit tests** (`v2/pkg/astnormalization`): feed pre-stamped
  operations and assert `__typename` is re-scoped to its enclosing object's
  defer id (or stripped). Cover: deferred `__typename` on a non-deferred object
  → stripped; deferred `__typename` on a deferred object → kept at that id;
  root-level; `__typename`-only fragment → empty-defer collapse.
- **Full-pipeline normalization test** (`TestNormalizeOperation`): the original
  `article` query normalizes with a plain non-deferred `__typename`.
- **Engine integration** (`execution_engine_defer_test.go`): the already-added
  3-subgraph `article / reviews / authors` section now succeeds end-to-end
  (the previously-`TBD` expected response gets filled with the captured,
  verified streaming output). Add the nested deferred-entity case
  (`articleFromOtherSubgraph`) to lock in that legitimately-deferred `__typename`
  still works.
- **Regression**: full `v2/pkg/astnormalization` + `execution/engine` defer and
  federation suites stay green.

## Out of scope

- Option B (strict delivery of `__typename` in its exact textual defer frame via
  decoupled planning/delivery ids).
- Any change to the resolver's literal `__typename` read or the planner's
  aliasing logic.
