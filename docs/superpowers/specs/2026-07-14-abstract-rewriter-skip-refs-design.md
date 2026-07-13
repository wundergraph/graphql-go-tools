# Abstract Selection Rewriter: Scope-Aware Field Ref Tracking

**Date:** 2026-07-14
**Status:** Implemented
**Files:** `v2/pkg/engine/plan/abstract_selection_rewriter.go`, `v2/pkg/engine/plan/node_selection_visitor.go`

## Problem

When the abstract selection rewriter flattens fragments, it reports which old field
refs became which new field refs via `RewriteResult.changedFieldRefs`. The mapping is
built by matching fields on their query path with inline fragment names stripped
(`AbstractFieldPathCollector` uses `Path.WithoutInlineFragmentNames()`).

Stripping fragment names collapses distinct response positions into one key:
`nodes.id`, `nodes.<on A>.id`, and `nodes.<on B>.id` all become `query.nodes.id`.
As a result, an old ref maps to **every** new ref with the same field name at the
same depth, regardless of type condition.

`nodeSelectionVisitor.updateSkipFieldRefs` then propagates skip status blindly: if a
skipped (planner-added) ref maps to new refs, all of them become skipped — even when
a merge partner was a user-requested field.

### Reproduced failure (exploratory test, subgraph missing type `C` triggers rewrite)

```
before:                              after rewrite:
nodes {                              nodes {
  ... on A { id }          ref 0       ... on A { id }            ref 5
  ... on B {                           ... on B {
    externalField          ref 1         externalField            ref 6
    id  (planner, skipped) ref 2         id                       ref 7
  }                                    }
  ... on C { id }          ref 3     }

changedFieldRefs:
  old 0 (user id on A)    -> [5 7]
  old 2 (planner id on B) -> [5 7]   <- identical to the user's mapping
  old 3 (id on C)         -> [5 7]
```

- **Current behavior (lost field):** skipped ref 2 maps to `[5 7]` → both marked
  skipped → user's `id` on A disappears from the response.
- **Naive fix insufficient (leak):** changing skip propagation to "skip only if all
  origins were skipped" makes ref 7 visible (origin 0 is not skipped) → `id` leaks
  into the response for B objects the user never requested it on. At path
  granularity refs 5 and 7 are indistinguishable — the fragment information is
  already destroyed by the time the visitor sees the mapping.

A second reproduced case (user `id` at interface level, planner `id` inside
`... on B`) shows the complementary failure: all `id` refs map to both fragment
copies, current code skips both, and the user's interface-level `id` is lost for
every type.

## Design

Two components; neither is sufficient alone.

### Component 1: scope-aware ref matching in the rewriter

Extend `AbstractFieldPathCollector` to record, per field ref, a **type-condition
scope** alongside the stripped name path:

- For each name-path segment (each ancestor field level), record the effective
  scope: the set of concrete type names implied by inline fragment conditions
  directly enclosing the field within its parent selection set.
  - Object type condition → `{Type}`
  - Interface condition → concrete types implementing it (resolved against
    `definition`)
  - Union condition → union member types
  - No enclosing fragment at that level → `nil`, meaning "unconstrained"
    (the parent's full scope)
  - Nested/stacked fragment conditions at the same level → intersection
- The collector maintains a scope stack: push/pop on inline fragment enter/leave,
  aligned with selection-set depth.

**Matching rule** in `collectChangedRefs`: old ref maps to new ref iff their name
paths are equal AND at every level the scopes intersect (`nil` intersects
everything).

**Path key change:** use `FieldAliasOrNameString` instead of `FieldNameString` for
the field's own path segment. Normalization merges by alias+name; keying by name
only conflates differently-aliased duplicates (`a: id` vs `id`), which are distinct
response fields.

**Result shape:** `RewriteResult` gains a second map:

```go
type RewriteResult struct {
    rewritten        bool
    changedFieldRefs map[int][]int // old -> new (as today, identity entries filtered)
    fieldRefOrigins  map[int][]int // new -> old, complete (identity entries included)
}
```

`fieldRefOrigins` must be complete (including a surviving ref mapping to itself),
because the skip decision for a new ref requires knowing *all* refs that occupy its
response position.

With scope matching, the reproduced case becomes:

```
changedFieldRefs:  0 -> [5],  2 -> [7]
fieldRefOrigins:   5 -> [0],  7 -> [2]
```

### Component 2: skip semantics in the visitor

Rewrite `nodeSelectionVisitor.updateSkipFieldRefs` to consume `fieldRefOrigins`:

- A new ref is added to `skipFieldsRefs` iff **all** of its origins are skipped.
- If any origin is not skipped and the new ref is currently present in
  `skipFieldsRefs` (surviving-ref case: a skipped ref survived the merge and now
  also represents a user field), remove it from `skipFieldsRefs`.

Old skipped refs that no longer exist in the operation stay in `skipFieldsRefs`;
they are never visited, so this is harmless (unchanged from today).

`updateFieldDependsOn` keeps consuming `changedFieldRefs` unchanged — it becomes
more precise automatically (a jump dependency on the planner's `id` in B now
updates to B's copy only, instead of fanning out to A's copy).

### Correctness across the reproduced cases

| Case | New ref origins | Result |
|---|---|---|
| user `... on A { id }` + skipped `id` in B | 5←{0 user}, 7←{2 planner} | 5 visible, 7 skipped ✓ |
| user `id` at interface level + skipped `id` in B | 5←{0}, 6←{0, 2} | both visible ✓ (user asked on all types) |
| user `id fullName` expanded into all fragments | each copy ← {user ref} | all visible ✓ |
| chained rewrites (secondary runs, force rewrite) | old scopes already concrete after first rewrite → 1:1 mapping per type | precise ✓ |

### Known non-goals

- **Disappearing paths** (`TODO` in `collectChangedRefs`): a user field whose
  fragment is dropped entirely (type not in datasource) still maps to nothing.
  Pre-existing behavior, unchanged.
- Fields planned on other datasources, `@defer` interactions, and provides trees
  are untouched — the change is local to path collection/matching and skip
  propagation.

## Testing

1. **Rewriter unit tests** (extend `abstract_selection_rewriter_test.go` or a new
   file): assert `changedFieldRefs`/`fieldRefOrigins` for the collision cases
   (fragment-scoped vs interface-level duplicates, aliased duplicates, chained
   rewrite). Convert the exploratory test
   (`zz_rewrite_collision_exploration_test.go`) into assertions or delete it.
2. **Planner-level test**: full plan for the reproduced federation scenario,
   asserting the response shape — `id` present under A, absent under B (skipped),
   `externalField` planned via the jump.
3. Run the existing plan + datasource federation test suites to catch regressions
   from the more precise `updateFieldDependsOn` mapping.
