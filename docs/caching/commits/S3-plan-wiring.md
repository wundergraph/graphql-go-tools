# Commit S3 — RFC-2 additive plan wiring + pass skeletons + plan-side config

Plan item: `docs/caching/PLAN.md` §2, S3.
RFC sections: RFC-2 §4 (pass inventory), §5 (additive insertion points), §5.4 (node_object fields), §9.3 (the *FetchInfo side-table), §11.2-§11.4 (cacheconfig + composed NO-OP gating); RFC-1 §7.2-§7.3 (policy shapes + provider).
Phase: 0 (Structure, pure no-op).

## Problem

The planner had no caching producer, no policy model, and no `ProvidesData` carrier.
These must exist as ADDITIVE wiring that produces NO config until a provider is supplied,
with zero body edits to the five forbidden visitor files.

## Solution

Additive-only wiring plus inert skeletons, all gated off by default.

- New `plan/cacheconfig` package declaring RFC-1 §7.2-§7.3's EXACT shapes (no invented fields):
  `CachingConfiguration`, `EntityCachePolicy`, `RootFieldCachePolicy`, `MutationCachePolicy`, `SubscriptionCachePolicy`, and the `CacheConfigProvider` interface (RFC-2 §11.2).
  Imports `resolve` + `time` only; does not import `plan` (no cycle).
- `dataSourceConfiguration[T].Caching() CacheConfigProvider` accessor over a new unexported `caching` field (nil default).
- New postprocess skeletons (INERT): `cachingPlanner` facade (the single NO-OP gate in `Annotate`), `cacheKeySpecFreezer` (freeze returns `false`), `cacheConfigStamper` (buildConfig returns nil), `optimizeL1Cache` (processTrees no-ops), plus the `deferTrees` helper.
- `postprocess.go` additive edits: the `cachingPlanner` field, the `EnableCaching(providers, federation, definition)` option, `NewProcessor` instantiation, and the `Annotate` call in all three `Process` arms (sync + defer + subscription). Every existing line is byte-identical.
- `plan/planner.go` additive blocks: the `cacheProvidesData` field, construction in `NewPlanner` ONLY when `len(config.CacheConfigProviders) > 0`, peer registration on `planningWalker` after the cost visitor, and `attachTo` after the walk — all `!= nil`-guarded.
- New `plan/cache_provides_data_visitor.go`: the P1 skeleton (`EnterField`/`LeaveField` no-op; `attachTo` sets an empty `*FetchInfo`-keyed side-table; `responseOf` covers the three plan kinds). The full walk-time port lands in A1.
- `resolve/node_object.go` additive fields: `Object.HasAliases`, `Field.OriginalName`, `Field.CacheArgs`, the `CacheFieldArg` type, carried in `Object.Copy()`/`Field.Copy()` (`CacheArgs` deep-copied).
- `resolve/response.go`: unexported `GraphQLResponse.cacheProvidesData` + `SetCacheProvidesData`/`CacheProvidesData` accessors.
- `plan/configuration.go`: additive `Configuration.CacheConfigProviders` field — the P1 enablement gate (defaults nil).

## Key decisions

- Composed NO-OP gating: with no providers, the facade `Annotate` returns at its guard, P1 is never constructed/registered, and every fetch's `Cache` stays nil (RFC-2 §11.3-§11.4).
- The `ProvidesData` carrier is a `*FetchInfo`-keyed side-table on `GraphQLResponse` (not a re-added `FetchInfo.ProvidesData` field, not a fetchID map), immune to defer's response-tree `Copy()` (RFC-2 §9.3, §13.3).
- The new `Object`/`Field` fields are `json:"-"` and only ever populated on the caching-owned `ProvidesData` tree; the plan no-op golden is safe because plan/postprocess/datasourcetesting tests compare `pretty.Sprint` of two Go values (no static `.golden` files), so a zero-valued additive field renders identically on both sides.

Deviations / notes:

- `Caching()` uses a value receiver (matching RFC-1 §7.3's literal `func (d dataSourceConfiguration[T]) Caching()`), unlike its pointer-receiver siblings; behavior-neutral.
- No builder/setter for the datasource `caching` field is added in this commit (no existing builder option fits); engine wiring lands later.
- The P1 skeleton's embedded `*astvisitor.Walker` is nil in this commit (never walked, since no test enables caching). A1 MUST wire it (mirror `NewCostVisitor(p.planningWalker, ...)`) when it ports the real `EnterField`/`LeaveField` body.

## Tests

- `plan/cacheconfig/cacheconfig_test.go` (own package): construct every policy + a fake provider; full-value `assert.Equal` on the `CachingConfiguration` and each `(policy, ok)` return.
- `postprocess/caching_planner_test.go` (in-package): `cachingPlanner.Annotate` with empty providers leaves a fetch's `Cache` nil (the facade touches no node).
- `resolve/node_object_cache_copy_test.go` (in-package): `Object.Copy()`/`Field.Copy()` carry `HasAliases`/`OriginalName`/`CacheArgs` (full-value `assert.Equal`).

Verification (from `v2/`):

- `go build ./pkg/...` — clean (cacheconfig has no import cycle).
- ZERO diff to `node_selection_visitor.go`, `path_builder_visitor.go`, `required_fields_visitor.go`, `node_selection_builder.go`, `visitor.go` (`git diff --stat` empty for all five).
- `go test ./pkg/engine/plan/... ./pkg/engine/postprocess/... ./pkg/engine/resolve/... ./pkg/engine/datasource/graphql_datasource/... -count=1` — PASS (the existing suites are the NO-OP golden; the three new tests pass).
- `go vet ./pkg/engine/plan/... ./pkg/engine/postprocess/... ./pkg/engine/resolve/...` — clean.

## Reviewer guidance

- Confirm ZERO body diff on the five forbidden files.
- Confirm the no-op golden is unchanged (no provider wired -> byte-identical plans).
- Confirm `cacheProvidesData` is never reached by defer's response-tree `Copy()` (it is its own side-tree).
- Confirm the skeleton bodies are inert (freezer false, stamper nil, L1 no-op, P1 no-op) — the real logic lands in A1/B1/C1/D1.
