# Commit S4a — the v2 `cachetesting` fakes package

Plan item: `docs/caching/PLAN.md` §2, S4 (first of two parts).
RFC sections: RFC-1 appendix §1.3 (two test layers / module boundary), §2 (the test doubles), §2.5 (`testing/synctest`), §3.3 (`CacheStage`), §9.3-§9.4 (assertion conventions).
Phase: 0 (Structure).

## Problem

Caching needs a shared, cosmo-FREE set of in-process fakes so the loader+cache glue and (later) the controller can be driven to ~100% coverage without a network or a real backend.
These fakes must live in ONE v2 support package so both test layers reuse them, and they must not drag the cosmo-proto dependency into v2.

## Why S4 is split

S4 is delivered in two commits.
This commit (S4a) is the v2-only fakes package — self-contained, no plans, no cosmo, no network.
The execution-module `Plan(...)` harness + the `commerce` supergraph + the plan-driven loader-seam rows are the follow-up commit (S4b), because they require real `wgc` composition and the cosmo proto, which live in the execution module.
`RealishCache` and the real `resolve/cache` controller are deferred further, to A2, because they need the controller's store interface, which does not exist yet.

## Solution

New non-`_test` support package `v2/pkg/engine/resolve/cache/cachetesting` (imports `resolve`, `astjson`, `httpclient`, stdlib only — NO cosmo, NO `plan`, NO `postprocess`):

- `CacheStage` enum (StageNoop, StageL2Entities, StageL2RootFields, StageL2RootReusesEntity, StageL1).
- Normalized, fully-assertable record types: `Call`, `ScriptedDecision`, `StoreOp`, `ShadowCompare`.
- `FakeCacheController` (counts `BeginRequest`, returns a fixed `RequestCache`).
- `FakeRequestCache` — the recording/scripted loader-seam driver: records every `PrepareFetch`/`OnFetchSkipped`/`OnFetchResult`/`EndRequest` with full normalized inputs (race-safe via a mutex), returns scripted `(Decision, *FetchCacheHandle)`, supports per-`(path,op)` error injection, and tracks the handle pointer reaching `OnFetchResult` so a test can prove prepare->merge handle identity.
- `RecordingController` / `NewRecordingCache(script)` — a `CacheController` whose `BeginRequest` always returns the same recorder, exposing `Calls()`/`Begins()`.
- `FakeStore` — an in-memory L2 backend with absolute-`ExpiresAt` TTL driven by REAL `time.Now` (so `testing/synctest` fakes it with no clock seam): `Seed` (no op recorded), `Set`/`Get` (record `StoreOp`s), expiry => miss.
- `GatedDataSource` — an in-process `resolve.DataSource` that announces arrival and blocks `Load` until released (deterministic ordering by gates, not latency).
- `RecordingObserver` — a `resolve.CacheObserver` recording `CompareShadow`.
- `FakeRegistry` + `SwapDataSources` — swap a planned fetch tree's transports for in-process fakes (used by S4b).
- `Compact` — canonical JSON normalization via `astjson` parse+marshal (never `assert.JSONEq`).

## Key decisions

- No custom clock / no injectable `now`: `FakeStore` calls real `time.Now`; tests fake it with `testing/synctest` (CODING_GUIDELINES §2, §4.5).
- `FakeRegistry` keys canned responses by `DataSourceName + ":" + ResponsePath`, with fallbacks to `DataSourceName`, `ResponsePath`, then `"*"`.
- `httpclient` is imported solely to match the current `resolve.DataSource.LoadWithFiles(... []*httpclient.FileUpload)` signature.

## Tests

`fakes_test.go` (run under `-race`):
- `FakeRequestCache` records a scripted `Prepare`+`Result` and `Calls()` returns the exact `[]Call`; the scripted handle returned from `PrepareFetch` is the same pointer passed to `OnFetchResult` (identity assertion).
- `FakeStore` TTL inside a `synctest.Test` bubble: seed with a 30s TTL, assert a hit; `time.Sleep(31s)` + `synctest.Wait()`; assert a miss; assert the full `[]StoreOp`.
- `GatedDataSource` blocks `Load` until `Release` is closed, then returns the canned bytes.

Verification (from `v2/`):

- `go build ./pkg/engine/resolve/cache/cachetesting/...` — clean.
- `go test ./pkg/engine/resolve/cache/cachetesting/... -count=1 -race` — PASS (3 focused tests).
- `go vet ./pkg/engine/resolve/cache/cachetesting/...` — clean.
- `go list -deps` confirms NO cosmo proto / `plan` / `postprocess` imports.

## Reviewer guidance

- Confirm the package takes no cosmo/plan/postprocess dependency (so v2 stays cosmo-free).
- Confirm time/TTL uses real `time` + `testing/synctest`, with no custom clock.
- Confirm the records are normalized to bytes/strings so a whole `[]Call`/`[]StoreOp` asserts with one `assert.Equal`.
- `RealishCache`/the real controller are intentionally absent (deferred to A2); the execution `Plan(...)` harness is S4b.
