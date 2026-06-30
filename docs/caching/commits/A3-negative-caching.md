# Commit A3 — negative caching

Plan item: `docs/caching/PLAN.md` §3, A3.
RFC sections: RFC-1 §3.3 (`EmptyEntity`, the write gate), §3.7 (`NegativeHit`, `FromCache=TypeNull`), §5.1(d); appendix §5.7 (G1-G6).
Phase: A (L2 entities).

## Problem

A successful-but-empty entity fetch should be cacheable as a null sentinel, but a failed fetch must never be cached.

## Solution

Extend the entity L2 controller:

- `OnFetchResult`: return early on `FetchFailed || HasErrors` (failure wins over emptiness). Then the negative branch fires when `EmptyEntity && ResponseData != nil && ResponseData.Type() == Null && NegativeCacheTTL > 0`: write a null sentinel (literal JSON `null` bytes, the v1 sentinel) under each item's keys with `NegativeCacheTTL`, stamping `NegativeHit = true`. The positive branch still requires non-null data, so positive and negative writes are mutually exclusive on data nullness (the F write-gate rows are unaffected).
- `PrepareFetch`: a `Get` hit whose parsed value is null is a NEGATIVE HIT -> `FromCache = null`, `NegativeHit = true`, counted as covered (so the AND-reduction can yield `DecisionSkipFullHit`) without the positive coverage walk.
- `OnFetchSkipped`: a `NegativeHit` item is spliced as null (the entity legitimately does not exist), preserving the loader's existing null-bubble behavior — no synthetic non-null error.

## Key decisions

- `isEmptyEntityFetch` (the loader's `EmptyEntity` signal) is broad — true for any entity fetch returning an `_entities` array — so the negative write is additionally gated on `ResponseData` being JSON null, which is what actually distinguishes a legitimately empty/absent entity from a populated one.
- Null sentinel is a literal `null` for v1 (OLD `negativeCachePositiveValue`'s explicit-null-object shape is a possible future refinement).
- The loader's `setSkipErrors`/`isEmptyEntityFetch` paths are untouched.

## Tests

`controller_negative_test.go` (white-box, full-value `assert.Equal`, `synctest` for expiry): G1 negative write on empty entity (key + null bytes + `NegativeCacheTTL`), G2 skip when TTL==0, G3 negative hit served (entity null, no network), G4 expired sentinel -> miss, G5 `EmptyEntity && FetchFailed` -> no write (FetchFailed wins), G6 null-bubble suppression preserved.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS (G + existing D/E/F/I/K rows).
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- `FetchFailed` (and `HasErrors`) win over `EmptyEntity` — a failed fetch caches nothing.
- Negative and positive writes are mutually exclusive on `ResponseData` nullness.
- A negative hit serves a null entity with no fabricated error.
- Shadow (A4) and L1 (D2) remain TODO; a stale `// TODO(A3/A4)` comment near the write path now covers only shadow.
