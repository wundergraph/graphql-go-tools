# Abstract Selection Rewriter: Field Ref Provenance at Construction

**Date:** 2026-07-15
**Status:** Approved
**Supersedes:** 2026-07-14-abstract-rewriter-skip-refs-design.md (scope-chain matching)
**Files:** `v2/pkg/ast/ast_field.go`, `v2/pkg/ast/ast.go`,
`v2/pkg/engine/plan/abstract_selection_rewriter.go`,
`v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go`,
`v2/pkg/engine/plan/abstract_selection_rewriter_info.go`,
`v2/pkg/engine/plan/node_selection_visitor.go`

## Problem

When the abstract selection rewriter flattens fragments, it rebuilds the field's
selection subtree from scratch and must remap two ref-keyed structures that still
point at pre-rewrite refs:

- `skipFieldsRefs` — planner-added fields hidden from the response
- `fieldDependsOn` / `fieldRefDependsOn` — jump dependencies

The previous approaches *reconstructed* the old→new mapping structurally, by
walking the subtree before and after the rewrite and matching fields on their
query path with inline fragment names stripped. Stripping fragment names
collapses distinct response positions (`nodes.<on A>.id` vs `nodes.<on B>.id`
both become `query.nodes.id`), which conflated a planner-added (skipped) field
with a user-requested field — losing user fields from the response or leaking
planner fields into it.

The first fix (superseded design) kept path matching and added a per-level
type-condition **scope chain** to disambiguate collapsed positions. It worked,
but it re-derived — with two extra subtree walks and set-intersection
heuristics — information the rewriter itself had at the moment it built the new
subtree.

## Insight

The old→new mapping does not need to be reconstructed at all. During a rewrite,
new field refs come into existence at exactly three points, and at each point
the origin is locally known:

1. **Copy.** `createFragmentSelection` copies original selections via
   `ast.Document.CopySelection`, which funnels every field through
   `Document.CopyField` — a single choke point. The (originalRef, copyRef)
   pair is exact at that moment. `preserveTypeNameSelection` copies an
   explicitly requested `__typename` through the same choke point, which also
   preserves its directives (e.g. defer) verbatim.
2. **Synthesize.** `typeNameSelection` creates a `__typename` with no original
   (empty selection set fallback, interface-object case). The rewriter already
   appends these to `skipFieldRefs` itself; no mapping is needed.

After building, `replaceFieldSelections` normalizes the subtree with
`AbstractFieldNormalizer`. The only operations that *destroy* a field ref during
that normalization are field merges, and both merge sites
(`deduplicateFields`, `mergeInlineFragmentSelections.mergeFields`) funnel
through a single call: `Document.MergeFieldsDefer(left, right)`, where `left`
survives and `right` is removed. (`inlineSelectionsFromInlineFragments` only
relocates selections; it never merges or removes fields.)

So complete, exact provenance = **copy log** (recorded at points 1–2) composed
with **merge log** (recorded at `MergeFieldsDefer`). No paths, no scopes, no
extra walks, no matching heuristics.

## Design

### Component 1: observation hooks on `ast.Document`

Two optional callback fields on `Document`, nil by default, invoked with a nil
check; cleared in `Document.Reset()` (documents are pooled):

```go
// OnCopyField is called after CopyField copies a field, with the source and the new field ref.
OnCopyField func(originalRef, copyRef int)
// OnMergeFields is called by MergeFieldsDefer when the right field is merged into the left field.
OnMergeFields func(survivorRef, removedRef int)
```

- `CopyField` invokes `OnCopyField(ref, newRef)` before returning.
- `MergeFieldsDefer` invokes `OnMergeFields(left, right)`.

No signatures change; `astminify` (the only other copy caller) and the main
normalization pipeline (the other merge caller) are unaffected because the
callbacks are nil outside the rewriter's window.

### Component 2: provenance recording in the rewriter

`fieldSelectionRewriter` gains two logs:

```go
copyLog  []refPair // (originalRef -> newRef): CopyField hook, chronological
mergeLog []refPair // (removedRef -> survivorRef): MergeFieldsDefer hook, chronological
```

- `RewriteFieldSelection` sets both hooks on `r.operation` on entry and clears
  them with `defer`. (Copies happen during `rewriteXxxSelection`; merges happen
  during the normalizer run inside `replaceFieldSelections` — both inside the
  window. The needs-rewrite checks perform neither.)
- `preserveTypeNameSelection` copies the original `__typename` selection via
  `CopySelection`, so its provenance is recorded by the hook like any other
  copy and its directives survive verbatim. To know what to copy,
  `selectionSetInfo` gains `typenameSelectionRef int` (populated in
  `selectionSetFieldSelections`, `ast.InvalidRef` when absent).

Copies always source from pre-rewrite refs (selection infos are collected
before any mutation), so the copy log never chains. Merges can chain
(A merged into B, B later merged into C); chronological processing resolves
chains.

### Component 3: composition replaces `collectChangedRefs`

A single function builds both result maps from the logs after the rewrite:

```go
// fieldRefOrigins: newRef -> pre-rewrite refs it represents
origins[copy.to] = append(origins[copy.to], copy.from)        // copy log, in order
for each merge (removed -> survivor), chronologically:
    origins[survivor] = append(origins[survivor], origins[removed]...)
    delete(origins, removed)

// changedFieldRefs: oldRef -> final new refs (deduplicated, empty entries omitted)
redirect := resolve merge chains (removed -> final survivor)
for each copy (from -> to), in order:
    changed[from] = appendUnique(changed[from], redirect(to))
```

`RewriteResult` keeps its shape (`changedFieldRefs`, `fieldRefOrigins`);
semantics tighten: both maps now contain only refs that participate in the
rewrite. The previous root-field identity entry (an artifact of the walk-based
collector) disappears.

### Component 4: consumer simplification

`nodeSelectionVisitor.updateSkipFieldRefs` consumes `fieldRefOrigins` directly.
Field refs created by a rewrite are fresh (`Fields` is append-only), so a key of
`fieldRefOrigins` is never a pre-rewrite skipped ref. However, a fresh ref CAN
already be in `skipFieldsRefs`: the rewriter pre-registers its synthesized
skipped `__typename` (interface-object case), and normalization can dedup-merge
a preserved user-requested `__typename` into it. The unskip branch covers
exactly that case:

```go
for newRef, originRefs := range fieldRefOrigins {
    if all originRefs are in skipFieldsRefs {
        add newRef to skipFieldsRefs (if not already present)
    } else if newRef is in skipFieldsRefs {
        remove newRef from skipFieldsRefs
    }
}
```

`updateFieldDependsOn` is unchanged — it just becomes precise automatically.

### Deletions (the point of the exercise)

- `AbstractFieldPathCollector`, `FieldLimitedVisitor`, `collectFieldPaths`
  (no usages outside the rewriter, verified incl. cosmo router)
- `collectedFieldPath`, `scopeChain`, `scopeChainsIntersect`, `scopesIntersect`,
  `intersectScopes`, `resolveTypeCondition`
- the pre-rewrite path collection calls in all three `processXxxSelection`
- `collectChangedRefs` (replaced by log composition)

Net effect: the plan package no longer reasons about "which fields could occupy
the same response position" — it records which fields *do*.

## Correctness across the known cases

| Case | Provenance | Result |
|---|---|---|
| user `... on A { id }` + skipped `id` in B | copy: 0→cA, 2→cB (distinct) | cA visible, cB skipped ✓ |
| user `id` at interface level + skipped `id` in B | copies 0→cA, 0→cB1, 2→cB2; merge cB2→cB1 | origins(cB1)={0,2} → visible ✓ |
| aliased duplicate `aliased: id` vs `id` | distinct originals, copies never merge (alias differs) | tracked separately ✓ |
| planner-added `__typename` re-created by a second rewrite | preserveTypeNameSelection logs the pair | skip status transfers ✓ |
| chained rewrites | each rewrite's logs are local; origins are that rewrite's pre-refs | precise per round ✓ |
| dropped fragment (type not in datasource) | never copied → no entry | disappears, unchanged behavior ✓ |
| defer directives on merged fields | `MergeFieldsDefer` semantics untouched; hook only observes | unchanged ✓ |

## Testing

1. `abstract_selection_rewriter_changed_refs_test.go` — keep all three cases;
   drop the root-field identity entry (`4: {4}`) from expectations; add a case
   asserting `__typename` provenance through `preserveTypeNameSelection`.
2. `TestNodeSelectionVisitor_UpdateSkipFieldRefs` — drop the now-impossible
   unskip input; keep the all-origins-skipped / mixed-origins cases.
3. Branch federation tests in `graphql_datasource_federation_test.go` — must
   pass unchanged (plan-level behavior identical).
4. Full `plan`, `astnormalization`, `graphql_datasource` suites for regressions.
