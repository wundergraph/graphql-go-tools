# Task 19 — Partial fetching (partial cache load + partial batch realign)

Phase: E.
Dependencies: tasks 10, 17.
References: maintainer feedback R2.3 (graduates the RFC-1 §9 v2 seam to core); OLD partial-batch machinery on the `caching-base` worktree (variable filtering + realign).

## Problem

Today a fetch is all-or-nothing: one uncovered item (or one expired field) refetches everything.
With partial fetching a fetch resolves LAYER BY LAYER: serve what L1 has, look up the still-missing keys in L2, fetch ONLY what is still missing from the subgraph, then splice + realign.
For batch entity fetches this means refetching only the missing representations.

## Scope

This is the one task expected to touch the loader beyond the task 02 seams (sanctioned: the cache may be more integrated with the loader where that improves clarity — keep the diff surgical and explained).

- `DecisionFetchPartial` graduates from seam to implemented: `PrepareFetch` returns it when SOME items/fields are covered and some are not (gated by `cfg.EnablePartialCacheLoad` / `cfg.PartialBatchLoad`).
- Single/entity fetches: the covered subset is spliced from cache (`OnFetchSkipped`-style), the fresh subset merges normally — BOTH happen for one fetch; per-field partial expiry (mixed TTLs on one value) serves the still-fresh fields and refetches the expired ones, using the task 09 transform's per-field granularity.
- Batch fetches: filter the representations variable to the MISSING items only (cache-aware dedup loop), send the reduced batch, then REALIGN the response using `ItemCacheState.BatchIndex` so fetched and cached items land at their original positions.
- Loader interplay: the partial path must compose with single-flight (dedup on the canonical input) and preserve error semantics (a failed partial fetch fails only the fetched subset's merge, never corrupts the spliced subset).
- Keep partial a separable module in `engine/cache` (the L1/L2/partial modularity requirement).

## Tests

Controller unit tests:

- Partial coverage split: exact partition of items into served/pending; the store-op log shows lookups for all, writes only for fetched.
- Batch realign: mixed batch of N with K hits → filtered variables contain exactly N−K representations (asserted bytes); response realigned to original positions (full merged value asserted).
- Per-field partial expiry: a value with field TTLs A fresh / B expired → B refetched, A served; full result asserted.
- Failure in the fetched subset: spliced subset intact, error propagated per loader semantics.

Plan-driven e2e rows (mixed-TTL and nested fixtures from task 04):

- Batch: prime a subset of entities, run the list query — the gated datasource receives ONLY the missing representations (assert its exact input); COMPLETE response asserted.
- Partial expiry end to end: sleep past the short TTL in a `synctest` bubble, re-run, assert the subgraph receives only the expired portion.

## Acceptance criteria

- [ ] Batch partial: the subgraph request contains exactly the missing representations (exact input bytes asserted).
- [ ] Realign proven via `BatchIndex` (original order restored, no misplacement).
- [ ] Per-field partial expiry works end to end.
- [ ] All-or-nothing behavior remains for fetches with partial DISABLED (config-gated; earlier tasks' rows still pass).
- [ ] Loader diff is minimal and each touch is explained in the commit notes.
- [ ] Lint-clean in both modules.

## Reviewer guidance

- The realign is the risk center: insist on adversarial shapes (duplicated representations, all-hit degenerating to skip, all-miss degenerating to full fetch, single-element batch).
- Verify the partial path never double-merges an item (spliced AND fetched).
