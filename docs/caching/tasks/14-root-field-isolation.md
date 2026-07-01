# Task 14 — Per-root-field cache isolation

Phase: B (L2 root fields; ships last of the root-field work).
Dependencies: task 13.
References: RFC-3 (now IN CORE per maintainer feedback R2.2); deviation D5 (PLAN §7).

## Problem

The path builder merges sibling query root fields from the same datasource into ONE fetch.
A merged fetch either loses caching entirely (mixed/uncached siblings → the task 13 all-or-nothing decline) or caches as one coarse unit (poor reuse, shared invalidation).
Splitting cached root fields into their own fetches recovers caching for mixed policies AND gives each field its own L2 key.

## Scope

Isolation is a MERGE decision made during path building — a post-pass cannot un-merge a fused fetch — so this task edits `path_builder_visitor.go` directly (allowed; there are no forbidden files).

- New unit `plan/root_field_isolation.go` owning the gate predicate `shouldIsolate(field, operationType)`:
  caching configured; `OperationTypeQuery` only; direct children of the operation root only; the exact `(typeName, fieldName)` has a `RootFieldPolicy` from the datasource's `CacheConfigProvider` (NEVER from `FederationMetaData`).
- `path_builder_visitor.go`, exactly three touches:
  1. one additive flag on `objectFetchConfiguration` (e.g. `isolatedRootField bool`);
  2. in `handlePlanningField`: isolate a cached query root into a NEW planner (second disjunct beside the existing mutation-root branch) and set the flag;
  3. at the top of the `planWithExistingPlanners` loop: refuse to fold another TOP-LEVEL operation field into an isolated planner — keyed off `isParentPathIsRootOperationPath(field.parentPath)`, NEVER off `IsRootNode` (entity types are also root nodes; using `IsRootNode` would tear the isolated field's own subtree apart — the entity-root-node trap).
- Per-field isolation (every cached root field its own planner), matching OLD; no cold-cache amplification cap (opt-in, bounded by configured fields; per-policy-group batching is a recorded future mitigation).
- Downstream passes need NO changes: P1 sees the isolated planners; the configurator sees single-root-field fetches (the all-or-nothing rule trivially passes); isolated fetches carry no mutual `DependsOnFetchIDs` and run in parallel.

## Tests

- NO-OP proof: with caching unconfigured, `shouldIsolate` is never true and plans are BYTE-IDENTICAL (the new branches are provably dead).
- Full-plan rows (task 04 fixture with differing sibling policies, `assert.Equal` on whole plans):
  1. two cached siblings with different policies → TWO parallel fetches, each with its own FULL `Cache` (own name/TTL), no dependency edge;
  2. cached + uncached sibling → cached isolated with `Cache`, uncached fetch without;
  3. caching off → ONE merged fetch, byte-identical to pre-task plans.
- Entity-root-node trap row: a nested entity under an isolated root field still merges into the isolated planner's subtree.
- Defer composition row: an isolated root field with a deferred fragment lands its deferred sub-fetch in the correct per-`DeferID` tree.
- e2e row: the two isolated siblings cached and served independently (one expires, the other still hits — uses the mixed-TTL fixture).

## Acceptance criteria

- [ ] `path_builder_visitor.go` diff is exactly the three touches above; all decision logic and tests live in the new unit.
- [ ] The entity-root-node trap row passes (guard keys off parentPath).
- [ ] The task 13 all-or-nothing rule remains as the residual safety net for any still-mixed fetch.
- [ ] Byte-identical plans with caching off.
- [ ] Lint-clean.

## Reviewer guidance

- Verify the gate reads `CacheConfigProvider.RootFieldPolicy`, never `FederationConfiguration()`.
- Verify isolated fetches execute in parallel (no dependency edges between them).
