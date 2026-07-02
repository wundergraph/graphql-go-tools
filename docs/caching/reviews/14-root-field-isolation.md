# Reviewer notes — task 14: per-root-field cache isolation

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/14-root-field-isolation.md](../tasks/14-root-field-isolation.md).
Spec background: RFC-3 (in core per maintainer feedback R2.2); deviation D5.

## What this commit adds

Cached query root fields now get their OWN planner during path building: sibling root fields with different (or no) cache policies never merge into one fetch, so each cached field keeps its own L2 key and TTL and the task-13 all-or-nothing decline becomes a rare residual.
There was NO first-pass implementation of this (RFC-3 was a follow-up there) — this is a fresh implementation from the RFC.

## Decisions made

- All decision logic lives in the new unit `plan/root_field_isolation.go` (`shouldIsolateRootField`); `path_builder_visitor.go` carries EXACTLY the three sanctioned touches:
  1. the additive `objectFetchConfiguration.isolatedRootField` flag;
  2. the isolation disjunct beside the mutation-root branch in `handlePlanningField` (isolated planners are created via the existing `addNewPlanner`, then flagged);
  3. the fold-refusal at the top of `planWithExistingPlanners`, keyed off `isParentPathIsRootOperationPath(field.parentPath)` — NEVER off `IsRootNode` (entity types are root nodes too; an `IsRootNode` guard would tear the isolated field's own nested entity subtree apart — the entity-root-node trap, pinned by its own test row).
- Gate condition: non-empty `CacheConfigProviders` (with caching off the branches are provably dead — plans byte-identical), `field.parentPath == "query"` (covers both "query operation only" and "direct child of the operation root" in one comparison), and the exact coordinate having a `RootFieldPolicy` from the datasource's provider — read from the provider ONLY, never `FederationMetaData`.
- Per-field isolation with no amplification cap (matching OLD; opt-in and bounded by configured fields; per-policy-group batching is the recorded future mitigation).
- Secondary-run safety: revisits of an already-planned fieldRef return early via the existing `fieldsPlannedOn` guard at the top of `handlePlanningField`, so the isolation branch cannot double-create planners.
- Downstream passes needed ZERO changes: P1, the configurator (single-root-field fetches trivially pass the all-or-nothing rule), and the defer machinery all see the isolated planners naturally — pinned by the defer-composition row.

## What was implemented

- `plan/root_field_isolation.go` — the gate predicate with the full rationale doc comment.
- `plan/path_builder_visitor.go` — the three touches (diff-checkable).

Tests (plan-level rows drive REAL plans through the harness; assertions render path + cache name/TTL + dependency edges per fetch):

- Two cached siblings with different policies → TWO fetches, each with its OWN cache name and TTL, no dependency edges (parallel).
- Cached + uncached sibling → the cached one isolated with config, the uncached one separate without.
- Caching off → ONE merged fetch (plus the pre-existing task-04 smoke row and the full plan suite as the byte-identical proof).
- ENTITY-ROOT-NODE TRAP row: `{ products { upc stock } }` under isolation yields exactly the isolated products fetch + the dependent inventory entity fetch — the isolated subtree stays intact — and resolves correctly end to end.
- Defer composition row: an isolated root with a deferred fragment keeps its deferred sub-fetch in the correct defer group.
- e2e independent serving: both siblings cached under DISTINCT keys; force-expiring ONE entry (store-double aging; TTL semantics are pinned by the synctest unit rows) makes only that fetch refetch while the sibling still hits.

## What to look into (review focus)

- The `path_builder_visitor.go` diff — confirm it is exactly the three touches and nothing else.
- The fold-refusal guard placement: after the datasource-hash check, before every other merge consideration — confirm no earlier `return plannerIdx, true` path (e.g. the secondary-run `HasPath` shortcut) can fold a top-level field into an isolated planner: the secondary-run shortcut only fires for paths the planner ALREADY has, which for an isolated planner is only its own field's subtree.
- The gate's `parentPath == "query"` literal: confirm the walker renders operation roots as bare `query` (the existing `isParentPathIsRootOperationPath` relies on the same convention).

## Verification evidence

- All isolation rows pass; the full `plan` package suite passes untouched (no-op proof); `-race` clean on the harness tests.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
