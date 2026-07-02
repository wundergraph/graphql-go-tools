# Reviewer notes — task 16: optimizeL1Cache cross-tree narrowing

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/16-optimize-l1-pass.md](../tasks/16-optimize-l1-pass.md).
Spec background: RFC-2 §10; OLD `postprocess/optimize_l1_cache.go`; CODING_GUIDELINES §10.5.

## What this commit adds

The `optimizeL1Cache` skeleton (inert since task 06) is now the real pass: after the configurator marks every entity fetch L1-eligible, the pass keeps `cfg.L1` only where a request-lifetime provider/consumer pair exists — `canRead` (a prior same-type fetch or union of priors provides a superset) or `canWrite` (a later same-type fetch needs a subset this fetch contributes to).
Narrowing only; the pass NEVER turns L1 on.

## Decisions made

- ORDERING HAS TWO SOURCES, not one: dependency edges (direct + transitive) AND tree order — the initial response tree always completes before any defer group starts, so an initial-tree fetch executes before every defer-group fetch even across branches with NO dependency edge.
  The task text said "purely from DependsOnFetchIDs", but that alone cannot see the initial-inventory → deferred-inventory pair in the defer fixture (different branches, no edge) and would defeat the cross-tree requirement the same task demands; tree order is the missing, still-conservative source.
  Defer groups among themselves stay UNORDERED (they may run in parallel) — pinned by its own row.
- Field matching uses `fieldNarrowingName` — SCHEMA name (`OriginalName` when aliased) plus the CacheArgs bindings — instead of the OLD raw-`Name` comparison: the L1 store will hold normalized values, and fields selected with different arguments are different cache fields.
  Mismatches only ever produce false NEGATIVES (over-narrowing → missed hits), never false positives.
- The union-fallback contribution gate (`objectSharesAnyField`) is ported as specified — a provider is kept via the union only when it contributes at least one field the shared consumer needs; the irrelevant-provider row pins the flaw both ways.
- FIXED A FIRST-PASS ALIASING BUG the task did not list: the OLD `unionObjects` wrote the recursive merge into `existing.Value`, where `existing` is a field of a LIVE `ProvidesData` tree on a fetch config — the union computation silently mutated fetch configs.
  The port copies the field struct before overriding; the "union never mutates provider trees" row pins it.
- Re-nil of fully-inert configs (`!L1 && !L2 && !ShadowMode`) implemented as the last step per the task's option.
- The unused OLD helpers (`treeContainsAllFields`/`nodeContainsAllFields`) were dead code and were NOT ported.

## What was implemented

- `optimize_l1_cache.go` — the full pass (collection across all trees with `treeIndex`, eligibility filter, provider/consumer/union predicates, dependency-chain ordering, coverage primitives, safe union).
- The facade wiring from task 06 (`ConfigureCaching` → `l1.optimize(trees)`) was already in place; no facade change.
- Existing plan-level pins updated: the SYNC fixture's lone entity fetch now renders `l1:false` (correct narrowing); the DEFER fixture keeps `l1:true` on BOTH inventory fetches (the cross-tree pair — this is the live proof the tree-order source works over real plans).

Tests (`optimize_l1_cache_test.go`, 13 rows):

- Pair kept; lone fetch off; inert re-nil; NEVER-turns-on (ineligible provider also strands its consumer); union coverage keeps all three; the FLAW PIN (irrelevant provider off, relevant kept); partial overlap (both off); empty union; transitive chain ordering; CROSS-TREE (root provider kept by a deferred consumer; per-tree-only behavior explicitly rejected by re-running with the root tree alone); defer↔defer unordered; determinism (second run identical); union-mutation regression row.

## What to look into (review focus)

- The tree-order ordering source — the one deviation from the task's letter, argued above; reject it and the task's own defer row becomes unsatisfiable.
- The narrowing-name choice (schema name + args): confirm over-narrowing-only is the correct failure direction for every row.
- The union-mutation fix: compare against the OLD `unionObjects` if in doubt (`existing.Value = unionObjects(...)` on a live tree).

## Verification evidence

- All 13 rows pass; the full plan suite and harness suite pass with the two updated pins; `-race` clean in both modules.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
