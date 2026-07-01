# Reviewer notes — task 03: planner wiring, cacheconfig package, engine SetCaching

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/03-planner-wiring-and-engine-config.md](../tasks/03-planner-wiring-and-engine-config.md).
Spec background: RFC-2 §5, §7.2–7.3, §11; deviations D3, D5, D6, D9 (PLAN §7).

## What this commit adds

The complete PLAN-side caching wiring, all gated and inert: the policy model, the postprocess pass pipeline (skeletons), the P1 second-walk registration seam, the per-field metadata carriers, and the public engine entry point `Configuration.SetCaching`.
With no `SetCaching`, plans are byte-identical to before (proven by a determinism test and the full existing suites).
Tasks 05/06/13/16 fill the pass and visitor bodies without touching the wiring again.

## Decisions made (and deviations applied)

- D6 names throughout: `cacheKeyBuilder`, `fetchCacheConfigurator`, `optimizeL1Cache`, and the `ConfigureCaching` entry point.
  The facade is a struct named `cache.Configurator` whose METHOD is `ConfigureCaching(response, trees...)` — a verb-named struct read awkwardly; the method carries the D6 name.
- D5 packaging: the pass pipeline lives in the new common `v2/pkg/engine/cache` package; `postprocess` holds only the option plumbing and three one-line `ConfigureCaching` calls.
- D3: the `cacheconfig` policy structs carry NO L1/L2 bools; L2 is derived from TTLs by later tasks.
- Provider shape: the first pass carried `KeySpecs []resolve.CacheKeySpec` + a `KeySpec(...)` provider method as an external key input; both are DROPPED per D10 (keys are structurally derived by `cacheKeyBuilder` from federation `@key` at plan time, the provider supplies only policies).
  This also makes `cacheconfig` a true leaf: it imports only `time`.
- `CachingConfiguration` itself implements `CacheConfigProvider` (linear-scan lookups; policy sets are small and the lookup runs at plan time), so `SetCaching` needs no adapter type.
- ENGINE ENTRY SHAPE (task file asked to settle and record): `SetCaching(map[string]cacheconfig.CachingConfiguration)` keyed by DATASOURCE ID — the same ID `NewDataSourceConfiguration` receives and the same key the runtime uses (`FetchInfo.DataSourceID`), where `SetFieldConfigurations` keys by type/field inside its slice.
  `NewExecutionEngine` validates every configured ID against the registered datasources and fails fast on an unknown ID (a typo must not silently disable caching).
- Hard wiring precondition applied: configuring caching force-sets `plannerConfig.DisableIncludeInfo = false` (providers are matched to fetches via `FetchInfo.DataSourceID`; without info, caching would silently no-op).
- D9 seam: the P1 visitor registers ONLY on the second walk (`ResetVisitors` + `SetVisitorFilter(nil)` + register P1 + `Walk` + `attachTo`), after the cost-calculator step.
  The first pass ALSO registered P1 on the main walk; that was not ported — the task file and D9 specify the second walk only, and task 05 can revisit if the port genuinely needs main-walk state.
- `dataSourceConfiguration[T].Caching()` accessor added with its `caching` field; NOTHING sets the field yet (same as the first pass) — it is the declared seam for per-datasource provider plumbing, first consumed in task 05.
- `postprocess.EnableCaching(providers, federation, definition)` stays internal-facing (exported for the execution module, but the engine `Configuration` is the only public entry point).
- `deferTrees` collects the initial tree plus every defer-group tree so cross-tree passes (`optimizeL1Cache`, task 16) see the full set of one response's trees in one call.

## What was implemented

- NEW `v2/pkg/engine/plan/cacheconfig`: `CachingConfiguration` (+ provider methods), `EntityCachePolicy`, `RootFieldCachePolicy`, `MutationCachePolicy`, `SubscriptionCachePolicy`, `CacheConfigProvider`.
- NEW `v2/pkg/engine/cache`: `Configurator`/`NewConfigurator`/`ConfigureCaching` (holding the single no-op gate `len(providers) == 0`), plus inert skeletons `cacheKeyBuilder` (task 06), `fetchCacheConfigurator.configureTree` (tasks 06/13), `optimizeL1Cache.optimize` (task 16).
- `plan`: `Configuration.CacheConfigProviders`; `dataSourceConfiguration.caching` + `Caching()`; `cacheProvidesDataVisitor` skeleton (`reset`/`EnterField`/`LeaveField`/`attachTo`); the gated second-walk seam in `Planner.Plan`.
- `postprocess`: `EnableCaching` option, `Processor.caching`, the three `ConfigureCaching` call sites (sync / defer incl. every defer group via `deferTrees` / subscription), each after `processFlatFetchTree`/`extractDeferFetches` and before `organizeFetchTree`/`buildDeferTree`.
- `resolve`: `Object.HasAliases`, `Field.OriginalName`, `Field.CacheArgs` (+ `CacheFieldArg`), carried through `Object.Copy`/`Field.Copy` (CacheArgs cloned, not aliased); `GraphQLResponse.cacheProvidesData` side-table + `SetCacheProvidesData`/`CacheProvidesData`.
- `execution/engine`: `Configuration.SetCaching`; `NewExecutionEngine` builds providers + federation maps, validates IDs, forces FetchInfo, and threads `postprocess.EnableCaching` into every per-execution processor (`newInternalExecutionContext` now takes processor options).

Tests (dedicated files, full-value `assert.Equal`):

- `cache/configure_caching_test.go` — no-providers and providers-but-inert rows both leave the fetch tree byte-identical (full-tree equality) with all `Cache` nil.
- `cacheconfig/cacheconfig_test.go` — provider lookup hit/miss rows for all four policy kinds, full policy values asserted; empty-configuration misses everywhere.
- `plan/cache_provides_data_visitor_test.go` — P1 constructed only with providers; the second-walk determinism proof: plan WITH inert caching configured == plan WITHOUT, compared via the package's pretty-print convention.
- `resolve/cache_node_copy_test.go` — `Copy()` carries `HasAliases`/`OriginalName`/`CacheArgs` (full-object equality) and clones `CacheArgs`; response side-table accessors.
- `execution/engine/engine_caching_config_test.go` — `SetCaching` reaches planner providers + forces `DisableIncludeInfo=false` + produces the postprocess option; without it nothing is constructed; unknown datasource ID errors.

## What to look into (review focus)

- Facade placement: `ConfigureCaching` runs after `createConcreteSingleFetchTypes` (last step of `processFlatFetchTree`) and before `organizeFetchTree`; for defer plans it runs after `extractDeferFetches` and before `organizeFetchTree`/`buildDeferTree` nils `Defers` — verify against `postprocess.go` `Process`.
- The four config gates: empty providers (facade returns), per-datasource nil provider and per-coordinate `(_, false)` (provider contract, exercised by the cacheconfig tests), all-flags-false config → nil Cache (trivially true while the configurator is inert; re-review at task 06).
- `cacheProvidesData` is a side-table on `GraphQLResponse`, not part of the response tree — defer's response-tree `Copy()` cannot reach it.
- The P1 seam calls `ResetVisitors()` on the planningWalker AFTER the plan is fully built (including cost calculation); confirm nothing later in `Plan` relies on the planning visitors still being registered.
- Import shape: `cacheconfig` imports only `time`; `engine/cache` imports `plan`/`cacheconfig`/`resolve`/`ast`; `plan` does not import `engine/cache` — no cycle.

## Verification evidence

- All new tests pass (`engine/cache`, `plan/cacheconfig`, `plan`, `resolve`, `execution/engine`).
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
