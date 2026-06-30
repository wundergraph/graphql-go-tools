# RFC-2: The caching planner passes for the defer branch

Status: final for review.
Author: caching working group.
Branch under change: `feat/eng-7770-add-defer-support-part-4`, code under `v2/pkg/engine/plan/` and `v2/pkg/engine/postprocess/`.
Companion contract: RFC-1 (`docs/caching/specs/2026-06-30-rfc-01-loader-cache-abstraction.md`) — the loader-side abstraction and the self-contained runtime config types.
Ground truth: `scratchpad/caching-rfc/DOSSIER.md` (esp. §6 "Caching planner contract"), plus `analysis-A-planner.md` (OLD caching planner/visitor/postprocess) and `analysis-B-planner-current.md` (CURRENT defer plan pipeline + add-a-pass recipe).

This RFC is the synthesis of three independent drafts (A minimal-derive, B faithful-phased, C layered-SRP) and three judge reviews.
The structural base is **B (faithful-phased)**: it wins the two correctness-critical dimensions the brief weights highest — `ProvidesData` fidelity (PR5) and RFC-1 contract fit (PR8) — while staying fully additive.
Onto B we graft, surgically, the ideas the reviewers judged strictly better:

- from C: the thin, logic-free `cachingPlanner` FACADE that houses the single NO-OP gate, and the named, physically-isolated `cacheKeySpecFreezer` unit so a reviewer confirms "no federation pointer reaches runtime" by reading one file, and folding `computeHasAliases` into the stamper rather than a separate pass;
- from A: the CROSS-TREE `optimizeL1Cache` (collect entity fetches across the root tree AND every `Defers[i].Fetches`, because the L1 store lives for the request lifetime and is shared across defer groups — the request-lifetime L1 store, RFC-1 §6.2), the no-op golden test as the hard PR1/PR8 zero-impact proof, and A's honest enumeration of derive-lossiness as the written justification for WHY B re-adds the visitor.

Every must-fix raised by the reviewers is resolved, with `file:line` proof for the correctness-critical ones (the P1 registration mechanism, the `*FetchInfo` carrier stability, the representation-helper absence, the policy-field shapes, and the L1 eligibility/narrow split).

---

## 1. Summary and goals

### 1.1 What this RFC delivers

RFC-1 landed the runtime side: the loader carries an opaque `Cache *FetchCacheConfig` on `SingleFetch`/`EntityFetch`/`BatchEntityFetch`, hands it to a `RequestCache` controller at the prepare/merge seams, and reads `cfg.ProvidesData` at lookup time for coverage validation (RFC-1 §3.6, §8).
RFC-2 is the PRODUCER that fills those types in.
It adds DEDICATED, NEW caching passes — not extensions of the existing planners — that stamp the self-contained `resolve.FetchCacheConfig` (and the per-field cache normalize metadata it needs) onto the fetch tree, reading federation `@key` info ONLY as a plan-time input frozen by value.

### 1.2 Goals

- G1. ADDITIVE ONLY: zero edits to the bodies of the five planner/visitor files the brief forbids; new files plus list-insertion registration wiring only (PR1).
- G2. Cross the federation→caching boundary EXACTLY ONCE, at plan time, by value, in one named unit; retain no federation pointer into runtime (PR2).
- G3. Stamp a populated `*FetchCacheConfig` onto all three concrete fetch types, AFTER `createConcreteSingleFetchTypes`, writing the concrete types directly (PR3).
- G4. Walk EVERY fetch tree (root + each defer group + subscription), `DeferID`-correct, request-independent (PR4).
- G5. RE-ADD a dedicated plan-time `ProvidesData` visitor (not derive), reproducing OLD entity-boundary / `__typename`-dedup / inline-fragment / alias-and-arg-aware semantics (PR5).
- G6. Keep the L1 optimization its own single-responsibility pass (PR6).
- G7. Decompose into small, single-responsibility, independently unit-testable passes behind a thin facade (PR7).
- G8. Consume RFC-1's `cacheconfig.CacheConfigProvider`/`*CachePolicy` types VERBATIM, with composed NO-OP gating so a mode mismatch is impossible by construction (PR8).
- G9. Record that the `@requestScoped` directive feature is OUT OF SCOPE entirely (removed by review) and prove the rest of caching works without it (PR9).
- G10. Determinism, plan-cache safety, the third `DeferResponsePlan` kind, per-node annotations, and the `Copy()`/side-table interaction with defer (PR10).
- G11. The whole pass set is a strict NO-OP until a `CacheConfigProvider` is supplied: adding RFC-2's code changes plans in ZERO ways until a provider exists.

### 1.3 Non-goals

- NOT re-designing RFC-1.
  RFC-2 CONSUMES, and does NOT redefine, the resolve-package types RFC-1 declares (`FetchCacheConfig`, the revised multi-key `CacheKeySpec`, its per-`@key` candidate type, `CacheScope`, `EntityKeyMapping`).
  The plan-side config types RFC-1 specifies in §7.2-7.3 (`CachingConfiguration`, `CacheConfigProvider`, the `*CachePolicy` structs) are physically DECLARED by RFC-2 in a new `plan/cacheconfig` package (§11.2), to RFC-1's exact shapes — RFC-2 declares them to spec, it does not invent fields or diverge.
- NOT implementing the cache store.
  No `RequestCache`/controller implementation, no key rendering, no L1/L2 backend — those live in the `resolve/cache` package behind RFC-1's interfaces (RFC-1 §7.1).
- NOT the runtime loader integration — that is RFC-1.

---

## 2. Background

### 2.1 The OLD caching coupling into the existing visitors/postprocess

The OLD branch wove caching into the planner at THREE layers, only one of which was a clean post-pass (analysis-A §0, §6):

1. The planning visitor (`visitor.go`, ~138 lines): a `caching *cachingPlannerState` field driven through the hot path of the response-shaping walk — `EnterField`/`LeaveField` build a parallel per-planner `ProvidesData` tree (`trackFieldForPlanner`/`popFieldsForPlanner`), `resolveFieldValue` stamps `Object.CacheAnalytics`, `configureFetch` derives the whole `FetchCacheConfiguration` (analysis-A §1, §2.1).
   `cachingPlannerState` (943 new lines) is technically a separate struct but is a SHADOW WALK fused into the primary walk, reading private `Visitor` state.
2. The path builder (`path_builder_visitor.go`, ~60 lines): `isolatedRootField` forces each cached query root field into its own fetch (analysis-A §2.2).
3. The node-selection visitor + a new 766-line sibling `node_selection_visitor_request_scoped.go`: the `@requestScoped` widening pre-planning analysis (analysis-A §2.3, §4).

Only `postprocess/optimize_l1_cache.go` (572 lines) was a genuinely standalone `FetchTreeProcessor` that read the finished fetch tree and flipped one boolean (`Caching.UseL1Cache`) per fetch (analysis-A §3).
That is the architectural template RFC-2 generalizes: the caching planner should be passes over the fully-planned fetch tree (plus ONE dedicated plan-time visitor for the one concern that genuinely needs walk-time context), NOT edits inside the core visitors.

All configuration was bolted onto `FederationMetaData` (6 collections + 4 `FederationInfo` methods + datasource forwarders, analysis-A §2.5, dossier §5.1); RFC-2 replaces that with a self-contained plan-side `cacheconfig` package.

### 2.2 The CURRENT defer plan pipeline and where a pass plugs in

The CURRENT pipeline (analysis-B §1), `Planner.Plan` (`planner.go:92`):

1. `selectOperation` (`planner.go:100`).
2. `prepareOperation` (`planner.go:105`) — runs `prepareOperationWalker` (`planner.go:58-60`: `InlineFragmentAddOnType` + `deferInfoCollector`), the precedent for a dedicated own-walker pass.
3. DataSource hashing (`planner.go:113-115`).
4. `SelectNodes` (`planner.go:118-128`) — assigns fields to datasources.
5. `CreatePlanningPaths` (`planner.go:133-142`) — `[]PlannerConfiguration`.
6. Planning visitor walk (`planner.go:154-219`) — builds `GraphQLResponse.Data` + flat `RawFetches`; the cost visitor is registered here as a PEER on the same walker (`planner.go:170-182`).
7. Postprocess (`execution_engine.go:304`) — `Processor.Process` converts `RawFetches`→`Fetches` (`*FetchTreeNode`), dedups, appends fetchIDs, resolves templates, creates concrete types, organizes into sequence/parallel/defer.
8. Plan cached and reused across requests (`execution_engine.go:305`) — so any stamped config MUST be request-independent.

The postprocess add-a-pass recipe (analysis-B §3): one new file (struct + `ProcessFetchTree` copying the Single/Parallel/Sequence walk), plus ~5 small edits to `postprocess.go` (field on a processor struct, an option, instantiation, a call in `Process`), with ZERO edits to existing pass bodies.
For caching the pass must run AFTER `createConcreteSingleFetchTypes` (inside `processFlatFetchTree`, `postprocess.go:47`), once per fetch tree (analysis-B §6).

Two structural facts force the design:

- `createConcreteSingleFetchTypes` builds NEW `EntityFetch`/`BatchEntityFetch` that embed only `FetchDependencies` (`fetch.go:166`/`:206`), NOT `FetchConfiguration`, and copy `Info` BY REFERENCE (`create_concrete_single_fetch_types.go:71`/`:116`); anything stamped on the raw `SingleFetch` before conversion is lost on the entity types (analysis-B §2, §6).
- A `DeferResponsePlan` (`plan.go:66`) has N fetch trees: the root `Response.Response.Fetches` plus one `*DeferFetchGroup{DeferID, Fetches}` per `DeferID` (`response.go:105`); a single-tree port silently skips deferred operations (analysis-B §5, dossier §7 risk 14).

---

## 3. How RFC-2 satisfies the RFC-1 contract

RFC-1 §8 lists eight obligations.
RFC-2 satisfies each, consuming RFC-1's resolve-package types verbatim (never redefining one) and declaring the plan-side `cacheconfig` types to RFC-1's exact §7.2-7.3 spec.

| RFC-1 §8 clause | RFC-2 mechanism | Section |
|---|---|---|
| 1. Stamp populated `*FetchCacheConfig` on all 3 concrete types, AFTER `createConcreteSingleFetchTypes`; nil = off | `cacheConfigStamper`, invoked by the facade after conversion, type-switches all three | §7 (PR3) |
| 2. Self-contained: no federation types/pointers; ALL `@key` sets frozen by value as multi-key candidates | `cacheKeySpecFreezer` is the SOLE federation reader, copies OUT by value via the shared `representationvariable` package | §6 (PR2) |
| 3. Carry the full runtime payload (flags, TTLs, the multi-key `CacheKeySpec`, `ProvidesData`, mutation flags) | the stamper assembles from provider policy + frozen spec + the P1 `ProvidesData` side-table | §7, §9 |
| 4. `Equals`-stable so dedup cannot lose policy | RFC-1's nil-safe `FetchConfiguration.Equals` clause (RFC-1 §3.8); RFC-2 stamps POST-dedup so `Cache` is nil at dedup time | §7.4 (PR4) |
| 5. Request-independent (plan is cached/reused) | only static config written; per-request key material derived at runtime in `PrepareFetch` | §8 (PR4, PR10) |
| 6. Walk every fetch tree, respect `DeferID` | the facade is invoked once per tree (root + each `Defers[i].Fetches` + subscription) | §8 (PR4) |
| 7. Gate off entirely when NO-OP | empty provider map ⇒ facade returns before touching a node ⇒ every `Cache` nil; composes with RFC-1's runtime nil-check | §11 (PR8) |
| 8. Provide `CacheConfigProvider`, decoupled from `FederationInfo` | new plan-side `cacheconfig` package, reached via `dataSourceConfiguration[T].Caching()`; passes read it, never `FederationInfo` | §11 (PR8) |

What RFC-1 CONSUMES from what RFC-2 stamps:

- `cfg.ProvidesData *resolve.Object` — read at RUNTIME by the coverage walk inside `PrepareFetch` (RFC-1 §3.6, dossier §2.10), alias-and-arg-aware; RFC-2 builds it (§9).
- `cfg.KeySpec` — the multi-key spec; the cache package builds ONE `CacheKeyTemplate` per candidate (the sole source of truth for read/write/invalidate keys, RFC-1 §7.4), then renders every candidate best-effort at lookup and re-renders the still-unrendered ones at write time; RFC-2 freezes ALL `@key` sets into it (§6).
- `cfg.L1`/`cfg.L2` and the scalar policy fields — gate which controller mode runs; RFC-2 sets eligibility, `optimizeL1Cache` narrows L1 (§7, §10).

---

## 4. The new pass / visitor inventory

Five units ship in v1 (one plan-time visitor, the facade, the freezer, the stamper, the L1 pass); one is staged to v2.
NONE edits an existing visitor or pass body.
The freezer additionally depends on a small, behavior-preserving refactor that extracts the representation-variable builder into a shared package (§6.1) — its own early structural commit, not a new pass.

| # | Name | File (new) | Single responsibility | Run phase | Input | Output | Stage |
|---|---|---|---|---|---|---|---|
| P1 | `cacheProvidesDataVisitor` | `plan/cache_provides_data_visitor.go` | Build, per fetch, the `*resolve.Object` field tree the fetch returns (entity-boundary skip, `__typename` dedup, inline-fragment path normalization, alias→`OriginalName`, `CacheArgs`); attach as a `*FetchInfo`-keyed side-table | plan-time, registered as a PEER on the EXISTING `planningWalker` (like `costVisitor`) | operation+definition AST, `planningVisitor.fieldPlanners`, planners | `map[*resolve.FetchInfo]*resolve.Object` on `GraphQLResponse` | v1 |
| F | `cachingPlanner` (facade) | `postprocess/caching_planner.go` | Thin, logic-free orchestrator: hold the single NO-OP gate, stamp each tree, then run the cross-tree L1 pass | post-plan, invoked once per plan from `Processor.Process` | provider map, federation metadata by DataSourceID, the trees, the `ProvidesData` side-table | (drives the leaf passes) | v1 |
| H | `cacheKeySpecFreezer` | `postprocess/cache_key_spec_freezer.go` | The SOLE federation reader: turn EVERY resolvable `@key` set (+ interfaceObject/entityInterface remaps + root-arg↔`@key` mappings) into a multi-key `resolve.CacheKeySpec` BY VALUE, one best-effort candidate per `@key` set, via the shared `representationvariable` package | post-plan, called per fetch by the stamper | `plan.FederationMetaData`, definition AST, fetch identity | multi-key `resolve.CacheKeySpec` (pure data) | v1 |
| P2 | `cacheConfigStamper` | `postprocess/cache_config_stamper.go` | Assemble + stamp a self-contained `*resolve.FetchCacheConfig` on `SingleFetch`/`EntityFetch`/`BatchEntityFetch` from policy + frozen spec + `ProvidesData`; fold `computeHasAliases`; leave nil when no policy | post-plan `FetchTreeProcessor`, AFTER `createConcreteSingleFetchTypes` | the tree, provider, freezer, side-table | `.Cache` on each concrete fetch | v1 |
| P3 | `optimizeL1Cache` | `postprocess/optimize_l1_cache.go` | Narrow `cfg.L1 = canRead \|\| canWrite` via `ProvidesData` subset/superset + `DependsOnFetchIDs` ordering, CROSS-tree | post-plan, AFTER P2, ONCE over all trees | all stamped trees | refined `cfg.L1` | v1 |
| S | `cacheSubscriptionAnnotator` | `postprocess/cache_subscription_annotator.go` | Stamp `GraphQLSubscription.EntityCachePopulation` + mutation `PopulateL2OnMutation`/`MutationTTLOverride` | post-plan, subscription/mutation trees | subscription/mutation policy | subscription/mutation annotations | v2 |

The OLD `@requestScoped` pre-planning rewrite (`cacheRequestScopedRewrite`) is NOT in this inventory: the `@requestScoped` directive feature is out of scope entirely (removed by review, §12) — it is neither a v1 nor a v2 pass.

Run-order diagram (CURRENT pipeline, NEW steps marked):

```
Planner.Plan (planner.go:92)
  selectOperation        (planner.go:100)
  prepareOperation       (planner.go:105)
  SelectNodes            (planner.go:118)
  CreatePlanningPaths    (planner.go:133)
  register planningVisitor field visitors    (planner.go:162-168)
  register costVisitor    (planner.go:170-182)   ← existing peer precedent
  ── register P1 cacheProvidesDataVisitor ──     ← NEW peer on the SAME walker, registered LAST (like costVisitor)
  planningWalker.Walk    (planner.go:216)        ← P1 runs here, reads fieldPlanners
  ── P1.attachTo(plan) ──                         ← NEW, after the walk (like costVisitor.finalCostTree at :222-224)
  return raw plan        (planner.go:229)

postprocess.Processor.Process (postprocess.go:190)
  ... mergeFields → createFetchTree → processFlatFetchTree (incl. createConcreteSingleFetchTypes, postprocess.go:47) ...
  [for DeferResponsePlan: extractDeferFetches (postprocess.go:205)]
  ── cachingPlanner.Annotate(resp, trees...) ──   ← NEW: H+P2 per tree, then P3 cross-tree
  organizeFetchTree / buildDeferTree (unchanged)
```

Why this granularity: see §10.
The short version: P1 is the only concern that genuinely needs walk-time context (enclosing type, inline-fragment ancestors, per-planner ownership, operation-AST arguments); H/P2/P3 are pure functions of the finished fetch tree; the freezer is a named helper invoked by the stamper so federation is read in exactly one file; the facade carries no logic and houses the single NO-OP gate.

---

## 5. Additive insertion points (PR1)

Every touch point is a list-insertion or a new registration call.
ZERO bodies of `node_selection_visitor.go`, `path_builder_visitor.go`, `required_fields_visitor.go`, `node_selection_builder.go`, or `visitor.go` change.
`planner.go` and `postprocess.go` are NOT in the forbidden list; the edits there are new orchestration calls and registrations, never edits to a pass/visitor body.

### 5.1 `postprocess/postprocess.go` — five additive edits, zero pass-body edits

1. One field on `Processor` (`postprocess.go:21-28`), mirroring the existing standalone `extractDeferFetches`/`buildDeferTree` fields (`:26-27`):

```go
type Processor struct {
	disableExtractFetches  bool
	collectDataSourceInfo  bool
	fetchTreeProcessors    *FetchTreeProcessors
	responseTreeProcessors *ResponseTreeProcessors
	extractDeferFetches    *extractDeferFetches
	buildDeferTree         *buildDeferTree
	cachingPlanner         *cachingPlanner // NEW (facade; nil-safe + empty-provider no-op)
}
```

2. One field + one option on `processorOptions` (`postprocess.go:61-74`) and a new `ProcessorOption`:

```go
type processorOptions struct {
	// ... existing fields ...
	cacheProviders  map[string]cacheconfig.CacheConfigProvider // NEW: DataSourceID -> provider; nil/empty => no-op
	cacheFederation map[string]plan.FederationMetaData         // NEW: DataSourceID -> @key input for the freezer
	cacheDefinition *ast.Document                              // NEW: schema AST; the shared representationvariable builder resolves field types against it (§6.1)
}

// EnableCaching wires the per-datasource cache config providers, the federation
// metadata the key-spec freezer reads as plan-time input, and the schema definition
// the shared representationvariable builder needs to freeze @key sets (§6.1). When the
// provider map is empty the caching passes are no-ops, so RFC-2 changes plans in zero
// ways until a provider is supplied (RFC-1 §10).
func EnableCaching(
	providers map[string]cacheconfig.CacheConfigProvider,
	federation map[string]plan.FederationMetaData,
	definition *ast.Document,
) ProcessorOption {
	return func(o *processorOptions) {
		o.cacheProviders = providers
		o.cacheFederation = federation
		o.cacheDefinition = definition
	}
}
```

3. Instantiate in `NewProcessor` (`postprocess.go:139`), additive:

```go
freezer := &cacheKeySpecFreezer{federation: opts.cacheFederation, definition: opts.cacheDefinition}
cachingPlanner: &cachingPlanner{
	providers: opts.cacheProviders,
	freezer:   freezer,
	stamper:   &cacheConfigStamper{providers: opts.cacheProviders, freezer: freezer},
	l1:        &optimizeL1Cache{disable: len(opts.cacheProviders) == 0},
},
```

4. Call the facade from `Process` (`postprocess.go:190`), once per plan.
This is the SANCTIONED additive insertion point (analysis-B §3); `Process` is the orchestrator, not a pass body.

Before (CURRENT `postprocess.go:190-232`, abridged — existing inline comments elided):

```go
func (p *Processor) Process(pre plan.Plan) {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Data)
		p.createFetchTree(t.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Fetches)
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Fetches)

	case *plan.DeferResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)
		p.extractDeferFetches.Process(t)
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)
		for _, deferResp := range t.Response.Defers {
			p.fetchTreeProcessors.organizeFetchTree(deferResp.Fetches)
		}
		p.buildDeferTree.Process(t.Response)
		t.Response.Defers = nil

	case *plan.SubscriptionResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.appendTriggerToFetchTree(t.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)
		p.fetchTreeProcessors.resolveInputTemplates.ProcessTrigger(&t.Response.Trigger)
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)
	}
}
```

After (only the `// NEW` lines are inserted; every existing line is byte-identical):

```go
func (p *Processor) Process(pre plan.Plan) {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Data)
		p.createFetchTree(t.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Fetches)
		p.cachingPlanner.Annotate(t.Response, t.Response.Fetches) // NEW
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Fetches)

	case *plan.DeferResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)
		p.extractDeferFetches.Process(t)
		// NEW: stamp root + every defer group; cross-tree L1 over all of them.
		p.cachingPlanner.Annotate(t.Response.Response, deferTrees(t.Response)...) // NEW
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)
		for _, deferResp := range t.Response.Defers {
			p.fetchTreeProcessors.organizeFetchTree(deferResp.Fetches)
		}
		p.buildDeferTree.Process(t.Response)
		t.Response.Defers = nil

	case *plan.SubscriptionResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.appendTriggerToFetchTree(t.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)
		p.cachingPlanner.Annotate(t.Response.Response, t.Response.Response.Fetches) // NEW
		p.fetchTreeProcessors.resolveInputTemplates.ProcessTrigger(&t.Response.Trigger)
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)
	}
}
```

5. One tiny free helper (`deferTrees`) that collects the root tree plus each `Defers[i].Fetches` into one slice:

```go
// deferTrees returns the root fetch tree plus every defer group's tree, so the
// facade stamps each tree and runs the cross-tree L1 pass over all of them.
func deferTrees(d *plan.DeferResponsePlan) []*resolve.FetchTreeNode {
	trees := make([]*resolve.FetchTreeNode, 0, 1+len(d.Response.Defers))
	trees = append(trees, d.Response.Response.Fetches)
	for _, g := range d.Response.Defers {
		trees = append(trees, g.Fetches)
	}
	return trees
}
```

Placement proof:
`Annotate` runs AFTER `processFlatFetchTree` (which ends at `createConcreteSingleFetchTypes`, `postprocess.go:47`) and BEFORE `organizeFetchTree`.
`organizeFetchTree` only reorders/groups nodes (`orderSequenceByDependencies`, `createParallelNodes`); it never mutates fetch contents (analysis-B §6), so stamping before it is order-independent and the annotations survive.
For defer, the facade runs after `extractDeferFetches` has split the flat tree (`postprocess.go:205`) and before `buildDeferTree` nils `Defers` (`postprocess.go:218`).

### 5.2 `plan/planner.go` — three additive blocks, all `!= nil`-guarded

1. P1 registration on the EXISTING `planningWalker`, mirroring the `costVisitor` precedent (VERIFIED `planner.go:170-182`).
The cost visitor is registered LAST on the same walker via `RegisterEnterFieldVisitor`/`RegisterLeaveFieldVisitor` and reads `p.planningVisitor.fieldPlanners` (`planner.go:177`); its own comment (`planner.go:170-174`) says it must be registered last because it depends on `fieldPlanners`.
P1 uses the IDENTICAL pattern, so its `EnterField` observes `fieldPlanners[ref]` after the planning visitor's `SetVisitorFilter` (`planner.go:162`) has populated it (`visitor.go:232-235`):

```go
	// existing cost-visitor registration block ends at planner.go:181 ...
	// NEW: provides-data visitor, registered AFTER the cost visitor, same pattern.
	// Reads fieldPlanners for per-field fetch ownership; edits no visitor body. (P1)
	if p.cacheProvidesData != nil {
		p.cacheProvidesData.planners = plannersConfigurations
		p.cacheProvidesData.fieldPlanners = p.planningVisitor.fieldPlanners
		p.planningWalker.RegisterEnterFieldVisitor(p.cacheProvidesData)
		p.planningWalker.RegisterLeaveFieldVisitor(p.cacheProvidesData)
	}
```

2. P1 carrier resolution after the walk (`planner.go:216-229`), mirroring how the cost calculator reads `p.costVisitor.finalCostTree()` after the walk (`planner.go:222-224`):

```go
	p.planningWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
	// NEW: resolve fetchID -> *FetchInfo and stash the side-table on the plan (§9.3).
	if p.cacheProvidesData != nil {
		p.cacheProvidesData.attachTo(p.planningVisitor.plan)
	}
```

3. Construction of P1 in `NewPlanner` (`planner.go:57-75`), an additive `Planner` field built only when a provider set is configured.

Net `planner.go` footprint: three small additive blocks (P1 registration, P1 carrier resolution, P1 construction), all guarded by `!= nil`; zero edits to the five forbidden visitor files.
(There is no pre-planning `@requestScoped` rewrite to insert between `prepareOperation` and `SelectNodes` — that feature is out of scope entirely, §12.)

### 5.3 Engine wiring

The engine builds `map[DataSourceID]cacheconfig.CacheConfigProvider` and `map[DataSourceID]plan.FederationMetaData` ONCE from `plan.Configuration` (each datasource's `Caching()` provider per RFC-1 §7.3, plus its `FederationConfiguration()`), and hands them — together with the schema `definition` AST the shared `representationvariable` builder needs (§6.1) — to the `Processor` via `postprocess.EnableCaching(providers, federation, definition)` at the existing `NewProcessor(...)` call site (analysis-B §1, `execution_engine.go:304`) and to the `Planner` for P1 enablement.
When the integrator supplies no providers, neither side is wired and the pipeline is unchanged.

### 5.4 The `resolve` per-field metadata addition (the one node_object.go edit)

The runtime coverage walk is alias-AND-arg-aware (RFC-1 §2.10, dossier §2.10): it matches `SchemaFieldName()+computeArgSuffix(CacheArgs)` against the cached bytes.
So `cfg.ProvidesData`'s `*resolve.Field` nodes MUST carry `OriginalName` (alias→schema name) and `CacheArgs`, and `*resolve.Object` needs the `HasAliases` fast-path gate (OLD `node_object.go:13/41-51/135/143`).
These fields do NOT exist on the defer branch (`node_object.go:8-16`, `:103-112`), and OLD's `resolve.ComputeHasAliases` was deleted (verified: no occurrence on the defer branch).

RFC-2 RE-ADDS them additively to `resolve` (a new `resolve` file plus four field additions), and updates `Object.Copy()`/`Field.Copy()` (`node_object.go:18-28`/`:119-134`) ADDITIVELY to carry them:

```go
// resolve/node_object.go (additive fields)
type Object struct {
	// ... existing fields ...
	HasAliases bool `json:"-"` // RFC-2: fast-path gate for L1 normalize
}
type Field struct {
	// ... existing fields ...
	OriginalName []byte         `json:"-"` // RFC-2: schema name when Name is an alias
	CacheArgs    []CacheFieldArg `json:"-"` // RFC-2: arg name + variable, sorted
}
type CacheFieldArg struct{ Name, VariableName string }
```

This is NOT a forbidden-body edit: `node_object.go` (the `resolve` package) is not in the PR1 list, and the change is purely additive (new fields + carrying them in `Copy()`).
It is also drift-safe by construction (PR10, §13.3): the only tree that carries these fields in v1 is the caching-owned `ProvidesData` tree, which is NEVER reached by defer's response-tree `Copy()`; the `Copy()` update is belt-and-suspenders against a future writer.

---

## 6. Federation→caching boundary (PR2)

The federation→caching boundary is crossed EXACTLY ONCE, at plan time, BY VALUE, inside the named `cacheKeySpecFreezer` unit (`postprocess/cache_key_spec_freezer.go`) — promoted from a helper to its own physically-isolated file (graft from C) so a reviewer confirms "no federation pointer reaches runtime" by reading ONE file.

Boundary diagram (RFC-1 §7.4):

```
plan.FederationMetaData.Keys (ALL resolvable @key selection sets)   +   interfaceObject/entityInterface remaps   +   root-arg↔@key mappings
        │  PLAN-TIME INPUT ONLY — read once, by value, by the freezer
        ▼
representationvariable.BuildRepresentationVariableNode(definition, keySet, fed)   (shared package, §6.1; one *resolve.Object per @key set, federation-pointer-free)
        │  one best-effort candidate per @key set
        ▼
cacheKeySpecFreezer.freeze(...) → multi-key resolve.CacheKeySpec{ Scope, TypeName, FieldName, Candidates []resolve.CacheKeyCandidate, EntityKeyMappings []resolve.EntityKeyMapping }
        │  packed into
        ▼
resolve.FetchCacheConfig.KeySpec   (federation-free; imports NO federation type)
        │  carried on the 3 fetch structs
        ▼
loader.go (RFC-1)  (forward-only; never reads a field)
```

The spec is MULTI-KEY (RFC-1's revised `CacheKeySpec`, §6.3): an entity type may declare more than one `@key`, and none is made required.
`Candidates` lists one independently, best-effort-renderable key template per resolvable `@key` set — at lookup the controller renders every candidate it CAN from the data on hand, and at write it re-renders the ones that were not yet renderable (RFC-1 §2.10, §3.7).

What is read as INPUT (legitimate key-derivation coupling, dossier §5.3):

- `FederationMetaData.HasEntity(typeName)` (`federation_metadata.go:39`) — which types get an entity spec.
- `FederationMetaData.RequiredFieldsByKey(typeName)` (`federation_metadata.go:35`) / `Keys` (`:11`) — ALL resolvable `@key` selection sets; each becomes one multi-key candidate (§6.3).
- interfaceObject/entityInterface remaps — the `__typename` written into each candidate's representation node (baked in by the shared builder, no separate `TypenameOverride` field needed per candidate).
- root-arg↔`@key` mappings (the OLD `EntityKeyMapping`/`FieldMapping`, dossier §5.3, listed there as acceptable plan-time input frozen into `CacheKeySpec`) — frozen into `EntityKeyMappings`.

What is NEVER read (policy coupling, removed, dossier §5.3): no `EntityCacheConfig`/`RootFieldCacheConfig` lookups on `FederationInfo`; policy comes only from `CacheConfigProvider` (§11).

### 6.1 The representation-variable builder: extract to a shared package (a reviewer must-fix)

All three drafts assumed reuse of `BuildRepresentationVariableNode`/`MergeRepresentationVariableNodes` from `plan/representation_variable.go`.
VERIFIED on the defer branch: that file does NOT exist in `plan`.
Equivalents exist but are UNEXPORTED and live in a sibling datasource package — `buildRepresentationVariableNode`/`mergeRepresentationVariableNodes` at `pkg/engine/datasource/graphql_datasource/representation_variable.go:21`/`:123`.
RFC-2 MUST NOT reach into another package's private helpers, and MUST NOT copy-paste them (two divergent `@key`→representation walkers is exactly the silent-key-skew hazard §6 exists to prevent).

The fix is a refactor-in-place, not a re-add: EXTRACT `BuildRepresentationVariableNode`/`MergeRepresentationVariableNodes` (and the internal `representationVariableVisitor` + merge helpers) into a NEW shared, exported package `v2/pkg/engine/plan/representationvariable` (verified: that path does not exist today), and REFACTOR `graphql_datasource` IN PLACE to import and call the exported functions instead of its local unexported copies (`graphql_datasource.go:855`/`:865`).
Then BOTH the GraphQL data source AND the caching `cacheKeySpecFreezer` share ONE implementation.

```go
// v2/pkg/engine/plan/representationvariable/representationvariable.go (NEW, exported)
//
// Shared by graphql_datasource (builds the _entities `representations` variable) and
// the caching cacheKeySpecFreezer (freezes @key sets into multi-key cache candidates).
// One @key selection set -> one *resolve.Object representation node, federation-
// pointer-free (it walks the parsed key fragment against the definition and allocates
// fresh resolve.Field/Object nodes), with the interfaceObject/entityInterface
// __typename remap already baked in.
package representationvariable

func BuildRepresentationVariableNode(
	definition *ast.Document,
	cfg plan.FederationFieldConfiguration, // one @key selection set
	federationCfg plan.FederationMetaData,
) (*resolve.Object, error)

// MergeRepresentationVariableNodes folds multiple key nodes into ONE representation
// object — used by graphql_datasource for the single `representations` variable. The
// caching freezer does NOT call it: multi-key caching keeps the candidates SEPARATE.
func MergeRepresentationVariableNodes(objects []*resolve.Object) *resolve.Object
```

This refactor is its OWN early structural commit (it precedes the caching passes), it touches `graphql_datasource` (the two call sites move to the exported names) and `plan/representationvariable` (the moved code), and it MUST preserve graphql_datasource behavior — guarded by the existing `graphql_datasource/representation_variable_test.go`, which exercises the same code paths through the call sites and pins the output byte-for-byte (PLAN note for §15).
No import cycle: `representationvariable` imports `plan`/`resolve`/`ast`/`astvisitor`; `plan` does not import it; both consumers already depend on `plan`.

The freezer then reuses the shared builder and freezes EVERY resolvable `@key` set as an independent, best-effort candidate (multi-key, §6.3) in ONE reviewable file:

```go
// postprocess/cache_key_spec_freezer.go
//
// cacheKeySpecFreezer is the SOLE federation -> caching boundary crossing. It reads
// FederationMetaData as plan-time INPUT and emits a multi-key resolve.CacheKeySpec BY
// VALUE. It retains NO pointer into FederationMetaData; the result is self-contained
// runtime data (RFC-1 §7.4). It builds each candidate via the shared
// representationvariable package (§6.1), so caching and the data source share one
// @key->representation implementation.
type cacheKeySpecFreezer struct {
	federation map[string]plan.FederationMetaData // by DataSourceID, read-only
	definition *ast.Document                      // schema, threaded in (§5.3); read-only
}

// freeze returns the frozen multi-key spec for a fetch, or ok=false when the fetch has
// no datasource/coordinate identity. Called once per fetch by the stamper.
func (f *cacheKeySpecFreezer) freeze(scope resolve.CacheScope, info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false // tolerate Info==nil (DisableIncludeInfo)
	}
	fed, ok := f.federation[info.DataSourceID]
	if !ok {
		return resolve.CacheKeySpec{}, false
	}
	typeName := info.RootFields[0].TypeName
	spec := resolve.CacheKeySpec{
		Scope:     scope,
		TypeName:  typeName,
		FieldName: info.RootFields[0].FieldName,
	}
	if scope == resolve.CacheScopeEntity {
		if !fed.HasEntity(typeName) {
			return resolve.CacheKeySpec{}, false // no @key -> no entity spec
		}
		// MULTI-KEY: freeze one candidate per resolvable @key set; none is privileged
		// as "required". Each candidate is INDEPENDENTLY, best-effort renderable at
		// RUNTIME (RFC-1 §2.10, §3.7, §6.3) — a candidate whose fields are absent in
		// the data is simply not rendered at lookup and is retried at write. Ordered
		// deterministically by selection-set string (§13.1).
		keySets := fed.RequiredFieldsByKey(typeName) // = Keys.FilterByTypeAndResolvability(typeName, true)
		slices.SortFunc(keySets, func(a, b plan.FederationFieldConfiguration) int {
			return strings.Compare(a.SelectionSet, b.SelectionSet)
		})
		for _, keySet := range keySets {
			node, err := representationvariable.BuildRepresentationVariableNode(f.definition, keySet, fed)
			if err != nil {
				continue // defensive: skip a malformed @key fragment rather than fail the whole spec
			}
			spec.Candidates = append(spec.Candidates, resolve.CacheKeyCandidate{Representation: node})
		}
		if len(spec.Candidates) == 0 {
			return resolve.CacheKeySpec{}, false // no usable @key -> no entity spec
		}
	}
	spec.EntityKeyMappings = freezeEntityKeyMappings(fed, typeName) // root-arg <-> @key, by value
	return spec, true
}
```

Each `resolve.CacheKeyCandidate.Representation` is the `*resolve.Object` the shared builder returns — federation-pointer-free, with the `__typename`/interfaceObject/entityInterface remap baked in, so the candidate is a complete key template on its own (it subsumes the old single-key `KeyFields`+`TypenameOverride` pair).
After `freeze` returns, nothing in the runtime path holds a reference into `FederationMetaData`: `CacheKeySpec` is strings + `[]CacheKeyCandidate` (value trees) + `[]EntityKeyMapping` (value types).
This is the PR2 guarantee: one crossing, plan-time, by value, no pointers retained into runtime — and now ONE shared builder, so caching and the data source cannot drift.

### 6.2 Why RFC-2 freezes the key spec and does NOT call `provider.KeySpec()`

RFC-1 §7.3's `CacheConfigProvider` exposes a `KeySpec(...)` method, and RFC-1 §7.2 carries `CachingConfiguration.KeySpecs`.
RFC-2 DELIBERATELY does not consume either: it freezes `CacheKeySpec` itself, reading `FederationMetaData` once (sanctioned by RFC-1 §7.4: "read ONCE by RFC-2 and frozen into `CacheKeySpec`").
This preserves the single-source-of-truth key invariant — the cache key mirrors entity identity, so `@key` is the only legitimate source; routing it through the provider would create a second source that could silently diverge.
A reviewer who sees the unused `KeySpec()` method should read it as a forward-compatibility hook, not a contract gap; this is stated so the freeze decision is explicit (a reviewer must-fix).

### 6.3 The multi-key model (canonical, shared with RFC-1)

An entity type may declare MORE THAN ONE `@key` set, and RFC-2 does NOT make them all required.
The freezer freezes ALL resolvable `@key` sets into RFC-1's revised multi-key `CacheKeySpec.Candidates` — one independently, best-effort-renderable candidate per set, none privileged as "required":

- LOOKUP (runtime `PrepareFetch`, RFC-1 §2.10): render EVERY candidate that CAN be rendered from the data available at fetch time; some candidates may be unrenderable because their fields are absent, and that is allowed; look the cache up under ALL successfully-rendered keys; a hit on ANY rendered candidate serves the item.
- WRITE / populate (runtime `OnFetchResult`, RFC-1 §3.7): after the subgraph response returns, RE-render the candidates that were NOT renderable at lookup time using the freshly returned data; if more candidates are now renderable, populate the cache for ALL renderable keys, otherwise populate just the keys rendered at lookup.

This generalizes and REPLACES the old single-key "requested-key vs rendered-key + missing-key set" write-back framing — RFC-2 expresses key derivation purely as a best-effort multi-key render-then-backfill, and the per-item handle payload (RFC-1 §3.7) carries the set of keys rendered at lookup plus the set still-unrendered (retried at write).
RFC-2's only obligation is to freeze the full candidate list by value; the runtime render/backfill is RFC-1's.

---

## 7. Stamping onto the three concrete fetch types after createConcreteSingleFetchTypes (PR3)

`createConcreteSingleFetchTypes` converts a `*SingleFetch` into a `*EntityFetch` (`create_concrete_single_fetch_types.go:104`) or `*BatchEntityFetch` (`:59`); both embed only `FetchDependencies` (`fetch.go:166`/`:206`), NOT `FetchConfiguration`, and copy `Info` BY REFERENCE (`:71`/`:116`).
So config stamped on the raw `SingleFetch.FetchConfiguration` before conversion would be LOST for the entity types.
RFC-1 therefore put `Cache *FetchCacheConfig` directly on all three concrete structs (RFC-1 §3.8, §4.1 row n), and RFC-2 runs the stamper AFTER conversion, writing the concrete types directly.

```go
// postprocess/cache_config_stamper.go
type cacheConfigStamper struct {
	providers map[string]cacheconfig.CacheConfigProvider
	freezer   *cacheKeySpecFreezer
}

// process walks one fetch tree and stamps each cache-eligible concrete fetch.
func (s *cacheConfigStamper) process(node *resolve.FetchTreeNode, pd map[*resolve.FetchInfo]*resolve.Object) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		s.stamp(node.Item.Fetch, pd)
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			s.process(child, pd)
		}
	}
}

func (s *cacheConfigStamper) stamp(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) {
	cfg := s.buildConfig(fetch, pd) // nil => not eligible
	if cfg == nil {
		return // leave Cache nil so the loader no-ops (PR8)
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		f.Cache = cfg
	case *resolve.EntityFetch:
		f.Cache = cfg
	case *resolve.BatchEntityFetch:
		f.Cache = cfg
	}
}
```

`buildConfig` is where policy lookup, the §6 freeze, and the §9 `ProvidesData` attach come together.
It consumes RFC-1's `*CachePolicy` types VERBATIM and reads ONLY their real fields (the must-fix against inventing `AllowL1`/`EnableL2`/`EntityKeyMappings()`):

```go
func (s *cacheConfigStamper) buildConfig(fetch resolve.Fetch, pd map[*resolve.FetchInfo]*resolve.Object) *resolve.FetchCacheConfig {
	info := fetch.FetchInfo()
	if info == nil || len(info.RootFields) == 0 {
		return nil // tolerate Info==nil (DisableIncludeInfo) -> no key material -> no-op
	}
	provider := s.providers[info.DataSourceID]
	if provider == nil {
		return nil // this datasource has no caching configured -> leave Cache nil
	}

	var cfg resolve.FetchCacheConfig
	switch {
	case fetchIsEntity(fetch): // *EntityFetch / *BatchEntityFetch / SingleFetch w/ RequiresEntity*(fetch.go:262/266)
		pol, ok := provider.EntityPolicy(info.RootFields[0].TypeName) // RFC-1 §7.3, verbatim
		if !ok {
			return nil
		}
		spec, ok := s.freezer.freeze(resolve.CacheScopeEntity, info)
		if !ok {
			return nil
		}
		cfg = resolve.FetchCacheConfig{
			L1: true, // L1-ELIGIBLE; optimizeL1Cache (P3) narrows to canRead||canWrite (§10)
			L2: pol.TTL > 0 || pol.NegativeCacheTTL > 0, // derived from real fields (see §7.1)
			CacheName:                   pol.CacheName,
			TTL:                         pol.TTL,
			NegativeCacheTTL:            pol.NegativeCacheTTL,
			IncludeSubgraphHeaderPrefix: pol.IncludeSubgraphHeaderPrefix,
			EnablePartialCacheLoad:      pol.EnablePartialCacheLoad,
			ShadowMode:                  pol.ShadowMode,
			HashAnalyticsKeys:           pol.HashAnalyticsKeys,
			KeySpec:                     spec,
		}
	default: // root-field fetch
		pol, ok := rootFieldPolicyForAllRootFields(provider, info) // all-or-nothing rule, §7.2
		if !ok {
			return nil
		}
		spec, _ := s.freezer.freeze(resolve.CacheScopeRootField, info)
		cfg = resolve.FetchCacheConfig{
			L1: false, // root fields only ACT as L1 providers (root->entity promotion, v2)
			L2: pol.TTL > 0,
			CacheName:                   pol.CacheName,
			TTL:                         pol.TTL,
			IncludeSubgraphHeaderPrefix: pol.IncludeSubgraphHeaderPrefix,
			ShadowMode:                  pol.ShadowMode,
			PartialBatchLoad:            pol.PartialBatchLoad,
			KeySpec:                     spec,
		}
	}

	cfg.ProvidesData = pd[info]                 // from P1 (§9); consumed at runtime by the coverage walk
	cfg.HasAliasesFoldIntoProvidesData()        // fold computeHasAliases into the stamper (graft from C)
	if !cfg.L1 && !cfg.L2 && !cfg.ShadowMode {
		return nil // nothing to do -> leave Cache nil so the loader no-ops (PR8)
	}
	return &cfg
}
```

`fetchIsEntity` keys off the concrete Go type (`*EntityFetch`/`*BatchEntityFetch`) or `RequiresEntityFetch`/`RequiresEntityBatchFetch` on a `SingleFetch` (`fetch.go:262`/`:266`).
`computeHasAliases` is re-added as a small caching-package function and folded into the stamper as ONE call on the freshly assembled `cfg.ProvidesData` (graft from C, avoiding over-fragmentation); it sets `cfg.ProvidesData.HasAliases` if any descendant carries an `OriginalName`/`CacheArgs`.

### 7.1 L2 enablement is derived from the policy's real fields (a reviewer must-fix)

RFC-1 §7.2's `EntityCachePolicy` has NO L1/L2 bool — its fields are `{TypeName, CacheName, TTL, NegativeCacheTTL, IncludeSubgraphHeaderPrefix, EnablePartialCacheLoad, HashAnalyticsKeys, ShadowMode}`, and `RootFieldCachePolicy` is `{TypeName, FieldName, CacheName, TTL, IncludeSubgraphHeaderPrefix, ShadowMode, PartialBatchLoad}`.
RFC-2 therefore DERIVES `cfg.L2` structurally: `pol.TTL > 0 || pol.NegativeCacheTTL > 0` for entities, `pol.TTL > 0` for root fields.
It NEVER reads non-existent fields like `AllowL1`/`EnableL2`/`EntityKeyMappings()` (draft A's verbatim violation).
`cfg.L1` is set STRUCTURALLY (entity fetch ⇒ eligible `true`; root field ⇒ `false`), then NARROWED by P3 (§10).
Open question (flag for RFC-1 authors, §15): confirm L2 should be inferred from TTL rather than carried as an explicit enable signal; if an explicit bool is intended, it must be added to the `*CachePolicy` structs in RFC-1, not invented here.

### 7.2 The root-field all-or-nothing rule (replacing the path-builder isolation hook)

The OLD branch forced each cached query root field into its own fetch via `isolatedRootField` in `path_builder_visitor.go` (analysis-A §2.2) so a fetch never mixed root fields with different policies; absent isolation, OLD's fallback rule was to DISABLE L2 when root fields in a fetch did not share an identical policy (dossier §5.5).
`path_builder_visitor.go` is in the PR1 forbidden list, so RFC-2 does NOT re-add that hook.
Instead `rootFieldPolicyForAllRootFields` walks `info.RootFields`, and returns `(policy, true)` ONLY when every root field resolves to the SAME policy; otherwise it returns `false` and the stamper leaves `Cache` nil.
This reproduces OLD's conservative fallback (disable rather than mis-cache) additively, with zero edits to the path builder.

Per-root-field fetch isolation (the OPTIMIZATION that let differing root fields each cache, OLD `isolatedRootField`) is NOT dropped: it is DEFERRED to a SEPARATE RFC, RFC-03 (`docs/caching/specs/2026-06-30-rfc-03-per-root-field-cache-isolation.md`).
That RFC will specify the additive mechanism (a pre-planning / path-building isolation that forces each cached root field into its own fetch so differing policies can each cache) without re-opening the PR1 forbidden visitor bodies.
v1 ships the conservative all-or-nothing decline above (correct, just less granular); RFC-03 is a pure optimization layered on top and changes none of RFC-2's v1 contract.

---

## 8. Fetch-tree walking: root + defer groups + subscription + DeferID + request-independence (PR4)

A plan has N fetch trees, not one (analysis-B §5, dossier §7 risk 14): the root tree plus one per `DeferFetchGroup` (`response.go:105`), plus the subscription tree.
The facade `Annotate` is invoked once per plan and stamps EVERY tree:

```go
// AnnotateFetchTree stamps each tree (root + every defer group) and then runs the
// CROSS-tree L1 pass once over all of them. The single NO-OP gate lives here.
func (c *cachingPlanner) Annotate(resp *resolve.GraphQLResponse, trees ...*resolve.FetchTreeNode) {
	if c == nil || len(c.providers) == 0 || resp == nil {
		return // composed NO-OP gate (§11): no provider => zero plan change
	}
	pd := resp.CacheProvidesData() // the *FetchInfo-keyed side-table from P1 (§9.3)
	for _, tree := range trees {
		c.stamper.process(tree, pd)
	}
	c.l1.processTrees(trees...) // cross-tree narrowing (§10)
}
```

`DeferID` identity (RFC-1 §8.6, dossier §6.5):
RFC-2 does NOT need to reason about `DeferID` itself.
`EqualSingleFetch` already refuses to dedup across defer scopes (`fetch.go:49-53`, the `DeferID` guard at `:51`), and `extractDeferFetches` (`postprocess.go:205`) has already split the flat tree into per-`DeferID` groups BEFORE `Annotate` runs.
Each group is a distinct tree of distinct fetches with their own `*FetchInfo` pointers, so stamping each tree independently is automatically `DeferID`-correct, and the same physical fetch never appears in two trees (no double-stamp).

Request-independence (RFC-1 §8.5, dossier §6.5):
the postprocessed plan is cached and reused across requests (`execution_engine.go:304-305`).
The stamper writes ONLY static config: flags, TTLs, `CacheName`, the frozen `KeySpec` (value data), and the `ProvidesData` tree (request-independent plan data).
It derives NO per-request key material — variable values, header hash, and global prefix are all derived at runtime inside `PrepareFetch` (RFC-1 §3.6, §8.5).
P3 reads only static structure (`ProvidesData` + `DependsOnFetchIDs`).
So the cached plan is safe to share.

---

## 9. ProvidesData: RE-ADD a dedicated visitor (PR5)

### 9.1 Decision: re-add the visitor, do NOT derive

RFC-2 re-introduces a dedicated plan-time `ProvidesData` visitor (P1) and does NOT derive `ProvidesData` from the merged response `Data` tree in a post-pass.
This is the correctness-critical decision; the justification is grounded in verified source (the lossiness analysis grafted from draft A, now proven against the defer branch):

1. After merge, per-fetch attribution is LOST.
   `mergeFields` (`postprocess.go:193`) collapses fields from multiple planners into one `*resolve.Object`, and `FieldInfo.Merge` (VERIFIED `node_object.go:173-185`) merges only `ParentTypeNames` and `Source.IDs` — it does NOT merge `FetchID`, so a `@shareable`/multi-subgraph field keeps a SINGLE surviving `FetchID`.
   Partitioning the merged tree by `Info.FetchID` therefore yields a TOO-SMALL `ProvidesData` for shared fields.
   At runtime the coverage walk would then wrongly accept an incomplete cached value as a full hit — a real under-coverage stale/incomplete serve, not a missed hit (the reviewer's blocking objection to deriving).

2. The merged response `Field` is ARG-BLIND.
   `resolve.Field` (VERIFIED `node_object.go:103-112`) carries `Name`, `Value`, `OnTypeNames`, `Info` — but NO arguments.
   RFC-1 §2.10 requires alias-AND-arg-aware coverage (a value cached under `friends_<hashA>` must not satisfy `friends(first:20)` = `friends_<hashB>`).
   A derived tree cannot recover `CacheArgs`, so arg-blind coverage can SERVE STALE values — a correctness hole.

3. The three OLD normalizations need WALK-TIME context the flat tree has lost: entity-boundary detection (enclosing type + planner transition), `__typename` injected-vs-requested disambiguation, and inline-fragment path normalization all live at field-visit time (OLD `caching_planner_state.go:91-370`, analysis-A §2.1).

The visitor gets per-field fetch ownership for FREE from `planningVisitor.fieldPlanners` (the same field the cost visitor reads, `planner.go:177`), captures alias+args at field-visit time, and is a verbatim port of OLD `trackFieldForPlanner`/`popFieldsForPlanner`/`createFieldValueForPlanner` — so it reproduces the runtime coverage input byte-for-byte.
The cost is one extra plan-time visitor reading a read-only planner output; that is a strictly smaller, better-understood surface than re-deriving ownership + three normalizations + args from a different data structure.

### 9.2 The registration mechanism (verified, a reviewer must-fix)

P1 is a NEW visitor file registered as a PEER on the EXISTING `planningWalker`, NOT a standalone pre-/parallel walker.
This is the ONLY mechanism that can see per-field fetch ownership: `fieldPlanners` is the planning visitor's own map (`visitor.go:64-66`), populated during the walk (`visitor.go:232-235`).
A separate `prepareOperationWalker`-style walk (`planner.go:57-60`) runs before planner ownership exists and could not produce correct `ProvidesData` — so a standalone own-walker pre-planning pass would be wrong for P1, and the cost-visitor peer pattern is right.
The precedent is VERIFIED: the `CostVisitor` registers on the same `planningWalker` via `RegisterEnterFieldVisitor`/`RegisterLeaveFieldVisitor` (`planner.go:180-181`), reads `p.planningVisitor.fieldPlanners` (`:177`), and the code comment (`:170-174`) says it must be registered LAST because it depends on `fieldPlanners`.
P1 registers right after it, identically (§5.2).

```go
// plan/cache_provides_data_visitor.go (body ported from OLD caching_planner_state.go)
type cacheProvidesDataVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	planners              []PlannerConfiguration
	fieldPlanners         map[int][]int           // fieldRef -> plannerIDs (read-only, from planningVisitor)
	objects               map[int]*resolve.Object // fetchID -> root ProvidesData object
}

func (v *cacheProvidesDataVisitor) EnterField(ref int) {
	for _, plannerID := range v.fieldPlanners[ref] {
		v.trackField(plannerID, ref) // append a resolve.Field with OriginalName + CacheArgs (from the operation AST);
		                             // descend object frames; entity-boundary reset (OLD trackFieldForPlanner)
	}
}
func (v *cacheProvidesDataVisitor) LeaveField(ref int) {
	for _, plannerID := range v.fieldPlanners[ref] {
		v.popField(plannerID, ref) // OLD popFieldsForPlanner
	}
}
```

### 9.3 How the tree is built and attached (the `*FetchInfo` carrier)

`ProvidesData` is built at plan time but consumed by the post-pass.
The carrier is a side-table `map[*resolve.FetchInfo]*resolve.Object`, NOT a fetchID-keyed map and NOT a re-added `FetchInfo.ProvidesData` field.
Using a side-table keyed by `*FetchInfo` (B's mechanism) rather than re-adding `FetchInfo.ProvidesData` (C's mechanism) honors RFC-1 §3.3, which deliberately avoids that field.

`*FetchInfo` is the identity-stable key across the plan→postprocess boundary (VERIFIED):

- `deduplicateSingleFetches` keeps the SURVIVOR's `Item` and its `Info` pointer; deduped duplicates have identical selections, so the survivor's `ProvidesData` is correct.
- `fetchIDAppender` does not reassign `FetchID` and never touches `Info`; even if it did, the `*FetchInfo` key is immune to a fetchID change.
- `createConcreteSingleFetchTypes` copies `Info` BY REFERENCE into the concrete types (`Info: fetch.Info`, `create_concrete_single_fetch_types.go:71`/`:116`), so the `*EntityFetch`/`*BatchEntityFetch` expose the SAME `*FetchInfo` pointer the plan-time visitor saw.

So the stamper does `pd[fetch.FetchInfo()]` and gets the right tree.
`attachTo` resolves fetchID→`*FetchInfo` once, after the walk, while fetchIDs still equal the planner IDs the visitor keyed on:

```go
// attachTo converts the fetchID-keyed map to a *FetchInfo-keyed side-table on the
// plan, the carrier the stamper consumes (§9.3). Iterating v.objects is sorted by
// fetchID for determinism (§13.1).
func (v *cacheProvidesDataVisitor) attachTo(p Plan) {
	resp := responseOf(p) // *GraphQLResponse for sync/defer/subscription
	out := make(map[*resolve.FetchInfo]*resolve.Object, len(v.objects))
	for _, fetchID := range sortedKeys(v.objects) {
		if info := infoForFetchID(resp, fetchID); info != nil {
			out[info] = v.objects[fetchID]
		}
	}
	resp.SetCacheProvidesData(out)
}
```

The carrier is a transient, request-independent, unexported field on `resolve.GraphQLResponse` with two accessors (additive, changes no existing behavior):

```go
// on resolve.GraphQLResponse (additive, unexported; request-independent plan data)
cacheProvidesData map[*FetchInfo]*Object

func (r *GraphQLResponse) SetCacheProvidesData(m map[*FetchInfo]*Object) { r.cacheProvidesData = m }
func (r *GraphQLResponse) CacheProvidesData() map[*FetchInfo]*Object     { return r.cacheProvidesData }
```

This avoids response-tree `Copy()` drift entirely (PR10, §13): the `ProvidesData` tree is its OWN object tree, never part of the response `Data` tree that defer's `build_defer_tree` deep-copies.
A regression assertion verifies the stamp pass resolves the right tree via `fetch.FetchInfo()` after dedup + appendFetchID + conversion (§14).

---

## 10. The dedicated L1-optimize pass (PR6)

`optimizeL1Cache` (P3) is its OWN single-responsibility `FetchTreeProcessor` (`postprocess/optimize_l1_cache.go`), a faithful re-home of OLD `optimize_l1_cache.go` (analysis-A §3).
It is a PURE NARROWING pass: the stamper sets `cfg.L1 = true` for L1-eligible entity fetches (§7), and P3 REFINES it to `cfg.L1 = canRead || canWrite`, turning L1 OFF where it cannot help.
This makes the "eligibility then narrow" description match the code (a reviewer must-fix against B's earlier ambiguity): the stamper is the sole eligibility setter, P3 the sole narrower, and P3 never turns L1 on.

It runs AFTER P2 so every cache-eligible fetch already carries a `*FetchCacheConfig` (with `ProvidesData`), and reads from there instead of the removed `FetchInfo.ProvidesData`:

- collect entity fetches across ALL trees: `*EntityFetch`, `*BatchEntityFetch`, or `*SingleFetch` with `RequiresEntityFetch`/`RequiresEntityBatchFetch` (`fetch.go:262`/`:266`), with `entityType = info.RootFields[0].TypeName`, `providesData = fetch.Cache.ProvidesData`, `dependsOn = deps.DependsOnFetchIDs` (port of OLD `collectEntityFetches`/`extractEntityFetchInfo`, OLD `optimize_l1_cache.go:76-146`).
- for each entity fetch, `cfg.L1 = canRead || canWrite`:
  - `canRead` = a prior fetch (or the union of priors) of the same entity type provides a SUPERSET of this fetch's fields (`hasValidProvider`/`collectAncestorUnion`, OLD `:229-265`, `:461-537`).
  - `canWrite` = a later fetch of the same type needs a SUBSET of this fetch's `ProvidesData` (`hasValidConsumer`, OLD `:271-297`).
  - ordering resolved purely from `DependsOnFetchIDs` (`executesBefore`/`isInDependencyChain`, OLD `:300-337`).

The field-coverage primitives (`objectProvidesAllFields`, `nodeProvidesAllFields`, `treeContainsAllFields`, `unionObjects`, OLD `:353-572`) port verbatim — they already operate on `resolve.Object`/`resolve.Field` (`node_object.go:8`/`:103`).
`setL1` is the OLD `setUseL1Cache` three-arm switch re-targeted onto `cfg.L1`, guarding `f.Cache != nil`:

```go
func (o *optimizeL1Cache) setL1(fetch resolve.Fetch, value bool) {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		if f.Cache != nil { f.Cache.L1 = value }
	case *resolve.EntityFetch:
		if f.Cache != nil { f.Cache.L1 = value }
	case *resolve.BatchEntityFetch:
		if f.Cache != nil { f.Cache.L1 = value }
	}
}
```

### 10.1 Cross-tree collection (graft from A)

`processTrees(roots ...*resolve.FetchTreeNode)` collects entity fetches across the root tree AND every `Defers[i].Fetches` in a SINGLE pass, then runs the subset/superset decision over the combined set:

```go
func (o *optimizeL1Cache) processTrees(roots ...*resolve.FetchTreeNode) {
	if o.disable {
		return
	}
	var entities []*entityFetchInfo
	for _, r := range roots {
		entities = append(entities, o.collectEntityFetches(r)...) // across all trees
	}
	for _, ef := range entities {
		if !cacheL1Eligible(ef.fetch) { // stamper marked it ineligible -> nothing to narrow
			continue
		}
		if !o.hasValidProvider(ef, entities) && !o.hasValidConsumer(ef, entities) {
			o.setL1(ef.fetch, false)
		}
	}
}
```

The L1 store lives for the request lifetime and is shared across defer groups (the request-lifetime L1 store, RFC-1 §6.2), so a root-tree entity fetch can provide L1 to a defer-group consumer.
A per-tree pass would turn L1 off for a root provider whose only consumer lives in a defer group (safe but suboptimal); the cross-tree pass captures these provider/consumer pairs.
Narrowing is conservative-safe in both directions: turning `cfg.L1` off only forgoes an optimization, never breaks correctness — so even if a future change reorders defer collection, the worst case is a missed L1 hit, never incorrect data.
Root-field→entity L1 promotion (OLD `:148-222`, `RootFieldL1EntityCacheKeyTemplates`) is staged to v2 with the rest of root-field L1; v1 ships the entity subset/superset decision, which the loader treats as a cache miss when a promotion is absent (correct, just less optimal).

### 10.2 The all-false residual `Cache` after narrowing

The stamper sets `cfg.L1 = true` for every entity-eligible fetch BEFORE P3 runs, so an entity fetch always carries a non-nil `Cache`.
When P3 then narrows `cfg.L1` off and the policy also left `L2` and `Shadow` false, the fetch is left holding a non-nil but all-false `FetchCacheConfig`.
This is harmless: RFC-1's runtime controller no-ops a config with no active layer (it produces no lookup and no write), so behaviour is identical to a nil `Cache`.
For plan cleanliness and to keep the no-op golden diff minimal, P3 MAY re-null such a fully-inert `Cache` as its last step (`if !cfg.L1 && !cfg.L2 && !cfg.Shadow { f.SetCache(nil) }`); this is an optional tidy, not a correctness requirement.

---

## 11. Separation of concerns (PR7) and composed NO-OP gating (PR8)

### 11.1 Separation of concerns

The OLD design failed review because it was a MEGA-VISITOR: `cachingPlannerState` (943 lines) fused into the primary walk, stamping four node kinds from inside `EnterField`/`LeaveField`/`resolveFieldValue`/`configureFetch` (analysis-A §2.1).
RFC-2 decomposes it along the axis that makes each piece independently reviewable and testable, plus a thin facade:

| Concern | Owner | Why it lives there | Could it move? |
|---|---|---|---|
| Build `ProvidesData` (P1) | peer visitor on `planningWalker` | Needs walk-time context + per-planner ownership (§9.1) | No — only phase with that context |
| Orchestrate + gate (facade) | `cachingPlanner` | Logic-free sequencing + the single NO-OP gate | n/a |
| Freeze `@key` (H) | `cacheKeySpecFreezer` | The SOLE federation reader; pure value output | Helper invoked by the stamper; own file for review |
| Assemble + stamp (P2) | `cacheConfigStamper` | Pure function of the finished concrete tree + policy + P1 output | No earlier (needs concrete types, §7) |
| L1 cross-fetch decision (P3) | `optimizeL1Cache` | Pure function of stamped `ProvidesData` + dependency ordering | No earlier (needs all configs present, §10) |

Granularity justification (not too coarse, not over-fragmented):

- NOT one pass: a single "do all caching" pass would re-fuse pre-planning, walk-time, and post-plan concerns — the OLD anti-pattern.
- NOT ten passes: the freezer is a helper INSIDE the stamper's traversal (one walk freezes and stamps, two testable units), and `computeHasAliases`/the `ProvidesData`→`cfg` copy are folded into the stamper (single calls, no independent meaning) rather than spun out (graft from C).
- L1 stays SEPARATE from stamp because it is a CROSS-fetch decision (reads OTHER fetches) whereas stamp is PER-fetch — different inputs, different testability, different reasons to change.
- Federation is touched in exactly ONE file (H), so "does caching leak federation into runtime" is a one-file review.
- Caching touches the new files plus the additive registrations; the five forbidden visitor files show ZERO diff.

### 11.2 CacheConfigProvider integration (consume RFC-1 verbatim)

RFC-2 declares the plan-side `cacheconfig` package implementing RFC-1 §7.2-7.3's EXACT type shapes — adding no fields, inventing none — and consumes them.
The provider is placed in the plan-side `cacheconfig` package, NOT in `resolve` (the must-fix against draft C): RFC-1 §7.2-7.3 put the policy model and provider plan-side, decoupled from `FederationInfo`, preserving the one-way `resolve`→`cacheconfig` direction (`resolve` must not import the policy types).

```go
// plan/cacheconfig/provider.go — RFC-1 §7.3, declared per spec, consumed verbatim.
type CacheConfigProvider interface {
	EntityPolicy(typeName string) (EntityCachePolicy, bool)
	RootFieldPolicy(typeName, fieldName string) (RootFieldCachePolicy, bool)
	MutationPolicy(fieldName string) (MutationCachePolicy, bool)
	SubscriptionPolicy(typeName, fieldName string) (SubscriptionCachePolicy, bool)
	KeySpec(scope resolve.CacheScope, typeName, fieldName string) (resolve.CacheKeySpec, bool) // present per spec; unused by RFC-2 (§6.2)
}
```

The provider is reached parallel to (not on) `FederationInfo`, via the accessor RFC-1 §7.3 specifies: `func (d dataSourceConfiguration[T]) Caching() CacheConfigProvider` — nil when caching is not configured.

### 11.3 Composed NO-OP gating — mode mismatch impossible by construction

Two independent gates compose (RFC-1 §10.1):

| | planner NO-OP (every `Cache` nil) | planner ON (configs stamped) |
|---|---|---|
| runtime NO-OP (`cacheController == nil`) | nothing happens (default) | configs present, never consulted → NO-OP |
| runtime ON (controller set) | controller present, `PrepareFetch` never called (cfg nil) → NO-OP | caching active |

RFC-2 owns the planner column; it gates at FOUR points, all defaulting OFF:

1. empty provider map ⇒ `Annotate` returns before touching a node; P1 is never registered (§5).
2. a datasource with `provider == nil` ⇒ `buildConfig` returns nil, `Cache` stays nil (§7).
3. a `(type[,field])` the provider returns `(_, false)` for ⇒ `buildConfig` returns nil (§7).
4. a config with `!L1 && !L2 && !ShadowMode` ⇒ `buildConfig` returns nil (§7).

The runtime side gates at `cacheController == nil` and `item.Fetch.Cache == nil` (RFC-1 §10.1).
The LOWER gate always wins and the lowest is NO-OP, so "L1-only runtime + NO-OP planner" and "configs stamped + no controller" both degrade to a full NO-OP.
A real cache requires BOTH a non-nil controller AND a non-nil per-fetch config — there is no third switch (RFC-1 §10.1).
This is the gate the OLD branch lacked: `DisableEntityCaching` only disabled L2 while still building L1 + key templates (dossier §5.5).

### 11.4 Strict no-op before any provider exists

With no `EnableCaching(...)` option: `opts.cacheProviders` is empty, the facade returns at its guard, P1 is not constructed/registered, every fetch's `Cache` stays nil, and RFC-1's `FetchConfiguration.Equals` clause sees both-`Cache`-nil and returns its prior result (RFC-1 §3.8).
Merging RFC-2's code ALONE — before any provider is wired — changes plans in ZERO ways (proven by the no-op golden test, §14).

---

## 12. @requestScoped: out of scope entirely (PR9)

The `@requestScoped` DIRECTIVE FEATURE is OUT OF SCOPE entirely — removed by review, not staged.
RFC-2 ships NO `@requestScoped` support in v1 OR v2: there is no `cacheRequestScopedRewrite` pre-planning pass, no rename-map response post-pass, no `RequestScopedPolicy` on the provider, no `RequestScoped` field on the stamped config, and no reference to the OLD `node_selection_visitor_request_scoped.go` widening rewrite.
The OLD pre-planning widening (`collectRequestScopedParticipants`/`computeRequestScopedMissing`/`addFieldRequirementsToOperation`, `visitor.go:396-403/425-427`) is therefore NOT ported.

Every other caching capability in RFC-1's v1 set works without it (entity L1/L2, root-field response cache, coverage/freshness/reorder, negative caching, shadow), so removing the feature changes nothing else in this RFC.

Important distinction (do not conflate the two):
removing the `@requestScoped` directive feature does NOT touch the request-lifetime L1 store — the per-request L1 cache state that lives for the lifetime of ONE request and is shared by reference across the per-defer-group Loaders (RFC-1 §6.2).
That shared request cache state is intrinsic to L1 caching (the final implementation phase) and is RETAINED; it is the reason `optimizeL1Cache` collects entity fetches cross-tree (§10.1).
Only the directive-driven field-widening feature is gone.

---

## 13. Determinism, plan cache, DeferResponsePlan, per-node annotations, Copy()/side-table (PR10)

### 13.1 Determinism

Every pass emits identical output for identical input across runs, or the cached plan (`execution_engine.go:304-305`) becomes nondeterministic.
Rules RFC-2 follows:

- the fetch-tree walk is structural (Single/Parallel/Sequence recursion, `fetchtree.go:17-22`), so output follows tree order, not map order.
- wherever a pass iterates a Go map (P1's `objects` in `attachTo`, P3's per-type provider sets), it SORTS by a stable key (fetchID, then coordinate) before producing output — the precedent is `inverseMap` sorting "for deterministic plans/tests" (`planner.go:159`).
- the freezer orders the multi-key `Candidates` deterministically (sort the `@key` sets by selection-set string before building, §6.1) so the candidate list is stable; it does NOT merge variants (multi-key keeps them separate). `CacheArgs` is sorted on the `ProvidesData` tree (OLD `caching_planner_state.go:165-170`).
- `EntityKeyMappings` and each candidate's representation node are value-cloned (the shared builder allocates fresh `resolve` nodes), so no shared mutable state leaks between plans.

### 13.2 Plan cache and DeferResponsePlan

Covered in §8: only static config is written; the third plan kind `DeferResponsePlan` (`plan.go:66`) is handled by stamping the root tree plus each `Defers[i].Fetches` (`postprocess.go:211-213` mirror) before `buildDeferTree` nils `Defers`; N trees per plan are all walked; the cross-tree L1 pass spans them.

### 13.3 Per-object / per-field annotations and the Copy() interaction

RFC-2 AVOIDS the defer `Copy()` drift (dossier §7 risk 9) by construction:

- The L1-normalize metadata (`OriginalName`, `CacheArgs`, `HasAliases`) is written ONTO THE CACHING-OWNED `ProvidesData` TREE, not the response tree.
  That tree is a SEPARATE `*resolve.Object` built by P1 and carried in `FetchCacheConfig.ProvidesData`; it is NEVER reached by defer's `build_defer_tree`/`Object.Copy()`/`Field.Copy()` (`node_object.go:18`/`:119`), which walk only the response `Data` tree.
  So even though §5.4 adds these fields to `resolve.Object`/`resolve.Field` and updates `Copy()` additively, there is no shared-node `Copy()` interaction with defer on the tree that actually carries the metadata.
- Response-tree analytics annotations (`Object.CacheAnalytics`, `Field.CacheAnalyticsHash`) are NOT written in v1, because RFC-1 ships the analytics observer NIL (RFC-1 §3.5, §9; the walker hooks are v2).
  So v1 writes NOTHING onto the shared response tree.
  When analytics lands in v2, those annotations use a NODE-KEYED SIDE-TABLE (keyed by stable coordinate) rather than fields on `Object`/`Field` (the explicit graft from C), dodging the defer shared-node copy hazard entirely.

This is the PR10 resolution: per-field cache metadata lives on the caching-owned `ProvidesData` side-tree (immune to defer `Copy()`), the plan→postprocess carrier is a `*FetchInfo`-keyed side-table (§9.3, immune to fetchID reassignment and to response-tree `Copy()`), and the shared response tree is left untouched in v1.

---

## 14. Testing strategy

Per-pass unit tests (each pass is a small, asserted-output unit, §11.1; assertions follow the repo convention — assert the FULL value with `assert.Equal`, never `Contains` or fuzzy comparison):

- H `cacheKeySpecFreezer`: table tests over `(FederationMetaData, definition, scope, type[, field])` → assert the FULL multi-key `resolve.CacheKeySpec` value inline (single `@key` → one candidate, composite `@key`, nested object `@key`, MULTIPLE `@key` sets → one independent candidate each, deterministically ordered, none required; interfaceObject/entityInterface `__typename` baked into the candidate representation; no-`@key` → `(zero, false)`).
  Mutate the source `FederationMetaData` after freezing and re-assert equality (no pointer aliasing into federation).
  The shared `representationvariable` package extraction (§6.1) is guarded SEPARATELY by the existing `graphql_datasource/representation_variable_test.go` (behavior preserved through the call sites); a golden assertion confirms the freezer's candidate node and the data source's representation node come from the SAME builder.
- P1 `cacheProvidesDataVisitor`: feed `(operation, definition, fieldPlanners)` and assert the full `map[*FetchInfo]*Object` per fetch — entity-boundary reset, `__typename` dedup, inline-fragment `OnTypeNames`, alias→`OriginalName`, `CacheArgs` capture.
  The PR5 FIDELITY GATE (a reviewer must-fix): golden-compare P1's per-fetch `ProvidesData` trees against the OLD branch's trees for the federation fixtures; per RFC-1 §2.10 a too-small `ProvidesData` silently disables hits and an arg-blind one serves stale data, so this is a HARD test, not a claim.
- P2 `cacheConfigStamper`: feed a finished concrete fetch tree + a fake provider; assert the FULL `*FetchCacheConfig` stamped on each `*SingleFetch`/`*EntityFetch`/`*BatchEntityFetch`, and assert nil where policy is absent (the four NO-OP gates, §11.3).
  Carrier regression assertion (a reviewer must-fix): build a tree that goes through dedup + appendFetchID + `createConcreteSingleFetchTypes`, then assert the stamper resolves the right `ProvidesData` via `fetch.FetchInfo()`.
- P3 `optimizeL1Cache`: port the OLD pass's tests; assert `cfg.L1` per fetch for provider/consumer/union/dependency-chain scenarios, PLUS a defer case feeding `processTrees(root, defer1, defer2)` asserting cross-tree provider/consumer narrowing (§10.1).

Golden plan tests (the integration proof, via `datasourcetesting.RunTest`):

- NO-OP golden (graft from A; the hard PR1/PR8 proof): run the EXISTING planner golden suite with NO provider wired and assert the postprocessed plans are BYTE-IDENTICAL to today (every `Cache` nil).
- Caching golden: with a provider configured, assert the WHOLE postprocessed plan across sync, defer (root + multiple defer groups, asserting each `Defers[i].Fetches` carries `Cache`), and subscription — exact-match snapshots via the plan pretty-printer (which nil-guards, commit 921e48ae).
- Defer + `DeferID`: assert a fetch and its deferred twin get independent `Cache` values and are not merged (`EqualSingleFetch` guard, `fetch.go:49-53`).
- Determinism: run each plan twice and assert byte-identical plans.
- Plan-cache safety: assert no per-request data appears in any stamped `Cache` (golden plan has only static fields).

---

## 15. v1 vs v2 staging and risks / open questions

### 15.1 Staging

| Capability | v1 | v2 |
|---|---|---|
| `cacheProvidesDataVisitor` (re-add, exact OLD semantics) + `*FetchInfo` carrier | yes | — |
| `representationvariable` shared-package extraction (refactor graphql_datasource in place, §6.1) | yes (early structural commit) | — |
| `cacheKeySpecFreezer` (ALL `@key` sets → multi-key `CacheKeySpec` by value, via shared `representationvariable`) | yes | — |
| `cacheConfigStamper` on all 3 concrete types + folded `computeHasAliases` | yes | — |
| `optimizeL1Cache` (entity subset/superset, CROSS-tree) | yes | root-field→entity L1 promotion |
| Entity L1/L2 + root-field response cache + negative + shadow (config side) | yes | — |
| `CacheConfigProvider` + composed NO-OP gates | yes | — |
| Per-field L1-normalize metadata on `ProvidesData` (`OriginalName`/`CacheArgs`/`HasAliases`) | yes | — |
| Per-root-field fetch isolation (vs the v1 all-or-nothing rule) | NO (conservative: decline when policies differ, §7.2) | — (deferred to a separate RFC, RFC-03, §7.2) |
| `@requestScoped` widening | NO (out of scope entirely, removed by review, §12) | NO (out of scope entirely; not a v2 pass) |
| Response-tree analytics (`Object.CacheAnalytics`, `Field.CacheAnalyticsHash`) | NO (observer nil, RFC-1 §3.5) | node-keyed side-table (not `Object`/`Field` fields) |
| Subscription `EntityCachePopulation` + mutation populate/invalidation | NO | `cacheSubscriptionAnnotator` |
| Partial L1 / partial batch realign | policy bits stamped only (`EnablePartialCacheLoad`/`PartialBatchLoad`); loader seam is v2 (RFC-1 §9) | loader impl |

Critical staging note (mirrors RFC-1 §9): v1 produces everything the v1 controller needs (NO-OP / L1 / L2 / L1+L2 / shadow, full-response + full-batch, always-on coverage/freshness/reorder).
Analytics, subscription/mutation caching, and partial realign are staged because they collide with the defer walker rewrite or need the v2 loader seams — exactly where RFC-1 draws its v1/v2 line.
(`@requestScoped` is not on this list: it is out of scope entirely, removed by review, §12; per-root-field isolation is likewise deferred to RFC-03, §7.2.)

### 15.2 Risks and open questions

- R1 (P1 ownership read): P1 reads `planningVisitor.fieldPlanners`; if that map ever diverges from what the planning visitor emitted, `ProvidesData` is subtly wrong.
  Mitigation: the golden-compare against OLD trees (§14); the cost visitor already relies on the same field (`planner.go:177`), so the dependency is precedented.
- R2 (`*FetchInfo` carrier requires `Info` enabled): when `DisableIncludeInfo` is set, `Info` is nil and the carrier degrades to no caching.
  Mitigation: codify "enable `FetchInfo` whenever a provider is configured" as a hard, tested wiring precondition, so a `DisableIncludeInfo`+provider combo degrades to a clean no-op rather than silent uncached behavior (graft from C).
- R3 (L2 inferred from TTL): RFC-1's `*CachePolicy` has no L1/L2 bool, so RFC-2 infers `L2 = TTL>0 || NegativeCacheTTL>0`.
  Open question: confirm with RFC-1 authors whether an explicit L2-enable signal is intended; if so it belongs in the `*CachePolicy` struct, not invented here (§7.1).
- R4 (`EntityKeyMappings` source): RFC-2 freezes `EntityKeyMappings` from federation (dossier §5.3), never from the policy struct.
  Open question: if operator-declared root-arg↔`@key` overrides are needed beyond what federation exposes (OLD carried them on `RootFieldCacheConfiguration`), that config must arrive through a dedicated provider method; v1 freezes the structurally-derivable mappings and stages overrides.
- R5 (CacheConfigProvider migration): moving policy off `FederationInfo` touches external producers (cosmo).
  Mitigation: RFC-1 §7.5's one-release `FromFederation` shim; RFC-2 only consumes the provider, so the migration is plan-side and isolated.
- R6 (the one `node_object.go` edit): re-adding `OriginalName`/`CacheArgs`/`HasAliases` + additive `Copy()` is the only `resolve`-type change beyond RFC-1's `Cache` field.
  Mitigation: it is additive, not a forbidden-body edit, and the tree it annotates is never defer-`Copy()`'d (§13.3); alternatively a node-keyed side-table avoids even that, at the cost of an extra map lookup in the runtime coverage walk.

---

## 16. Appendix A: OLD-concern → new-home mapping (no existing visitor edited; scope decisions called out)

| OLD caching concern (analysis-A) | OLD site | New home in RFC-2 | Existing visitor edited? |
|---|---|---|---|
| Per-planner `ProvidesData` tree build (`trackFieldForPlanner`/`popFieldsForPlanner`/`createFieldValueForPlanner`) | `caching_planner_state.go:91-197,234-284,372-384`, driven from `visitor.go` `EnterField`/`LeaveField` | P1 `cacheProvidesDataVisitor`, PEER on `planningWalker` (§9) | No — `visitor.go` untouched; P1 is a separate registered visitor |
| Entity-boundary / `__typename`-dedup / inline-fragment path normalization | `caching_planner_state.go:286-370` | P1 (ported verbatim) | No |
| Alias→`OriginalName`, `CacheArgs`, `HasAliases` | `visitor.go:425-427`, `caching_planner_state.go:159-170`, `node_object.go` | P1 captures onto the `ProvidesData` tree; `computeHasAliases` folded into the stamper; fields re-added additively to `resolve` (§5.4) | No — `node_object.go` (resolve) is additive, not a forbidden file |
| `FetchCacheConfiguration` derivation from fed config | `caching_planner_state.go:594-746`, `configureFetch` `visitor.go:1450` | P2 `cacheConfigStamper` from `CacheConfigProvider` policy (§7) | No |
| Multi-key `CacheKeySpec` candidates from ALL `@key` sets | `representation_variable.go`, `graphql_datasource.go` | H `cacheKeySpecFreezer` via the shared `representationvariable` package (§6.1) — helpers EXTRACTED from `graphql_datasource` (unexported there on defer) into `plan/representationvariable`, `graphql_datasource` refactored in place to call them; one best-effort candidate per `@key` set | No forbidden-visitor edit (`graphql_datasource.go` refactor is behavior-preserving, guarded by its tests) |
| `optimize_l1_cache` `UseL1Cache` decision | `postprocess/optimize_l1_cache.go` (OLD) | P3 `optimizeL1Cache`, re-homed, CROSS-tree, narrows `cfg.L1` (§10) | No — own pass |
| Per-root-field cache isolation (`isolatedRootField`) | `path_builder_visitor.go:113-119,564-588,1332-1359` | v1 all-or-nothing decline in the stamper (§7.2); per-fetch isolation NOT dropped — DEFERRED to a separate RFC, RFC-03 (`docs/caching/specs/2026-06-30-rfc-03-per-root-field-cache-isolation.md`, §7.2) | No — `path_builder_visitor.go` untouched |
| `@requestScoped` widening + rename maps | `node_selection_visitor_request_scoped.go` (766), `visitor.go:396-403/425-427` | OUT OF SCOPE entirely (removed by review, §12) — no v1 or v2 pass | No |
| Required-field alias preservation | `required_fields_visitor.go:226,238,313-318` | Only relevant to `@requestScoped`, which is out of scope (§12); no v1 or v2 change | No — `required_fields_visitor.go` never needs a change |
| Subscription `EntityCachePopulation`, mutation impact/populate | `caching_planner_state.go:463,807-852` | v2 `cacheSubscriptionAnnotator` (§4) | No |
| Cache config on `FederationMetaData`/`FederationInfo` | `federation_metadata.go` (+323), 4 methods, forwarders | plan-side `cacheconfig` package + `CacheConfigProvider` (§11.2) | No — `federation_metadata.go` untouched |

The five PR1-forbidden files — `node_selection_visitor.go`, `path_builder_visitor.go`, `required_fields_visitor.go`, `node_selection_builder.go`, `visitor.go` — have ZERO body edits in v1.
Every OLD concern lands in a new file or an additive registration, with two DELIBERATE scope decisions made explicit by review: `@requestScoped` widening is out of scope entirely (removed, §12), and per-root-field cache isolation is deferred to a separate RFC, RFC-03 (§7.2) — neither is a silent drop.
The one non-additive touch is the behavior-preserving extraction of the representation-variable builder into the shared `representationvariable` package (`graphql_datasource` refactored in place, guarded by its existing tests, §6.1) — not a forbidden-visitor edit.
