# Commit A2b — multi-candidate render-then-backfill + freshness + reorder

Plan item: `docs/caching/PLAN.md` §3, A2 (second of three parts).
RFC sections: RFC-1 §3.6/§3.7 (multi-key `CacheKeySpec`/`ItemCacheState`); appendix §5.4 (D7-D13), §5.5/§8 (E1-E7).
Phase: A (L2 entities).
OLD reference: `caching-base` `resolve/loader.go` (`reorderCacheValueToSelectionOrder`), `resolve/loader_cache.go` (multi-candidate freshness/synthesis).

## Problem

A2a served only the first `@key` candidate. The model is best-effort MULTI-KEY: an entity may declare several `@key` sets, each independently renderable; a hit on ANY rendered key serves, and the unrendered/absent keys are backfilled.

## Solution

Extend the L2 entity controller to best-effort multi-key.

- `PrepareFetch`: per item, render EVERY candidate (unrenderable -> `PendingCandidates` + flag writeback); `Get` under ALL `RenderedKeys`; record each hit as a `CacheCandidate{Value, RemainingTTL}`; sort by freshness (largest remaining TTL, known-beats-unknown stable tie-break); select a covering value — freshest single, else MERGE-SYNTHESIS of the union (sets `NeedsWriteback`), else older-single fallback (`NeedsWriteback`); REORDER the chosen value into `ProvidesData`/selection field order; AND-reduce to `DecisionSkipFullHit`/`DecisionFetch`. `handle.MustWriteBack = allCovered && (any pending/missed/needs-writeback)`.
- `OnFetchSkipped` (full hit): after splicing, re-render the `PendingCandidates` and the missed `RenderedKeys` from the SERVED value and emit backfill `Set`s; rewrite canonical values flagged `NeedsWriteback` as `refresh`. No network.
- `OnFetchResult` (after a fetch): write the `RenderedKeys` as `refresh` and re-render `PendingCandidates` from the FRESH entity value, emitting backfill `Set`s for the now-renderable keys.
- Deferred writes carry a `WriteReason` (refresh/backfill); metadata only, does not gate.

## Key decisions

- Backfill on a full hit only flows through `OnFetchSkipped` (gated by `MustWriteBack`); on a miss the fetch path (`OnFetchResult`) writes all keys + backfills pending from fresh data.
- Per-handle bookkeeping (`configs`, `prefixes`, `renderedMissedKeys`) lives on the `requestCache`, accessed only inside hooks under the `MergeSession`'s `DataBuffer.Lock` -> race-free (verified `-race`).
- `WriteReason` is made observable in tests via an optional white-box `RecordWriteReason` hook on the fake store (the real `Store.Set(key,value,ttl)` signature is unchanged).

## Tests

`controller_multikey_test.go` (+ a couple of `controller_test.go` tweaks), white-box, full-value `assert.Equal`, `synctest` for the TTL/freshness rows:
- E1 all renderable; E2 non-primary hit; E3 lookup-then-backfill-all after response (exact ordered `[Get upc, Set upc refresh, Set sku backfill]`); E4 none renderable; E5 read-hit backfill (no network); E6 refresh/backfill reason tags; E7 single-key degenerate.
- D7 freshest pick; D8 tie / known-beats-unknown; D9 merge-synthesis (+`NeedsWriteback`/`MustWriteBack`/reorder/canonical rewrite); D10 older-candidate fallback; D11 reorder to selection order; D12/D13 AND-reduction.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS.
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- Render every renderable candidate at lookup; re-render the pending ones from fresh data at write; backfill all renderable keys; read key == write key.
- Freshest-covering selection + merge-synthesis + reorder are deterministic (stable tie-break, sorted args).
- Batch shapes, negative, shadow, and L1 remain TODO (A2c/A3/A4/D2).
