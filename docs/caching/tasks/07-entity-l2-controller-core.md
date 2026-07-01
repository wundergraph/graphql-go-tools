# Task 07 — Entity L2 controller core (single candidate)

Phase: A (L2 entities).
Dependencies: tasks 02, 04, 06.
References: RFC-1 §3, §4.4, §5, §6; appendix rows A/B/C/D(core)/F/K/L/O; deviations D2, D4, D5 (PLAN §7).

## Problem

Nothing implements `RequestCache`, so configured entity fetches do nothing.
This task lands the L2-only entity controller for the SINGLE-candidate case; multi-key, normalization, and batch build on it (tasks 08–10).

## Scope

The controller lives in the common `v2/pkg/engine/cache` package, behind the task 02 interfaces, as clean, separable modules (L2 here; L1 in task 17; partial in task 19).

- `Controller` (implements `CacheController`) + a request-scoped `requestCache` (implements `RequestCache`), created lazily once per request.
- A `Store` interface for the L2 backend (`Get`/`Set` with TTL, batch-capable), with the in-memory `FakeStore`/`RealishCache` backing from `cachetesting` completed against it.
- `PrepareFetch`: open ONE `CacheTransaction`; render the candidate key from the canonical input (`PrepareFetchInput.Input` + `HeaderHash`); `Get`; parse the cached bytes ONCE via `tx.ParseBytes`; run the always-on per-item coverage walk from `cfg.ProvidesData` (null accepted only where nullable; `ProvidesData == nil` disables the walk → miss); AND-reduce → `DecisionSkipFullHit` or `DecisionFetch`; fill `ItemCacheState`.
- `OnFetchSkipped`: open a transaction; splice `FromCache` into the items at the surfaced MERGE PATH (D4), using `tx.StructuralCopy` to avoid aliasing.
- `OnFetchResult`: apply the write gate (`!FetchFailed && !HasErrors && ResponseData != nil && Type() != Null`); marshal ONCE and defer the L2 `Set` (bytes) to the request-end flush.
- `EndRequest`: flush deferred L2 writes, one `Set` batch per cache instance; bytes only — no lock, no arena, no transaction.
- One `CacheKeyTemplate` per candidate — the sole source of read/write/invalidate keys; keys hashed with the repo's pooled xxhash pattern; L1 and L2 will SHARE these keys (derive once; task 17 reuses them).
- TTL math calls real `time` directly (`time.Now`/`time.Since`/`time.Until`); NO injectable clock — tests use `testing/synctest`.
- Shared request-lifetime state (deferred write set, later the L1 map) lives on the `requestCache`, guarded by the transaction's `DataBuffer.Lock`; document that invariant at the struct (external-lock guard, no internal mutex).

## Tests

Controller unit tests (v2, constructed astjson + fakes, no plans):

- D core rows: D1 (full hit), D2 (miss), D3 (coverage fail → miss, stale partial NOT served), D4/D5 (nullability), D14 (`ProvidesData == nil`).
- F write-gate rows F1–F8: clean write; transport/empty/parse failure → ZERO writes (gate keys off `FetchFailed`/`ResponseData == nil`, NOT `HasErrors`); GraphQL errors; JSON-null data; status fallback; TTL stamped exactly (`synctest`).
- K flush rows K1–K4: accumulate then ONE batch per instance; flush holds bytes, never `*astjson.Value`; nothing-to-flush.

Plan-driven e2e rows (execution module, task 04 harness, public entry points):

- A gating rows A1–A7 (controller nil / config nil / flags false / eligible), B lifecycle rows B1–B8 (lazy once `BeginRequest`, `EndRequest` per entry function, clone/Free, idempotent end), C dispatch rows C1–C8 (incl. C3 full-hit skip with NO spurious error, C7 LoaderHooks not fired on a hit, C8 handle identity prepare→merge).
- L transaction rows L1–L7 asserted observably + under `-race` (lock once per hook; `ParseBytes` on malformed bytes errors without panic; no transaction from the load phase).
- O edge rows O1–O7 (hook error propagation, nil-guards, key fidelity: read key == write key from canonical input).
- The end-to-end entity L2 hit: request 1 misses + writes; request 2 serves from L2 with the gated datasource proving zero network; COMPLETE responses asserted.

## Acceptance criteria

- [ ] All listed scenario rows implemented with full-value `assert.Equal`; no placeholders.
- [ ] Write gate proven against transport/empty/parse failures (F2–F4) — the historical blocking bug.
- [ ] One transaction per hook under `-race`; each response parsed once; each key derived once.
- [ ] Splice honors the surfaced merge path (D4) — include one non-root-merge-path row.
- [ ] The runtime no-op gate still holds (A rows).
- [ ] Every exported function of the controller/store/template modules has a meaningful unit test and a responsibility doc comment.
- [ ] Lint-clean in both modules.

## Reviewer guidance

- The full-hit path must set both `skipLoad` and `fetchSkipped` (no spurious "failed to fetch" error — row C6).
- Watch for helper duplication with later tasks: predicates/type helpers belong on the `Fetch` interface or in ONE place in `engine/cache`.
