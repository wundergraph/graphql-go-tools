# Task 16 — optimizeL1Cache cross-tree narrowing

Phase: D (L1).
Dependencies: task 06.
References: RFC-2 §10; OLD `postprocess/optimize_l1_cache.go` on the `caching-base` worktree; CODING_GUIDELINES §10.5.

## Problem

The configurator marks every entity fetch L1-eligible, but L1 only helps where a request-lifetime provider/consumer pair exists; everywhere else the eligibility is wasted work at runtime.

## Scope

A pure NARROWING pass in `v2/pkg/engine/cache`, run by the facade AFTER the configurator, ONCE over ALL trees:

- Collect entity fetches across the root tree AND every `Defers[i].Fetches` (the L1 store is request-lifetime and shared across defer groups, so provider/consumer pairs span trees).
- For each: `cfg.L1 = canRead || canWrite` —
  `canRead`: a prior fetch (or union of priors) of the same entity type provides a SUPERSET of this fetch's `ProvidesData`;
  `canWrite`: a later fetch of the same type needs a SUBSET of this fetch's `ProvidesData`;
  ordering resolved purely from `DependsOnFetchIDs`.
- The configurator is the sole eligibility setter; this pass NEVER turns L1 on.
- Port the OLD field-coverage primitives (`objectProvidesAllFields`, union helpers, dependency-chain ordering); fix, do not inherit, the OLD `hasValidConsumer` union-fallback flaw (a provider counted as reused even when it contributed no field the shared consumer needed).
- Optionally re-nil a fully-inert config (`!L1 && !L2 && !ShadowMode`) as the last step (tidy, not correctness).

## Tests

- Port the OLD provider/consumer/union/dependency-chain rows, PLUS adversarial rows the OLD set lacked: irrelevant-provider-sharing-a-consumer, partial overlap, empty union.
- A defer case: `processTrees(root, defer1, defer2)` asserting CROSS-tree provider/consumer narrowing (root provider kept L1 because its only consumer lives in a defer group).
- Never-turns-on row: a fetch the configurator left `L1 = false` stays false.
- Determinism: run twice, identical output.

## Acceptance criteria

- [ ] Narrowing only (no row can flip L1 on).
- [ ] Cross-tree pairs captured; per-tree-only behavior rejected by the defer row.
- [ ] The OLD union-fallback flaw is fixed and pinned by a test.
- [ ] Lint-clean.

## Reviewer guidance

- Wrong narrowing costs only a missed hit, never correctness — but insist on the adversarial rows anyway; this logic was the first pass's latent-bug source.
