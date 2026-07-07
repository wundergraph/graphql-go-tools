# Reviewer notes — task 04: caching subgraphs, e2e harness, cachetesting fakes

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/04-test-infrastructure.md](../tasks/04-test-infrastructure.md).
Spec background: RFC-1 testing appendix, re-based per D7 (no goldens, full-value assertions).

## What this commit adds

The complete caching test infrastructure the later tasks build on: dedicated wgc-composed caching subgraphs (committed `config.json`), a real-planner harness in the execution module, and the cosmo-free `cachetesting` fake set in `v2`.
It proves itself with the Phase 0 no-op e2e row, one smoke row per fixture shape, and fake self-tests.

## Decisions made

- Fixture home: NEW `execution/cachingtesting/` (parallel to `federationtesting`, whose compose approach it copies): `subgraphs/{products,inventory,reviews,users}.graphql` + `graph.yaml` + `compose.sh` + committed `config.json`.
  Fed-v1 `extend type` syntax matches the proven existing fixtures; wgc's `@external`-on-extension-key notes are informational relics, and rover cross-validation also passes (exit 0).
- Fixture shapes (each smoke-tested):
  multi-key `Product @key(upc) @key(sku)`; cross-subgraph nested entities (`users → products → inventory` via `me.favoriteProduct.stock`); by-key root fields `product(upc:)`/`productBySku(sku:)`/`user(id:)`; sibling root fields `products`/`promotions` on one datasource; mixed-TTL sibling fields via Product base data (products DS policy) vs `Product.stock` (inventory DS policy); batch entities via `products { reviews }`.
- The FIXTURE-GAP closure (load-bearing): `TestDeferSupersetShape` plans `me.favoriteProduct { upc stock warehouse { id location } }` + `products { upc ... @defer { stock } }` and pins (full-value) that the defer group holds exactly one inventory entity fetch selecting ONLY `stock` — a strict subset of the initial inventory fetch for the same entity type, making cross-defer-group L1 serving provable (task 18 rows N1/N2/M3).
- Harness wiring: the factory-built datasources are ID-named (`"0"`–`"3"`), so the harness accepts caching config AND response keys by SUBGRAPH NAME and translates via the router config's subgraph list; it wires caching exactly as `NewExecutionEngine` does (providers + federation by datasource ID, `DisableIncludeInfo=false`, `postprocess.EnableCaching`) rather than building a full engine, using the NEW exported `Configuration.PlannerConfig()` accessor.
  The small wiring duplication is deliberate: the engine wiring itself is covered by the task 03 wiring test, and the harness stays a plain planner driver.
- Query plans are ALWAYS included by the harness (`plan.IncludeQueryPlanInResponse()`), so plan-shape assertions are inline full-value `assert.Equal` on rendered fetch trees instead of golden files.
- `Fetch.SetDataSource(DataSource)` added to the `Fetch` interface (implemented on all three concrete types): the first pass swapped datasources with a `switch` over concrete fetch types inside its test util — exactly the D8 pattern this port removes, so the swap is now polymorphic.
- Fakes live in `v2/pkg/engine/cache/cachetesting` (D5 packaging), adapted from the first pass with these deviations:
  the first-pass `CacheStage`/`Mode`/`storeAdapter`/`NewRealishCache` are NOT ported — they depend on the controller (task 07) and would be dead code today (the task file caps RealishCache at "what task 02's contract allows", which is nothing runnable);
  `RecordingObserver` records only lifecycle counts, observed handles, and shadow cache keys (deep shadow-compare recording is task 12 logic);
  `Call` gains the `MergePath` carrier (task 02's D4 addition).
- No custom clock anywhere: `FakeStore` calls real `time.Now`, and the self-tests fake time with `testing/synctest` (TTL via bubble sleeps; ordering via gates + `synctest.Wait()`).

## What was implemented

- Fixtures + composition: `execution/cachingtesting/{subgraphs/*.graphql,graph.yaml,compose.sh,config.json}`; composed with real wgc (`npx -y wgc@latest router compose`), cross-validated with rover.
- Harness: `execution/cachingtesting/cachingtesting.go` — `Plan(tb, query, caching, responses)`, `ResolveResponse` (public sync entry point), `DeferGroups`; config loaded relative to the package file via `runtime.Caller`, so any execution-module package can use it.
- `execution/engine/engine_config.go`: exported `PlannerConfig()` accessor (refactor in place).
- `v2/pkg/engine/resolve/fetch.go`: `SetDataSource` on the `Fetch` interface + three concrete implementations (+ the three test-double stubs).
- Fakes: `v2/pkg/engine/cache/cachetesting/fakes.go` — `FakeCacheController`, `FakeRequestCache` (normalized `Call` log, scripted decisions, error injection, result-handle identity), `RecordingController`, `FakeStore` (absolute `ExpiresAt`, ordered `StoreOp` log, non-logging `Seed`), `GatedDataSource`/`DataSourceGate`, `RecordingObserver`, `FakeRegistry` + `SwapDataSources` + `Compact`.

Tests:

- `execution/cachingtesting/cachingtesting_test.go` — `TestNoOpBaseline` (byte-identical response with a recording controller set and caching unconfigured; zero `BeginRequest`s, zero calls), `TestFixtureSmoke` (six shape rows, complete response bodies), `TestDeferSupersetShape` (full-value rendered defer-group plan).
- `v2/pkg/engine/cache/cachetesting/fakes_test.go` — `FakeStore` TTL expiry + Seed-does-not-log (synctest bubbles, full op-log asserts), `GatedDataSource` gating (arrival observable, blocked until release, via `synctest.Wait`), `FakeRequestCache` full `Call`-log round-trip incl. `MergePath` and handle identity, `FakeRegistry` response-key fallback order + load counting.

## What to look into (review focus)

- Re-run `./compose.sh` in `execution/cachingtesting` (needs `npx`): it must reproduce the committed `config.json` (the composability guard).
- The defer-superset pin: confirm the asserted defer-group plan really is a strict subset of the initial inventory fetch selection for the same entity (`upc stock warehouse{id location}` vs `stock`).
- `SetDataSource` on the `Fetch` interface: confirm the production surface is acceptable for what is primarily a test seam (the alternative was a concrete-type switch in caching code, which D8 forbids).
- `v2` cosmo-freedom: `cachetesting` imports only `resolve`, `astjson`, `httpclient`; the cosmo proto stays in the execution module.
- Goroutine discipline: all goroutine-spawning fake tests run inside `synctest.Test` bodies with in-process fakes only.

## Verification evidence

- wgc composition clean (config.json written); rover composition exit 0.
- All new tests pass: `execution/cachingtesting` (no-op baseline, 6 smoke rows, defer superset), `v2 cachetesting` (5 self-tests).
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
