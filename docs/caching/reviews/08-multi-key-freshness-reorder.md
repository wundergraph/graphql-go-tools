# Reviewer notes — task 08: multi-key candidates: render, freshness, reorder, backfill

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/08-multi-key-freshness-reorder.md](../tasks/08-multi-key-freshness-reorder.md).
Spec background: RFC-1 §3.6–3.7, §5.1; RFC-2 §6.3; appendix rows D7–D13, E1–E7.

## What this commit adds

The best-effort multi-key model inside the task-07 controller: every frozen candidate renders independently (renderable → `RenderedKeys`, not renderable → `PendingCandidates`), lookup runs under ALL rendered keys (a hit on ANY serves), candidates are collected freshest-first, the selection ladder picks or synthesizes a covering value, the chosen value is reordered to selection order, and the write side refreshes and backfills every renderable key.

## Decisions made

- Port source is the first pass's `prepareItemCacheState`/`applyHits`/`selectMultiCandidateCacheValue`/`reorderCacheValueToSelectionOrder`, reshaped: the per-item ladder is one function (`prepareItemState`), L1/shadow/negative arms are still gated out (their tasks), and the selection ladder takes the parsed values explicitly (each candidate parsed lazily, ONCE, via `tx.ParseBytes`).
- Malformed cached bytes count as a MISS for that key (added to the item's missed set, so the write path refreshes it) — the first pass silently dropped such keys from both hit and backfill sets, leaving poison entries in place.
- The freshness order is `compareCacheCandidateFreshness`: known remaining TTL beats unknown (non-positive), larger beats smaller, stable for ties.
- The selection ladder (documented at `selectMultiCandidateCacheValue`): freshest-covers → serve; union-covers (merged OLDEST-first so the freshest wins conflicts) → serve + `NeedsWriteback`; older-single-covers → serve + `NeedsWriteback`; nothing → miss.
  A merge failure voids the synthesis and falls through to the older-single rung (never a half-merged value).
- Reorder keeps cached-only extras APPENDED after the selection-ordered fields, so a write-back never loses data another fetch needs (e.g. key fields).
- `MustWriteBack` = full hit AND (missed keys ∥ pending candidates ∥ `NeedsWriteback`); `OnFetchSkipped` then: refresh ALL rendered keys after a synthesized/older selection, else backfill the missed keys, plus best-effort pending-candidate renders from the SERVED value (which can carry more fields than the request item).
- `WriteReason` (refresh/backfill) is metadata carried on the deferred set and surfaced through the NEW optional `WriteReasonRecorder` store extension — reasons never gate writes; stores that don't implement it see plain Sets.
- FIXTURE EXTENSION (composition re-validated with wgc AND rover): a `deals` subgraph referencing `Product` by the SKU key plus a single-object `featuredReview` root field on reviews.
  Without a key-availability asymmetry (one path providing only upc, another only sku) a true plan-driven cross-key hit is not expressible; datasource IDs 0–3 are unchanged (deals is "4"), so all pinned plans stay valid.

## What was implemented

- `controller.go` — `prepareItemState` (render/lookup/collect per item), handle-side `MustWriteBack`, the `prefixes`/`missedKeys` per-handle side maps (same external-lock invariant), `OnFetchSkipped` write-back arms, `OnFetchResult` pending re-render + reasons, reason-aware flush.
- `multikey.go` — `selectMultiCandidateCacheValue`, `compareCacheCandidateFreshness`, `reorderToSelectionOrder`.
- `cachetesting` untouched; the unit `testStore` gained `seed`/`seedNoTTL` (unknown-TTL hits) and the `Reason` op column.

Tests (row IDs in subtest names; every row asserts the EXACT ordered `[]testStoreOp`/`[]StoreOp` where writes occur):

- `multikey_test.go` — E1 (both render, both refreshed), E3 (`[Get k_upc, Set k_upc refresh, Set k_sku backfill]` exact), E4 (none renderable: ZERO Gets, two backfills), E7 (single-key degenerate), E2 (hit on the non-primary key + missed-key backfill on skip), E5 (pending candidate renders from the SERVED value), D7 (freshest wins, exact `SelectedRemainingTTL` under synctest, candidates recorded freshest-first), D8 (known beats unknown via `seedNoTTL`), D9 (merge synthesis with conflict won by the fresher value, `NeedsWriteback` + canonical refresh of BOTH keys), D10 (older-single fallback), adversarial empty-union and all-stale rows, D12/D13 AND-reduction.
- `multikey_e2e_test.go` (plan-driven) — prime through the upc-keyed reviews path (response carries sku → sku key BACKFILLED), serve through the sku-keyed deals path: complete responses, ZERO products loads on the serve request, and the full ordered store-op log across both requests.
- Task-07 rows updated where behavior legitimately changed: served values are now reordered to selection order (D1 and the splice rows pin the new canonical order); F/K expected ops carry `Reason: refresh`.

## What to look into (review focus)

- The ladder ordering (reviewer-flagged subtlest logic): freshest-single before union before older-single; the union merges OLDEST-first — verify the conflict-winner direction against the D9 row.
- `NeedsWriteback` refreshes ALL rendered keys (not only the stale one) — intentional: the synthesized value is the new canonical entry everywhere.
- The pending-candidate render on `OnFetchSkipped` tries ONLY the served value (the first pass also fell back to the request item; the served value is a superset of what the item can render for entity data — flag if you see a case where the item renders and the value does not).
- Fixture diff: `deals.graphql` + `featuredReview`; re-run `./compose.sh` as the composability guard.

## Verification evidence

- All multi-key unit rows and the cross-key e2e pass (first run); `-race` clean over `engine/cache` and the execution harness tests.
- wgc + rover composition clean after the fixture extension; all pre-existing harness tests still pass (IDs stable).
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
