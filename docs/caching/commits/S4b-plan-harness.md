# Commit S4b — execution-module `Plan(...)` harness + commerce supergraph + StageNoop golden + loader-seam rows

Plan item: `docs/caching/PLAN.md` §2, S4 (second of two parts; completes Phase 0).
RFC sections: RFC-1 appendix §1.3 (two layers), §4 (the plan-drift harness), §5.1-§5.3 (loader-seam rows), §9.2 (table shape); CODING_GUIDELINES §4.6 (real-wgc federation fixtures).
Phase: 0 (Structure).

## Problem

The plan-driven loader+cache tests need a PROVABLY-composable, drift-proof plan: a real `wgc`-composed supergraph fed through the execution `config_factory` into the real v2 planner + postprocess, golden-snapshotted so it cannot drift.
This must live in the EXECUTION module (where `config_factory` + the cosmo proto live), keeping v2 cosmo-free.

## Solution

- The `commerce` supergraph as committed testdata at `execution/engine/testdata/cache_commerce/`: `account_sdl.graphql` (`User @key(id)`, `me`, `user(id:)`), `product_sdl.graphql` (`Product @key(upc) @key(sku)`, `topProducts`, `product(upc:)`), `review_sdl.graphql` (`Review`, `latestReviews`, cross-subgraph `User`/`Product`), `graph.yaml`, `compose.sh`, and the committed `config.json`.
  Composed with REAL `wgc` (`compose.sh`) and additionally validated with `rover` — both compose cleanly, so the subgraphs are provably composable and the planner config is provably valid. Re-running `compose.sh` is the composability guard.
- The exported accessor `engine.Configuration.PlannerConfig() plan.Configuration` (refactor-in-place in `engine_config.go`), so the harness can reach the built `plan.Configuration`.
- The `Plan(tb, stage, query, responses)` harness (`execution/engine/cachetesting_plan.go`): read the committed `config.json` -> `BuildEngineConfiguration` -> `PlannerConfig()` -> parse/normalize/validate the operation -> real v2 planner + postprocess WITH `EnableCaching(...)` -> GOLDEN-snapshot the rendered plan (`renderPlanWithCache`: the fetch-tree pretty print PLUS per-fetch `cfg.String()` + `KeySpec` dump) -> swap each fetch's transport for an in-process fake via `cachetesting.SwapDataSources`.
- Loader-seam test rows (`caching_seam_test.go`) driving the real loader through the PUBLIC `ResolveGraphQLResponse`, asserting the FULL `[]cachetesting.Call` and the FULL response bytes:
  - StageNoop golden (byte-identical REAL plan; the no-op drift sentinel).
  - [A1] controller nil -> normal fetched response.
  - [A2] controller set, all `Cache` nil (StageNoop) -> zero `PrepareFetch`, `BeginRequest` not called, baseline response.
  - [C1] miss -> `DecisionFetch` -> `OnFetchResult`: full `[Prepare, Result, End]` call log (pinning the canonical `InputBytes` the loader passes) + the fetched response.

## Key decisions

- `cacheProvidersForStage` is intentionally inert for every stage in this commit (`TODO(A1+)`), because the stamper logic that produces real configs lands in A1; so the meaningful golden here is StageNoop, and the dispatch row uses a SYNTHETIC `injectCache` (a loader-seam driver that sets `.Cache` on a planned fetch) to exercise the loader's decision dispatch independent of the still-inert stamper.
- `federationByDS` keys by `ds.Id()` (the config-factory datasource IDs, matching `FetchInfo.DataSourceID`).
- Uses the PUBLIC `astparser` (not v2-internal `unsafeparser`), since the execution module cannot import v2 internal packages.
- [C3] full-hit skip is DEFERRED to A2: with only a synthetic config and the recording fake there is no real cached splice, so a skipped root fetch leaves the value uncompleted; it lands with the real controller's splice path.

## Tests / verification (from `execution/`)

- `go build ./...` — clean.
- `go test ./engine/ -run 'Caching' -count=1` — PASS (goldens generated once with `-update`, then verified WITHOUT `-update` so the committed golden is authoritative). The committed `.golden` files: `TestCaching_StageNoop_Golden.golden`, the A1/A2 config-gate goldens, and the C1 dispatch golden.
- `go vet ./engine/` — clean.
- `config.json` is used as-is (no `wgc` at test time); re-running `compose.sh` (needs network) reproduces it and is the composability guard.

## Reviewer guidance

- Confirm `compose.sh` re-composes cleanly and `config.json` is committed (the composability guard); the subgraphs were validated with both wgc and rover.
- Confirm the plan comes from the real planner (one source of truth) and the golden shows both plan shape and stamped config; for StageNoop every `Cache` renders `<nil>` (the sentinel A1 will flip).
- Confirm v2 takes no cosmo import (the harness lives in the execution module; v2 only contributes the cosmo-free `cachetesting` fakes from S4a).
- Confirm the seam rows assert FULL values (`[]Call` + response bytes), no `Contains`/`JSONEq`/fuzzy.
- The synthetic `injectCache` is a documented loader-seam driver; real stamped-config rows arrive in A1+.
