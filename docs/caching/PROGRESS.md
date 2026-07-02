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
| 10 | batch entity caching | done | def25586 | Full-batch semantics per unique representation; prepareItemState reused per bucket; splice copies per target; reviews/10-*.md. |
| 11 | negative caching | done | d8888bff | DEVIATION from first pass: negative hits splice NOTHING so cached and uncached responses are byte-identical (incl. the null-bubble error); reviews/11-*.md. |
| 12 | shadow mode | done | 09d5775b | Stash-after-selection clears serving fields; ShadowCacheEntry gained CacheTTL (reserved in task-02 log); RecordingObserver materializes compares; H4 re-runs at task 17; reviews/12-*.md. |
| 13 | root-field L2 | done | be3295de + 29443089 | Key excludes the query text (coordinate + canonical variables) for alias reuse; shadow hit = plain Fetch (compare structurally impossible); reviews/13-*.md. |
| 14 | per-root-field isolation | done | 1b23ea0c | Fresh RFC-3 implementation (no first-pass reference); exactly three path-builder touches; gate on parentPath=="query" + provider policy; reviews/14-*.md. |
| 15 | entity-cache reuse | done | 44011a84 | Spec carries the FULL entity candidate set (first pass had mapping-only — E3 backfill impossible there); EntityMergePath finally populated; v1 variable-name constraint documented; reviews/15-*.md. |
| 16 | optimizeL1Cache pass | done | abe90ce7 | Ordering = dependency edges + TREE order (deviation, argued in reviews/16); schema-name+args field matching; first-pass union aliasing bug fixed and pinned; reviews/16-*.md. |
| 17 | L1 runtime store | done | 36ac68c5 | Pointer store, shared keys, L1-first ladder; fixed heap-mode StructuralCopy passthrough + optimize-pass chain break; H4 resolved (shadow stashes L1 selections); reviews/17-*.md. |
| 18 | defer + concurrency coverage | done | a0ce527a | First-pass gap CLOSED (N1/N2/M3 proven e2e); flushed-out fix: defer-group ANCESTRY ordering (treeParents via DeferDescriptors.ParentID); N4 via Flushed gate channel (synctest incompatible with engine goroutines); reviews/18-*.md. |
| 19 | partial fetching | done | (see git log) | Batch partial (filter+realign) in cache/partial.go; four explained loader touches; per-field expiry = mixed-TTL-across-fetches (interpretation documented); reviews/19-*.md. |
| 20 | ART observability | todo | — | — |

## Current focus

- Next step: task 20 (ART observability; final task).
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
- Task 10: batch buckets use `bucket[0]` as the representative (loader dedup guarantees homogeneous buckets); non-array batch responses write nothing; the loader's batch dedup loop is untouched (task 19).
- Task 11: negative hits splice NOTHING (first pass replaced the target with null) — the uncached empty fetch leaves targets unmerged and renders a null bubble WITH a non-null error; the cached path must be byte-identical.
- Task 11: `EmptyEntity` from the loader means "entities-shaped response", not "empty" — the negative branch's `ResponseData TypeNull` conjunct is load-bearing.
- Task 12: `ShadowCacheEntry.CacheTTL` added (the task-02 log reserved it); the observer derives age without re-deriving config.
- Task 12: shadow stashes AFTER the selection ladder and clears the serving fields (incl. NeedsWriteback — OnFetchResult refreshes from fresh anyway); H4 (L1-hit-wins) re-runs at task 17.
- Task 13: the root-field key preimage is coordinate + name-sorted variables, EXCLUDING the query text (alias reuse requires it); safe under the engine's always-on variable extraction (documented precondition at rootFieldCacheKey).
- Task 13: merged-fetch policy equality compares VALUES excluding coordinates; a root-field shadow hit is a plain DecisionFetch (no stash → compare structurally impossible).
- Task 14: no first-pass reference existed (RFC-3 was a follow-up there); implemented fresh: `shouldIsolateRootField` gate + exactly three path-builder touches; the fold-refusal keys off parentPath (the entity-root-node trap row pins why).
- Task 15: by-key root-field specs carry the entity's FULL candidate set (arg-coverable render at lookup, the rest backfill) — the first pass froze mapping-only candidates, making the E3 data-derived backfill impossible.
- Task 15: v1 argument binding reads the request variable NAMED like the key field (documented at deriveEntityKeyMappings); other bindings degrade to plain fetch + backfill, never wrong data.
- Task 15: `ItemCacheState.EntityMergePath` is now populated (reserved since task 02/D4) — the reuse splice/extract path.
- Task 16: executesBefore has TWO sources — dependency edges AND tree order (initial tree precedes every defer group); DependsOnFetchIDs alone cannot see cross-branch defer pairs, which the task's own defer row requires.
- Task 16: the OLD unionObjects mutated live ProvidesData trees (existing.Value overwrite) — fixed with field copies and pinned by a regression row.
- Task 17: `CacheTransaction.StructuralCopy` was an identity passthrough in heap mode (DeepCopy(nil) returns v; resolve.go:361 runs a nil-arena loader) — heap mode now forces a real copy via marshal round-trip.
- Task 17: the optimize-pass ordering walk now indexes EVERY fetch (chains pass through unconfigured hops: products→reviews→products); the l1_e2e chain fixture is the live proof.
- Task 17: H4 = shadow stashes L1-selected values too (read-never-serve absolute); the L1 negative sentinel ignores the NegativeCacheTTL knob (in-request facts).
- Task 18: executesBefore now uses defer-group ANCESTRY (treeEncloses over treeParents; postprocess derives them from DeferDescriptors.ParentID) — nested groups order after their parents; siblings stay unordered.
- Task 18: a nested @defer with a SUBSET selection is normalized away — nested-group fixtures must select via a different path (the reviews hop back to the same entity).
- Task 18: synctest bubbles deadlock on engine-lifetime goroutines (WS ping loops, resolver heartbeat) — defer-frame ordering tests gate on the writer's Flushed channel instead.
- Task 19: FetchPartial dispatches in OnFetchResult BEFORE the failure gate (covered splice must survive a failed fetch); mergeResult returns after response/error processing for partial fetches (res.cachePartial).
- Task 19: per-field partial expiry = mixed-TTL semantics ACROSS fetches (per-request query rewriting is out of scope; documented in reviews/19); entity policies gate batch partial via EnablePartialCacheLoad, root-field policies via PartialBatchLoad.
