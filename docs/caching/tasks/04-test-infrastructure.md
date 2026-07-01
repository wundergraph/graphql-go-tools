# Task 04 — Test infrastructure: dedicated caching subgraphs + e2e framework + fakes

Phase: 0 (Structure).
Dependencies: tasks 02, 03.
References: RFC-1 testing appendix (scenario catalog and doubles, re-based per D7); CODING_GUIDELINES §4; deviation D7 (PLAN §7).

## Problem

Caching needs provably-composable test subgraphs, drift-proof real-planner inputs, and a shared fake set, so the loader glue and the controller reach high coverage without a network or a real backend — and the integration layer must be built ON TOP OF the `execution` package, reusing the existing `federationtesting` approach, with NO golden snapshots.

## Scope

Dedicated caching subgraphs (execution module):

- REUSE the existing `execution/federationtesting` package and COPY its approach; do NOT re-implement execution.
- Create DEDICATED caching subgraphs (not `commerce` alone) with good NESTING and real VARIANCE of TTL and other cache options.
  Minimum shapes the fixture set must express (they drive later tasks' scenarios):
  - an entity with MULTIPLE `@key` sets (multi-key rows, tasks 08/15);
  - cross-subgraph entity references and nested entities (coverage + batch rows, tasks 07/10);
  - by-key root fields whose args map onto an entity `@key` (task 15);
  - sibling root fields with DIFFERENT cache policies on one datasource (task 14);
  - mixed TTLs on sibling fields of one type, so PARTIAL EXPIRY of some fields is expressible (tasks 09/19);
  - a defer shape where an initial entity fetch's `ProvidesData` is a STRICT SUPERSET of a deferred same-entity fetch in a LATER group — this closes the first-pass fixture gap that made cross-defer-group L1 serving unprovable (task 18 rows N1/N2/M3).
- Compose with REAL `wgc` (`compose.sh` running `npx -y wgc@latest router compose`), COMMIT the resulting `config.json`; re-running `compose.sh` is the composability guard.
  Cross-validate composition with rover where the tooling allows.
  Composition is the one network step: run `compose.sh` explicitly when creating or changing fixtures (expect a permission prompt for `npx wgc`), commit the resulting `config.json`, and build/test against the committed artifact.

Harness (execution module):

- A `Plan(tb, query, cachingConfig, responses)`-style helper: load the committed `config.json` (`protojson.Unmarshal` → `nodev1.RouterConfig`), build `plan.Configuration` via `engine.NewFederationEngineConfigFactory(...).BuildEngineConfiguration(&rc)` (add a small EXPORTED `PlannerConfig()` accessor to the execution `engine.Configuration` — refactor in place), run the REAL v2 planner + postprocess with caching configured through the engine `Configuration` (task 03), and swap each fetch's transport for an in-process fake/gated datasource.
- NO `.golden` files (D7): plan-shape assertions are full-value `assert.Equal` on the rendered plan/config where a test needs them; response assertions are COMPLETE response strings.
- e2e tests drive the PUBLIC entry points (`ResolveGraphQLResponse`, `ArenaResolveGraphQLResponse`, `ResolveGraphQLDeferResponse`); seams are asserted OBSERVABLY (the recording fake owns its handle → pointer identity; gated datasource + store ops → no network on a hit; `-race` → lock discipline). No `export_test.go` bridge.

Fakes (v2, cosmo-free): `v2/pkg/engine/cache/cachetesting`:

- `FakeCacheController` (counts `BeginRequest`), `FakeRequestCache` (records normalized `Call`s, returns scripted decisions), `FakeStore` (in-memory L2 with absolute `ExpiresAt`, ordered `StoreOp` log), `GatedDataSource` (gate channels for deterministic ordering), `RecordingObserver`.
- NO custom clock and NO injectable `now`: time/TTL is faked with the Go 1.25 stdlib `testing/synctest` (sleeps advance fake time for TTL ONLY; ordering uses gates + `synctest.Wait()`).
- `RealishCache` (an in-memory controller-backed cache for end-to-end rows) is SCAFFOLDED here only to the extent task 02's contract allows; its real backing grows with tasks 07–17.

Module boundary: the plan-driven tests live in the EXECUTION module (where `wgc`/`config_factory`/cosmo proto are usable); `v2` never takes the cosmo-proto dependency; controller unit tests live in `v2` next to the cache package.

## Tests

This task's deliverable IS test infrastructure; it proves itself with:

- The no-op e2e row: caching unconfigured → response and plan byte-identical to the non-caching baseline (the Phase 0 exit proof, appendix row A1-shaped).
- A smoke row per fixture shape: each dedicated subgraph composes (`compose.sh` clean), plans, and resolves through the harness with canned responses, asserting the COMPLETE response.
- Fake self-tests: `FakeStore` TTL expiry inside a `synctest` bubble; `GatedDataSource` ordering; `FakeRequestCache` records full normalized calls.

## Acceptance criteria

- [ ] `compose.sh` re-composes cleanly; `config.json` committed; the fixture set expresses ALL the shapes listed above (spot-check by planning each representative operation).
- [ ] The harness produces real planner output; no hand-written `plan.Configuration` or fetch-tree literals anywhere.
- [ ] No `.golden` files; no `assert.Contains`/`JSONEq`/fuzzy assertions; no custom clock.
- [ ] `v2` takes no cosmo import; the execution module resolves local `v2` via `go.work` (`cd execution && go test ./...`).
- [ ] Lint-clean in both modules.

## Reviewer guidance

- The fixture-gap closure (initial-superset-over-deferred-entity shape) is the load-bearing addition — verify a defer plan over it actually produces a real defer group whose deferred entity selection is covered by the initial fetch.
- Verify all goroutine-spawning tests keep everything inside the `synctest` bubble and use only in-process fakes (no sockets).
