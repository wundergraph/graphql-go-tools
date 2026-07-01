# Caching port — execution progress

This file is the LIVE EXECUTION STATE of `PLAN.md`.
Any session (fresh or resumed) starts here: reconcile against `git log`, then execute the next incomplete step per PLAN §2.
Update this file BEFORE ending any session and after every task-state change.

Status legend: `todo` | `in-progress` | `blocked` | `review` (done, awaiting human approval) | `done` (committed).

## Task board

| # | Task | Status | Commit(s) | Notes / deviations |
|---|---|---|---|---|
| 01 | representationvariable extraction | done | ca0ec6fb | Pure move; tests moved and extended with an entity-interface case per the task file. |
| 02 | runtime contract + loader seam | done | e79ebbe8 | D2/D4/D8 applied; ShadowCacheEntry/ItemCacheState kept to RFC shape (first-pass extras not ported); reviewer notes in reviews/02-*.md. |
| 03 | planner wiring + engine SetCaching | done | 4653a8e1 | SetCaching keyed by datasource ID; provider drops first-pass KeySpecs (D10); P1 registers on the second walk only; reviewer notes in reviews/03-*.md. |
| 04 | test infrastructure | done | (see git log) | Fixtures in execution/cachingtesting (wgc+rover clean); fakes in v2 cache/cachetesting; first-pass RealishCache/Mode/Stage NOT ported (dead until task 07); Fetch.SetDataSource added (D8 swap); reviews/04-*.md. |
| 05 | ProvidesData visitor (P1) | todo | — | — |
| 06 | entity cache configuration | todo | — | — |
| 07 | entity L2 controller core | todo | — | — |
| 08 | multi-key / freshness / reorder | todo | — | — |
| 09 | store normalization + arg keys | todo | — | — |
| 10 | batch entity caching | todo | — | — |
| 11 | negative caching | todo | — | — |
| 12 | shadow mode | todo | — | — |
| 13 | root-field L2 | todo | — | — |
| 14 | per-root-field isolation | todo | — | — |
| 15 | entity-cache reuse | todo | — | — |
| 16 | optimizeL1Cache pass | todo | — | — |
| 17 | L1 runtime store | todo | — | — |
| 18 | defer + concurrency coverage | todo | — | — |
| 19 | partial fetching | todo | — | — |
| 20 | ART observability | todo | — | — |

## Current focus

- Next step: task 05 (ProvidesData visitor; deps 03 + 04 are done). Phase 0 is complete.
- Mid-task state: none.

## Blockers awaiting human input

- none

## Decision log (execution-time decisions not already in PLAN §7)

- 2026-07-01 (user directive): every task commit ships a reviewer document under `docs/caching/reviews/NN-<task>.md`,
  explaining the decisions of that turn, what was implemented, and what the reviewer should look into.
  Task 01's document was backfilled in the task 02 commit.
- Task 02: `ShadowCacheEntry` and `ItemCacheState` follow the RFC-1 §3.7 field set;
  the first-pass extras (`ShadowCacheEntry.CacheTTL`, per-item `BatchEntityKey`) were not ported — tasks 10/12 add them only if actually needed.
- Task 02: no existing fetch-type-switch site qualified for the sanctioned predicate cleanup
  (`preparePhase` needs the concrete types; `isEmptyEntityFetch` already dispatches via `FetchKind()`).
- Task 03: `SetCaching(map[string]cacheconfig.CachingConfiguration)` is keyed by DATASOURCE ID
  (matches `FetchInfo.DataSourceID`, the runtime provider key); unknown IDs fail `NewExecutionEngine`.
- Task 03: the provider interface drops the first-pass `KeySpecs`/`KeySpec(...)` external key input (D10 — keys derive structurally in `cacheKeyBuilder`);
  `cacheconfig` therefore imports only `time`.
- Task 03: P1 registers ONLY on the gated second walk (the first pass also registered it on the main walk);
  task 05 may revisit if the ported visitor body genuinely needs main-walk state.
- Task 03: `dataSourceConfiguration.caching` has no producer yet (accessor-only seam, same as the first pass); first consumer lands with task 05.
- Task 04: harness caching/response keys use SUBGRAPH NAMES, translated to the ID-named datasources of factory-built configs.
- Task 04: `Fetch.SetDataSource` added to the interface so datasource swapping needs no concrete-type switch (D8 spirit).
- Task 04: first-pass `RealishCache`/`Mode`/`CacheStage`/`storeAdapter` NOT ported — they need the task-07 controller and would be dead code now; task 07 introduces the controller-backed test cache.
- Task 04: the harness always plans with `IncludeQueryPlanInResponse` so plan-shape tests assert rendered trees inline (no goldens).
