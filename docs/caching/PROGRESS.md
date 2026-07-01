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
| 03 | planner wiring + engine SetCaching | todo | — | — |
| 04 | test infrastructure | todo | — | — |
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

- Next step: task 03 (planner wiring + engine SetCaching; deps 01 + 02 are done).
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
