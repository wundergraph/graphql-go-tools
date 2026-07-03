# Parallel & Nested Defer Execution — Design Analysis

**Date:** 2026-05-15  
**Branch:** feat/eng-7770-add-defer-support-additional  
**Status:** Draft — pending review

---

## Problem

Current defer execution is fully sequential, driven by defer ID order. Each
`DeferFetchGroup` is processed one at a time inside `ResolveGraphQLDeferResponse()`
regardless of whether defers are independent siblings or truly nested.

Two execution patterns need to be supported correctly:

- **Parallel defers** — sibling defers (same parent) are independent and can execute
  concurrently.
- **Nested defers** — a child defer must not start until its parent defer has
  completed and flushed its payload.

This mirrors the existing fetch-tree problem, where fetches also form a mix of
sequential (dependency-ordered) and parallel (independent) steps.

---

## Spec Behaviour (from graphql-js reference)

Verified against `/graphql-js/src/execution/incremental/__tests__/defer-test.ts`:

| Scenario | Required behaviour |
|---|---|
| Sibling defers | Execute concurrently, flush each as it completes (no ordering guarantee) |
| Child defer | Must not start until parent defer is released (even if data-independent) |
| Parent fails (non-nullable error) | Child defers within that scope are cancelled |
| Sibling fails | Other siblings continue independently |
| Completely independent branches | Fully isolated; errors do not cross branch boundaries |

The "release" model is a spec requirement, not just a data-dependency concern.
A nested defer that requests no parent fields still waits for the parent's
payload to be sent before it can begin.

---

## Current Architecture

### Key types

```
resolve.DeferDescriptor          // ParentID, ID, Label, Path — built at plan time
resolve.DeferFetchGroup          // DeferID + FetchTreeNode of fetches for that defer
resolve.GraphQLDeferResponse     // Response + []DeferFetchGroup + map[int]DeferDescriptor
```

### Current execution flow

```
ResolveGraphQLDeferResponse()
  → load initial response fetches
      if error → return immediately, nothing written, no defers execute
  → render initial response, flush
      if hasErrors() → return, initial payload already sent, no defers execute
  → for i, deferGroup := range response.Defers   // ← fully sequential
      load fetches (Loader.ResolveFetchNode)       //   parallel WITHIN one defer group
      render defer payload
      flush
```

Both early-exit conditions gate the defer tree — if the initial response fails
or contains non-nullable errors, the defer tree is never entered. This
behaviour is unchanged by the new design.

`Loader.ResolveFetchNode` already uses `errgroup.WithContext` to run parallel
fetch branches concurrently within a single defer group. The parallelism gap is
at the _defer group_ level, not inside it.

### Existing infrastructure that can be reused

| Component | Location | Reuse opportunity |
|---|---|---|
| `FetchTreeNode` (Sequential/Parallel/Single) | `resolve/fetchtree.go` | Shape to copy for a `DeferTreeNode` |
| `resolveParallel()` with errgroup | `resolve/loader.go:231` | Same pattern for concurrent defer branches |
| `createParallelNodes` post-processor | `postprocess/create_parallel_nodes.go` | Pattern for building parallel groups |
| `orderSequenceByDependencies` post-processor | `postprocess/order_sequence_by_dependencies.go` | Pattern for dependency-ordered sequencing |
| `DeferDescriptor.ParentID` | `resolve/response.go:75` | Already encodes the parent→child relationship |

---

## Approach — Defer Execution Tree

Introduce a `DeferTreeNode` type that mirrors `FetchTreeNode`:

```go
type DeferTreeNodeKind int

const (
    DeferTreeNodeKindSingle   DeferTreeNodeKind = iota // one DeferFetchGroup
    DeferTreeNodeKindSequence                          // parent → child chain
    DeferTreeNodeKindParallel                          // sibling group
)

type DeferTreeNode struct {
    Kind        DeferTreeNodeKind
    Item        *DeferFetchGroup   // non-nil for Single
    ChildNodes  []*DeferTreeNode
}
```

A new post-processor (`build_defer_tree.go`) reads `DeferDescriptor.ParentID`
and converts the flat `[]DeferFetchGroup` into this tree. Roots (ParentID == 0
or no parent in map) form a `Parallel` node. Each root's children form a
`Sequence` node under it (root executes, then child parallel group executes,
etc.).

The resolver replaces the sequential loop with a recursive tree-walk:

```
resolveDeferTree(node)
  Parallel → spawn goroutine per child, errgroup.Wait()
              flush each payload as its goroutine returns
  Sequence → for each child: resolveDeferTree(child)
              if child fails and has children: cancel them, return
  Single   → load fetches, render, flush
```

**Execution shape example**

```
Query
  @defer A          @defer B (sibling of A)
    @defer C            @defer D (sibling of B's child)
      (child of A)

DeferTreeNode(Parallel)
  DeferTreeNode(Sequence)        DeferTreeNode(Sequence)
    Single(A)                      Single(B)
    Single(C)                      Single(D)
```

A and B execute concurrently. C starts only after A finishes. D starts only
after B finishes. If A fails, C is cancelled; B and D are unaffected.

**Why this fits**

- Direct structural analogy to `FetchTreeNode` — same concepts, same mental
  model, readable alongside the existing code.
- Children are naturally blocked until their parent `Sequence` step returns.
- Cancellation is implicit: if a `Sequence` step errors, the remaining steps
  in that sequence are skipped.
- Post-processing keeps tree construction separate from execution, consistent
  with `extractDeferFetches` / `createParallelNodes` pattern.
- `resolveParallel` in the loader is ~40 lines using `errgroup`; the defer
  equivalent is the same size.

**Estimated scope**

| File | Change |
|---|---|
| `resolve/deferred_tree.go` (new) | `DeferTreeNode` type + constructors (~60 lines) |
| `postprocess/build_defer_tree.go` (new) | Tree builder from `DeferDescriptor` map (~80 lines) |
| `postprocess/postprocess.go` | Register new post-processor |
| `resolve/resolve.go` | Replace sequential loop with tree-walker (~40 lines) |
| Tests | Unit tests for tree builder + integration tests for parallel/nested flush order |

---

## Open Questions

1. **Shared `Resolvable` / data buffer and response writer** — execution for
   each defer group has three phases:
   - **Fetch** — network calls; run in parallel within a single defer group today
   - **Merge** — fetched bytes are parsed and merged into `Resolvable.data`
     (a single shared `*astjson.Value`) via `mergeResult()` in loader.go:458;
     uses the shared `jsonArena` for both parsing (`ParseBytesWithArena`) and
     in-place value merging (`MergeValuesWithPath`)
   - **Render** — reads from `Resolvable.data`, marshals via the shared
     `marshalBuf`, writes to `r.out` (the HTTP response writer)

   In the current loader the parallel goroutines only do the fetch phase; merge
   and render are sequential on the main thread, so sharing `Resolvable` and
   `jsonArena` is safe today.

   For parallel sibling defer groups both merge and render would run
   concurrently. Two fine-grained mutexes are sufficient — no restructuring of
   the loader is required:
   - **Merge mutex** — scoped around `mergeResult()`; protects `Resolvable.data`,
     `Resolvable.errors`, and `jsonArena` (all written during this call)
   - **Render mutex** — scoped around the response-write calls (`printBytes` /
     `printNode`); protects `marshalBuf` and `r.out`

   Fetch latency (the expensive part) remains fully parallel. The two critical
   sections are short and CPU-bound. Flush still happens as each sibling
   completes, satisfying the spec's "deliver when available" requirement.

2. **Flushing concurrency** — covered by the render mutex above; no separate
   mechanism needed.

3. **Context propagation** — the existing plumbing is sufficient for
   client-disconnect cancellation. `ResolveGraphQLDeferResponse` initialises the
   loader with `ctx` (line 457 in resolve.go); `resolveParallel` already derives
   its errgroup from `l.ctx.ctx` (loader.go:243), so cancellation flows through
   to all fetch goroutines automatically.

   One subtlety: `errgroup.WithContext` cancels the group context when any
   goroutine returns a non-nil error. For sibling defer branches this would
   incorrectly cancel healthy siblings when one fails — the spec requires
   siblings to be independent. The fix: each sibling goroutine must catch
   defer-level errors internally (recording them as that defer's error payload)
   and return `nil` to the errgroup. Client-disconnect errors should still
   propagate as non-nil so the whole group aborts on disconnect.

4. **Incremental delivery ordering within a parallel group** — ~~resolved~~.
   Clients must not assume a fixed delivery order for sibling defers; the only
   ordering guarantee is parent before child, which the `Sequence` node enforces
   structurally. Parallel siblings may be flushed in any order.

5. **Error-scope boundary for independent nested defers** — ~~resolved~~.
   The tree builder must derive parent-child relationships solely from
   `DeferDescriptor.ParentID`, which is assigned at plan time based on syntactic
   `@defer` nesting — never from data-path analysis. A child defer that requests
   no fields from its parent still carries a `ParentID` pointing to that parent
   and must be placed in its `Sequence` node. Optimising it into a sibling
   `Parallel` node would violate the spec's release model and is explicitly
   forbidden.
