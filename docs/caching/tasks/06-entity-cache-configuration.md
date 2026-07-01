# Task 06 — Entity cache configuration: cacheKeyBuilder + fetchCacheConfigurator (entity arm)

Phase: A (L2 entities).
Dependencies: tasks 01, 03, 05.
References: RFC-2 §6, §7 (renamed per D6); deviations D3, D5, D6 (PLAN §7).

## Problem

Entity fetches carry no cache config, so the runtime has no keys, no coverage tree, and no policy to act on.
Federation `@key` data must cross into runtime config exactly once, by value, in one reviewable unit.

## Scope

Both units live in the common `v2/pkg/engine/cache` package (D5), invoked by the postprocess facade from task 03.

`cacheKeyBuilder` (the SOLE federation reader; was "freezer"):

- For an entity fetch, build one `CacheKeyCandidate` per resolvable `@key` set via `representationvariable.BuildRepresentationVariableNode` (task 01) — deterministically ordered by selection-set string, none marked required (best-effort multi-key).
- Skip a malformed `@key` fragment rather than fail the whole spec (annotate WHY at the skip site); zero usable keys → no entity spec → not cached.
- Copies OUT by value; retains no pointer into `FederationMetaData`.

`fetchCacheConfigurator` (was "stamper"), entity arm:

- Walk the finished fetch tree (after `createConcreteSingleFetchTypes`), and for each entity fetch (via the task 02 `Fetch` predicates, no type switch):
  look up `EntityPolicy(typeName)`; build the spec via `cacheKeyBuilder`; assemble `FetchCacheConfig` — `L1 = true` (eligible; task 16 narrows), `L2 = TTL > 0 || NegativeCacheTTL > 0` (D3), scalar policy fields, `KeySpec`, `ProvidesData` from the task 05 side-table (`pd[fetch.FetchInfo()]`), fold `ComputeHasAliases`.
- Set via `fetch.SetCacheConfig(cfg)`; leave nil when no policy / no keys / all-flags-false.
- Tolerate `Info == nil` → nil config (belt-and-suspenders under the task 03 FetchInfo precondition).

## Tests

- `cacheKeyBuilder` table tests: FULL multi-key `CacheKeySpec` for single/composite/nested/MULTIPLE `@key` sets (one candidate each, ordered); no-`@key` → `(zero, false)`; mutate the source `FederationMetaData` after building and re-assert equality (no pointer aliasing); assert the builder's candidate node equals the datasource's representation node for the same `@key` (same shared builder).
- Configurator: FULL `*FetchCacheConfig` asserted on entity and batch-entity fetches; nil where policy absent (all four no-op gates); the carrier regression row — a tree through dedup + fetchID-append + concrete-type conversion still resolves the right `ProvidesData` via `fetch.FetchInfo()`.
- Plan-level rows via the task 04 harness: entity fetches across sync AND defer plans (each `Defers[i].Fetches` entity fetch carries `Cache`), asserted with full-value `assert.Equal` on the rendered config; determinism (plan twice, identical).

## Acceptance criteria

- [ ] Federation is read in exactly ONE unit (`cacheKeyBuilder`); no federation type or pointer reaches `FetchCacheConfig`.
- [ ] All three concrete fetch types receive config through the `Fetch` interface methods.
- [ ] The no-op plan proof still holds with caching unconfigured.
- [ ] No "freeze"/"stamp" vocabulary in code, comments, or tests (D6).
- [ ] Lint-clean.

## Reviewer guidance

- One-file review for the federation boundary: read `cacheKeyBuilder` and confirm value-copy out, no retained pointers.
- Confirm the configurator runs after concrete-type conversion (config set before conversion is lost for entity types).
