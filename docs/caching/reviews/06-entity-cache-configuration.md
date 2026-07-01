# Reviewer notes — task 06: cacheKeyBuilder + fetchCacheConfigurator (entity arm)

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/06-entity-cache-configuration.md](../tasks/06-entity-cache-configuration.md).
Spec background: RFC-2 §6–§7 (D6 names); deviations D3, D5, D6.

## What this commit adds

Entity fetches now carry runtime cache config: the `cacheKeyBuilder` turns every resolvable `@key` set into multi-key candidates BY VALUE (the sole federation reader), and the `fetchCacheConfigurator`'s entity arm assembles and sets `*resolve.FetchCacheConfig` on entity/batch-entity fetches through the `Fetch` interface.
Root-field configuration remains for task 13; entity-key mappings for task 15.

## Decisions made

- Port source: the first-pass "freezer"/"stamper", renamed and reshaped per D6, with the entity arm only:
  the root-field branch, `EntityKeyMappings` derivation, and the first pass's `fetchIsEntity` type-switch (replaced by the D8 `IsEntityFetch`/`IsBatchEntityFetch` predicates; the switch also special-cased pre-conversion `SingleFetch`, unnecessary because the configurator runs strictly after `createConcreteSingleFetchTypes`) were not ported.
- HARDENING beyond the first pass: a `@key` whose fields do not exist in the schema is now rejected as malformed.
  The representation walker silently drops unknown fields, so such a key degraded to a `__typename`-only candidate in the first pass — a key that would collide across ALL entities of the type.
  `buildEntitySpec` skips any candidate whose representation has no field beyond `__typename` (WHY comment at the site); if every key is malformed the entity is simply not cached.
- Candidates are deterministically ordered by selection-set string; value-copy out is guaranteed structurally (each candidate node is built fresh by the shared `representationvariable` builder) and pinned by the aliasing-gate test.
- `L1 = true` marks ELIGIBILITY (task 16 narrows); `L2 = TTL > 0 || NegativeCacheTTL > 0` (D3).
- `ComputeHasAliases` (task 05 leftover) lands here with its first caller: the configurator folds it over the fetch's `ProvidesData` tree (the side-table's tree itself, not a copy — pinned by a `assert.Same` row).
- The all-flags-false safety net (`!L1 && !L2 && !ShadowMode → nil`) is unreachable for the entity arm today (a found policy implies `L1`); it exists for the task-13 root-field arm where all-off configs are possible.

## What was implemented

- `v2/pkg/engine/cache/cache_key_builder.go` — `buildEntitySpec` (entity index check, sorted key sets, per-candidate build via `representationvariable.BuildRepresentationVariableNode`, malformed-candidate skip, zero-candidate bail).
- `v2/pkg/engine/cache/fetch_cache_configurator.go` — `configureTree` (recursive walk, `SetCacheConfig` via the interface) and `buildConfig` (provider lookup by `FetchInfo.DataSourceID`, entity policy by `RootFields[0].TypeName`, spec + scalar policy fields + `ProvidesData` + `ComputeHasAliases`).
- `v2/pkg/engine/resolve/node_object.go` — `ComputeHasAliases`/`computeNodeHasAliases` (ported; sets the flag on every object along the way).

Tests:

- `cache_key_builder_test.go` — full `CacheKeySpec` literals for single and composite/nested keys; MULTIPLE key sets registered in reverse order come out sorted; no-`@key`/unknown-datasource/nil-info → `(zero, false)`; broken-candidate-skipped and all-broken rows; the ALIASING GATE (mutate the source `FederationMetaData` and the builder's map copy after building; spec unchanged); candidate node equals the datasource's representation node for the same `@key` (same shared builder).
- `fetch_cache_configurator_test.go` — FULL `*FetchCacheConfig` on `EntityFetch` (incl. `HasAliases` folded and `assert.Same` on `ProvidesData`) and `BatchEntityFetch`; zero-TTL row keeps `L1` with `L2` off; five nil rows (SingleFetch-until-13, no provider, no policy, no key, nil info).
- Plan-level via the task-04 harness (`execution/cachingtesting/entity_config_test.go`) — full rendered config per fetch for a sync plan (reviews batch entity configured, root fetch nil), a DEFER plan (initial `me.favoriteProduct` inventory fetch AND the deferred-group inventory fetch both configured), and determinism (plan twice, identical rendering).

## What to look into (review focus)

- The one-file federation boundary: `cache_key_builder.go` — confirm nothing retains a pointer into `FederationMetaData` (candidates are freshly built nodes; the spec carries strings and nodes only).
- The `__typename`-only candidate rejection: confirm the `len(node.Fields) < 2` guard is the right malformed-key detector (the representation node always leads with `__typename`).
- Configurator placement is unchanged from task 03 (after `createConcreteSingleFetchTypes`, before `organizeFetchTree`); the defer-plan test proves defer-group fetches are covered via `deferTrees`.
- No "freeze"/"stamp" vocabulary anywhere (D6) — grep `freeze|stamp` over `v2/pkg/engine/cache` and the tests.

## Verification evidence

- All cache-package and harness tests pass (first run for the plan-level rows).
- Full `v2` and `execution` suites pass (see PROGRESS.md notes for the run).
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
