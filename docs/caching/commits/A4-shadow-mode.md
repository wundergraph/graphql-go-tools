# Commit A4 — shadow mode (entity compare)

Plan item: `docs/caching/PLAN.md` §3, A4 (completes Phase A).
RFC sections: RFC-1 §3.2 (`DecisionFetchShadow`), §3.5 (`CompareShadow`, compare->write-L1->write-L2, entity-only), §3.7 (`Shadow`/`ShadowStash`/`ShadowCacheEntry`), §5/§5.1(b); appendix §5.8 (H1-H4,H6,H7), §2.8 (`RecordingObserver`).
Phase: A (L2 entities).

## Problem

Shadow mode must read L2 but never serve it: force a real fetch, serve FRESH, then compare cached vs fresh — without leaking onto the loader surface.

## Solution

- `PrepareFetch`: when `cfg.ShadowMode` and a covering L2 value is found, do NOT set `FromCache` (so the loader fetches normally; `skipLoad` stays false); stash the value into `handle.ShadowStash[i] = ShadowCacheEntry{CachedValue, CacheKey, RemainingTTL, CacheTTL}`, set `handle.Shadow = true`, and return `DecisionFetchShadow`. Shadow is L2-ONLY — NoOp/L1 modes never return `DecisionFetchShadow` (H7). A shadow read that misses behaves like a normal miss.
- `OnFetchResult`: when `h.Shadow && obs != nil && cfg.ProvidesData != nil && cfg.KeySpec.Scope == CacheScopeEntity`, call `CompareShadow(h, fresh, session)` BEFORE the write-back (preserving compare -> write order), then overwrite L2 with the fresh value as a normal fetch. With a nil observer, the compare is skipped but the fetch still ran and L2 is still overwritten with fresh (H6).
- `CompareShadow` runs inside `OnFetchResult`'s open `MergeSession` (it does not open its own).
- `RecordingObserver.CompareShadow` reads the stashed entry, deep-compares cached vs fresh JSON, and records `EntityType` + `IsFresh` + `CacheAge` (`CacheTTL - RemainingTTL`, exact under the `synctest` fake clock).

## Key decisions

- `ShadowCacheEntry.CacheTTL` added (additive contract field): `CompareShadow` does not receive the fetch config, so `RemainingTTL` alone cannot yield an absolute `CacheAge`; carrying the original TTL lets the observer compute it exactly.
- Shadow is a cross-cutting L2 variant, not a fifth mode; entity-only compare (root-field force-refetch asymmetry is B2/H5).

## Tests

`controller_shadow_test.go` (white-box + `RecordingObserver`, full-value `assert.Equal`, `synctest` for CacheAge): H1 stash + force-fetch (FromCache nil, `DecisionFetchShadow`, fresh response), H2 compare MATCH before write (recorded `CacheAge`, L2 overwritten), H3 compare MISMATCH still overwrites, H6 nil observer / nil ProvidesData force-fetch without compare, H7 NoOp/L1 never yield `DecisionFetchShadow`.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS (H + existing rows).
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- Shadow always force-fetches and serves FRESH (the cache value is never served).
- Compare runs BEFORE the writes; nil observer records nothing but still overwrites L2.
- Root-field shadow asymmetry (H5) and L1 (H4) are out of scope (B2 / D2).
