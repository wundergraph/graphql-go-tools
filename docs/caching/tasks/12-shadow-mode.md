# Task 12 — Shadow mode (entity compare)

Phase: A (L2 entities).
Dependencies: task 07.
References: RFC-1 §3.2, §3.5, §5.1(b); appendix rows H1–H4, H6, H7.

## Problem

Shadow mode must READ L2 but never SERVE it: force a real fetch, then compare cached vs fresh — a staleness probe that is byte-identical to a miss on the loader side and leaks nothing onto the loader surface.

## Scope

- `PrepareFetch`: on `cfg.ShadowMode` + an L2 hit, stash the cached value into `handle.ShadowStash` and return `DecisionFetchShadow` (`skipLoad` stays false; the loader treats it exactly like `DecisionFetch`).
- `OnFetchResult`: when `handle.Shadow`, run `CacheObserver.CompareShadow` BEFORE the writes (compare → write-L1 → write-L2 order preserved), inside the hook's already-open transaction; entity fetches only (the root-field asymmetry lands in task 13).
- A nil observer force-fetches and records nothing; NO-OP and L1-only modes never yield `DecisionFetchShadow`.
- `RecordingObserver` (task 04) is the test double; production observer wiring arrives with ART (task 20).

## Tests

Controller unit tests (`synctest` where an entry is aged for `CacheAge`):

- H1 read + stash + force-fetch (fresh response served; store shows the Get; network ran).
- H2 compare MATCH (recorded `CacheAge` equals the slept duration; compare precedes writes; L2 overwritten).
- H3 compare MISMATCH (`IsFresh = false`; L2 overwritten with fresh).
- H4 L1-hit-wins (once task 17 lands, re-run: an L1 hit serves and no shadow compare fires; until then the row is L2-scoped).
- H6 nil observer: force-fetch, nothing recorded.
- H7 mode matrix: NO-OP / L1-only never produce `DecisionFetchShadow`.

Plan-driven e2e row: shadow-configured entity — response is ALWAYS the fresh network value even on an L2 hit; the full observer record asserted.

## Acceptance criteria

- [ ] Shadow always force-fetches and serves FRESH data (never the cached value).
- [ ] Compare → write-L1 → write-L2 order proven.
- [ ] Loader-side behavior on `DecisionFetchShadow` is byte-identical to `DecisionFetch` (no new loader branch).
- [ ] Lint-clean.

## Reviewer guidance

- Shadow is a cross-cutting L2 variant, not a fifth mode — confirm no mode enum grew.
- The compare must run inside the existing transaction (no second lock acquisition).
