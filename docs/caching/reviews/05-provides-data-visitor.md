# Reviewer notes — task 05: ProvidesData visitor (P1)

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/05-provides-data-visitor.md](../tasks/05-provides-data-visitor.md).
Spec background: RFC-2 §9 with the §9.2 correction (D9); the OLD builder via the first-pass port.

## What this commit adds

The full `cacheProvidesDataVisitor` body: per fetch, the exact field tree the fetch returns — alias-aware (`OriginalName`), argument-aware (sorted `CacheArgs`), with inline-fragment `OnTypeNames`, `__typename` dedup, and the entity-boundary reset (a nested entity fetch's tree starts at the ENTITY, not at the query root).
The task-03 registration seam is filled in: the second walk now receives `operation`/`definition`/`config`/`planners`/`fieldPlanners` before it runs.

## Decisions made

- The port source is the FIRST-PASS visitor (itself the OLD-builder port adapted to this planner); logic preserved verbatim except:
  modern-idiom touch-ups (`slices.Sorted(maps.Keys(...))` in `attachTo`, `strings.CutPrefix` in the entity-root check), removal of a redundant `v.objects == nil` re-init inside `ensurePlanner` (the planner seam always calls `reset()` first), and `addInterfaceObjectNameToTypeNames` returning at the first match instead of tracking flags.
- D9 holds: P1 registers ONLY on the second walk.
  The first pass additionally registered P1 on the main walk, where `fieldPlanners` is still incomplete and `reset()` discards whatever that pass collected — porting that would have been wasted work per plan; nothing in the visitor body needs main-walk state.
- `attachTo` keys the side-table by `*FetchInfo` — the identity-stable key across dedup, fetchID-append, and concrete-type conversion (all copy `Info` by reference) — and iterates planner IDs in sorted order for determinism.
- Root-operation-field arguments are NOT captured as `CacheArgs` (they are part of the root-field key rendered from the fetch input); only variable-bound arguments are captured, sorted by name; inline literal arguments are part of the normalized operation and thus already in the fetch input.
- `ComputeHasAliases` is NOT added yet: the task file notes it is "folded into the configurator in task 06", and nothing calls it before then — adding it now would be dead code.

## What was implemented

- `v2/pkg/engine/plan/cache_provides_data_visitor.go` — the full visitor (replacing the task-03 skeleton): `trackField` (boundary reset, `__typename` dedup, alias/args capture, frame management), `createFieldValue` (schema type → Scalar/Object/Array with nullability), `isEntityBoundaryField`/`isEntityRootField` (fragment-marker-normalized path comparison), `resolveEntityOnTypeNames`/`resolveOnTypeNames` (entity root type condition; inline-fragment expansion with union/interface/object grandparent narrowing), `addInterfaceObjectNameToTypeNames`, `attachTo`, `responseOf`.
- `v2/pkg/engine/plan/planner.go` — the seam now assigns the visitor's inputs; a comment records that `fieldPlanners` is complete because the main walk has finished (the reviewer-guidance invariant).
- Tests `cache_provides_data_visitor_port_test.go`:
  - `TestCacheProvidesDataVisitor` — the fidelity rows ported 1:1 (single key, nested object, alias, args-except-root, `__typename` dedup) PLUS a new inline-fragment row (the ported set had none; adding it immediately exposed that synthetic test planners must carry datasource + paths configurations, now provided by `newTestProvidesDataPlanner`).
  - `TestCacheProvidesDataVisitorAdversarial` — beyond the OLD set: irrelevant provider sharing the consumer query (no leakage between trees); partial overlap (two entity fetches share `username`, each gets its own tree); empty selections (a boundary-only entity planner yields an EMPTY tree — zero coverage — while a never-attributed planner is absent entirely).
  - `TestCacheProvidesDataVisitorDeterminism` — same op twice, identical side-tables (re-keyed by datasource).
- Execution-module fidelity over REAL plans (`execution/cachingtesting/provides_data_test.go`):
  - `TestProvidesDataFidelity` — full side-table pinned for a root + batch-entity plan (real `fieldPlanners`, entity-boundary reset visible in the reviews tree starting at `Product`).
  - `TestProvidesDataDeferredFetchOwnTree` — the DEFERRED inventory fetch has its own `{stock}` entry in the shared side-table, independent of the initial inventory fetch's larger tree (the defer-`Copy()` immunity criterion).

## What to look into (review focus)

- The port diff against the first-pass visitor (behavioral equivalence apart from the listed touch-ups); the fidelity rows pin the OLD tree shapes.
- `fieldPlanners` is read only in the second walk, after the main walk populated it in `LeaveField` (planner.go comment + D9 seam).
- The empty-tree semantics for a boundary-only entity planner: an empty ProvidesData tree means ZERO coverage — the controller (task 07) must treat it as "never a full hit", not "vacuously covered".
- The side-table stays a side-table (`GraphQLResponse.cacheProvidesData`); no `FetchInfo.ProvidesData` field was re-added.

## Verification evidence

- All visitor tests pass (fidelity, adversarial, determinism, gating, plan-identity).
- Execution-module fidelity tests pass against real plans on the committed fixture config.
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues; `gci`/`gofmt` clean.
