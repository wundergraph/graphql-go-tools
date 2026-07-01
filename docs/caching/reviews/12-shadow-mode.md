# Reviewer notes — task 12: shadow mode (entity compare)

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/12-shadow-mode.md](../tasks/12-shadow-mode.md).
Spec background: RFC-1 §3.2, §3.5, §5.1(b); appendix rows H1–H4, H6, H7.

## What this commit adds

Shadow mode: a shadow-configured fetch READS L2 but never serves it — the read is stashed on the handle, the loader force-fetches (`DecisionFetchShadow` is byte-identical to `DecisionFetch` on the loader side; no loader change in this commit), and `CacheObserver.CompareShadow` runs the staleness probe BEFORE the writes, inside the hook's already-open transaction.

## Decisions made

- The stash happens AFTER the normal lookup/selection ladder: `shadowStashEntry` moves the would-be-served value (the exact selection, including the negative sentinel case with its `NegativeCacheTTL`) into `ShadowStash[i]` and CLEARS the serving fields, so no code path downstream can serve it.
  `NeedsWriteback` is cleared too — under shadow, `OnFetchResult` refreshes every rendered key from the FRESH value anyway.
- `resolve.ShadowCacheEntry` gained `CacheTTL` (the task-02 decision log reserved this: "tasks 10/12 add them only if actually needed") — the observer derives the entry's age (`CacheTTL − RemainingTTL`) without re-deriving config.
- Decision precedence: any stash → `DecisionFetchShadow`, else all-covered → `SkipFullHit`; `WasHit` stays false under shadow (nothing served); both the single and the batch arm stash identically.
- No mode enum grew (reviewer guidance): shadow rides `cfg.ShadowMode` on the existing L2 path; L2-off shadow never reaches the controller (H7).
- A shadow MISS is byte-identical to a plain miss (fetch + write), and `CompareShadow` fires only when `h.Shadow && obs != nil` — a nil observer force-fetches and records nothing (H6).
- `cachetesting.RecordingObserver` now materializes compares (`ShadowCompare{CacheKey, IsFresh, CacheAge}`, byte-equality of stashed vs fresh, batch-aware via `BatchIndex`), replacing the never-consumed `ShadowKeys`; production observer wiring stays with ART (task 20).

## What was implemented

- `controller.go` — `shadowStashEntry`; stash collection + `DecisionFetchShadow` in both prepare arms; the `CompareShadow` call in `OnFetchResult` before any write.
- `resolve/cache_controller.go` — `ShadowCacheEntry.CacheTTL`.
- `cachetesting/fakes.go` — the compare-materializing `RecordingObserver` (+ `ShadowCompare`).

Tests:

- `controller_shadow_test.go` — H1 (stash carries the exact key + cached bytes; nothing servable; the store log shows the read; force-fetch implied by `DecisionFetchShadow`), H2 (synctest-aged entry: compare records the EXACT 20s age, runs before the flush, L2 overwritten after), H3 (mismatch: `IsFresh=false`, fresh value wins the store), H6 (nil observer), H7 (L2-off never shadows), and the shadow-miss row.
  H4 (L1-hit-wins) is deferred to task 17 as the task file specifies — until L1 exists the row is L2-scoped and covered by H1.
- `shadow_e2e_test.go` — three requests over real plans: miss (no compare), hit-with-changed-data (response shows the FRESH value, compare `IsFresh=false`, L2 overwritten — pinned bytes), hit-with-unchanged-data (`IsFresh=true`); inventory loads on EVERY request (never served from cache); real-clock `CacheAge` normalized to zero with the exact age pinned by the synctest H2 unit row.

## What to look into (review focus)

- The loader has NO new branch for `DecisionFetchShadow` (it falls into the `default:` dispatch to `OnFetchResult` from task 02) — grep `FetchShadow` in `loader.go` to confirm nothing grew.
- The compare runs inside the hook's existing transaction (no second `Begin`); confirm `CompareShadow` sits after `tx := in.Arena.Begin()` and before the write loop.
- The stash copies via `tx.StructuralCopy` — the stashed value must not alias the parse buffer the selection used.
- The negative-sentinel-under-shadow path: a stashed sentinel carries `CacheTTL = NegativeCacheTTL`; nothing serves and nothing special-cases it downstream.

## Verification evidence

- All H rows and the e2e row pass; `-race` clean over `engine/cache`, `cachetesting`, `resolve`, and the execution harness tests.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
