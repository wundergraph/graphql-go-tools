# Commit A2a — `resolve/cache` controller core (single-candidate L2 entity)

Plan item: `docs/caching/PLAN.md` §3, A2 (first of three parts).
RFC sections: RFC-1 §3 (interfaces), §3.4 (MergeSession lock), §3.6/§3.7 (config + handle), §5/§5.1 (modes + capabilities), §6.4 (off-lock rule); appendix §5.4 (D), §5.6 (F), §5.9 (K).
Phase: A (L2 entities).
OLD algorithm reference: `caching-base` worktree `resolve/caching.go` (key render), `resolve/loader_cache.go` (coverage, write/flush), `resolve/loader.go` (splice).

## Problem

Nothing implemented `RequestCache`, so the entity config stamped by A1 did nothing at runtime.

## Solution

New package `v2/pkg/engine/resolve/cache` (one-way: `resolve` never imports it) implementing the L2 entity controller for the SINGLE-CANDIDATE happy path. Re-expressed against the new loader seams (not a port of the OLD 6-phase pipeline).

- `Store` interface (`Get(key) (value, remainingTTL, ok)`, `Set(key, value, ttl)`) + `Mode` (Noop/L1/L2/L1L2).
- `Controller` implements `resolve.CacheController`; `BeginRequest` returns a per-request `requestCache` owning the deferred-write set.
- `PrepareFetch` (L2): opens ONE `MergeSession`, renders the first candidate key, `Get`s it, parses the cached value onto the session arena, coverage-validates it against `cfg.ProvidesData`, AND-reduces to `DecisionSkipFullHit` (every item covered) or `DecisionFetch`, and writes the per-item `ItemCacheState`. `cfg.ProvidesData == nil` disables the walk -> miss (D14).
- `OnFetchSkipped`: opens a session and splices each covering `FromCache` into its item via `StructuralCopy` + `MergeValues`/`MergeValuesWithPath`.
- `OnFetchResult`: opens a session, applies the write gate `!FetchFailed && !HasErrors && ResponseData != nil && Type() != Null`, then extracts each entity, `StructuralCopy`s it, marshals to HEAP-cloned bytes, and accumulates a `deferredSet{key, bytes, ttl}` (TTL = `cfg.TTL`, or `MutationTTLOverride`).
- `EndRequest`: flushes the deferred `Set`s (bytes only, no lock, no arena) and clears the set.
- Key rendering: `<prefix>:<16-hex xxhash64>` hashing `<prefix>:<{"__typename":..,"key":{..}} JSON>`, numbers coerced to strings, prefix = `CacheName` (+ `:h<headerHash>` when `IncludeSubgraphHeaderPrefix`); pooled `pool.Hash64`.
- Coverage: alias-and-arg-aware (`cacheFieldName` = `OriginalName`-or-`Name` + sorted-`CacheArgs` xxhash suffix, RemapVariables-aware), recursive over Object/Array with nullability rules.
- `cachetesting.RealishCache`: wraps `cache.NewController` over a `storeAdapter` that delegates to `FakeStore` (records `Get`/`Set` ops, computes remaining TTL from `ExpiresAt`).

## Key decisions

- Every hook is a single `MergeSession` (one `DataBuffer.Lock` acquisition); `requestCache.configs`/`deferred` are mutated only inside hooks (under that lock), read in `EndRequest` single-threaded -> race-free (verified under `-race`).
- Per-handle `configs` map carries the `cfg` from `PrepareFetch` to `OnFetchResult` (the handle has no cfg field).
- Key building uses ephemeral heap astjson (nil arena), never polluting the merge arena.
- Scope: single candidate, entity scope, L2 + Noop modes only. TODOs mark the deferrals: multi-candidate render/backfill + freshness/reorder/merge-synthesis (A2b), batch shapes + defer/concurrency (A2c), negative (A3), shadow (A4), L1 (D2).

## Tests

`v2/pkg/engine/resolve/cache/controller_test.go` (white-box `package cache`, constructed astjson + an in-package recording `Store`; `testing/synctest` for TTL): D1 full hit, D2 miss->write->flush, D3/D4/D5 coverage (missing field / nullable rules), D14 implicitly (no ProvidesData -> no serve), F1/F2/F5/F6 write gate, K1/K4 flush, and a TTL-expiry row. Full-value `assert.Equal` on the recorded `[]storeOp`, the returned `Decision`/handle, and the spliced item bytes. Keys recomputed the same way the controller renders them (no magic numbers).

Verification:

- `cd v2 && go build ./pkg/...` — clean (no `resolve`->`cache` cycle).
- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS.
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS (no-op invariant unaffected).
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (unchanged).
- `cd v2 && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- The write gate cannot reduce to `!HasErrors` (F2 keys off `FetchFailed`/`ResponseData==nil`).
- `MergeSession` is the single lock acquisition per hook; the deferred flush holds bytes, not `*Value`.
- Keys derive from the candidate `Representation` + item data; read key == write key.
- Multi-candidate/freshness/reorder/batch/negative/shadow/L1 are intentionally absent (later commits).
