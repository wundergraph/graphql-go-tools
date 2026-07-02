# Task 05 — ProvidesData visitor (P1)

Phase: A (L2 entities).
Dependencies: tasks 03, 04.
References: RFC-2 §9 (with the §9.2 first-pass correction, deviation D9); OLD `caching_planner_state.go` on the `caching-base` worktree.

## Problem

The runtime coverage walk needs, per fetch, the exact field tree the fetch returns — alias-aware and argument-aware.
Deriving it from the merged response tree is lossy (per-fetch attribution is lost for shared fields; the merged tree is arg-blind), so a too-small tree silently disables hits and an arg-blind one serves stale data.

## Scope

- Full port of the OLD `ProvidesData` builder into `plan/cache_provides_data_visitor.go` as `cacheProvidesDataVisitor`:
  entity-boundary reset, `__typename` dedup, inline-fragment `OnTypeNames`, alias→`OriginalName`, `CacheArgs` capture (sorted).
- Execution model (D9): a gated SECOND, filter-free walk on the same `planningWalker` AFTER the main walk (`ResetVisitors()` + `SetVisitorFilter(nil)` + register only P1 + `Walk`), reading `planningVisitor.fieldPlanners` (populated by the main walk's `LeaveField`).
  The second walk runs ONLY P1 and never rebuilds the plan.
- `attachTo(plan)`: resolve fetchID → `*FetchInfo` after the walk (sorted by fetchID for determinism) and stash the `map[*FetchInfo]*Object` side-table on the `GraphQLResponse` via the task 03 accessors.
  `*FetchInfo` is the identity-stable key across dedup, fetchID-append, and concrete-type conversion (all copy `Info` by reference).
- `ComputeHasAliases` helper (folded into the configurator in task 06) sets the `HasAliases` fast-path flag when any descendant carries `OriginalName`/`CacheArgs`.

## Tests

- Unit: feed `(operation, definition, fieldPlanners)` and assert the FULL `map[*FetchInfo]*Object` per fetch with `assert.Equal` — entity-boundary reset, `__typename` dedup, inline fragments, aliases, args.
- FIDELITY GATE: compare P1's per-fetch `ProvidesData` trees against the OLD branch's trees for the shared federation fixtures (full-tree `assert.Equal`, not spot checks).
- ADVERSARIAL rows beyond the OLD set (first-pass lesson: verbatim ports carry latent OLD bugs): irrelevant-provider-sharing-a-consumer shapes, partial overlap, empty selections.
- No-op proof: with caching unconfigured the second walk does not run; existing planner tests byte-identical.
- Determinism: plan twice, assert identical side-tables.

## Acceptance criteria

- [ ] Per-fetch `ProvidesData` matches the OLD trees for every shared fixture (the fidelity gate).
- [ ] Alias and argument capture verified (`OriginalName`, sorted `CacheArgs`).
- [ ] The second walk is gated, runs only P1, and adds zero cost when caching is off.
- [ ] Deferred fetches get their own trees (side-table keyed per `*FetchInfo`, immune to defer `Copy()`).
- [ ] Lint-clean.

## Reviewer guidance

- The carrier must remain a side-table; never re-add a `FetchInfo.ProvidesData` field.
- Verify `fieldPlanners` is read only after the main walk (it is populated in `LeaveField`).
