# Task 20 — ART observability (verbose caching trace)

Phase: E.
Dependencies: tasks 12, 17.
References: maintainer feedback R2.9 (graduates the observer from v2-staged to core); RFC-1 §3.5 (`CacheObserver`).

## Problem

Operators need to SEE caching behave: the playground (via ART — Advanced Request Tracing) must surface every caching aspect per fetch — L1 vs L2, hits/misses, shadow comparisons, backfills, key derivations, and remaining TTLs — without the observability concern leaking back onto the lookup/write surface (the coupling that sank the OLD implementation).

## Scope

- A production `CacheObserver` implementation in `engine/cache` that accumulates per-fetch cache trace from the `FetchCacheHandle` (`OnFetchObserved`) and the shadow compares (`CompareShadow`), keyed per fetch.
- Wire the accumulated data into ART's existing per-fetch trace output (extend the fetch trace shape additively): decision, per-layer hit/miss, rendered keys (respecting `HashAnalyticsKeys` — hash key material when set), candidate freshness / `SelectedRemainingTTL`, write reasons (refresh/backfill), negative hits, shadow compare results with `CacheAge`.
- The observer stays composed INSIDE the controller (the loader never calls it); a nil observer remains zero-cost and records nothing.
- Trace assembly happens at `EndRequest`/finalize time — single-threaded, no lock, no arena; accumulation during hooks rides the existing transaction.
- Scope guard: this is TRACE output; metrics/analytics export pipelines and the walker-inlined per-field hooks (`OnEntity`/`OnFieldValue`) stay follow-ups.

## Tests

- Observer unit tests: full accumulated trace asserted (`assert.Equal` on the complete structure) for hit, miss, backfill, negative, and shadow scenarios; `HashAnalyticsKeys` on/off both asserted; nil observer records nothing and costs no allocations on the hook path (benchmark-guard or allocation assert where the repo has precedent).
- Plan-driven e2e rows: run representative scenarios (L2 hit, L1 hit, shadow compare, partial fetch) with ART enabled and assert the COMPLETE cache section of the trace output; `synctest` pins remaining-TTL values so they assert exactly.
- Regression: with ART disabled and observer nil, response bytes are byte-identical to the pre-task suite (the no-op gate extended to observability).

## Acceptance criteria

- [ ] Every caching aspect listed above appears in ART output and is asserted exactly (no fuzzy TTL assertions — `synctest` makes them deterministic).
- [ ] Nil observer: zero cost, zero output, byte-identical responses.
- [ ] No observability code on the loader surface; the controller composes the observer.
- [ ] Lint-clean in both modules.

## Reviewer guidance

- This concern caused the OLD implementation's worst coupling (~461 call sites); reject any change that adds observer calls outside the controller/handle boundary.
