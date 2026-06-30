# Commit C2 — root-field → entity-cache reuse at lookup

Plan item: `docs/caching/PLAN.md` §5, C2 (completes Phase C).
RFC sections: RFC-1 §3.6/§3.7 (`EntityKeyMappings` -> rendered candidates, best-effort render-then-backfill); RFC-2 §6.3.
Phase: C (root fields that re-use the entity cache).

## Problem

A by-key root field (`product(upc:"1")`, `user(id:"1")`) should hit the shared entity cache, but the controller did not render entity candidates from its `EntityKeyMappings`.

## Solution

Extend the root-field path in the controller: when `cfg.KeySpec.EntityKeyMappings` is non-empty, render ENTITY candidates from the request variables and look up the entity key space (reusing the entity-key hashing), so the key COINCIDES with what an entity fetch wrote.

- For each `EntityKeyMapping{EntityTypeName, FieldMappings}`, build `{"__typename": EntityTypeName, "key": {<EntityKeyField>: <argValue>, ...}}` where `argValue = ctx.VariablesView().Get(FieldMapping.ArgumentPath...)` (honoring `RemapVariables`), numbers coerced to strings; render the key via the existing entity-key path. A mapping renders only when every field's arg value is present; unrenderable mappings become pending and are re-rendered from fetched/served data for backfill.
- `PrepareFetch`: `Get` under the rendered entity key(s); a covering hit serves (splice + reorder) and AND-reduces to `DecisionSkipFullHit`.
- `OnFetchResult`/`OnFetchSkipped`: write the entity under all renderable entity keys (refresh) and backfill the keys that became renderable from fresh/served data (E3/E5), reusing the A2b backfill machinery.
- For a mapped by-key root field the entity candidate keys are the read AND write targets; an unmapped (B2) root field keeps its whole-response key.

## Key decisions

- V1 cache-name reuse constraint: a by-key root-field policy must share `CacheName` with the corresponding entity policy for entries to coincide (read key == write key). Documented in PLAN.md C2; the `StageL2RootReusesEntity` test stage now configures `product`/`user` with the `entities` cache name (the golden's `cacheName` reflects this).
- Reuse is L2 only (root->entity L1 promotion is v2).

## Tests

- White-box `controller_rootreuse_test.go`: **E3** `OnFetchResult` backfills the mapped entity keys after a fetch (exact ordered `[Get upc, Set upc refresh, Set sku backfill]`), **E5** `OnFetchSkipped` read-hit backfill (`[Get upc, Set sku backfill]`, no network), and a reuse HIT served from a seeded `Product:upc` entity entry. Full-value `assert.Equal`.
- Execution `caching_rootreuse_e2e_test.go` `TestCaching_EndToEnd_RootReusesEntity`: over a shared `FakeStore`, prime the `Product` entity entry, then `{ product(upc:"1"){ name } }` (StageL2RootReusesEntity, shared `entities` cache name) is served from the entity entry with zero `product` subgraph load + identical bytes.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS.
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (StageL2RootReusesEntity golden updated to the shared cache name; other goldens unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- The entity candidate is rendered from `EntityKeyMappings` + request variables, keyed identically to an entity fetch (read key == write key).
- Backfill writes the keys that become renderable from the response (E3) or the served value (E5).
- Reuse requires the shared `CacheName` (the v1 constraint); root->entity L1 promotion and L1 are out of scope (D2/v2).
