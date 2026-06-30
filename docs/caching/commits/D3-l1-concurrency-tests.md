# Commit D3 — defer + L1 concurrency execution rows (M1, M2)

Plan item: `docs/caching/PLAN.md` §6, D2 (the deferred test rows).
RFC sections: appendix §6 (M1-M5), §7 (N1-N3).
Phase: D (L1 caching). Test-only.

## Problem

D2 implemented the request-lifetime shared L1 path but deferred the deterministic, gate-ordered defer+L1 concurrency PROOFS (it refused to fake determinism with latency). This commit adds the ones expressible with the `commerce` plan shapes.

## Solution (test-only)

Through the PUBLIC `ResolveGraphQLDeferResponse` inside a `synctest` bubble, under `-race`, ordered with GATES + `synctest.Wait()` (never latency):

- **M1** lazy `BeginRequest` once: parallel eligible entity fetches in one request -> exactly ONE `BeginRequest` (counted via a `beginCountingCacheController` wrapping `RealishCache`); full response frame, load counts, empty L2 ops.
- **M2** parallel writes to the shared L1: two parallel same-type entity fetches gated until BOTH arrive, then released together; full sorted `store.Ops()` + load counts + frames under `ModeL1L2`, `-race` clean.

Support: `cachetesting` gains per-fetch `DataSourceGate` (Arrived/Release) wiring on the registry (`SetGate`) and load counters; a test-local `forceEntityL1` sets `cfg.L1=true` on the planned entity fetches (the optimizer correctly narrows sibling-parallel fetches' L1 off, since they have no ordered provider/consumer relation, so the test re-enables it to exercise the parallel L1-write path).

## Could NOT be expressed (documented gap, NOT faked)

N1 (entity cached by the initial fetch served to a deferred fetch), N2 (deferred fetch populates L1 visible to a later group), and M3 (cross-defer-group L1 share) could NOT be expressed with the current `commerce` defer plan shapes without weakening the proof or changing production behavior:

- a duplicate initial/deferred selection of the same entity NORMALIZES to a synchronous plan (no public defer group);
- a nested defer shape DOES produce a real defer group, but the initial cached entity selection does not COVER the deferred selection, so `optimizeL1Cache` correctly narrows `l1:false` (no provider/consumer pair) — the deferred fetch legitimately would not hit L1.

These rows require a richer supergraph (an initial fetch whose `ProvidesData` is a superset of a deferred same-entity fetch across groups) — future fixture work. The L1 sharing they would prove is STRUCTURALLY guaranteed (the `requestCache` on the by-reference `Context`, exercised by M1/M2) and `-race`-clean. No determinism was faked; no assertion was weakened; no planner/runtime behavior was forced.

## Verification

- `cd execution && go test ./engine/ -run 'Caching' -count=1 -race` — PASS (M1/M2 + all existing rows).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/cachetesting/...` — clean.
- Existing goldens unchanged.

## Reviewer guidance

- M1/M2 order with gates + `synctest.Wait()`, never latency; full-value asserts; `-race` clean.
- The N1/N2/M3 gap is a fixture/plan-shape limitation, honestly documented — not an implementation gap; the shared L1 is structurally guaranteed and race-clean.
