# Task 08 — Multi-key candidates: render, freshness, reorder, backfill

Phase: A (L2 entities).
Dependencies: task 07.
References: RFC-1 §3.6–3.7, §5.1; RFC-2 §6.3; appendix rows D7–D13, E1–E7.

## Problem

An entity may declare multiple `@key` sets.
Lookup and write must treat every frozen candidate as independently, best-effort renderable — render what can be rendered now, look up under ALL rendered keys, and backfill the rest from fresh data — with multi-candidate freshness selection and reorder-to-selection-order when several cached values compete.

## Scope

Extends the task 07 controller; all inside the existing hook/transaction structure.

- `PrepareFetch`: render EVERY renderable candidate (renderable → `ItemCacheState.RenderedKeys`; not renderable → `PendingCandidates`); `Get` under all rendered keys (a hit on ANY serves); collect `FromCacheCandidates` freshest-first; multi-candidate freshness selection (freshest wins; known TTL beats unknown; merge-synthesis when no single value covers but a union does → `NeedsWriteback`; older-single fallback); reorder the chosen value to selection order before it becomes `FromCache`.
- `OnFetchResult`: re-render `PendingCandidates` from the fresh response data; write ALL renderable keys; tag `WriteReason` refresh vs backfill (metadata only, never gates).
- `OnFetchSkipped`: on a hit that leaves other candidates renderable from the served value, emit best-effort backfill writes (`MustWriteBack`), no network.
- Candidate values parse lazily via `tx.ParseBytes`, once each.

## Tests

Controller unit tests (constructed astjson, `synctest` for TTL variance):

- D freshness rows D7–D13: freshest pick, tie/known-beats-unknown, merge-synthesis (`NeedsWriteback`/`MustWriteBack`), older-single fallback, reorder-to-selection-order, AND-reduction all-covered vs one-uncovered.
- E multi-key rows E1–E7: all renderable; hit on a NON-primary key; backfill-all-after-response (exact ordered store ops: `[Get k0, Set k0 refresh, Set k1 backfill]` with exact bytes + TTL); none renderable → no Get, write post-fetch; read-hit backfill via `OnFetchSkipped`; refresh-vs-backfill tags; single-`@key` degenerate (one-element list).

Plan-driven e2e rows (multi-key entity fixture from task 04):

- Prime under key A, serve a request that renders only key B (cross-key hit), assert the COMPLETE response and the full ordered store-op log.

## Acceptance criteria

- [ ] Every E/D row asserts the EXACT ordered `[]StoreOp` — a missing/extra Get or Set, wrong key, wrong bytes, wrong reason, or wrong TTL fails.
- [ ] No candidate is ever "required": unrenderable candidates skip silently at lookup and retry at write (each intentional skip commented WHY).
- [ ] Read key == write key per candidate (one `CacheKeyTemplate` each).
- [ ] Lint-clean; no helper duplicated from task 07.

## Reviewer guidance

- The freshness/merge ladder is the subtlest logic in the controller — insist on adversarial rows beyond the OLD test set (partial-overlap unions, empty union, all-stale candidates).
