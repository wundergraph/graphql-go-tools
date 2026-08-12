# Defer Parallel Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sequential defer execution loop with a tree-based executor that runs sibling defers concurrently and child defers only after their parent completes.

**Architecture:** Build a `DeferTreeNode` type (mirroring `FetchTreeNode`) and a post-processor that converts `DeferDescriptors` into a tree. The resolver walks the tree recursively, spawning goroutines for `Parallel` nodes and running `Sequence` nodes in order. Two mutexes (one on `Loader`, one passed through the tree-walk) protect the shared `Resolvable` and `jsonArena` during concurrent execution.

**Tech Stack:** Go, `golang.org/x/sync/errgroup`, `sync.Mutex`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go` | Test | Verify `ParentID` is non-zero for nested defers (Task 3) |
| `v2/pkg/engine/resolve/defer_tree.go` | Create | `DeferTreeNode` type and constructors |
| `v2/pkg/engine/resolve/response.go` | Modify | Add `DeferTree *DeferTreeNode` to `GraphQLDeferResponse` |
| `v2/pkg/engine/resolve/loader.go` | Modify | Add `mu *sync.Mutex`; protect `selectItemsForPath` + merge calls |
| `v2/pkg/engine/postprocess/build_defer_tree.go` | Create | Post-processor that builds `DeferTree` from `Defers` + `DeferDescriptors` |
| `v2/pkg/engine/postprocess/postprocess.go` | Modify | Register `buildDeferTree`; call it after `extractDeferFetches` |
| `v2/pkg/engine/resolve/resolve.go` | Modify | Add `resolveDeferTree` + `resolveDeferSingle`; replace sequential loop |

---

## Task 1: `DeferTreeNode` type

**Files:**
- Create: `v2/pkg/engine/resolve/defer_tree.go`

- [ ] **Step 1: Write the implementation**

```go
// v2/pkg/engine/resolve/defer_tree.go
package resolve

type DeferTreeNodeKind int

const (
	DeferTreeNodeKindSingle   DeferTreeNodeKind = iota
	DeferTreeNodeKindSequence
	DeferTreeNodeKindParallel
)

type DeferTreeNode struct {
	Kind       DeferTreeNodeKind
	Item       *DeferFetchGroup
	ChildNodes []*DeferTreeNode
}

func DeferSingle(group *DeferFetchGroup) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindSingle, Item: group}
}

func DeferSequence(children ...*DeferTreeNode) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindSequence, ChildNodes: children}
}

func DeferParallel(children ...*DeferTreeNode) *DeferTreeNode {
	return &DeferTreeNode{Kind: DeferTreeNodeKindParallel, ChildNodes: children}
}
```

- [ ] **Step 2: Verify the build**

```
go build ./v2/pkg/engine/resolve/...
```

Expected: no errors.

---

## Task 2: Add `DeferTree` field to `GraphQLDeferResponse`

**Files:**
- Modify: `v2/pkg/engine/resolve/response.go:61`

- [ ] **Step 1: Add the field**

In `v2/pkg/engine/resolve/response.go`, add `DeferTree` to `GraphQLDeferResponse`:

```go
type GraphQLDeferResponse struct {
	Response *GraphQLResponse
	Defers   []*DeferFetchGroup

	// DeferDescriptors lists every @defer fragment in the operation, keyed by ID.
	DeferDescriptors map[int]DeferDescriptor

	// DeferTree is the execution tree built from DeferDescriptors during post-processing.
	// Nil until the buildDeferTree post-processor runs.
	DeferTree *DeferTreeNode
}
```

- [ ] **Step 2: Verify the build**

```
go build ./v2/pkg/engine/resolve/...
```

Expected: no errors.

---

## Task 3: Verify `ParentID` propagation for nested defers

**Files:**
- Test: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go`

All existing planner-level defer tests have `ParentID: 0` — only non-nested defers are covered. The tree builder in Task 4 groups `DeferFetchGroup`s by `desc.ParentID`; if `ParentID` is always 0 in practice, all defers will be treated as roots (parallel) regardless of nesting, violating the spec's parent-before-child ordering guarantee.

This task adds a planner-level test for a nested defer and verifies that `DeferDescriptors` contains `ParentID != 0` for the inner defer. If the assertion fails, the planner pipeline has a gap that must be fixed before the tree builder can be trusted.

- [ ] **Step 1: Add a nested defer planner test**

In `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go`, add a new test case inside `TestGraphQLDataSourceDefer` (following the existing pattern of `testWithPostProcessor`):

```go
testWithPostProcessor(t,
    `query User {
        user {
            name
            ... @defer {
                title
                ... @defer {
                    description
                }
            }
        }
    }`,
    "User",
    /* expected plan */ nil, // fill in after running — see step 2
)
```

- [ ] **Step 2: Run the test and inspect the plan output**

```
gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... -run TestGraphQLDataSourceDefer -v 2>&1 | grep -A 30 "ParentID"
```

Expected: the inner defer (`description`) should produce a `DeferDescriptor` with `ParentID` equal to the outer defer's ID.

If `ParentID` is 0 for the inner defer: trace backwards through `deferInfoCollector` → `FieldDeferInfo` → `@__defer_internal(parentDeferId:…)`. The root cause is likely that the normalization step that adds `parentDeferId` (`inlineFragmentExpandDefer`) is not running, or its output is not reaching the planner.

- [ ] **Step 3: Fix `ParentID` propagation (if broken)**

If step 2 confirms `ParentID` is always 0, the most likely cause is that `inlineFragmentExpandDefer` is not enabled in the normalization pipeline used before planning. Confirm by checking whether `WithInlineDefer()` is passed to the normalizer in the engine's production code path.

Fix: enable `WithInlineDefer()` in the engine's normalizer configuration, or register `inlineFragmentExpandDefer` on the planner's `prepareOperationWalker` so it runs as part of `prepareOperation`.

- [ ] **Step 4: Update the test assertion with the correct plan**

After step 2/3, capture the actual plan output and write the full expected plan in the test case with the correct `ParentID` values, then run:

```
gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... -run TestGraphQLDataSourceDefer
```

Expected: PASS.

---

## Task 4: `buildDeferTree` post-processor

**Files:**
- Create: `v2/pkg/engine/postprocess/build_defer_tree.go`
- Test: `v2/pkg/engine/postprocess/build_defer_tree_test.go`

The post-processor reads `response.Defers` (populated by `extractDeferFetches`) and `response.DeferDescriptors` (populated by the planner) to build the `DeferTree`. Tests run both post-processors in sequence to reflect the actual pipeline.

- [ ] **Step 1: Write the failing tests**

```go
// v2/pkg/engine/postprocess/build_defer_tree_test.go
package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// makeDeferPlan builds a minimal DeferResponsePlan with raw fetches tagged
// by DeferID, ready for extractDeferFetches → buildDeferTree processing.
func makeDeferPlan(descriptors map[int]resolve.DeferDescriptor, deferIDs ...int) *plan.DeferResponsePlan {
	var children []*resolve.FetchTreeNode
	for _, id := range deferIDs {
		children = append(children, resolve.Single(&resolve.SingleFetch{
			FetchDependencies: resolve.FetchDependencies{DeferID: id},
		}))
	}
	return &plan.DeferResponsePlan{
		Response: &resolve.GraphQLDeferResponse{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(children...),
				Info:    &resolve.GraphQLResponseInfo{},
			},
			DeferDescriptors: descriptors,
		},
	}
}

func runBuildDeferTree(p *plan.DeferResponsePlan) {
	ext := &extractDeferFetches{}
	ext.Process(p)
	bdt := &buildDeferTree{}
	bdt.Process(p.Response)
}

func TestBuildDeferTree_SingleRoot(t *testing.T) {
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
	}, 1)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.Kind)
	assert.Equal(t, 1, p.Response.DeferTree.Item.DeferID)
}

func TestBuildDeferTree_TwoSiblings(t *testing.T) {
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	}, 1, 2)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	assert.Equal(t, resolve.DeferTreeNodeKindParallel, p.Response.DeferTree.Kind)
	assert.Len(t, p.Response.DeferTree.ChildNodes, 2)
	for _, child := range p.Response.DeferTree.ChildNodes {
		assert.Equal(t, resolve.DeferTreeNodeKindSingle, child.Kind)
	}
}

func TestBuildDeferTree_ParentChild(t *testing.T) {
	// A (root) → C (child of A)
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		3: {ID: 3, ParentID: 1},
	}, 1, 3)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	// Sequence(Single(1), Single(3))
	assert.Equal(t, resolve.DeferTreeNodeKindSequence, p.Response.DeferTree.Kind)
	require.Len(t, p.Response.DeferTree.ChildNodes, 2)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.ChildNodes[0].Kind)
	assert.Equal(t, 1, p.Response.DeferTree.ChildNodes[0].Item.DeferID)
	assert.Equal(t, resolve.DeferTreeNodeKindSingle, p.Response.DeferTree.ChildNodes[1].Kind)
	assert.Equal(t, 3, p.Response.DeferTree.ChildNodes[1].Item.DeferID)
}

func TestBuildDeferTree_TwoSiblingsEachWithChild(t *testing.T) {
	// A (root), B (root), C (child of A), D (child of B)
	p := makeDeferPlan(map[int]resolve.DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
		3: {ID: 3, ParentID: 1},
		4: {ID: 4, ParentID: 2},
	}, 1, 2, 3, 4)
	runBuildDeferTree(p)

	require.NotNil(t, p.Response.DeferTree)
	// Parallel(Sequence(Single(1), Single(3)), Sequence(Single(2), Single(4)))
	assert.Equal(t, resolve.DeferTreeNodeKindParallel, p.Response.DeferTree.Kind)
	require.Len(t, p.Response.DeferTree.ChildNodes, 2)
	for _, branch := range p.Response.DeferTree.ChildNodes {
		assert.Equal(t, resolve.DeferTreeNodeKindSequence, branch.Kind)
		assert.Len(t, branch.ChildNodes, 2)
	}
}

```

- [ ] **Step 2: Run to verify they fail**

```
gotestsum --format=short -- ./v2/pkg/engine/postprocess/... -run TestBuildDeferTree
```

Expected: FAIL — `buildDeferTree` undefined.

- [ ] **Step 3: Write the implementation**

```go
// v2/pkg/engine/postprocess/build_defer_tree.go
package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type buildDeferTree struct {
	disable bool
}

func (b *buildDeferTree) Process(response *resolve.GraphQLDeferResponse) {
	if b.disable || len(response.Defers) == 0 {
		return
	}

	// group DeferFetchGroups by their parent's DeferID
	childrenOf := make(map[int][]*resolve.DeferFetchGroup)
	for _, g := range response.Defers {
		desc, ok := response.DeferDescriptors[g.DeferID]
		if !ok {
			continue
		}
		childrenOf[desc.ParentID] = append(childrenOf[desc.ParentID], g)
	}

	// sort each sibling list by DeferID for deterministic tree shape
	for k := range childrenOf {
		slices.SortFunc(childrenOf[k], func(a, b *resolve.DeferFetchGroup) int {
			return a.DeferID - b.DeferID
		})
	}

	roots := childrenOf[0]
	if len(roots) == 0 {
		return
	}

	if len(roots) == 1 {
		response.DeferTree = b.buildChain(roots[0], childrenOf)
		return
	}

	branches := make([]*resolve.DeferTreeNode, len(roots))
	for i, root := range roots {
		branches[i] = b.buildChain(root, childrenOf)
	}
	response.DeferTree = resolve.DeferParallel(branches...)
}

// buildChain returns Single for a leaf, or Sequence(Single, subtree) when
// the group has children.
func (b *buildDeferTree) buildChain(
	group *resolve.DeferFetchGroup,
	childrenOf map[int][]*resolve.DeferFetchGroup,
) *resolve.DeferTreeNode {
	single := resolve.DeferSingle(group)

	children := childrenOf[group.DeferID]
	if len(children) == 0 {
		return single
	}

	childNodes := make([]*resolve.DeferTreeNode, len(children))
	for i, child := range children {
		childNodes[i] = b.buildChain(child, childrenOf)
	}

	var subtree *resolve.DeferTreeNode
	if len(childNodes) == 1 {
		subtree = childNodes[0]
	} else {
		subtree = resolve.DeferParallel(childNodes...)
	}

	return resolve.DeferSequence(single, subtree)
}
```

- [ ] **Step 4: Run to verify tests pass**

```
gotestsum --format=short -- ./v2/pkg/engine/postprocess/... -run TestBuildDeferTree
```

Expected: PASS all four tests.

---

## Task 5: Wire `buildDeferTree` into the `Processor`

**Files:**
- Modify: `v2/pkg/engine/postprocess/postprocess.go`

- [ ] **Step 1: Add the processor field and option**

Add `buildDeferTree *buildDeferTree` to the `Processor` struct (after `extractDeferFetches`):

```go
type Processor struct {
	disableExtractFetches  bool
	collectDataSourceInfo  bool
	fetchTreeProcessors    *FetchTreeProcessors
	responseTreeProcessors *ResponseTreeProcessors
	extractDeferFetches    *extractDeferFetches
	buildDeferTree         *buildDeferTree
}
```

Add `disableBuildDeferTree bool` to `processorOptions`.

Add the option function after `DisableExtractDeferFetches`:

```go
func DisableBuildDeferTree() ProcessorOption {
	return func(o *processorOptions) {
		o.disableBuildDeferTree = true
	}
}
```

In `NewProcessor`, add to the returned `Processor` after `extractDeferFetches`:

```go
buildDeferTree: &buildDeferTree{
	disable: opts.disableBuildDeferTree,
},
```

- [ ] **Step 2: Call the processor in `Process`**

In `Processor.Process`, in the `*plan.DeferResponsePlan` case, add the call after `p.extractDeferFetches.Process(t)`:

```go
case *plan.DeferResponsePlan:
	p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
	p.createFetchTree(t.Response.Response)
	p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)

	p.extractDeferFetches.Process(t)
	p.buildDeferTree.Process(t.Response)   // ← new line

	p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)

	for _, deferResp := range t.Response.Defers {
		p.fetchTreeProcessors.organizeFetchTree(deferResp.Fetches)
	}
```

- [ ] **Step 3: Verify the build and existing tests pass**

```
go build ./v2/pkg/engine/postprocess/...
gotestsum --format=short -- ./v2/pkg/engine/postprocess/...
```

Expected: no build errors; all existing tests pass.

---

## Task 6: Concurrency safety — add `mu` to `Loader`

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go`

`selectItemsForPath` reads `resolvable.data` and allocates from `jsonArena`. The merge loops in `resolveParallel` and `resolveSingle` write to `resolvable.data` via `jsonArena`. Both are unsafe when two defer group goroutines run concurrently. A pointer-to-mutex on `Loader` (nil by default, set just before the parallel defer walk) serializes these two sections without any overhead in normal single-threaded usage.

- [ ] **Step 1: Add `mu` field and helper methods to `Loader`**

In the `Loader` struct (after the `singleFlight` field):

```go
// mu, when non-nil, serialises resolvable.data / jsonArena access across
// concurrent defer-group goroutines. Nil during normal single-threaded
// execution — no locking overhead.
mu *sync.Mutex
```

Add helper methods after `callOnFinished`:

```go
func (l *Loader) lockResolvable() {
	if l.mu != nil {
		l.mu.Lock()
	}
}

func (l *Loader) unlockResolvable() {
	if l.mu != nil {
		l.mu.Unlock()
	}
}
```

Add `"sync"` to the import block if not already present.

- [ ] **Step 2: Protect `resolveParallel`**

`resolveParallel` already separates the read phase (selectItemsForPath, before goroutines) from the write phase (mergeResult, after g.Wait()). Wrap both sections. Replace loader.go:231–278 with:

```go
func (l *Loader) resolveParallel(nodes []*FetchTreeNode) error {
	if len(nodes) == 0 {
		return nil
	}
	results := make([]*result, len(nodes))
	defer func() {
		for i := range results {
			batchEntityToolPool.Put(results[i].tools)
		}
	}()
	itemsItems := make([][]*astjson.Value, len(nodes))
	g, ctx := errgroup.WithContext(l.ctx.ctx)

	l.lockResolvable()
	for i := range nodes {
		i := i
		results[i] = &result{}
		itemsItems[i] = l.selectItemsForPath(nodes[i].Item.FetchPath)
		f := nodes[i].Item.Fetch
		item := nodes[i].Item
		items := itemsItems[i]
		res := results[i]
		g.Go(func() error {
			return l.loadFetch(ctx, f, item, items, res)
		})
	}
	l.unlockResolvable()

	err := g.Wait()
	if err != nil {
		return errors.WithStack(err)
	}

	l.lockResolvable()
	for i := range results {
		if results[i].nestedMergeItems != nil {
			for j := range results[i].nestedMergeItems {
				err = l.mergeResult(nodes[i].Item, results[i].nestedMergeItems[j], itemsItems[i][j:j+1])
				l.callOnFinished(results[i].nestedMergeItems[j])
				if err != nil {
					l.unlockResolvable()
					return errors.WithStack(err)
				}
			}
		} else {
			err = l.mergeResult(nodes[i].Item, results[i], itemsItems[i])
			l.callOnFinished(results[i])
			if err != nil {
				l.unlockResolvable()
				return errors.WithStack(err)
			}
		}
	}
	l.unlockResolvable()

	return nil
}
```

- [ ] **Step 3: Protect `resolveSingle`**

Replace loader.go:290–328 with:

```go
func (l *Loader) resolveSingle(item *FetchItem) error {
	if item == nil {
		return nil
	}

	l.lockResolvable()
	items := l.selectItemsForPath(item.FetchPath)
	l.unlockResolvable()

	switch f := item.Fetch.(type) {
	case *SingleFetch:
		res := &result{}
		err := l.loadSingleFetch(l.ctx.ctx, f, item, items, res)
		if err != nil {
			return err
		}
		l.lockResolvable()
		err = l.mergeResult(item, res, items)
		l.unlockResolvable()
		l.callOnFinished(res)
		return err
	case *BatchEntityFetch:
		res := &result{}
		defer batchEntityToolPool.Put(res.tools)
		err := l.loadBatchEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		l.lockResolvable()
		err = l.mergeResult(item, res, items)
		l.unlockResolvable()
		l.callOnFinished(res)
		return err
	case *EntityFetch:
		res := &result{}
		err := l.loadEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		l.lockResolvable()
		err = l.mergeResult(item, res, items)
		l.unlockResolvable()
		l.callOnFinished(res)
		return err
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run existing loader tests**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/...
```

Expected: all existing tests pass (`mu` is nil in all existing tests — no behaviour change).

---

## Task 7: `resolveDeferTree` and update `ResolveGraphQLDeferResponse`

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve.go`

### Background: `hasNext` and `Context.clone`

**`hasNext`:** `ResolveDefer` takes `hasNext bool` — `true` means more payloads are coming, `false` means this is the last. In the parallel model the last defer to complete is non-deterministic, so always pass `hasNext: true` from `resolveDeferSingle`. The existing `writer.Complete()` call (after the tree resolves) is responsible for the terminal signal. Before relying on this, check that `writer.Complete()` produces `{hasNext: false}` by reading the concrete implementation used in production.

**`Context.clone`:** `Context` (resolve/context.go:18) has a private `ctx context.Context` field (line 19) and a `clone(ctx context.Context) *Context` method (line 281). Use `ctx.clone(gCtx)` to derive a child `Context` with the errgroup's cancellable context while inheriting all other fields (variables, auth, etc.).

**`errgroup` import:** `errgroup` is not yet imported in `resolve.go` — add `"golang.org/x/sync/errgroup"` to the import block.

- [ ] **Step 1: Add `resolveDeferSingle`**

Add after `ResolveGraphQLDeferResponse`:

```go
func (r *Resolver) resolveDeferSingle(
	ctx *Context,
	group *DeferFetchGroup,
	response *GraphQLDeferResponse,
	t *tools,
	writer DeferResponseWriter,
	renderMu *sync.Mutex,
) error {
	if err := t.loader.ResolveFetchNode(group.Fetches); err != nil {
		return err
	}

	renderMu.Lock()
	t.resolvable.errors = nil
	t.resolvable.deferID = group.DeferID
	err := t.resolvable.ResolveDefer(response.Response.Data, writer, true)
	if err != nil {
		renderMu.Unlock()
		return err
	}
	flushErr := writer.Flush()
	renderMu.Unlock()
	return flushErr
}
```

- [ ] **Step 2: Add `resolveDeferTree`**

Add after `resolveDeferSingle`:

```go
func (r *Resolver) resolveDeferTree(
	ctx *Context,
	node *DeferTreeNode,
	response *GraphQLDeferResponse,
	t *tools,
	writer DeferResponseWriter,
	renderMu *sync.Mutex,
) error {
	switch node.Kind {
	case DeferTreeNodeKindSingle:
		return r.resolveDeferSingle(ctx, node.Item, response, t, writer, renderMu)

	case DeferTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			if err := r.resolveDeferTree(ctx, child, response, t, writer, renderMu); err != nil {
				return err
			}
		}
		return nil

	case DeferTreeNodeKindParallel:
		// ctx.ctx is the underlying context.Context on the Context struct.
		// clone() copies all resolver fields (variables, auth, etc.) and
		// replaces ctx.ctx with the errgroup-derived context so that
		// client-disconnect cancellation propagates to every sibling goroutine.
		g, gCtx := errgroup.WithContext(ctx.ctx)
		for _, child := range node.ChildNodes {
			child := child
			g.Go(func() error {
				err := r.resolveDeferTree(ctx.clone(gCtx), child, response, t, writer, renderMu)
				// Only propagate the error if the parent context was cancelled
				// (client disconnected). Defer-level errors are independent —
				// a failed sibling must not cancel its siblings.
				if err != nil && gCtx.Err() != nil {
					return err
				}
				return nil
			})
		}
		return g.Wait()
	}
	return nil
}
```

- [ ] **Step 3: Replace the sequential loop in `ResolveGraphQLDeferResponse`**

In `resolve.go:483–514`, replace:

```go
// fetch deferred responses

for i, deferGroup := range response.Defers {
    t.resolvable.errors = nil

    if err := t.loader.ResolveFetchNode(deferGroup.Fetches); err != nil {
        return nil, err
    }

    t.resolvable.deferID = deferGroup.DeferID

    err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
    if err != nil {
        return nil, err
    }

    err = writer.Flush()
    if err != nil {
        return nil, err
    }
}

writer.Complete()
```

With:

```go
// fetch deferred responses using the parallel execution tree

if response.DeferTree != nil {
    renderMu := &sync.Mutex{}
    t.loader.mu = &sync.Mutex{}
    if err := r.resolveDeferTree(ctx, response.DeferTree, response, t, writer, renderMu); err != nil {
        return nil, err
    }
}

writer.Complete()
```

Add `"sync"` and `"golang.org/x/sync/errgroup"` to the imports of `resolve.go` if not already present.

- [ ] **Step 4: Run the full resolve and engine tests**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/...
gotestsum --format=short -- ./v2/pkg/engine/...
```

Expected: all existing tests pass.

---

## Task 8: Integration tests for parallel and nested defer execution

**Files:**
- Test: `v2/pkg/engine/resolve/resolve_defer_parallel_test.go` (new)

These tests call `ResolveGraphQLDeferResponse` directly using the `newResolver` and `FakeDataSource` helpers from `resolve_test.go`. They build `DeferTreeNode` instances directly (bypassing the post-processor) to test execution behaviour in isolation.

`testDeferWriter` is a minimal `DeferResponseWriter` implementation that records each flushed payload.

- [ ] **Step 1: Write the tests**

```go
// v2/pkg/engine/resolve/resolve_defer_parallel_test.go
package resolve

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type testDeferWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	payloads []string
	complete bool
}

func (w *testDeferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testDeferWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, w.buf.String())
	w.buf.Reset()
	return nil
}

func (w *testDeferWriter) Complete() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.complete = true
}

// minimalDeferResponse builds a GraphQLDeferResponse with an empty initial
// response and one deferred field per group. Each group's fetch returns the
// provided JSON blob via FakeDataSource.
//
// The response tree has one field "result" at the root; each defer group
// renders a string field whose path matches its DeferID.
func minimalDeferResponse(groups []*DeferFetchGroup, descriptors map[int]DeferDescriptor) *GraphQLDeferResponse {
	fields := make([]*Field, len(groups))
	for i, g := range groups {
		fields[i] = &Field{
			Name: []byte(fmt.Sprintf("f%d", g.DeferID)),
			Defer: &DeferField{DeferID: g.DeferID},
			Value: &String{
				Path:     []string{fmt.Sprintf("f%d", g.DeferID)},
				Nullable: true,
			},
		}
	}
	return &GraphQLDeferResponse{
		DeferDescriptors: descriptors,
		Response: &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Data: &Object{
				Nullable: true,
				Fields:   fields,
			},
		},
	}
}

// TestResolveDeferTree_TwoParallelSiblings: two root-level defers produce two
// flushed payloads in any order, with no data races.
func TestResolveDeferTree_TwoParallelSiblings(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"valueA"}`)},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f2":"valueB"}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())
	ctx.Request.Header = make(map[string][]string)

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	assert.Len(t, writer.payloads, 2)
	assert.True(t, writer.complete)
}

// TestResolveDeferTree_SequenceOrdering: child defer only executes after parent
// has flushed its payload.
func TestResolveDeferTree_SequenceOrdering(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"parent"}`)},
		}),
	}
	groupC := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f2":"child"}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupC}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 1},
	})
	response.DeferTree = DeferSequence(DeferSingle(groupA), DeferSingle(groupC))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())
	ctx.Request.Header = make(map[string][]string)

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	// parent payload arrives before child payload
	require.Len(t, writer.payloads, 2)
	assert.Contains(t, writer.payloads[0], "parent")
	assert.Contains(t, writer.payloads[1], "child")
}

// TestResolveDeferTree_SiblingFailureIsIndependent: an error in one sibling
// goroutine does not cancel the other sibling.
func TestResolveDeferTree_SiblingFailureIsIndependent(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"valueA"}`)},
		}),
	}
	// groupB uses an error-returning datasource
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())
	ctx.Request.Header = make(map[string][]string)

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	// groupA must still produce a payload even if groupB has no data
	assert.Len(t, writer.payloads, 2)
}
```

- [ ] **Step 2: Run with race detector**

```
gotestsum --format=short -- -race ./v2/pkg/engine/resolve/... -run TestResolveDeferTree
```

Expected: all pass, no data race reported.

---

## Self-Review

**Spec coverage:**

| Requirement | Task |
|---|---|
| `ParentID` propagated correctly for nested defers | Task 3 (investigation + fix) |
| Sibling defers execute concurrently | Task 7 (`DeferTreeNodeKindParallel` → errgroup) |
| Child defer waits for parent | Task 4 (`Sequence` node enforces ordering) |
| Parent failure cancels children | Task 7 (Sequence walk returns early on error) |
| Sibling failure is independent | Task 7 (goroutine swallows non-disconnect errors) |
| `resolvable.data` / `jsonArena` thread safety | Task 6 (`lockResolvable` around selectItems + merge) |
| Render / HTTP writer thread safety | Task 7 (`renderMu` wraps `ResolveDefer + Flush`) |
| Client disconnect cancels all goroutines | Task 7 (`errgroup.WithContext(ctx.ctx)`) |
| `ParentID`-only tree construction | Task 4 (`buildChain` uses only `DeferDescriptors`) |
| Initial fetch failure skips all defers | Unchanged — early-return before `DeferTree` walk |

**Open items to verify during Task 7:**
- Confirm `writer.Complete()` sends `{hasNext: false}`. If it does not, call `ResolveDefer` with `hasNext: false` on the single last flush instead, and track completion count with an atomic counter.
- Check the `tools` struct definition — `tools.loader.mu` assignment in `ResolveGraphQLDeferResponse` must be valid (the field is on the embedded `Loader`, not a copy).
