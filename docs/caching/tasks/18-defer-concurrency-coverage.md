# Task 18 — Defer + concurrency scenario coverage

Phase: D (L1).
Dependencies: task 17 (uses the task 04 fixture set, which was designed to close the first-pass fixture gap).
References: appendix §6–§7 (rows M3–M5, N1–N5); CODING_GUIDELINES §4.5 (synctest), the gates-not-latency rule.

## Problem

The defer × caching interaction is where the first pass could not complete its proofs: the old `commerce` fixture could not express an initial fetch whose L1 entry is reused by a same-entity deferred fetch in a later group, so cross-defer-group L1 SERVING was never proven end to end.
The task 04 fixture set includes the required shape (initial `ProvidesData` a strict superset of a deferred same-entity fetch); this task lands the proofs.

## Scope

Test-only task (plus any small fixes it flushes out); all rows run in the execution module through `ResolveGraphQLDeferResponse`, inside `synctest` bubbles, ordered with gate channels + `synctest.Wait()` (sleeps advance fake time for TTL ONLY), under `-race`.

- N1: entity cached by the initial fetch SERVED to a deferred fetch — the deferred group's subgraph is never hit (gated datasource proves it); both frames asserted as COMPLETE responses.
- N2: a deferred fetch populates L1 visible to a LATER group.
- M3: per-defer-group loaders share ONE L1 via the by-reference `Context`.
- N3: exactly ONE `EndRequest` after ALL groups; the flush carries deferred L2 writes from the initial fetch AND every group.
- N4: a `SkipFullHit` on one branch does not reorder defer frames.
- N5/M4: a deferred fetch's lookup/merge happens inside its group's transaction; each hook is a single lock acquisition; no cross-group arena race.
- M5: one parallel fetch's hook error propagates out of the group; siblings unaffected.
- If `optimizeL1Cache` narrows L1 off for a shape a row needs, the row's fixture (not the pass) is wrong — the fixture must present a genuine provider/consumer pair; fix the fixture.

## Acceptance criteria

- [ ] N1/N2/M3 pass — the first-pass carry-forward gap is CLOSED (cross-defer-group L1 serving proven end to end).
- [ ] N3–N5, M4–M5 pass under `-race`.
- [ ] No latency-based ordering anywhere (gates + `synctest.Wait()` only).
- [ ] All frame assertions are COMPLETE response strings.

## Reviewer guidance

- Verify the N1 plan actually contains a real defer group whose entity selection is covered by the initial fetch (inspect the plan in the test, not just the frames) — this is exactly where the first pass was silently unable to test what it claimed.
