# Caching port — implementation plan (PLAN.md)

Status: living plan, drives the Codex coding sessions.
Branch under change: `feat/eng-7770-add-defer-support-part-4`, all code under `v2/` (Go 1.25).

This plan sequences the caching port as a chain of small, self-contained, individually reviewable commits/PRs.
It is the running index referenced by `CODING_GUIDELINES.md` §8 ("cross-reference the running plan for which commit you are on and its acceptance goals").

Ground truth, in priority order (these override this plan on any conflict):

- RFC-1 (loader cache abstraction + self-contained config): `docs/caching/specs/2026-06-30-rfc-01-loader-cache-abstraction.md`.
- RFC-2 (caching planner passes): `docs/caching/specs/2026-06-30-rfc-02-caching-planner.md`.
- RFC-03 (per-root-field cache isolation, out-of-core follow-up): `docs/caching/specs/2026-06-30-rfc-03-per-root-field-cache-isolation.md`.
- Testing appendix (~100% loader+cache coverage with fakes): `docs/caching/specs/2026-06-30-rfc-01-appendix-testing-strategy.md`.
- Coding guidelines (injected into EVERY Codex session): `docs/caching/CODING_GUIDELINES.md`.
- Dossier (file:line evidence): `scratchpad/caching-rfc/DOSSIER.md`.

---

## 0. How to use this plan

### 0.1 Workflow (who does what)

- Claude ORCHESTRATES, REVIEWS, and gives feedback;
  Codex does ALL the coding (CODING_GUIDELINES §0).
- Each PR below is handed to a Codex session with its problem statement, scope, predefined success goals (the listed tests),
  and `CODING_GUIDELINES.md` injected verbatim.
- After each Codex pass, Claude reviews against the PR's goals and the cited RFC sections;
  iterate in fresh Codex sessions until both agree the goals are met.
- No commit lands without explicit human approval (CODING_GUIDELINES §0, §8).

### 0.2 What every PR entry contains

- A one-line PROBLEM statement.
- SCOPE — the smallest useful slice.
- TESTS it adds, referencing the appendix scenario IDs (A1, D7, E3, N1, …), including defer+caching and multi-key where relevant.
- An IMPLEMENTATION outline (named files, from the RFCs).
- A LIGHTWEIGHT RFC block: problem / solution / key decisions / reviewer guidance.

### 0.3 The two no-op invariants this plan protects

- RUNTIME no-op: with no `SetCacheController`, the loader path is byte-identical to today (RFC-1 §10.2).
- PLANNER no-op: with no `EnableCaching(...)` provider, postprocessed plans are byte-identical to today (RFC-2 §11.4).
- The whole Structure phase (Phase 0) must be a pure no-op end to end;
  every feature phase keeps both no-op gates intact for any integrator who does not opt in.

### 0.4 Note on "the first commit establishes structure only"

The brief asks the FIRST commit to establish structure only.
Two RFC mandates make a single literal commit impossible to keep "as small as possible while useful":
the representation-variable extraction "is its OWN early structural commit" (RFC-2 §6.1, CODING_GUIDELINES §3),
and the RFC-1 runtime seam, the RFC-2 plan wiring, and the test harness are independently reviewable surfaces.
So Phase 0 is a tight STRUCTURE PHASE of the smallest individually-reviewable commits (S1–S4),
the first of which (S1, repr-var) is mandated separate,
and the phase AS A WHOLE is a pure no-op end to end.
If a single squashed structure commit is preferred at merge time, S1–S4 squash cleanly in order.

---

## 1. Dependency order at a glance

```
Phase 0 — STRUCTURE (pure no-op end to end)
  S1 representationvariable extraction        (no deps)
  S2 RFC-1 contract types + no-op loader seam (no deps; independent of S1)
  S3 RFC-2 additive plan wiring + skeletons   (deps: S1, S2)
  S4 test harness: v2 fakes/synctest + execution wgc supergraph + golden plan (deps: S2, S3)

Phase A — L2 ENTITIES (+ multi-key, negative, shadow)   (deps: S1–S4)
  A1 plan:    ProvidesData visitor + entity freezer (multi-key) + entity stamper
  A2 runtime: resolve/cache entity L2 controller (lookup→coverage→freshness→reorder→write→backfill→flush)
  A3 runtime: negative caching                 (deps: A2)
  A4 runtime: shadow mode (entity compare)     (deps: A2)

Phase B — L2 ROOT FIELDS                                  (deps: Phase A)
  B1 plan:    root-field stamper (all-or-nothing) + root-field freezer scope
  B2 runtime: root-field L2 controller + root-field shadow asymmetry

Phase C — L2 ROOT FIELDS THAT RE-USE THE ENTITY CACHE     (deps: Phase A, Phase B)
  C1 plan:    EntityKeyMappings freeze (root-arg ↔ @key)
  C2 runtime: root-field → entity-cache reuse at lookup

Phase D — L1 CACHING                                       (deps: Phase A; C optional)
  D1 plan:    optimizeL1Cache cross-tree narrowing
  D2 runtime: request-lifetime shared L1 store + L1 controller path

Out-of-core follow-ups (sequenced AFTER the core A–D)
  RFC-03 per-root-field cache isolation (own RFC; enhances B/C; ships last of the L2-root-field work)
  v2-staged: analytics observer, subscription/mutation caching, partial L1 / partial batch realign, root→entity L1 promotion
  cosmo migration shim (cacheconfig.FromFederation)
```

Each phase is a reviewable increment with its own tests;
no feature phase may regress the no-op goldens from S4.

---

## 2. Phase 0 — Structure (pure no-op)

### S1 — Extract the representation-variable builder into a shared package

PROBLEM: the `@key` → representation-node builder caching needs is unexported and lives in `graphql_datasource`,
and copy-pasting it would create two divergent `@key`→representation walkers (the silent key-skew hazard RFC-2 §6 exists to prevent).

SCOPE (smallest useful):
extract, do not re-add;
no caching code yet.

- New exported package `v2/pkg/engine/plan/representationvariable` (verified absent today).
- Move `buildRepresentationVariableNode` → `BuildRepresentationVariableNode` and `mergeRepresentationVariableNodes` → `MergeRepresentationVariableNodes`
  (verified at `graphql_datasource/representation_variable.go:21` and `:123`), plus the internal `representationVariableVisitor` and merge helpers.
- Refactor `graphql_datasource` IN PLACE to call the exported functions at its two call sites
  (verified `graphql_datasource.go:855` build, `:865` merge).

TESTS added:

- The EXISTING `graphql_datasource/representation_variable_test.go` must still pass byte-for-byte (the behavior-preservation guard, RFC-2 §6.1).
- New `representationvariable` package test (own package): table tests over `(definition, one @key set, federation)` asserting the FULL `*resolve.Object` node with `assert.Equal`
  (single `@key`, composite, nested-object; interfaceObject/entityInterface `__typename` baked in).

IMPLEMENTATION outline:
pure move + re-export + two-call-site rewrite;
no import cycle — `representationvariable` imports `plan`/`resolve`/`ast`/`astvisitor`, and `plan` does not import it (RFC-2 §6.1).

LIGHTWEIGHT RFC:

- Problem: shared builder is private to a datasource package; caching must reuse it without copy or cross-package private access.
- Solution: refactor-in-place extraction to a new shared exported package; both `graphql_datasource` and the future freezer call one implementation.
- Key decisions: extract (not re-add), preserve behavior, keep `MergeRepresentationVariableNodes` for the datasource's single `representations` variable (the freezer will NOT call it — multi-key keeps candidates separate).
- Reviewer guidance: confirm zero behavior change via the existing datasource test, confirm no new import cycle, confirm both call sites now use the exported names.

---

### S2 — RFC-1 contract types + the no-op loader seam

PROBLEM: the loader has no cache abstraction;
RFC-2 and the controller package need the runtime OUTPUT contract to exist and be wired as a strict no-op first.

SCOPE (smallest useful):
ONE mostly-additive commit that compiles and behaves identically (RFC-1 §4, G8);
no cache implementation.

- New `resolve/cache_controller.go`: `CacheController`, `RequestCache`, `Decision` (incl. `DecisionFetchShadow`), `PrepareFetchInput`, `MergeInput`, `MergeArena`, `MergeSession`, `CacheObserver`, `FetchCacheHandle`, `ItemCacheState`, `CacheCandidate`, `ShadowCacheEntry`, plus the loader-side `mergeArena`/`mergeSession` impl (RFC-1 §3, §4.1).
- New `resolve/cache_config.go`: `FetchCacheConfig` (+ `Equals`, `String`), `CacheKeySpec`, `CacheKeyCandidate`, `CacheScope`, `CacheWriteReason`, `EntityKeyMapping` (RFC-1 §3.6).
- `loader.go`: `preparedFetch.cacheHandle`; `result.{response, responseData, responseHasErrors}`; the three carrier assignments in `mergeResult`; the two call sites in `resolveSingle` (`cachePrepare` after prepare, `cacheMerge` after merge, both lock-free); helpers `cacheRequest`/`cachePrepare`/`cacheMerge`/`mergeArena` (RFC-1 §4.1, §4.3, §4.4).
- `context.go`: `cacheController`/`requestCache` fields, `SetCacheController`, `endCacheRequest`, `clone` sets `requestCache=nil`, `Free` defensive teardown (RFC-1 §4.1 i–m, §6.3).
- `fetch.go`: `Cache *FetchCacheConfig` on `FetchConfiguration`, `EntityFetch`, `BatchEntityFetch`; the nil-safe cache clause in `FetchConfiguration.Equals` (RFC-1 §3.8).
- `resolve.go`: `defer ctx.endCacheRequest()` at all FOUR entry functions (`ResolveGraphQLResponse`, `ArenaResolveGraphQLResponse`, `ResolveGraphQLDeferResponse`, `executeSubscriptionUpdate`) (RFC-1 §4.5).

TESTS added (no harness yet — pure, self-contained):

- `FetchConfiguration.Equals` cache clause: P1 (both nil → prior result), P2 (one nil → not equal), P3 (both equal → dedup), P4/P5 (differ in any field / one candidate `Representation` → not equal via `slices.EqualFunc` + `Object.Equals`).
- Minimal nil-controller no-op assertion: an existing-style loader test with no controller stays byte-identical (the runtime no-op invariant); the full existing resolve suite still passes.
- `FetchCacheConfig.String` / `FetchCacheHandle.String` render nil-safely (used by logs and the plan pretty-printer, which already nil-guards, commit 921e48ae).

IMPLEMENTATION outline:
every hook guarded by `l.ctx.cacheController == nil` then `cfg == nil || (!cfg.L1 && !cfg.L2)`;
`mergeArena` built only after the controller is known non-nil;
`MergeSession.Begin()` is the single `DataBuffer.Lock` acquisition per hook (RFC-1 §3.4, §6.4);
the three `result` carriers are written by `mergeResult` and never read while caching is off.

LIGHTWEIGHT RFC:

- Problem: no clean seam to plug L1/L2 caching outside `loader.go`.
- Solution: a mode-blind `RequestCache` reached through `Context`, a scoped `MergeArena.Begin()→MergeSession→Close()` lock, a two-level opaque handle, all nil-checked and zero-cost when off.
- Key decisions: `DecisionSkipFullHit` reuses the existing `res.fetchSkipped` early-return (RFC-1 §4.3); `ProvidesData` folded into config (no `FetchInfo.ProvidesData` re-add); `Cache` is a POINTER so nil is the gate.
- Reviewer guidance: merging this alone must change runtime behavior in ZERO ways; verify the two call sites are OUTSIDE the phase locks; verify `clone`/`Free` reset the per-request surface; verify no `*Object` is referenced by `loader.go`.

---

### S3 — RFC-2 additive plan wiring + pass skeletons + plan-side config

PROBLEM: the planner has no caching producer, no policy model, and no `ProvidesData` carrier;
these must exist as additive wiring that produces NO config until a provider is supplied.

SCOPE (smallest useful):
additive only — zero body edits to the five forbidden visitor files (`node_selection_visitor.go`, `path_builder_visitor.go`, `required_fields_visitor.go`, `node_selection_builder.go`, `visitor.go`) (RFC-2 §5, PR1).

- New `plan/cacheconfig` package: `CachingConfiguration`, `EntityCachePolicy`, `RootFieldCachePolicy`, `MutationCachePolicy`, `SubscriptionCachePolicy`, the `CacheConfigProvider` interface, and the `dataSourceConfiguration[T].Caching() CacheConfigProvider` accessor (RFC-2 §11.2, RFC-1 §7.2–7.3) — declared to RFC-1's EXACT shapes, no invented fields.
- New postprocess skeletons: `cachingPlanner` facade (with the single NO-OP gate), `cacheKeySpecFreezer`, `cacheConfigStamper`, `optimizeL1Cache` — wired but inert; the facade returns immediately when `len(providers) == 0` (RFC-2 §8 `Annotate`).
- `postprocess.go` additive edits: one `Processor` field, the `EnableCaching(providers, federation, definition)` option, `NewProcessor` instantiation, the `Annotate` call in `Process` (sync + defer + subscription arms), the `deferTrees` helper (RFC-2 §5.1).
- `plan/planner.go` additive blocks: P1 `cacheProvidesDataVisitor` construction, peer registration on `planningWalker` (after the cost visitor), and `attachTo` after the walk — all `!= nil`-guarded (RFC-2 §5.2). The visitor body may be a thin skeleton here; the full port lands in A1.
- `resolve/node_object.go` additive fields: `Object.HasAliases`, `Field.OriginalName`, `Field.CacheArgs`, the `CacheFieldArg` type, carried in `Object.Copy()`/`Field.Copy()` (RFC-2 §5.4). Additive, not a forbidden-body edit.
- `resolve.GraphQLResponse`: unexported `cacheProvidesData map[*FetchInfo]*Object` + `SetCacheProvidesData`/`CacheProvidesData` accessors (RFC-2 §9.3).

TESTS added:

- NO-OP planner golden: the EXISTING planner golden suite with NO provider wired produces byte-identical postprocessed plans (every `Cache` nil) (RFC-2 §14, the PR1/PR8 hard proof).
- Skeleton unit tests: facade `Annotate` with empty providers touches no node; `cacheconfig` policy structs round-trip; `node_object` `Copy()` carries the new fields (full-value `assert.Equal`).

IMPLEMENTATION outline:
the four gates default OFF (RFC-2 §11.3);
the freezer/stamper bodies are scaffolding whose entity/root-field logic is filled in A1/B1;
P1 registers identically to `CostVisitor` (reads `planningVisitor.fieldPlanners`).

LIGHTWEIGHT RFC:

- Problem: need a producer surface and a policy model decoupled from `FederationMetaData`, with zero diff to the core visitors.
- Solution: additive `cacheconfig` package + a thin facade + four single-responsibility passes + a `*FetchInfo`-keyed `ProvidesData` side-table, all gated off when no provider exists.
- Key decisions: side-table carrier (not `FetchInfo.ProvidesData`); per-field metadata on the caching-owned `ProvidesData` tree (immune to defer `Copy()`); declare RFC-1's policy shapes verbatim.
- Reviewer guidance: confirm ZERO body diff on the five forbidden files; confirm the no-op golden is unchanged; confirm `cacheProvidesData` is never reached by defer's response-tree `Copy()`.

---

### S4 — Test harness: v2 fakes + `testing/synctest`; execution-module wgc `commerce` supergraph + `config_factory` golden-plan harness

PROBLEM: caching needs PROVABLY-COMPOSABLE test subgraphs and drift-proof, real-planner loader inputs, plus a shared fake set, so the loader+glue and the controller reach ~100% coverage without a network or real backend.

SCOPE (smallest useful):
build the harness ONCE across the two modules: the controller unit tests over constructed astjson in v2, and the plan-driven loader+cache catalog (real plan -> loader -> fakes) in the execution module.

- New v2 support package `v2/pkg/engine/resolve/cache/cachetesting`:
  the `CacheStage` enum and ALL fakes (`FakeCacheController`, `FakeRequestCache`, `FakeStore`, `GatedDataSource`, `RecordingObserver`);
  NO custom clock — time and TTL are faked with the Go 1.25 stdlib `testing/synctest` (CODING_GUIDELINES §2).
  `RealishCache` is scaffolded here but its real controller backing grows with A–D (appendix §2.4).
- The `resolve/cache` controller UNIT tests live in a NEW v2 test package:
  CONSTRUCTED astjson inputs + the `cachetesting` fakes, no plans (lookup decision, ProvidesData coverage, multi-candidate freshness, reorder, multi-key render/backfill, shadow compare, negative/write-back);
  TTL/expiry is tested by sleeping PAST the TTL inside a `synctest.Test` bubble (instantaneous in fake time), then asserting the entry is treated as expired.
  S4 establishes the package + fakes; the logic rows land with A–D.
- The PLAN-DRIVEN loader+cache tests live in a NEW execution-module test package (where `wgc`, the `config_factory`, and the cosmo proto are usable — v2 stays free of the cosmo-proto dependency):
  - the `commerce` 3-subgraph supergraph as COMMITTED testdata, exactly the `execution/engine/testdata/config_factory_federation/` pattern:
    `account_sdl.graphql`/`product_sdl.graphql`/`review_sdl.graphql` (`Product @key(upc) @key(sku)`, cross-subgraph `User`, root fields `topProducts`/`latestReviews`/`product(upc:)`/`user(id:)`),
    `graph.yaml` (per-subgraph name/routing_url/schema.file),
    `compose.sh` (`npx -y wgc@latest router compose -i graph.yaml -o config.json` then `jq .`),
    and the committed `config.json`; re-running `compose.sh` is the composability guard (wgc FAILS if the subgraphs do not compose).
  - the `Plan(tb, stage, query, responses)` harness: read `config.json` into `nodev1.RouterConfig` via `protojson.Unmarshal`,
    build `plan.Configuration` via `engine.NewFederationEngineConfigFactory(ctx, ...).BuildEngineConfiguration(&rc)`,
    run the v2 planner + postprocess WITH RFC-2 `EnableCaching`, GOLDEN-snapshot the REAL plan (so reviewers SEE it and it cannot drift),
    then drive the v2 loader with the plan + a fake in-process datasource (planner-output transport swapped to a `GatedDataSource`) + the cache controller.
  - a refactor-in-place adds a small EXPORTED test accessor to the execution `engine` package returning the built `plan.Configuration`
    (today `Configuration.plannerConfig` is unexported; only `DataSources()`/`FieldConfigurations()` are exposed).
- NO `export_test.go` bridge in v2 `package resolve`:
  the loader seams an earlier draft reached through unexported access (full-hit skip, prepare->merge handle identity, the `MergeSession` lock-once seam) are asserted OBSERVABLY in the execution-module layer instead — the recording fake owns the handle it returns (proving pointer identity prepare->merge), the gated datasource + `store.Ops()` prove no network ran on a full hit, and `-race` proves the lock discipline (appendix §1.3, §4.4, §9.4).

TESTS added:

- execution-module plan-driven loader+cache rows, driven through the PUBLIC `ResolveGraphQLResponse`/`ArenaResolveGraphQLResponse`/`ResolveGraphQLDeferResponse` against REAL `Plan(...)` plans (the smallest real plan the planner can produce for each seam — a single root-field query, a single by-key entity query — so even the seam fixtures cannot drift), asserted on the observable surfaces (`fake.Calls()`, `store.Ops()`, response bytes, `-race`):
  - A. loader-seam gates A1–A7 (NO-OP and config gating).
  - B. lazy init + lifecycle B1–B8 (incl. B4 `ArenaResolveGraphQLResponse`, B6 clone, B7 `Free`, B8 idempotent end).
  - C. decision dispatch C1–C8 (incl. C3 `SkipFullHit`, C4 `Shadow`, C5 `Partial` seam, C6 spurious-error guard, C7 OnLoad/OnFinished not fired, C8 handle identity).
  - L. MergeSession L1–L7 (asserted observably + under `-race`); O. edge paths O1–O7 (incl. O6 header nil-guard, O7 key fidelity from `Input`); the EndRequest-called-once parts of K/N3.
- execution-module plan-driven goldens: the NO-OP golden (StageNoop) asserting byte-identical REAL plans, and the golden-plan render (`cfg.String()` + `KeySpec` dump per fetch) as the plan-drift guard.

IMPLEMENTATION outline:
all assertions are full-value `assert.Equal` on the normalized `[]Call`/`[]StoreOp`/response bytes;
concurrent fetches are kept inside the `synctest` bubble and ordered deterministically by gates plus `synctest.Wait()` (sleeps only advance the fake clock for TTL, NEVER for ordering), or by sorting records before the single `assert.Equal` (appendix §9.3);
`t.Context()`, table tests, `slices.SortFunc`, typed atomics throughout.

LIGHTWEIGHT RFC:

- Problem: hand-written plan literals drift from the real planner AND can silently encode subgraphs that do not actually compose; ad-hoc fakes fragment the suite; a custom fake clock duplicates the stdlib.
- Solution: compose the `commerce` supergraph with REAL `wgc` (committed `config.json`, `compose.sh` as the composability guard), build `plan.Configuration` through the execution `config_factory`, and golden the REAL plan; split the suite into a v2 home (controller unit tests over constructed astjson + fakes + `testing/synctest`) and an execution-module home (plan-driven loader+cache + golden plan, including the loader-seam/lifecycle/dispatch rows driven through the public `resolve` API and asserted observably); keep every fake in one v2 `cachetesting` support package.
- Key decisions: plan-driven tests live in the EXECUTION module so v2 never takes the cosmo-proto dependency; controller unit tests live in v2 over constructed astjson + fakes; `testing/synctest` replaces any custom clock (sleeps advance the fake clock for TTL, gates + `synctest.Wait()` order concurrent fetches — commit e509453b); a small exported test accessor is added to the execution `engine` package (refactor-in-place) to reach the built `plan.Configuration`.
- Reviewer guidance: confirm `compose.sh` re-composes cleanly (subgraphs provably compose) and `config.json` is committed; confirm exactly one source of truth for any plausible plan (`config_factory` + the real planner) with the golden showing BOTH plan shape and stamped config; confirm v2 takes no cosmo import; confirm no FakeClock/injectable-now and no `assert.Contains`/`JSONEq`/fuzzy anywhere.

---

## 3. Phase A — L2 caching for ENTITIES only

Multi-key best-effort keys land here (A1/A2);
negative caching attaches here (A3);
shadow mode's entity compare attaches here (A4).

### A1 — Plan: ProvidesData visitor + entity key-spec freezer (multi-key) + entity stamper

PROBLEM: entity fetches carry no cache config, so the runtime has no keys, no coverage tree, and no policy to act on.

SCOPE: the plan-side producer for ENTITY fetches only.

- Full port of `cacheProvidesDataVisitor` (P1) from OLD `caching_planner_state.go` into `plan/cache_provides_data_visitor.go`: entity-boundary reset, `__typename` dedup, inline-fragment `OnTypeNames`, alias→`OriginalName`, `CacheArgs` capture (RFC-2 §9).
- `cacheKeySpecFreezer` (H) entity path: freeze EVERY resolvable `@key` set into `CacheKeySpec.Candidates` (one best-effort candidate each, deterministically ordered by selection-set string) via `representationvariable.BuildRepresentationVariableNode`; the SOLE federation reader (RFC-2 §6).
- `cacheConfigStamper` (P2) entity arm: `EntityPolicy` lookup, `L1=true` (eligible), `L2 = TTL>0 || NegativeCacheTTL>0`, fold `computeHasAliases`, attach `ProvidesData` from the P1 side-table (RFC-2 §7, §7.1).

TESTS added:

- P1 fidelity gate: golden-compare per-fetch `ProvidesData` trees against the OLD branch for the federation fixtures (a too-small tree silently disables hits; an arg-blind one serves stale data) (RFC-2 §14, CODING_GUIDELINES §4.4).
- H freezer table tests: FULL multi-key `CacheKeySpec` for single/composite/nested/MULTIPLE `@key` sets (one candidate each, ordered, none required); no-`@key` → `(zero, false)`; mutate the source `FederationMetaData` after freezing and re-assert equality (no pointer aliasing); a golden confirming the freezer candidate node and the datasource representation node come from the SAME builder (appendix §3, RFC-2 §14).
- P2 stamper: FULL `*FetchCacheConfig` on `*EntityFetch`/`*BatchEntityFetch`; nil where policy absent (the four NO-OP gates); the carrier regression assertion through dedup + appendFetchID + `createConcreteSingleFetchTypes` (RFC-2 §14).
- Caching golden (StageL2Entities) across sync and defer, asserting each `Defers[i].Fetches` entity fetch carries `Cache`; determinism (plan twice, byte-identical).

IMPLEMENTATION outline:
P1 registered as a peer on `planningWalker` (full body now);
freezer crosses the federation boundary exactly once by value;
stamper runs after `createConcreteSingleFetchTypes`, writes concrete types directly.

LIGHTWEIGHT RFC:

- Problem: deriving `ProvidesData` post-merge loses per-fetch attribution and args (under-coverage and stale serves, RFC-2 §9.1).
- Solution: re-add the dedicated walk-time visitor; freeze all `@key` sets by value as multi-key candidates; stamp self-contained config on the concrete entity types.
- Key decisions: side-table keyed by `*FetchInfo`; multi-key best-effort (none required); L2 derived from TTL (open question flagged to RFC-1 authors, RFC-2 §7.1).
- Reviewer guidance: no federation pointer escapes the freezer (one-file review); P1 trees match OLD byte-for-byte; stamper resolves the right tree via `fetch.FetchInfo()` after conversion.

### A2 — Runtime: `resolve/cache` entity L2 controller

PROBLEM: nothing implements `RequestCache`, so stamped entity config does nothing.

SCOPE: the L2-only entity controller in the new `v2/pkg/engine/resolve/cache` package (RFC-1 §7.1), full best-effort multi-key.

- `PrepareFetch`: render every renderable candidate (renderable → `RenderedKeys`, rest → `PendingCandidates`), `Get` under all rendered keys, the always-on coverage walk from `cfg.ProvidesData`, multi-candidate freshness sort, reorder-to-selection-order, AND-reduction → `DecisionSkipFullHit`/`DecisionFetch`, write `ItemCacheState` (RFC-1 §3.1, §3.7, §5.1a).
- `OnFetchResult`: the write gate `!FetchFailed && !HasErrors && ResponseData != nil && Type()!=Null`, re-render `PendingCandidates` from fresh data, defer L2 `Set` for all renderable keys (RFC-1 §3.3, §4.4).
- `OnFetchSkipped`: splice reordered `FromCache` into items, emit best-effort backfill/refresh writes on `MustWriteBack` (RFC-1 §4.4).
- `EndRequest`: flush deferred L2 writes (bytes, no lock, no arena), one `Set` batch per cache instance (RFC-1 §4.5).
- One `CacheKeyTemplate` per candidate (sole source of read/write/invalidate keys), keys hashed with the repo's pooled xxhash pattern; TTL math calls real `time` directly (`time.Now`/`time.Since`/`time.Until`) with NO injectable clock — tests fake time with `testing/synctest` (CODING_GUIDELINES §2).

TESTS added (`RealishCache`, white-box `package cache` + black-box `cache_test`):

- D hit-determination D1–D14 (coverage, freshness, reorder, AND-reduction, `ProvidesData==nil` disables walk).
- E multi-key E1–E7 (all renderable, hit on non-primary key, backfill-all-after-response, none renderable, read-hit backfill, refresh-vs-backfill tags, single-`@key` degenerate).
- F write gate F1–F8 (incl. F2 transport failure / F4 parse failure gate keys off `FetchFailed`/`ResponseData==nil`, NOT `HasErrors` — the §3.3 blocking-bug guard).
- I entity/batch shapes (I-rows: entity-scope keys + `EntityMergePath`, `BatchEntityKey` + `BatchIndex`, batch full-hit/all-miss/mixed→full-refetch, batch empty short-circuit).
- K flush K1–K4 (accumulate then one batch; flush holds bytes not `*Value`).
- Defer+caching: N3 (single `EndRequest` after all groups), N5 (lookup/merge inside the group's locked region, `-race`); M4 (each hook = single lock acquisition), M5 (error propagation under parallel).

IMPLEMENTATION outline:
all arena work inside one `MergeSession` per hook;
candidate bytes lazily parsed via `MergeSession.ParseBytes`;
`StructuralCopy` to avoid merge aliasing.

LIGHTWEIGHT RFC:

- Problem: implement the entity L2 lookup/write surface behind RFC-1's interfaces.
- Solution: a mode-parameterized controller doing best-effort multi-key render → coverage → freshness → reorder → gated write → deferred flush.
- Key decisions: L2 read inside the prepare session in v1 (off-lock read is a v2 optimization); deferred flush at request end (bytes only).
- Reviewer guidance: the write gate cannot reduce to `!HasErrors`; `MergeSession` is the single lock acquisition; keys derive from `Input` (canonical pre-injection), read key == write key.

### A3 — Runtime: negative caching

PROBLEM: a successful-but-empty entity fetch should be cacheable as a null sentinel, but a failed fetch must never be cached.

SCOPE: negative write + negative hit, gated by `EmptyEntity` and `NegativeCacheTTL>0`.

- `OnFetchResult`: when `EmptyEntity && !FetchFailed && !HasErrors && NegativeCacheTTL>0`, write a null sentinel under the keys with `NegativeCacheTTL`; stamp `ItemCacheState.{NegativeHit, FromCache=TypeNull}` (RFC-1 §3.3, §5.1d).
- `PrepareFetch`: a null-sentinel hit serves as `SkipFullHit` with `FromCache=TypeNull`, merge skipped.

TESTS added: G negative caching G1–G6 (write on empty entity, skip when TTL==0, hit served, expiry via `testing/synctest` G4, `EmptyEntity && FetchFailed` → gate blocks G5, null-bubble suppression preserved G6).

LIGHTWEIGHT RFC:

- Problem: distinguish "legitimately empty entity" (cache a sentinel) from "fetch failed" (cache nothing).
- Solution: route `EmptyEntity` (the one non-failure that still writes) to the negative path; all five failure signals block positive AND negative writes.
- Key decisions: negative TTL is its own knob; the loader's `setSkipErrors`/`isEmptyEntityFetch` paths are untouched.
- Reviewer guidance: `FetchFailed` wins over `EmptyEntity`; expired negative entry → miss (TTL expiry tested by sleeping past it inside a `testing/synctest` bubble).

### A4 — Runtime: shadow mode (entity compare)

PROBLEM: shadow must read L2 but never serve it, force a real fetch, then compare — without leaking onto the loader surface.

SCOPE: the `DecisionFetchShadow` path for ENTITY fetches.

- `PrepareFetch`: on `cfg.ShadowMode` + L2 hit, stash into `ShadowStash`, return `DecisionFetchShadow` (`skipLoad` stays false) (RFC-1 §3.2, §5.1b).
- `OnFetchResult`: when `h.Shadow`, run `CacheObserver.CompareShadow` BEFORE writes (compare → write-L1 → write-L2), entity-fetch only (RFC-1 §3.5).
- `RecordingObserver` wired; v1 ships the observer nil (force-fetch, record nothing).

TESTS added: H shadow H1–H4, H6, H7 (read+stash+force-fetch, compare match/mismatch, L1-hit-wins-no-shadow, no-op without analytics, NO-OP/L1 never yield `DecisionFetchShadow`). (H5 root-field asymmetry lands in B2.)

LIGHTWEIGHT RFC:

- Problem: decouple "read a cache value" from "serve a cache value" for a staleness probe.
- Solution: a dedicated decision + handle fields + an observer compare step, byte-identical to a miss on the loader side.
- Key decisions: shadow is a cross-cutting L2 variant, not a fifth mode; compare runs inside `OnFetchResult`'s open session.
- Reviewer guidance: shadow always force-fetches and serves FRESH; compare preserves compare→write-L1→write-L2 order; nil observer records nothing.

---

## 4. Phase B — L2 caching for ROOT FIELDS

### B1 — Plan: root-field stamper + root-field freezer scope

PROBLEM: root-field fetches carry no cache config, and a merged fetch may mix policies.

SCOPE: the plan-side producer for ROOT-FIELD fetches.

- `cacheConfigStamper` root-field arm: `rootFieldPolicyForAllRootFields` (all-or-nothing — a policy only when every root field resolves to the SAME one, else leave `Cache` nil), `L1=false`, `L2 = TTL>0` (RFC-2 §7, §7.2).
- `cacheKeySpecFreezer` root-field scope: `CacheScopeRootField` spec (type/field; candidates empty for a plain root field) (RFC-2 §6).

TESTS added:

- P2 root-field stamper rows: single cached root field → `Cache` set; mixed-policy / cached+uncached merge → `Cache` nil (the conservative decline); full-value `assert.Equal`.
- Caching golden (StageL2RootFields) for `topProducts`/`latestReviews`; determinism.

LIGHTWEIGHT RFC:

- Problem: a merged root-field fetch must not mis-cache when policies differ.
- Solution: reproduce OLD's all-or-nothing decline additively in the post-plan stamper, with zero path-builder edits.
- Key decisions: per-field isolation (the optimization) is deferred to RFC-03; v1 declines rather than mis-caches.
- Reviewer guidance: confirm `path_builder_visitor.go` shows zero diff; confirm mixed-policy fetch declines L2.

### B2 — Runtime: root-field L2 controller + shadow asymmetry

PROBLEM: root-field config does nothing at runtime, and root-field shadow must force-refetch without comparing.

SCOPE: the root-field L2 path in `resolve/cache`.

- Render the root-field-scope key, `Get`/coverage/`Set` (reusing the entity path's primitives), `cfg.L1=false`.
- Root-field shadow asymmetry: force-refetch + overwrite L2 but DO NOT `CompareShadow` (the OLD asymmetry) (RFC-1 §3.5, §5.1b).

TESTS added: root-field rows in D/F/I/J; H5 (root-field shadow force-refetch, no compare recorded); root-field flush; a StageL2RootFields end-to-end mode-matrix row (J).

LIGHTWEIGHT RFC:

- Problem: root fields cache as a whole-response L2 unit, with a shadow asymmetry vs entities.
- Solution: reuse the entity controller primitives at root-field scope; gate the compare to entity scope only.
- Key decisions: root fields only act as L2 providers in v1 (L1=false); root→entity promotion is v2.
- Reviewer guidance: root-field shadow records NO compare; root-field key is whole-response scoped.

---

## 5. Phase C — L2 for ROOT FIELDS that RE-USE the entity cache

### C1 — Plan: freeze EntityKeyMappings (root-arg ↔ @key)

PROBLEM: a by-key root field (`product(upc:)`, `user(id:)`) cannot reuse an entity-cache entry because its root args are not linked to the entity `@key`.

SCOPE: freeze the root-arg↔`@key` mappings so a root-field fetch carries entity-cache candidates.

- `cacheKeySpecFreezer`: populate `CacheKeySpec.EntityKeyMappings` (the OLD `EntityKeyMapping`, by value) for root-field fetches whose args map onto an entity `@key` (RFC-2 §6, §6.3).

TESTS added: freezer table rows for `product(upc:)`/`user(id:)` asserting the FULL `EntityKeyMappings`; caching golden (StageL2RootReusesEntity).

LIGHTWEIGHT RFC:

- Problem: link root-field args to entity identity for cache reuse.
- Solution: freeze the structurally-derivable root-arg↔`@key` mappings into the spec by value.
- Key decisions: mappings are additional CANDIDATES in the best-effort model, not a separate key space (RFC-1 §3.6); operator overrides beyond federation are staged (RFC-2 R4).
- Reviewer guidance: mappings frozen from federation, never from the policy struct; no federation pointer retained.

### C2 — Runtime: root-field → entity-cache reuse at lookup

PROBLEM: the controller does not try entity-cache entries when serving a by-key root field.

SCOPE: at `PrepareFetch` for a root field with `EntityKeyMappings`, render an entity candidate from the root args and look up the entity cache; backfill on write.

TESTS added: StageL2RootReusesEntity rows — `{ product(upc:"1"){ name } }` served from an entity entry primed by `topProducts`; E3 (lookup renders only the arg-derived candidate, backfills `sku` after response, exact ordered `[]StoreOp`); E5 (read-hit backfill via `OnFetchSkipped`).

LIGHTWEIGHT RFC:

- Problem: by-key root fields should hit the shared entity cache.
- Solution: render entity candidates from root args via `EntityKeyMappings`, look up the entity key space, backfill missing candidates after the fetch.
- Key decisions: reuse is L2 here (root→entity L1 promotion is v2); best-effort multi-key backfill covers the arg-vs-data renderability split.
- V1 cache-name constraint: by-key root-field policies must share `CacheName` with the corresponding entity policy to reuse entries.
- Reviewer guidance: assert the EXACT ordered `Get`/`Set` sequence (a wrong key or missing backfill fails); read key == write key.

---

## 6. Phase D — L1 caching

### D1 — Plan: `optimizeL1Cache` cross-tree narrowing

PROBLEM: every entity fetch is marked L1-eligible, but L1 only helps where a provider/consumer pair exists.

SCOPE: fill in the `optimizeL1Cache` (P3) pass — a pure NARROWING of `cfg.L1` (the stamper is the sole eligibility setter; P3 never turns L1 on) (RFC-2 §10).

- Collect entity fetches across ALL trees (root + every `Defers[i].Fetches`), `cfg.L1 = canRead || canWrite` via `ProvidesData` subset/superset + `DependsOnFetchIDs` ordering, cross-tree (RFC-2 §10.1).

TESTS added: port the OLD P3 tests (provider/consumer/union/dependency-chain), PLUS a defer case `processTrees(root, defer1, defer2)` asserting cross-tree provider/consumer narrowing; determinism.

LIGHTWEIGHT RFC:

- Problem: L1 eligibility must be narrowed to where a request-lifetime provider/consumer pair exists, across defer groups.
- Solution: a cross-tree single-responsibility pass refining `cfg.L1`, conservative-safe (narrowing only ever forgoes an optimization).
- Key decisions: cross-tree because the L1 store is request-lifetime and shared across defer groups; root→entity L1 promotion staged to v2.
- Reviewer guidance: P3 never turns L1 on; narrowing wrong only costs a hit, never correctness.

### D2 — Runtime: request-lifetime shared L1 store + L1 controller path

PROBLEM: there is no L1 entity store shared across a request's per-defer-group loaders.

SCOPE: the L1 path in `resolve/cache`, with the store owned by the `RequestCache` on `Context`.

- L1 entity store on the by-reference-shared `RequestCache` (RFC-1 §6.2), guarded by `DataBuffer.Lock` via the `MergeSession`.
- `PrepareFetch`: L1 → L2 → subgraph ordering, coverage at each layer; `OnFetchResult`: write normalized entities to L1 (and L2 when enabled); `OnFetchSkipped`: splice from L1.

TESTS added:

- J modes J1–J7 over the same query (NO-OP/L1/L2/L1+L2): loader branches only on `Decision`; L1 hit short-circuits L2 `Get`; L2 hit populates L1; mode-blindness.
- Concurrency M1–M3 (`-race`, gates): lazy init race-free, parallel writes to shared L1, per-defer-group loaders share ONE L1.
- Defer+caching N1, N2, N6: entity cached by initial fetch served to a deferred fetch; deferred fetch populates L1 visible to a later group; subscription event isolation (clone-nilled L1).

LIGHTWEIGHT RFC:

- Problem: L1 must live for the request and be visible across defer groups, race-free.
- Solution: home the store on the `RequestCache` on the by-reference-shared `Context`, guarded by the scoped `MergeSession` lock; lazy once-per-request init under `DataBuffer.Lock`.
- Key decisions: this request-lifetime L1 store is NOT `@requestScoped` (which is removed entirely); clone resets it so subscription events isolate.
- Reviewer guidance: one `BeginRequest` per request; L1 written by one group visible to groups scheduled after; no cross-event bleed.

---

## 7. Out-of-core-scope follow-ups (sequenced after the core)

These are NOT part of the core A–D plan;
they are listed for completeness and sequenced after it.

- RFC-03 — per-root-field cache isolation (`isolatedRootField`):
  its OWN RFC (`docs/caching/specs/2026-06-30-rfc-03-per-root-field-cache-isolation.md`).
  It is a pure optimization that upgrades B1's conservative all-or-nothing decline to per-field caching,
  functionally enhancing the L2-root-field stage (B) and the root-reuses-entity stage (C);
  per RFC-03 §7 it ships LAST among the L2-root-field work, ahead of L1.
  It is the ONE caching feature that touches `path_builder_visitor.go` (a single gated, separately-reviewed seam),
  which is why it is carved out of RFC-2's zero-diff core.
- v2-staged runtime (config bits may already be stamped; do NOT build the runtime now, RFC-1 §9, RFC-2 §15.1):
  analytics/trace observer walker hooks (`OnEntity`/`OnFieldValue`);
  subscription/mutation caching (`cacheSubscriptionAnnotator`, trigger lifecycle);
  partial L1 / partial batch realign (`DecisionFetchPartial`, `ItemCacheState.BatchIndex`);
  root-field→entity L1 promotion.
- cosmo migration shim: `cacheconfig.FromFederation(...)` one-release compatibility shim (RFC-1 §7.5).
- EXCLUDED ENTIRELY (not a follow-up, removed by review): the `@requestScoped` directive feature — no v1 or v2 pass, no provider method, no stamped field (RFC-2 §12, CODING_GUIDELINES §6).

---

## 8. Completion checklist

The port is "finished" when every item below is checked.

Structure and config:

- [ ] `representationvariable` shared package extracted; `graphql_datasource` refactored in place; existing datasource test passes byte-for-byte (S1).
- [ ] RFC-1 runtime contract types exist (`cache_controller.go`, `cache_config.go`) and the loader seam is wired as a strict no-op (S2).
- [ ] `FetchConfiguration.Equals` cache clause is nil-safe and dedup-correct (S2; P1–P5).
- [ ] Self-contained `FetchCacheConfig` + multi-key `CacheKeySpec` carry NO federation types or pointers into runtime (S2/A1).
- [ ] Plan-side `cacheconfig` package (policies + `CacheConfigProvider` + `Caching()` accessor) declared to RFC-1's exact shapes (S3).
- [ ] `node_object.go` additive fields (`HasAliases`/`OriginalName`/`CacheArgs`) carried in `Copy()`; `GraphQLResponse` `ProvidesData` side-table accessors (S3).

Planner passes:

- [ ] `cacheProvidesDataVisitor` (P1) re-added; per-fetch `ProvidesData` matches OLD byte-for-byte (fidelity gate) (A1).
- [ ] `cacheKeySpecFreezer` (H) is the sole federation reader; freezes ALL `@key` sets as multi-key candidates by value; no aliasing into federation (A1).
- [ ] `cacheConfigStamper` (P2) stamps all three concrete fetch types after `createConcreteSingleFetchTypes`; nil where no policy (A1/B1).
- [ ] `optimizeL1Cache` (P3) narrows `cfg.L1` cross-tree; never turns L1 on (D1).

Runtime hooks and cache modes:

- [ ] `resolve/cache` controller implements `RequestCache`; the `MergeArena.Begin()→MergeSession→Close()` scoped lock is the single acquisition per hook.
- [ ] All FOUR modes ride the same interface: NO-OP, L1-only, L2-only, L1+L2 (J-row mode matrix).
- [ ] Shadow mode (entity compare + root-field force-refetch asymmetry) via `DecisionFetchShadow` + `CacheObserver` (A4, B2).
- [ ] Negative caching (empty-entity sentinel; failed fetch never cached) (A3).
- [ ] Multi-key best-effort render-then-backfill (lookup under all rendered keys, re-render pending from fresh data, backfill all renderable keys) (A2, C2).
- [ ] Root-field L2 (B2) and root-field → entity-cache reuse via `EntityKeyMappings` (C2).
- [ ] Request-lifetime shared L1 store on `Context`, visible across per-defer-group loaders, clone-reset for subscription events (D2).
- [ ] End-of-tree batched L2 flush via `EndRequest` at all four entry functions (A2; covers sync, arena-sync, defer, subscription).

Tests and quality:

- [ ] NO-OP goldens hold: no controller → runtime byte-identical; no provider → plans byte-identical (S3, S4).
- [ ] `commerce` supergraph composed by REAL wgc (committed `config.json` + `compose.sh` composability guard) → `plan.Configuration` via the execution `config_factory` (with the small exported test accessor) → golden REAL plan; plan-driven tests in the new execution-module package, v2 fakes in `cachetesting`, v2 never imports the cosmo proto (S4).
- [ ] Defer + caching scenarios pass: entity served to deferred fetch, deferred fetch populates shared L1, single `EndRequest` after all groups, defer ordering preserved, subscription isolation (N1–N6).
- [ ] Concurrency scenarios pass under `-race` with gates (not latency): lazy init, parallel L1 writes, cross-loader L1 share, single lock per hook, error propagation (M1–M5).
- [ ] ~100% statement/branch coverage of the loader cache glue AND the `resolve/cache` package (`-coverpkg`, appendix §1.2), via fakes (no network, no real backend); time and TTL faked with `testing/synctest` (no custom clock, no injectable `now`).
- [ ] All caching tests assert FULL values with `assert.Equal`; no `assert.Contains`/`JSONEq`/fuzzy; non-deterministic bits normalized then asserted structurally.
- [ ] Modern Go 1.25 idioms; no reinvented stdlib; reuse-first; surgical diffs; zero body edits to the five forbidden visitor files.

Docs and process:

- [ ] `CODING_GUIDELINES.md` injected into every Codex session; each commit meets the §8 definition of done.
- [ ] Each PR ships its lightweight RFC block (problem / solution / key decisions / reviewer guidance) and links its PLAN.md item + RFC sections.
- [ ] Out-of-core follow-ups recorded (RFC-03 isolation; v2-staged runtime; cosmo `FromFederation` shim); `@requestScoped` confirmed absent everywhere.
