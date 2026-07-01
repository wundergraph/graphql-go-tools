# Task 10 — Batch entity caching

Phase: A (L2 entities).
Dependencies: task 08.
References: RFC-1 §5.1(e), appendix rows I1–I8; deviation D4 (PLAN §7).

## Problem

Batch entity fetches (`BatchEntityFetch`) carry many representations in one request; the controller must key, look up, and write PER ITEM, and splice batch hits back to the right merge targets — full-batch in this task (all-hit serves, any-miss refetches everything; partial refetch is task 19).

## Scope

- `PrepareFetch` batch arm: per unique representation (via `BatchStats`), render candidates and look up; record `ItemCacheState.BatchIndex` (original batch position) and `BatchEntityKey`; AND-reduce across ALL items — all covered → `DecisionSkipFullHit`, any uncovered → `DecisionFetch` (full refetch in this task).
- `OnFetchSkipped`: splice each item's `FromCache` to its merge targets at the surfaced merge path (D4).
- `OnFetchResult`: per-element multi-key render-then-backfill from the batch response.
- Batch empty short-circuit preserved (the loader's existing empty-batch skip never reaches the cache).
- Keep the per-item state exactly what task 19 (partial batch) will need — `BatchIndex` and per-item rendered/pending keys are load-bearing there.

## Tests

Controller unit tests: I rows — batch full hit (zero network), all-miss, MIXED → full refetch (assert the full store-op log shows lookups but the response comes entirely from the network), per-element multi-key backfill, batch empty short-circuit, `BatchIndex` recorded per item.

Plan-driven e2e rows (task 04 nested/cross-subgraph fixtures):

- A nested list query producing a real `BatchEntityFetch`: request 1 populates per-entity entries; request 2 full-batch hit with the gated datasource proving zero network; COMPLETE responses asserted.
- A mixed run (some entities primed, some not) → full refetch, correct response, and writes for ALL entities afterward.

## Acceptance criteria

- [ ] Per-item keying proven: N distinct entities in one batch produce N `Get`s / N `Set`s with distinct keys (exact ordered ops asserted, normalized where ordering is free).
- [ ] Batch splice lands each value at its correct merge target (non-root merge path row included).
- [ ] Mixed batches never partially serve in this task (that is task 19), and never mis-merge.
- [ ] Lint-clean.

## Reviewer guidance

- The batch dedup loop in the loader must remain UNCHANGED here; cache awareness of the dedup loop arrives only with partial fetching (task 19).
