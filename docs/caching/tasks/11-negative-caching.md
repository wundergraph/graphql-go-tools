# Task 11 — Negative caching

Phase: A (L2 entities).
Dependencies: task 07.
References: RFC-1 §3.3, §5.1(d); appendix rows G1–G6.

## Problem

A SUCCESSFUL-but-empty entity fetch should be cacheable as a null sentinel (so repeated lookups for a nonexistent entity skip the network), but a FAILED fetch must never be cached — confusing the two persists transient errors.

## Scope

- `OnFetchResult`: when `EmptyEntity && !FetchFailed && !HasErrors && cfg.NegativeCacheTTL > 0`, write a null sentinel under the item's keys with `NegativeCacheTTL`; stamp `ItemCacheState.NegativeHit` / `FromCache = TypeNull`.
  `EmptyEntity` is the ONE non-failure that still writes; all failure signals block negative writes too (`FetchFailed` wins over `EmptyEntity`).
- `PrepareFetch`: a null-sentinel hit serves as `DecisionSkipFullHit` with `FromCache = TypeNull`; merge skipped; the loader's null-bubble / `setSkipErrors` behavior untouched.
- Negative TTL is its own knob; `NegativeCacheTTL == 0` disables the path entirely.

## Tests

Controller unit tests (`synctest` for TTL):

- G1 negative write on empty entity (exact sentinel bytes + `NegativeCacheTTL` in the store op).
- G2 skipped when `NegativeCacheTTL == 0` (zero writes).
- G3 negative hit served (null entity, zero network, `NegativeHit` recorded).
- G4 expiry: sleep past `NegativeCacheTTL` inside the bubble → miss → network runs.
- G5 `EmptyEntity && FetchFailed` → gate blocks, NO negative write.
- G6 null-bubble suppression preserved (no synthetic non-null error introduced by a negative hit).

Plan-driven e2e row: a by-key entity query for a nonexistent entity — request 1 fetches empty and writes the sentinel; request 2 serves null with zero network; COMPLETE responses asserted.

## Acceptance criteria

- [ ] All G rows pass with full-value assertions.
- [ ] `FetchFailed`/`HasErrors` block negative writes in every combination.
- [ ] Expiry proven via `testing/synctest` (no custom clock).
- [ ] Lint-clean.

## Reviewer guidance

- The sentinel must be distinguishable from "no entry" AND from a positive null-field value at read time; verify the read path routes on the sentinel, not on JSON null alone.
