# Commit D1 — optimizeL1Cache cross-tree narrowing

Plan item: `docs/caching/PLAN.md` §6, D1.
RFC sections: RFC-2 §10, §10.1 (cross-tree), §10.2, PR6.
Phase: D (L1 caching).
OLD reference: `caching-base` `postprocess/optimize_l1_cache.go`.

## Problem

The stamper marks every entity fetch L1-eligible (`cfg.L1 = true`), but L1 only helps where a request-lifetime provider/consumer pair exists.

## Solution

Fill the inert `optimizeL1Cache` (P3) skeleton with a PURE NARROWING pass (ported from OLD):

- `processTrees(roots...)` no-ops when `disable` (no providers). Otherwise it collects entity fetches across ALL trees (root + every `Defers[i].Fetches`), capturing `entityType`, `ProvidesData`, and `DependsOnFetchIDs`.
- For each L1-eligible entity fetch, `cfg.L1 = canRead || canWrite`:
  - `canRead` = a prior fetch (or the union of priors) of the same entity type provides a SUPERSET of this fetch's fields (`hasValidProvider`/union);
  - `canWrite` = a later fetch of the same type needs a SUBSET (`hasValidConsumer`);
  - ordering resolved purely from `DependsOnFetchIDs`.
  When neither holds, `setL1(fetch, false)` (the three-arm switch guarding `f.Cache != nil`).
- The field-coverage primitives (`objectProvidesAllFields`/`nodeProvidesAllFields`/`treeContainsAllFields`/`unionObjects`) port verbatim (they already operate on `resolve.Object`/`resolve.Field`).
- P3 NEVER sets `cfg.L1 = true` (narrowing only; conservative-safe — a wrong narrowing only forgoes an optimization).

## Key decisions

- Cross-tree: the L1 store is request-lifetime and shared across defer groups, so a root-tree provider can serve a defer-group consumer; the pass spans all trees in one collection (RFC-2 §10.1).
- The optional §10.2 re-null of a fully-inert `Cache` was NOT applied (kept the diff minimal).

## Tests

`postprocess/optimize_l1_cache_test.go` (ported): a fetch with no provider/consumer -> L1 narrowed off; a provider+consumer pair -> L1 stays on; union-of-priors keeps L1 on; transitive dependency chain establishes a prior provider; an ineligible fetch is never turned on or used as a provider; a CROSS-TREE `processTrees(root, deferGroup)` case keeps a provider's L1 on for a defer-group consumer; determinism. Full-value `assert.Equal`.

Golden updates: `StageL2Entities` and `EndToEnd_L2EntityHit` now show the solitary Product entity fetch as `l1:false` — the correct, expected effect of narrowing (no L1 provider/consumer pair in those single-entity-fetch queries).

Verification:

- `cd v2 && go test ./pkg/engine/postprocess/... ./pkg/engine/plan/... ./pkg/engine/resolve/... -count=1` — PASS; StageNoop planner no-op golden byte-identical (P3 no-ops when disabled).
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (two single-entity goldens updated to `l1:false`).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/postprocess/...` — clean.

## Reviewer guidance

- P3 never turns L1 on; narrowing wrong only costs a hit, never correctness.
- The golden `l1:true -> l1:false` changes are the feature (no provider/consumer pair).
- The L1 RUNTIME is D2.
