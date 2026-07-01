# Reviewer notes — task 10: batch entity caching

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/10-batch-entity-caching.md](../tasks/10-batch-entity-caching.md).
Spec background: RFC-1 §5.1(e); appendix rows I1–I8; deviation D4.

## What this commit adds

The batch arm of the controller: batch entity fetches are keyed, looked up, and written PER unique representation, with full-batch semantics — all covered serves, any uncovered refetches everything (partial refetch is task 19).
The task-07 "batch fetches until task 10" deferral gate is replaced by real behavior.

## Decisions made

- `prepareBatchFetch` mirrors the item loop over `BatchStats` buckets, taking `bucket[0]` as the representative (all targets of a bucket share one representation by construction of the loader's dedup), reusing `prepareItemState` — no duplicated ladder.
- `ItemCacheState.BatchIndex` records the ORIGINAL batch position and `FetchCacheHandle.BatchEntityKey` marks the mode; both are exactly what task 19's partial realign needs (per-item rendered/pending keys already exist from task 08).
- Splice: a batch item denormalizes ONCE PER TARGET (each target gets its own transaction-owned copy) into every merge target of its bucket, at the surfaced merge path (D4).
- Write: the batch response's `_entities` array indexes by `BatchIndex`; a NON-ARRAY batch response writes nothing (fail closed); the merge-path extraction applies per element, after batch indexing.
- The loader's batch dedup loop is UNTOUCHED (cache awareness of that loop arrives with task 19, per the reviewer guidance).
- The empty-batch short-circuit (`BatchStats != nil && len == 0`) returns no handle — the loader's own empty-batch skip normally prevents the call entirely.

## What was implemented

- `controller.go` — `prepareBatchFetch`; batch-aware splice targets in `OnFetchSkipped`; batch element extraction in `OnFetchResult`.
- No new helpers: keying, selection, normalization, and backfill all reuse the task 07–09 modules per element.

Tests:

- `controller_batch_test.go` — I1 per-item keying (two entities → two distinct keys, exact ordered ops), I2 full-batch hit spliced into EVERY target of a deduplicated bucket at a NON-ROOT merge path (all three targets pinned), I3 mixed batch (covered item recorded but the batch refetches fully; the exact op log shows the lookups AND the post-refetch writes for ALL entities), I4 per-element multi-key backfill from the batch response, I5 non-array batch response writes nothing; the empty-batch short-circuit row replaced the superseded task-07 gate row.
- `batch_e2e_test.go` (plan-driven, the real reviews `BatchEntityFetch`) — full-batch hit end to end (request 1 writes one entry per entity with the exact op log; request 2 serves with ZERO reviews loads and byte-identical response); the mixed run (one primed, one not → full refetch with the correct response, then a repeat proves BOTH entities were written — its reviews response is intentionally empty, so any accidental network use fails loudly).

## What to look into (review focus)

- Bucket-representative soundness: `bucket[0]` stands for the whole bucket — all targets of one `BatchStats` bucket share one unique representation by the loader's dedup construction; flag if any loader path can produce heterogeneous buckets.
- The splice copies per TARGET (not per item) — one shared copy across targets would alias mutations between response branches.
- Mixed batches must never partially serve here: I3/the e2e mixed run pin `DecisionFetch` with the covered item's `FromCache` populated but unused for serving.
- `OnFetchResult` indexes strictly by `BatchIndex` against the response array and skips out-of-range items — the loader guarantees `len(batchStats) == len(batch)` for successful batch responses.

## Verification evidence

- All I rows and both e2e rows pass (first run); `-race` clean over `engine/cache`, `cachetesting`, and the execution harness tests.
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
