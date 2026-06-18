# 03 — Stacked PR Plan (graphql-go-tools)

> Part of the entity-caching re-implementation plan.
> See [00-OVERVIEW.md](./00-OVERVIEW.md) for navigation,
> [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) for the clean architecture and integration seam,
> [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md) for the directive table,
> [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) for the astjson dependency spec,
> [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md) for the full test and benchmark plan,
> [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) for out-of-scope topics,
> [08-EXECUTION-RUNBOOK.md](./08-EXECUTION-RUNBOOK.md) for the implementation loop.
> The router-side stack is in [04-PR-PLAN-router.md](./04-PR-PLAN-router.md).

This document defines the **graphql-go-tools** (gqtools) PR stack only.
Each PR is sized to be reviewable in **under ~30 minutes**.

---

## 0. Reading guide for someone with no prior context

Entity caching adds a two-level cache to the federation router.
**L1** is a per-request, in-memory map that deduplicates entity fetches within a single GraphQL operation.
**L2** is an external, cross-request cache (Redis or in-memory) that the router plugs in.
The cache stores JSON fragments of resolved entities and root-field results,
keyed by the entity's `@key` fields (for L1/L2 entity entries)
or by the root-field name plus arguments (for L2 root-field entries).

The feature lives in three layers of gqtools:

1. **resolve** (`v2/pkg/engine/resolve`) — the runtime.
   It owns the loader that fetches data, the `LoaderCache` backend interface the router implements,
   the cache-key templates, the L1 map, the merge logic, the analytics collector, and the cache trace.
2. **plan** (`v2/pkg/engine/plan`) — the planner.
   It reads declarative per-subgraph cache config from `FederationMetaData`
   and attaches a per-fetch `resolve.FetchCacheConfiguration` plus a `ProvidesData` field-shape tree to every fetch.
3. **datasource + postprocess** — the GraphQL datasource builds the raw cache-key templates;
   the `optimizeL1Cache` postprocess pass decides which fetches actually turn L1 on.

The router-facing entry point is `execution/engine`,
which exposes `WithSubgraphEntityCachingConfigs(...)`.

There is one **hard external prerequisite**: the foundation depends on astjson primitives
(`StructuralCopy`, `StructuralCopyWithTransform`, `Transform`, package-level `DeepCopy`,
and a breaking 2-return `MergeValues`) that exist **only on the open astjson PR #16 branch**.
They are not in any released astjson tag.
So PR 0 of this stack is "cut + pin a real astjson release". Nothing compiles without it.
Full detail in [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md).

---

## 1. Branching model and how each PR stays mergeable

### 1.1 The feature branch

Create one long-lived feature branch off `master`:

```
git checkout master
git pull
git checkout -b feat/entity-caching
git push -u origin feat/entity-caching
```

> Diff hygiene note: in this worktree the **local** `master` ref is frozen at 2026-02-16
> and is NOT the same as `origin/master` (2026-06-12).
> Always branch from and diff against `origin/master`.
> See [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) for why the stale-ref diff overcounts by ~181 files.

Every PR below targets `feat/entity-caching` as its base (except PR 0, whose first half targets the astjson repo).
The feature branch is merged into `master` only once at the very end, after the full stack lands.

### 1.2 The stacking discipline

- PRs stack linearly: PR N branches off PR N-1's branch, not off the feature branch tip.
- Each PR's "Dependencies" field names the single PR it stacks on.
- A PR is merged into `feat/entity-caching` (squash) only after the PR(s) it depends on have merged.
  When a dependency merges, rebase the child branch onto the new feature-branch tip before merging it.
- Tools like `gh pr create --base <parent-branch>` keep the review diff scoped to just that PR's changes.

### 1.3 Why every PR is independently mergeable into the feature branch

The recurring technique is **additive-then-wired**.
Almost every PR adds new types, new files, or new struct fields that are **inert** until a later PR reads them.
A PR that only adds an unused struct field, an unused interface, or a default-off pass
compiles, passes the existing test suite unchanged, and changes zero runtime behavior.
This is what keeps each PR small AND safe to merge on its own.

Three concrete levers make this work:

1. **Default-off flags.** The two behavior-bearing seams default to off:
   - per-request `CachingOptions{EnableL1Cache, EnableL2Cache, EnableCacheAnalytics}` all default `false`,
     so the loader's caching branches are dead until a request opts in.
   - `FetchCacheConfiguration.UseL1Cache` defaults `false`,
     set true only by the `optimizeL1Cache` postprocess pass.
2. **Opt-in config.** L2 is per-type/per-field opt-in.
   A nil lookup in `FederationMetaData` means "L2 disabled for this coordinate",
   so adding the config types and lookups changes nothing until a router populates them.
3. **No-op-safe passes.** The `optimizeL1Cache` pass is gated behind `disableOptimizeL1Cache`
   and, until wired, is a no-op that leaves `UseL1Cache` false everywhere.

The result: you can merge PR 1 through PR N into the feature branch one at a time,
run the full pre-existing test suite after each, and never see a regression,
because the new code paths are not yet reachable at runtime.

### 1.4 The two "wire it on" PRs

Two PRs flip behavior on and therefore deserve extra review scrutiny:

- **The resolve loader-integration PR** (PR 7) — the first PR where `EnableL1Cache`/`EnableL2Cache`
  actually change loader behavior.
- **The visitor-wiring PR** (PR 11) — the first PR where the planner emits non-empty cache config.

Everything before PR 7 is pure data-shape and interface plumbing.

---

## 2. The stack at a glance

| PR | Title | Layer | Wires behavior? | Stacks on |
|----|-------|-------|-----------------|-----------|
| 0 | astjson release + pin | dep | n/a | (master) |
| 1 | Foundation: interfaces, seams, ADRs, specs | resolve (decl) | no | 0 |
| 2 | astjson Loader copy helpers (transform wrappers) | resolve | no (helpers only) | 1 |
| 3 | Cache-key templates | resolve | no | 1 |
| 4 | Plan-time config structs + lookups | plan | no | 1 |
| 5 | ProvidesData field-tree types + ComputeHasAliases | resolve | no | 1 |
| 6 | Per-request CachingOptions + Context plumbing | resolve | no (flags off) | 1 |
| 7 | Loader L1/L2 read+write integration | resolve | **yes** | 2, 3, 5, 6 |
| 8 | Batch + partial entity cache load | resolve | yes | 7 |
| 9 | Negative cache | resolve | yes | 7 |
| 10 | Cache analytics collector + snapshot | resolve | yes (gated) | 7 |
| 11 | Datasource key-template + visitor caching wiring | plan + datasource | **yes** | 3, 4, 5 |
| 12 | optimizeL1Cache postprocess pass | postprocess | yes (gated) | 5, 11 |
| 13 | Mutation impact (populate + invalidate) | plan + resolve | yes | 7, 11 |
| 14 | Extension-driven invalidation | resolve | yes | 7 |
| 15 | Subscription entity population | plan + resolve | yes | 7, 11 |
| 16 | Shadow mode | resolve + plan | yes | 7, 10 |
| 17 | @requestScoped composition (metadata + lookups) | plan | no | 4 |
| 18 | @requestScoped datasource emission | datasource | no (data only) | 11, 17 |
| 19 | @requestScoped selection-set widening pass | plan | yes | 11, 17 |
| 20 | @requestScoped coordinate-L1 runtime | resolve | yes | 7, 18, 19 |
| 21 | Cache trace + execution/engine config factory | resolve + execution | yes | 7, 11 |

Test and benchmark PRs are interleaved per the rules in section 4.

---

## 3. The PRs

For each PR: **Goal**, **Scope**, **Excludes**, **Dependencies**, **Acceptance criteria**, **Reviewer-guide doc**, **Mergeability**.

---

### PR 0 — astjson release and pin

**Goal:** ship a real astjson tag containing the copy primitives, then pin gqtools to it.

**Scope:**
- (astjson repo, separate) Land PR #16 / cut a tagged release containing:
  `StructuralCopy` + `StructuralCopyWithTransform` as `*Parser` methods,
  `Transform` / `TransformEntry` (`Entries`, `ArrayItem`, `Passthrough` — note: NO `Forced` field),
  package-level `DeepCopy`, the 2-return `MergeValues` / `MergeValuesWithPath`,
  and the existing value constructors + `MarshalTo`.
- (gqtools) Bump `v2/go.mod` and `execution/go.mod` from the current pseudo-version
  to the new real tag; confirm `go.work` and `go-arena v1.2.0` agree.

**Excludes:** any caching code; the `DeepCopyWithTransform`, `CoerceToString`,
and `MarshalToWithTransform`/`ParseBytesWithTransform` surface
(unused by the foundation — do not pull them in just because they exist on the branch).

**Dependencies:** none (root of the stack).

**Acceptance criteria:**
- astjson tag exists and is fetchable.
- `go build ./...` succeeds in both modules against the new tag.
- All call sites use the 2-return `MergeValues` form (grep shows no 3-tuple usage).

**Reviewer-guide doc:** [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md)
(documents which primitives the foundation requires vs the ship-along surface to drop,
the breaking `MergeValues` change, and the plan/fill alignment hazard).

**Mergeability:** pure dependency bump; no behavior change in gqtools.
Merge to the feature branch first so every later PR builds.

---

### PR 1 — Foundation: interfaces, integration seam, ADRs, specs

**Goal:** define the minimal stable contracts and the integration seam, with NO implementation.

**Scope:**
- `resolve` package: declare the backend interface `LoaderCache`
  (tiny: `Get(ctx, keys) ([]*CacheEntry, error)`, `Set(ctx, entries) error`, `Delete(ctx, keys) error`)
  and the `CacheEntry` struct (`Key`, `Value []byte`, `TTL`, `RemainingTTL`, `WriteReason`).
- Declare `EntityCacheInvalidationConfig` (`CacheName`, `IncludeSubgraphHeaderPrefix`) —
  deliberately separate from `plan.EntityCacheConfiguration` to keep `resolve` free of a `plan` import.
- Declare the empty `FetchCacheConfiguration` struct skeleton (field set, no behavior)
  and the `CacheKeyTemplate` interface signature only.
- Ship all the planning/spec docs: ADRs and the architecture spec.

**Excludes:** any method bodies, any loader changes, any flag plumbing, any key rendering.
The interface compiles but nothing implements or calls it inside gqtools.

**Dependencies:** PR 0.

**Acceptance criteria:**
- `go build ./...` passes.
- No existing test changes (pure addition).
- `LoaderCache` and `CacheEntry` doc comments match the contracts in
  [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md).

**Reviewer-guide doc:** [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md),
[adr/0001-foundation.md](./adr/0001-foundation.md).
The ADR records the foundational decisions:
two-level model, arena lifetime / StructuralCopy isolation invariant,
opt-in L2, default-off L1, and the resolve-must-not-import-plan boundary.

**Mergeability:** declarations only; zero runtime reach.

---

### PR 2 — Loader astjson copy helpers

**Goal:** add the four StructuralCopy wrapper helpers the cache read/write paths will use.

**Scope:**
- `loader_cache_transform.go`: four `Loader` helpers wrapping `p.StructuralCopy` / `p.StructuralCopyWithTransform`:
  `structuralCopyNormalized` (L2 write: rename alias→schema + project, `Passthrough=false`),
  `structuralCopyDenormalized` (L2 read: schema→alias, project),
  `structuralCopyNormalizedPassthrough` (L1 write: rename, keep all fields, `Passthrough=true`),
  `structuralCopyDenormalizedPassthrough` (L1 read: restore alias, keep all).
- The ephemeral `*Transform` builders (`buildNormalizeTransform` / `buildDenormalizeTransform`)
  over reusable `l.transformEntries` / `l.transforms` slabs.

**Excludes:** any caller in the loader (helpers are unused dead code until PR 7);
any cache key logic; any flag checks.

**Dependencies:** PR 1.

**Acceptance criteria:**
- Unit tests in `loader_cache_transform_test.go` cover all four helpers:
  alias normalize/denormalize round-trip, passthrough-keeps-unlisted, project-drops-unlisted,
  arena residency of `OutputKey` strings (GC-safety).
- `go test ./v2/pkg/engine/resolve/...` passes; `-race` clean.

**Reviewer-guide doc:** [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) section "Loader copy helpers".

**Mergeability:** new file with new methods + their tests; nothing else calls them.

---

### PR 3 — Cache-key templates

**Goal:** implement the two `CacheKeyTemplate` implementations and the `CacheKey` value type.

**Scope:**
- `caching.go`: `EntityQueryCacheKeyTemplate{Keys *ResolvableObjectVariable, TypeName}`
  → key JSON `{"__typename":"User","key":{"id":"123"}}`;
  `RootQueryCacheKeyTemplate{RootFields, EntityKeyMappings}`
  → key JSON `{"__typename":"Query","field":"x","args":{...}}` (args sorted alphabetically).
- `NewRootQueryCacheKeyTemplate(rootFields, entityKeyMappings)` constructor (precomputes batch metadata).
- `RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)`
  plus `IsEntityFetch()`, `BatchEntityKeyArgumentPath()`, `EntityMergePath(...)`.
- `EntityKeyMappingConfig` / `EntityFieldMappingConfig`, and `KeyField` + `ParseKeyFields`.
- Number-to-string coercion for entity keys (so int and string `@key` args produce identical keys).

**Excludes:** wiring templates into any fetch (the planner attaches them in PR 11);
arg-hash suffix application at resolve time (that consumes `Field.CacheArgs`, added with PR 5/11).

**Dependencies:** PR 1.

**Acceptance criteria:**
- Table-driven `cache_key_test.go` asserts the FULL `[]*CacheKey` (including `Item` pointer)
  for: single/composite/nested/array keys, prefix, missing/null variable → 0 keys,
  number coercion parity, root-field no-args/single/multiple/string/bool/array/object/null,
  and `EntityKeyMappings` derivation (simple ID, int→string, nested path, array-index, multiple mappings,
  partial-missing skips only that key).
- All assertions use `assert.Equal` on full values per repo convention.

**Reviewer-guide doc:** [directives/key-entity-caching.md](./directives/key-entity-caching.md)
section "Cache key shapes".

**Mergeability:** self-contained file + tests; templates are constructed only by tests, never by the planner yet.

---

### PR 4 — Plan-time config structs + lookups

**Goal:** add the declarative per-subgraph cache config carried in `FederationMetaData`.

**Scope:**
- `plan/federation_metadata.go`: add (with their JSON tags) and their lookup methods:
  `EntityCacheConfiguration` (+ `FindByTypeName`),
  `RootFieldCacheConfiguration` + `EntityKeyMapping` + `FieldMapping` (+ `FindByTypeAndField`),
  `MutationFieldCacheConfiguration` (+ `FindByFieldName`),
  `MutationCacheInvalidationConfiguration` (+ `FindByFieldName`),
  `SubscriptionEntityPopulationConfiguration` (+ `FindByTypeAndFieldName` — matches BOTH `TypeName` AND `FieldName`).
- Add the corresponding collections to the `FederationMetaData` / `FederationInfo` lookup surface:
  `EntityCacheConfig`, `RootFieldCacheConfig`, `MutationFieldCacheConfig`,
  `MutationCacheInvalidationConfig`.

**Excludes:** `RequestScopedField` (PR 17); any consumer of these lookups (visitor reads them in PR 11);
the `execution/engine` `SubgraphCachingConfig` container (PR 21).

**Dependencies:** PR 1.

**Acceptance criteria:**
- Pure additive struct + method additions, zero behavior change.
- Unit tests for each lookup: hit, miss (nil), and the `FindByTypeAndFieldName`
  both-fields-required footgun (empty `FieldName` must miss).
- `go build ./...` and existing plan tests pass unchanged.

**Reviewer-guide doc:** [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md)
and the per-directive specs under `directives/` that these config structs back.

**Mergeability:** additive structs + lookups; nothing populates or reads them yet.

---

### PR 5 — ProvidesData field-tree types + ComputeHasAliases

**Goal:** add the `Object`/`Field` shape metadata the cache uses for alias-aware normalization.

**Scope:**
- `resolve/node_object.go`: the cache-relevant fields on `Object`
  (`HasAliases`, `CacheAnalytics *ObjectCacheAnalytics`)
  and on `Field` (`OriginalName`, `CacheArgs []CacheFieldArg`).
- `CacheFieldArg{ArgName, VariableName}`, `ObjectCacheAnalytics{KeyFields, HashKeys, ByTypeName}`.
- `ComputeHasAliases(*Object) bool` (fast-path flag: true if any field is aliased or has `CacheArgs`).

**Excludes:** populating the tree (planner does that in PR 11);
the L1 optimizer that walks it (PR 12); any runtime consumption.

**Dependencies:** PR 1.

**Acceptance criteria:**
- `ComputeHasAliases` unit tests: no-alias→false, alias→true, CacheArg→true, nested.
- Existing `node_object` behavior unchanged (new fields default zero).

**Reviewer-guide doc:** [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) section "ProvidesData".

**Mergeability:** additive struct fields + one pure function; default-zero fields are inert.

---

### PR 6 — Per-request CachingOptions + Context plumbing

**Goal:** add the per-request toggles, all defaulting off.

**Scope:**
- `resolve/context.go`: `CachingOptions{EnableL1Cache, EnableL2Cache, EnableCacheAnalytics,
  L2CacheKeyInterceptor, GlobalCacheKeyPrefix}` and the `ExecutionOptions.Caching` field.
- `L2CacheKeyInterceptor` func type + `L2CacheKeyInterceptorInfo{SubgraphName, CacheName}`.
- The single heap-mode `astjson.DeepCopy(nil, c.Variables)` site for per-request variable isolation.

**Excludes:** the `Resolver`-level registry (`Caches`, `EntityCacheConfigs`, subscription callbacks) —
those land with the loader integration in PR 7 where they are first read;
any loader branch that checks the flags (PR 7).

**Dependencies:** PR 1.

**Acceptance criteria:**
- All flags default `false`; a request with zero config behaves identically to today.
- Unit test confirms `GetCacheStats()` returns an empty snapshot when `EnableCacheAnalytics` is false
  (zero-overhead guarantee).
- `go build ./...` passes.

**Reviewer-guide doc:** [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) section "Per-request seam".

**Mergeability:** new fields on an existing options struct; no branch reads them yet.

---

### PR 7 — Loader L1/L2 read+write integration (the first behavior PR)

**Goal:** make `EnableL1Cache`/`EnableL2Cache` actually cache single (non-batch) entity and root fetches.

**Scope:**
- `loader.go` / `loader_cache.go`:
  the L1 map (`l1Cache map[string]*astjson.Value`, main-thread, entity-only, field-widening),
  the bulk L2 lookup (one `Get` per instance batch, batch-failure-falls-through-to-subgraph),
  and the `mergeResult` funnel that merges `FromCache` via `MergeValues` with the load-bearing StructuralCopy.
- The working-copy-and-swap pattern for merge-into-existing L1 entries (never mutate a live entry in place).
- L2 write path: `structuralCopyNormalized` then `value.MarshalTo(nil)` to heap bytes for the backend.
- `ResolverOptions` caching fields: `Caches map[string]LoaderCache`,
  `EntityCacheConfigs map[string]map[string]*EntityCacheInvalidationConfig`.
- The key transform pipeline (`GlobalCacheKeyPrefix` → subgraph header hash → `L2CacheKeyInterceptor`),
  applied identically on read/write/delete.

**Excludes:** batch/partial loading (PR 8), negative cache (PR 9), analytics (PR 10),
mutation/subscription/extension paths (PR 13/14/15), shadow (PR 16), trace (PR 21),
@requestScoped coordinate L1 (PR 20).

**Dependencies:** PR 2 (copy helpers), PR 3 (key templates), PR 5 (ProvidesData), PR 6 (options).

**Acceptance criteria:**
- `l1_cache_test.go`: dedup (same entity fetched twice → 1 fetch), L1 field-widening guard,
  UseL1Cache-disabled gate.
- `l1_l2_cache_e2e_test.go` (in-package, mock DataSource): miss-then-hit for both L1 and L2.
- `loader_cache_copy_invariant_test.go`: the 2 invariant tests for the merge sites this PR introduces
  (cache-skip-fetch + the L1 merge path) — mutate a nested container post-merge,
  assert the cache entry stays intact.
- `-race` clean; with both flags off, all pre-existing loader tests pass byte-identically.

**Reviewer-guide doc:** [directives/key-entity-caching.md](./directives/key-entity-caching.md)
and the Copy Budget table in the resolve package's CLAUDE.md
(this PR establishes 2 of the 4 budgeted copies).

**Mergeability:** every new path is guarded by `EnableL1Cache`/`EnableL2Cache`,
which default off; the existing test suite exercises the off path unchanged.

---

### PR 8 — Batch + partial entity cache load

**Goal:** extend caching to batch entity fetches with partial-hit support.

**Scope:**
- `loader.go` batch path: all-miss, all-hit, partial-hit (only missed entities refetched when
  `EnablePartialCacheLoad`), multi-candidate projection merge, entity splice into batch arrays.
- The 2 remaining StructuralCopy merge sites (batch cache hit + batch partial response).

**Excludes:** negative cache, analytics, mutation, the root-field smart-backfill nuance (keep minimal).

**Dependencies:** PR 7.

**Acceptance criteria:**
- `batch_entity_cache_test.go`: all-miss→all-hit, partial-hit, multi-candidate merge, L2-disabled.
- The 2 remaining `TestCopyInvariant_*` tests (batch cache hit + batch partial response).
- Copy Budget table updated to 4 copies; benches added in the matching bench PR (section 4).

**Reviewer-guide doc:** [directives/key-entity-caching.md](./directives/key-entity-caching.md)
section "Batch + partial".

**Mergeability:** batch path only reachable when flags on and the fetch is a batch entity fetch.

---

### PR 9 — Negative cache

**Goal:** cache null / not-found entities as sentinels when `NegativeCacheTTL > 0`.

**Scope:** `loader.go` / `loader_cache.go` null-sentinel store/serve/TTL lifecycle;
`CacheKey.NegativeCacheHit`; `EntityCacheConfiguration.NegativeCacheTTL` consumption.

**Excludes:** analytics emission for negative hits (folded into PR 10).

**Dependencies:** PR 7.

**Acceptance criteria:** `negative_cache_test.go` — store/serve/TTL, mutation interaction,
overwrite-after-expiry, nullable-field regression guards. `-race` clean.

**Reviewer-guide doc:** [directives/key-entity-caching.md](./directives/key-entity-caching.md)
section "Negative cache".

**Mergeability:** off unless `NegativeCacheTTL > 0` in config; default zero.

---

### PR 10 — Cache analytics collector + snapshot

**Goal:** add the analytics collector and `GetCacheStats()` snapshot, gated by `EnableCacheAnalytics`.

**Scope:**
- `cache_analytics.go`: the pooled collector, `CacheAnalyticsSnapshot` and all event types
  (`CacheKeyEvent`, `CacheWriteEvent`, `FetchTimingEvent`, `ShadowComparisonEvent`,
  `MutationEvent`, `HeaderImpactEvent`, `CacheOperationError`, etc.) and enums.
- The derived-metric convenience methods (`L1HitRate`, `CachedBytesServed`, `EventsByEntityType`, …).
- `ctx.GetCacheStats()` snapshot-and-release semantics (call exactly once).
- Wire the collector record calls into the loader paths added in PR 7–9.

**Excludes:** shadow comparison events (PR 16), mutation events (PR 13), subscription (PR 15) —
those PRs add their own record calls; this PR ships the collector and the read paths.

**Dependencies:** PR 7.

**Acceptance criteria:**
- `cache_analytics_test.go`: collector record/merge, field hashing, entity counts,
  derived metrics, snapshot independence (snapshot dedups one event per `(CacheKey,Kind)`).
- With `EnableCacheAnalytics=false`, snapshot is empty and the collector is never allocated.

**Reviewer-guide doc:** [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) section "Analytics".

**Mergeability:** gated by `EnableCacheAnalytics`; default off → zero overhead, zero behavior change.

---

### PR 11 — Datasource key-template + visitor caching wiring (the second behavior PR)

**Goal:** make the planner emit non-empty `FetchCacheConfiguration` and `ProvidesData` per fetch.

**Scope:**
- `graphql_datasource.go` `ConfigureFetch`: build `CacheKeyTemplate`
  (entity: `EntityQueryCacheKeyTemplate` over `@key`-only fields; root: `NewRootQueryCacheKeyTemplate`),
  fill `RootFieldL1EntityCacheKeyTemplates`.
- `plan/caching_planner_state.go`: `cachingPlannerState` (`plannerObjects`, field-stack, response paths,
  entity-boundary paths), `trackFieldForPlanner`, `createFieldValueForPlanner`, `captureFieldCacheArgs`,
  `isEntityBoundaryField`, and `configureFetchCaching` (the central L2 on/off decision).
- `visitor.go` wiring: `v.caching = newCachingPlannerState(...)`, the walk hooks, and the final
  `external.Caching = v.caching.configureFetchCaching(...)` + `ProvidesData` attachment.
- `plan.Configuration` flags `DisableEntityCaching` and `DisableFetchProvidesData`.

**Excludes:** `UseL1Cache` enablement (still false here; PR 12 sets it),
mutation impact (PR 13), subscription population (PR 15), @requestScoped population (PR 18/19).

**Dependencies:** PR 3 (templates), PR 4 (config lookups), PR 5 (ProvidesData types).

**Acceptance criteria:**
- Datasource-package tests assert the full rendered key shape for entity and root fetches.
- `configureFetchCaching` unit tests: entity path (config present → Enabled config; absent → L2 off but KeyFields kept),
  root-field path (all root fields must share identical config else L2 off), hard gates.
- Existing planner snapshot tests updated where `Caching`/`ProvidesData` now appear on fetches;
  with `DisableEntityCaching`, plans are byte-identical to today.

**Reviewer-guide doc:** [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) section "Planner wiring",
plus the ADR for the entity-boundary path-derivation risk
([adr/0002-planner-cache-config.md](./adr/0002-planner-cache-config.md)).

**Mergeability:** behavior is gated by config presence (opt-in) and `DisableEntityCaching`;
no router populates `FederationMetaData` caching collections until PR 21 + the router stack,
so on the feature branch this PR only changes plans for tests that explicitly set config.

---

### PR 12 — optimizeL1Cache postprocess pass

**Goal:** turn `UseL1Cache` on only where a provider/consumer relationship exists for the same entity type.

**Scope:**
- `postprocess/optimize_l1_cache.go`: the `FetchTreeProcessor` collecting `entityFetchInfo` /
  `rootFieldProviderInfo`, computing `canRead`/`canWrite` via field-tree containment
  (`objectProvidesAllFields`), and setting `UseL1Cache`.
- Register it LAST in `postprocess.go`'s `processFetchTree` chain (after `createConcreteSingleFetchTypes`),
  gated by `opts.disableOptimizeL1Cache`.

**Excludes:** anything outside `resolve.FetchTreeNode` + public fetch types (the pass must not import `plan`).

**Dependencies:** PR 5 (ProvidesData), PR 11 (so concrete fetches carry `Caching`/`ProvidesData`).

**Acceptance criteria:**
- `optimize_l1_cache_test.go`: provider-then-consumer → UseL1Cache true on both;
  no consumer → root field UseL1Cache false; ancestor-union coverage; disable flag → no-op.
- Ordering test: pass runs after concrete-fetch conversion.

**Reviewer-guide doc:** [adr/0003-l1-optimizer.md](./adr/0003-l1-optimizer.md)
(records the false-then-widen default decision and the must-run-last ordering coupling).

**Mergeability:** behind `disableOptimizeL1Cache`; when disabled it is a safe no-op
(UseL1Cache stays false, L1 simply off everywhere).

---

### PR 13 — Mutation impact (populate + invalidate)

**Goal:** support mutation-triggered L2 populate and entity invalidation.

**Scope:**
- `plan/caching_planner_state.go`: `configureMutationEntityImpact` →
  `resolve.MutationEntityImpactConfig`; `MutationFieldCacheConfig` / `MutationCacheInvalidationConfig` reads.
- `resolve` loader: mutations always skip L2 reads; skip L2 writes unless `EnableEntityL2CachePopulation`;
  delete the impacted entity key (skip delete if being written same fetch); `MutationEvent` analytics.

**Excludes:** subscription (PR 15), extension invalidation (PR 14).

**Dependencies:** PR 7 (loader), PR 11 (planner).

**Acceptance criteria:**
- `mutation_cache_test.go`: `navigateProvidesDataToField`, `buildEntityKeyValue`,
  `buildMutationEntityCacheKey`, `detectMutationEntityImpact`, TTL override.
- E2E mutation-skips-L2-read + mutation-invalidation (Delete log entry) tests.

**Reviewer-guide doc:** [directives/mutation-invalidation.md](./directives/mutation-invalidation.md).

**Mergeability:** mutation paths only reached for mutation operations with config present.

---

### PR 14 — Extension-driven invalidation

**Goal:** delete L2 entries when a subgraph returns `extensions.cacheInvalidation.keys`.

**Scope:** `loader_cache.go` parse of the extension payload, build matching L2 keys
(full transform pipeline), `LoaderCache.Delete` per key, skip keys being written same fetch.
Requires `ResolverOptions.EntityCacheConfigs` populated + `EnableL2Cache`.

**Excludes:** mutation/subscription invalidation (their own PRs).

**Dependencies:** PR 7.

**Acceptance criteria:** `extensions_cache_invalidation_test.go` — Delete + analytics;
key reconstruction includes `GlobalCacheKeyPrefix`.

**Reviewer-guide doc:** [directives/extension-invalidation.md](./directives/extension-invalidation.md).

**Mergeability:** only fires when a subgraph emits the extension AND L2 is enabled AND configs present.

---

### PR 15 — Subscription entity population

**Goal:** per-event subscription populate / invalidate of L2.

**Scope:**
- `plan/caching_planner_state.go`: `configureSubscriptionEntityCachePopulation` →
  `resolve.SubscriptionEntityCachePopulation` (Populate vs Invalidate;
  abstract-type resolution via union/interface; `FindByTypeAndFieldName` needs BOTH fields).
- `resolve`: per-event write (Populate) or delete-when-only-@key (Invalidate);
  `ResolverOptions.OnSubscriptionCacheWrite` / `OnSubscriptionCacheInvalidate` callbacks.

**Excludes:** non-subscription paths.

**Dependencies:** PR 7, PR 11.

**Acceptance criteria:**
- `federation_subscription_caching_test.go` via `NewManualFederationSetup` + product subscription `Emit()`:
  populate writes across events; invalidate deletes; `t.Cleanup` registered immediately after creation.
- Empty-`FieldName` config is a silent no-op (regression guard test).

**Reviewer-guide doc:** [directives/subscription-population.md](./directives/subscription-population.md).

**Mergeability:** subscription-only; config-gated; both fields mandatory.

---

### PR 16 — Shadow mode

**Goal:** read/write L2 but always serve fresh and compare (staleness measurement).

**Scope:** `resolve` loader shadow path (`ShadowMode` on entity + root config);
`ShadowComparisonEvent` analytics (cached vs fresh hash/bytes, freshness rate);
never-serve-cached behavior.

**Excludes:** any change to non-shadow serve logic.

**Dependencies:** PR 7, PR 10 (analytics).

**Acceptance criteria:** analytics tests assert `Shadow:true` on reads,
`ShadowFreshnessRate`, and that cached values are never served in shadow mode.

**Reviewer-guide doc:** [directives/shadow-mode.md](./directives/shadow-mode.md).

**Mergeability:** off unless `ShadowMode` set in config.

---

### PR 17 — @requestScoped composition (metadata + lookups)

**Goal:** add the `@requestScoped` plan-time metadata and lookups (composition side, no runtime).

**Scope:**
- `plan/federation_metadata.go`: `RequestScopedField{FieldName, TypeName, L1Key}`
  in `FederationMetaData.RequestScopedFields`; lookups `RequestScopedFieldsForType`,
  `RequestScopedExportsForField`; `InterfaceObjects []EntityInterfaceConfiguration`
  for concrete→interface mapping; `RequiredFieldsByKey`.
- Composition validation: `key` mandatory; warn when a key appears on only one field in a subgraph.

**Excludes:** datasource emission (PR 18), widening (PR 19), runtime injection (PR 20).

**Dependencies:** PR 4.

**Acceptance criteria:**
- Lookup unit tests (symmetric model: each field is both reader and writer; same `L1Key` → shared entry).
- Composition validation tests for the mandatory-key and single-field-warning rules.

**Reviewer-guide doc:** [directives/request-scoped.md](./directives/request-scoped.md),
[adr/0004-request-scoped.md](./adr/0004-request-scoped.md).

**Mergeability:** additive metadata + lookups; no emitter or runtime reads them yet.

---

### PR 18 — @requestScoped datasource emission

**Goal:** emit a `resolve.RequestScopedField` per `@requestScoped` field during `ConfigureFetch`.

**Scope:** `graphql_datasource.go`: for root fetches iterate root fields →
`RequestScopedExportsForField`; for entity fetches iterate `RequestScopedFieldsForType`
PLUS interface types via `InterfaceObjects`; dedup by `FieldName\x00L1Key`;
map schema name → response key. `ProvidesData` left nil here (visitor fills it).
Add `resolve.RequestScopedField{FieldName, FieldPath, L1Key, ProvidesData *Object}`
to `FetchCacheConfiguration`.

**Excludes:** `ProvidesData` population (the visitor step is in PR 19/the visitor-completion step);
runtime injection (PR 20); widening (PR 19).

**Dependencies:** PR 11 (datasource ConfigureFetch caching path), PR 17 (lookups).

**Acceptance criteria:** datasource tests assert the emitted `RequestScopedField` slice
(response-key mapping, interface-object dedup, symmetric emission).

**Reviewer-guide doc:** [directives/request-scoped.md](./directives/request-scoped.md) section "Emission".

**Mergeability:** emits data only; the runtime ignores `RequestScopedFields` until PR 20.

---

### PR 19 — @requestScoped selection-set widening + ProvidesData population

**Goal:** widen co-keyed `@requestScoped` selection sets to an identical L1 shape,
and populate `ProvidesData` on each `RequestScopedField`.

**Scope:**
- `node_selection_visitor_request_scoped.go`: `propagateRequestScopedWidening()` in LeaveDocument —
  group by `{l1Key, dsHash}`, build the union selection set for groups of ≥2 same-return-type participants,
  mint synthetic aliases on collision, inject missing fragments back into the operation;
  share the `requestScopedVisibleResponseKeys` / `requestScopedFetchAliases` maps via `setRequestScopedMaps`.
- `populateRequestScopedFieldsProvidesData` in the visitor: match each field by response key,
  drop hints whose field is not an Object, set `ProvidesData` + `ComputeHasAliases`.

**Excludes:** runtime injection/export (PR 20).

**Dependencies:** PR 11 (visitor), PR 17 (lookups).

**Acceptance criteria:**
- `request_scoped_widening_test.go` (datasource): union built, conservative early-returns,
  interface-object fallback.
- `request_scoped_provides_data_test.go`: nil plannerObj leaves fields unchanged;
  no-match/scalar drops the hint; alias matches by response key; per-field sub-Object pointer identity.

**Reviewer-guide doc:** [directives/request-scoped.md](./directives/request-scoped.md) section "Widening".

**Mergeability:** widening rewrites the AST only when ≥2 co-keyed fields exist;
absent the directive it is a no-op. `ProvidesData` is data the runtime ignores until PR 20.

---

### PR 20 — @requestScoped coordinate-L1 runtime

**Goal:** inject from / export to the per-request coordinate L1 so co-keyed fields share a fetch.

**Scope:**
- `loader.go`: `requestScopedL1 map[string]*astjson.Value` (main-thread only);
  `tryRequestScopedInjection` (Phase 1.5, Phase 3.5, `resolveSingle`) and `exportRequestScopedFields`.
- Field-widening check via `validateItemHasRequiredData` (collect-then-inject, all-or-nothing);
  copy-on-inject (`structuralCopyDenormalized`) and copy-on-export (`structuralCopyNormalized`);
  `EnableL1Cache` gating; `LoadSkipped` trace + the `cacheTraceRequestScopedHits` counter fold.

**Excludes:** any change to entity L1/L2 paths.

**Dependencies:** PR 7 (loader + L1), PR 18 (emitted fields), PR 19 (ProvidesData + widening).

**Acceptance criteria:**
- `request_scoped_test.go`: injection (no-hints/missing-key/widening-rejects-narrow/all-or-nothing/L1-gating),
  export (copy-on-export, independence), round-trip, GC-survival + arena-residency, alias handling,
  synthetic alias.
- E2E `federation_caching_request_scoped_test.go` un-skipped with EXACT subgraph call-count assertions
  (no fuzzy `if calls == 0` smoke checks).

**Reviewer-guide doc:** [directives/request-scoped.md](./directives/request-scoped.md) section "Runtime",
[adr/0004-request-scoped.md](./adr/0004-request-scoped.md).

**Mergeability:** gated on `EnableL1Cache`; no `RequestScopedFields` are emitted unless the directive
is present in composition, so default behavior is unchanged.

---

### PR 21 — Cache trace + execution/engine config factory

**Goal:** add the per-fetch cache trace and the router-facing config container.

**Scope:**
- `resolve/trace.go`: `CacheTrace` + `CacheTraceEntity`, gated on
  `ctx.TracingOptions.Enable && !ctx.TracingOptions.ExcludeCacheStats`,
  emitted under `extensions.trace.fetches[].fetch.trace.cache_trace`.
- `execution/engine/config_factory_federation.go`: `SubgraphCachingConfig` container (5 plan.* slices)
  + `SubgraphCachingConfigs.FindBySubgraphName`,
  `WithSubgraphEntityCachingConfigs(...)` option, and `dataSourceMetaData()` copying the slices
  onto each datasource's `FederationMetaData` with the 3-tier subgraph-name fallback.

**Excludes:** router-side wiring (that is the [04-PR-PLAN-router.md](./04-PR-PLAN-router.md) stack).

**Dependencies:** PR 7 (runtime), PR 11 (planner config consumed by the factory).

**Acceptance criteria:**
- `federation_caching_trace_test.go`: trace populated when tracing on, absent when off,
  `Keys` present only when `!ExcludeRawInputData`.
- `config_factory_federation` test: the 5 slices land on `FederationMetaData` for the right subgraph.
- This is the first PR where a full E2E gateway can opt in via `WithSubgraphEntityCachingConfigs`.

**Reviewer-guide doc:** the public-API integration guide referenced in
[01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) section "execution/engine surface".

**Mergeability:** the factory copies config only when a caller passes
`WithSubgraphEntityCachingConfigs`; trace only fires under tracing options.
Both default off.

---

## 4. Test and benchmark PRs (interleaving rule)

Per the repo convention, tests ship **with** the PR that implements the behavior they cover
(every behavior PR above lists its unit/E2E tests in its acceptance criteria).
Two categories are pulled out into their own small PRs to keep each diff under ~30 minutes:

- **Large E2E analytics suites.** `federation_caching_analytics_test.go` (~120KB) is split per concern
  (L1 integration, L2 integration, shadow, mutation events) into separate files,
  each landing right after its corresponding behavior PR (10, 13, 16).
  Reason: full-snapshot assertions are correct but too large to review bundled with the implementation.

- **Benchmarks.** Benchmarks land in dedicated PRs immediately after the behavior they measure:
  - After PR 7–8: `loader_cache_copy_bench_test.go` + `loader_noncaching_bench_test.go`
    (the Copy-Budget pair — must stay 1:1 with the 4 `TestCopyInvariant_*` tests and the Copy Budget table;
    update all three together).
  - After PR 7: `caching_overhead_bench_test.go` (the Disabled → ConfiguredButDisabled → L1Only →
    L1L2_Miss → L1L2_Hit ladder; `ConfiguredButDisabled` catches guard leaks).
  - After PR 2: `structural_copy_bench_test.go` (the 8 primitive benches, NoTransform vs WithTransform).
  - After PR 10: `cache_analytics` micro-benches (Disabled vs Enabled, field hashing).

The full taxonomy, conventions, and the acceptance-criteria sync rule are in
[06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md).
Every test PR must update `docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md`
(path + line + test name per AC) — this is a hard requirement.

---

## 5. Explicitly out of scope for this stack

These topics surfaced during analysis but are NOT entity caching.
They must NOT be bundled into the PRs above — see [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md):

- the `onError` / `ErrorBehavior` request-extension feature (shares `ExecutionOptions` + `execution_engine.go`);
- the `service_datasource` `__service` capabilities endpoint;
- embedded `@requires`/`@provides` planner correctness changes that alter NON-cached planning;
- the federationtesting gateway/test-harness rewrite (land as its own harness PR BEFORE the E2E test PRs);
- subscription-client transport bug fixes (SSE leak eviction + WS legacy subprotocol);
- dependency DOWNGRADES in `v2/go.mod` (stale-base artifacts — only the astjson + go-arena BUMPS are real).
