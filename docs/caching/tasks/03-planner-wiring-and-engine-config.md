# Task 03 — Planner wiring, cacheconfig package, engine SetCaching

Phase: 0 (Structure).
Dependencies: tasks 01, 02.
References: RFC-2 §5, §7.2–7.3, §11; deviations D3, D5, D6, D9 (PLAN §7).

## Problem

The planner has no caching producer, no policy model, and no `ProvidesData` carrier.
These must exist as additive wiring that produces NO config until caching configuration is supplied, and the public entry point must be the engine `Configuration`, not a postprocess option.

## Scope

Policy model (leaf package, no cycle):

- New `v2/pkg/engine/plan/cacheconfig` package: `CachingConfiguration`, `EntityCachePolicy`, `RootFieldCachePolicy`, `MutationCachePolicy`, `SubscriptionCachePolicy`, and the `CacheConfigProvider` interface (`EntityPolicy`, `RootFieldPolicy`, `MutationPolicy`, `SubscriptionPolicy`).
- `dataSourceConfiguration[T].Caching() cacheconfig.CacheConfigProvider` accessor (nil = no caching for that datasource).
- L2 is DERIVED from TTLs (D3): the policy structs carry NO explicit L1/L2 bools.

Pass skeletons (logic lands in later tasks; D6 names):

- Pass logic homes in the common `v2/pkg/engine/cache` package (D5); `postprocess` holds thin calls.
- `ConfigureCaching` facade with the single NO-OP gate: returns immediately when no providers are configured.
- `cacheKeyBuilder`, `fetchCacheConfigurator`, `optimizeL1Cache` — wired but inert skeletons.
- `postprocess.go` additive edits: a `Processor` field, the `EnableCaching(providers, federation, definition)` option (internal), the facade call in `Process` for sync + defer (root tree plus every `Defers[i].Fetches`, after `processFlatFetchTree`/`extractDeferFetches`, before `organizeFetchTree`/`buildDeferTree`) + subscription arms, and a `deferTrees` helper collecting root + defer-group trees.

P1 registration seam (visitor body lands in task 05; D9):

- `plan/planner.go`: construct `cacheProvidesDataVisitor` only when caching is configured; after the main `planningWalker.Walk`, run the gated SECOND, filter-free walk (`ResetVisitors()` + `SetVisitorFilter(nil)` + register only P1 + `Walk` + `attachTo`), all `!= nil`-guarded.
- Extending existing planner/visitor files is ALLOWED when it is the cleaner path; prefer new single-responsibility visitors.

Per-field metadata carriers (additive):

- `resolve/node_object.go`: `Object.HasAliases`, `Field.OriginalName`, `Field.CacheArgs` (+ `CacheFieldArg`), carried in `Object.Copy()`/`Field.Copy()`.
- `resolve.GraphQLResponse`: unexported `cacheProvidesData map[*FetchInfo]*Object` + `SetCacheProvidesData`/`CacheProvidesData` accessors.

Engine entry point (the public API):

- Engine `Configuration` gains `SetCaching(...)` alongside `SetDataSources`/`SetFieldConfigurations`; `NewExecutionEngine(...)` picks it up and wires `postprocess.EnableCaching` + the P1 enablement internally.
- Signature sketch: `engineConf.SetCaching(cacheconfig.CachingConfiguration)` per datasource (or a keyed set) — settle the exact shape against how `SetFieldConfigurations` is keyed, and record the decision in the commit notes.
- Hard wiring precondition: configuring caching force-enables `FetchInfo` (a `DisableIncludeInfo` + caching combination degrades to a clean, tested no-op, never to silent uncached behavior).

## Tests

- NO-OP planner proof: the EXISTING planner/postprocess suites with NO caching configured produce byte-identical plans (every `Cache` nil).
- Skeleton unit tests: facade with empty providers touches no node; `cacheconfig` policy structs round-trip; `node_object` `Copy()` carries the new fields (full-value `assert.Equal`).
- Engine wiring test: `SetCaching` reaches the postprocess option and the P1 constructor; without `SetCaching` neither is constructed.

## Acceptance criteria

- [ ] With no `SetCaching`, plans are byte-identical to today (the planner no-op gate) and no caching type is constructed.
- [ ] The four config gates default OFF: empty providers; per-datasource nil provider; per-coordinate `(_, false)`; all-flags-false config → nil `Cache`.
- [ ] `cacheconfig` is a leaf package (imports `resolve` at most; no plan↔cache cycle).
- [ ] The P1 second-walk seam never re-runs the planning visitor and never rebuilds the plan.
- [ ] Lint-clean in `v2` and `execution` (the engine wiring touches execution).

## Reviewer guidance

- Confirm the facade runs after `createConcreteSingleFetchTypes` and before `organizeFetchTree`, and for defer plans before `buildDeferTree` nils `Defers`.
- Confirm `cacheProvidesData` is a side-table on the response, never reached by defer's response-tree `Copy()`.
- Confirm the engine `Configuration` is the ONLY public entry point; `postprocess.EnableCaching` stays internal.
