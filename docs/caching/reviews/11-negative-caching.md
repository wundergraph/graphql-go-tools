# Reviewer notes — task 11: negative caching

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/11-negative-caching.md](../tasks/11-negative-caching.md).
Spec background: RFC-1 §3.3, §5.1(d); appendix rows G1–G6.

## What this commit adds

The negative-cache path: a SUCCESSFUL-but-empty entity fetch writes a null sentinel under the item's keys with `NegativeCacheTTL`, and a sentinel hit serves as a full hit with zero network — while every failure signal keeps blocking ALL writes, negative ones included.

## Decisions made

- Write condition (in `OnFetchResult`, after the failure gate): `EmptyEntity && ResponseData != nil && TypeNull && NegativeCacheTTL > 0`.
  `FetchFailed`/`HasErrors` return BEFORE this branch, so a failure can never masquerade as nonexistence (G5, all combinations).
  Note the task-02 loader computes `EmptyEntity` unconditionally from the parsed response shape (any `_entities` array), so the `ResponseData TypeNull` conjunct is load-bearing — `EmptyEntity` alone does NOT mean "empty".
- Sentinel semantics at read time: a TOP-LEVEL `TypeNull` cached value routes as a negative hit (before, and instead of, the coverage walk — there is nothing to cover); a positive value with a null FIELD lives inside an object and can never be mistaken for it (G6 unit row).
  The freshest sentinel wins among candidates.
- SPLICE BEHAVIOR DELIBERATELY DIFFERS FROM THE FIRST PASS: a negative hit splices NOTHING.
  The first pass replaced the merge target with null; the e2e row exposed that this makes the cached response DIFFER from the uncached one (the real empty fetch leaves the target unmerged, and the resolvable renders a null bubble WITH its non-null error; the null-replacement rendered a clean null WITHOUT the error).
  Caching must never change the response, so the controller now reproduces the uncached shape byte-identically — the e2e row pins `firstBody == secondBody` including the error entry.
- `NegativeCacheTTL == 0` disables the path entirely (G2); the sentinel TTL is its own knob, independent of `cfg.TTL`.
- The sentinel write branch opens its own transaction — `deferSet` mutates request-shared state and must run under the lock like every other hook body.

## What was implemented

- `controller.go` — the `negativeCacheSentinel` const (with the distinguishability rationale), the sentinel-routing block in `prepareItemState`, the no-splice negative branch in `OnFetchSkipped`, and the negative-write branch in `OnFetchResult`.

Tests:

- `controller_negative_test.go` — G1 exact sentinel bytes + `NegativeCacheTTL` in the op log; G2 zero writes at TTL 0; G3 sentinel hit (SkipFullHit, `NegativeHit`, `FromCache` TypeNull, target UNTOUCHED by the splice); G4 expiry inside a synctest bubble (network runs again); G5 failure-signal matrix (FetchFailed / HasErrors / both) with zero writes; G6 positive-null-field row (normal hit, never the negative path).
- `negative_e2e_test.go` — the nonexistent-entity row over real plans: request 1 fetches `_entities:[null]` and writes the sentinel (exact op log with the negative TTL); request 2 zero inventory loads and a BYTE-IDENTICAL response (null bubble + the same non-null error, pinned in full).

## What to look into (review focus)

- The splice-nothing decision (the deviation from the first pass): confirm you agree that reproducing the uncached response (including its error entry) beats the "cleaner" null replacement; the e2e pin makes the tradeoff explicit.
- Gate ordering in `OnFetchResult`: failure gate → negative branch → null-data gate — verify no ordering lets a failed fetch write a sentinel.
- The sentinel routing happens before candidate selection and skips the coverage walk — confirm a sentinel candidate cannot leak into merge synthesis.

## Verification evidence

- All G rows and the e2e row pass; `-race` clean over `engine/cache`, `cachetesting`, and the execution harness tests.
- Full `v2` (43 pkgs) and `execution` (6 pkgs) suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
