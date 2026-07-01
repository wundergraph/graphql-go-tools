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
| 04 | test infrastructure | done | b0a6b045 | Fixtures in execution/cachingtesting (wgc+rover clean); fakes in v2 cache/cachetesting; first-pass RealishCache/Mode/Stage NOT ported (dead until task 07); Fetch.SetDataSource added (D8 swap); reviews/04-*.md. |
| 05 | ProvidesData visitor (P1) | done | 648a768b | Full port + adversarial rows; ComputeHasAliases deferred to task 06 (first caller); empty-boundary tree pinned as zero coverage; reviews/05-*.md. |
| 06 | entity cache configuration | done | 3f7e3ca5 | Entity arm only (root fields task 13, mappings task 15); NEW hardening: __typename-only candidates rejected as malformed; ComputeHasAliases landed with its first caller; reviews/06-*.md. |
| 07 | entity L2 controller core | done | 29606414 | L2-only single-candidate core; deferral gates fail closed (shadow/batch/root/negative/L1/multi-key → plain fetch); no Mode enum; resolve.NewTransactionBeginner exported for controller tests; reviews/07-*.md. |
| 08 | multi-key / freshness / reorder | done | 3372187b | Full ladder + backfill; malformed cached bytes now refresh (first pass left poison entries); fixtures grew deals subgraph + featuredReview for the plan-driven cross-key row (wgc+rover clean, IDs stable); reviews/08-*.md. |
| 09 | store normalization + arg keys | done | 5cbd5244 | FromCache stays NORMALIZED; denormalize-at-splice subsumes the task-08 reorder (deleted); pending renders now use the normalized value (first-pass alias bug fixed); inventory grew stockHistory(days) for the arg e2e; reviews/09-*.md. |
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

- Next step: task 10 (batch entity caching; dep 08 is done). Tasks 11/12 are also unblocked.
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
- Task 05: `ComputeHasAliases` deferred to task 06 (its first caller, per the task file's "folded into the configurator" note).
- Task 05: a boundary-only entity planner yields an EMPTY ProvidesData tree — the task-07 controller must treat an empty tree as ZERO coverage (never a vacuous full hit).
- Task 05: `dataSourceConfiguration.Caching()` still unconsumed (the visitor reads nothing datasource-scoped); tasks 06+ read providers from the postprocess options instead.
- Task 06: `buildEntitySpec` rejects candidates whose representation carries no field beyond `__typename`
  (unknown-field @keys silently degrade to __typename-only nodes in the representation walker — a cross-entity key-collision hazard the first pass missed).
- Task 06: the all-flags-false nil gate in `buildConfig` is unreachable for entities (found policy ⇒ L1); it serves the task-13 root-field arm.
- Task 07: the controller has NO Mode enum (first pass had one); L2 enablement is `cfg.L2 && store != nil`, L1 composes in task 17.
- Task 07: every not-yet-implemented feature fails CLOSED in PrepareFetch (shadow/root-field/batch → plain fetch, no reads served); tasks 08–13 replace those gates.
- Task 07: merge hooks read `MergeInput.MergePath` (D4); `ItemCacheState.EntityMergePath` stays unset (prepare has no path input).
- Task 07: `resolve.NewTransactionBeginner` is new exported API for controller unit tests; the loader wires its own beginner internally.
- Task 07: `ttlForConfig`/MutationTTLOverride not ported (mutation caching is out-of-core, D12); writes use `cfg.TTL`.
- Task 08: served values are reordered to selection order with cached-only extras appended (task-07 splice pins updated accordingly).
- Task 08: malformed cached bytes count as a MISS for that key and get refreshed by the write path (the first pass left poison entries in place).
- Task 08: `WriteReasonRecorder` is an optional Store extension for refresh/backfill visibility; reasons never gate writes.
- Task 08: fixtures grew the `deals` subgraph (sku-keyed Product reference) + `featuredReview` (single-object upc path) to make the cross-key plan expressible; datasource IDs 0–3 unchanged, deals is "4".
- Task 08: pending candidates re-render on skip from the SERVED value only (the first pass also tried the request item).
- Task 09: `FromCache` stays NORMALIZED on the handle; denormalization to the requesting aliases happens at splice time and subsumes the task-08 reorder (`reorderToSelectionOrder` deleted).
- Task 09: the `HasAliases` fast path gates only write-side normalization; the read side always walks (it is also the selection-order pass).
- Task 09: pending-candidate re-render uses the NORMALIZED value (representation fields carry schema names — a latent first-pass bug for aliased key fields).
- Task 09: fixtures grew `Product.stockHistory(days: Int!)` on inventory for the entity-level argument e2e row (wgc + rover clean).
