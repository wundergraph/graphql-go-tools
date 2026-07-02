# Reviewer notes — task 19: partial fetching

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/19-partial-fetching.md](../tasks/19-partial-fetching.md).
Spec background: maintainer feedback R2.3; OLD partial-batch machinery on `caching-base`.

## What this commit adds

`DecisionFetchPartial` graduates from seam to implemented for BATCH entity fetches: a batch with SOME buckets covered serves those from cache, sends the subgraph a REDUCED representations list, and realigns the response to the original bucket positions via `ItemCacheState.BatchIndex`.
All partial semantics live in the new `engine/cache/partial.go` (the separable-module requirement); the loader diff is four small, explained touches.

## The loader touches (the sanctioned beyond-seam changes)

1. `cachePrepare`: on `DecisionFetchPartial`, swap `prepared.input` for `handle.PartialInput` (so single-flight dedups on the REDUCED input) and set `res.cachePartial`.
2. `result.cachePartial` field (one bool).
3. `mergeResult`: after response/error processing (error rendering, response parse, `responseData` extraction, subgraph-error merging — all UNCHANGED), return before the positional data merge — the reduced response is misaligned with `batchStats`, and the cache hook owns the merge.
4. `cacheMerge` dispatch: `FetchPartial` now routes to `OnFetchResult` (the task-02 seam sent it to `OnFetchSkipped`, which could splice but never write the fetched values).

## Decisions made

- Splice + realign + write happen in ONE hook (`onPartialBatchResult`) inside one transaction — the single-lock-per-hook invariant holds for partial too.
  The arm dispatches BEFORE the failure gate: on a failed partial fetch the covered splice still happens (the cached data is valid; the loader has already rendered the fetch errors) and only the fetched subset's merge/writes are skipped — pinned by the failure row.
- `filterBatchInput` is best-effort: any unexpected input shape (no `body.variables.representations`, length mismatch, degenerate all/none) falls back to a plain FULL fetch — never a wrong request.
  `keep[i]` mirrors the BatchStats bucket order, which IS the representations order (the dedup loop builds both), so the realign consumes the reduced `_entities` in fetched-bucket order.
- Config gates: entity policies expose the knob as `EnablePartialCacheLoad`, root-field policies as `PartialBatchLoad` — either enables the batch path; SHADOW WINS over partial (read-never-serve is absolute).
  With the knob off, behavior is byte-for-byte the old all-or-nothing (every earlier task's row passes unchanged).
- Shared helpers factored, not duplicated: `spliceCachedItem` (extracted from `OnFetchSkipped` — including the negative-sentinel splice-NOTHING-write-NOTHING rule) and `writeFetchedValue` (extracted from `OnFetchResult`) serve both the normal and the partial paths.
- PER-FIELD PARTIAL EXPIRY, interpretation documented: a fetch's subgraph query is FIXED at plan time, so "refetch only the expired fields" within one fetch would require per-request query rewriting — out of scope and not what the OLD implementation did either.
  The deliverable is the mixed-TTL semantics ACROSS fetches: each policy's fields live under its own key with its own TTL, so when the short-TTL portion expires only THAT subgraph is re-fetched (the e2e row pins it: inventory refetches, reviews serves from cache).
- A missing entity (null) in the reduced response merges nothing and writes nothing — negative caching rides the loader's whole-response `EmptyEntity` signal, which the partial path deliberately does not reinterpret.
- `cachetesting` gained exact-input recording (`GatedDataSource.RecordInput`, `FakeRegistry.Inputs`, `PlanResult.Inputs`) for the acceptance criterion "the subgraph request contains exactly the missing representations".

## What was implemented

- `resolve/cache_controller.go` — `FetchCacheHandle.PartialInput`.
- `resolve/loader.go` — the four touches above.
- `cache/partial.go` — `filterBatchInput` + `onPartialBatchResult`.
- `cache/controller.go` — the partial decision in the batch arm; the extracted shared helpers; the partial dispatch in `OnFetchResult`.

Tests:

- `partial_test.go` — `filterBatchInput` byte-exact rows + fallbacks; the partial split (exact partition, EXACT reduced input bytes, lookups for all buckets); realign with FULL merged targets at original positions and writes ONLY for fetched buckets; the adversarial set from the reviewer guidance (duplicated representations/multi-target buckets, all-hit → `SkipFullHit`, all-miss → `Fetch`, single-element batch); failure-in-fetched-subset (splice intact, zero writes); the config gate; shadow-wins.
- `partial_e2e_test.go` — batch partial over real plans: the reviews subgraph receives EXACTLY the missing representation (recorded input asserted; the canned response only matches a reduced request, so a full batch would fail loudly), complete response with cached and fetched reviews at their original positions; the mixed-TTL expiry row (only the expired subgraph re-fetched).

## What to look into (review focus)

- The realign (the task's declared risk center): `fetchedIndex` consumes the reduced batch strictly in uncovered-bucket order — verify against `filterBatchInput`'s keep-order and the adversarial rows.
- Double-merge audit: a covered bucket is spliced by `spliceCachedItem` and can never consume a response entity (the fetched branch is the `else`); a fetched bucket is merged only by the hook because `mergeResult` returned early (`res.cachePartial`).
- The four loader touches — confirm nothing else changed in `loader.go` (diff is small by design).
- The per-field-expiry interpretation above — push back if per-request query rewriting was actually intended; it would be a plan/loader redesign, not a cache feature.

## Verification evidence

- All unit + e2e rows pass; `-race` clean over `engine/cache`, `cachetesting`, `resolve`, and the execution harness; every earlier task's row passes unchanged (the all-or-nothing acceptance criterion).
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
