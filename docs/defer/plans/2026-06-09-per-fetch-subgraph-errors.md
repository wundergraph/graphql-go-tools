# Per-fetch Subgraph Errors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **User preference:** No `git add`/`git commit` steps appear in this plan — the user manages commits. Run Go tests with `gotestsum --format=short -- <pkg> -run <Test>`.

**Goal:** Make `LoaderHooks.OnFinished` report each fetch's *own* subgraph error instead of the accumulated, subgraph-name-keyed blob, and isolate subgraph-error accumulation onto the `Loader`, flushing into `Context` once after fetch resolution.

**Architecture:** Add a per-`result` `subgraphError` field (the fetch's own error, accumulated with `errors.Join`) and a loader-owned `subgraphErrors map[string]error`. A single helper `recordSubgraphError` replaces the five direct `l.ctx.appendSubgraphErrors(...)` calls, writing both the result field (for `OnFinished`) and the loader map (for aggregation). `OnFinished` reads the result field; the loader map is flushed into `Context.subgraphErrors` once after the fetch tree resolves — mirroring how `l.errors` is already harvested in `resolve.go`.

**Tech Stack:** Go, `github.com/wundergraph/astjson`, `golang.org/x/sync/errgroup`, package `github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve`.

---

## Background (why this is safe)

- **All loader-side subgraph-error writes funnel through `mergeResult`.** The five
  `l.ctx.appendSubgraphErrors` sites are reached only from `mergeResult` (directly,
  or via `mergeErrors` → `appendSubgraphError`). `mergeResult` is called only from
  `resolveParallel` (loader.go:289,297) and `resolveSingle` (335,347,358).
- **Within one Loader these writes are serial.** In `resolveParallel`, the network
  `loadFetch` runs concurrently, but every `mergeResult` runs in the post-`g.Wait()`
  loop under `dataBuffer.Lock()` (single goroutine). `resolveSingle` is serial.
  Therefore `l.subgraphErrors` and `res.subgraphError` need **no extra lock**.
- **The sites are NOT mutually exclusive** — a single fetch with `errors` *and* a
  malformed/mismatched data shape appends via `mergeErrors` (loader.go:573) and then
  again via `renderErrorsFailedToFetch` (610/632/637/660); `renderAuthorizationRejectedErrors`
  appends once per reason. So `res.subgraphError` and the loader map MUST accumulate
  with `errors.Join`, never overwrite.
- **Timing is safe.** The only mid-resolution reader of `ctx.subgraphErrors` is
  `OnFinished` (redirected to `result` here). `Context.SubgraphErrors()` has no
  in-package caller; it is read by the router/engine after resolution returns. So
  deferring the `ctx` write to an end-of-tree flush changes nothing observable.
- **`Resolvable` is a second, separate producer.** `Resolvable.addRejectFieldError`
  (resolvable.go:1269) writes `ctx.subgraphErrors` directly at render time and is
  field-scoped, not fetch-scoped. It is intentionally untouched; the final
  `ctx.subgraphErrors` remains `loader-flush ∪ resolvable-render-time-appends`.

## File structure

- Modify `v2/pkg/engine/resolve/loader.go` — `result` + `Loader` fields, `Init`,
  `Free`, `newResponseInfo`, `callOnFinished`, `recordSubgraphError`,
  `flushSubgraphErrors`, the 5 append sites, `LoadGraphQLResponseData`.
- Modify `v2/pkg/engine/resolve/resolve.go` — flush at the initial-defer harvest
  point and inside `resolveDeferSingle` (under the lock); optional clone removal.
- Test `v2/pkg/engine/resolve/per_fetch_subgraph_errors_test.go` — new unit tests.
- Existing regression: `v2/pkg/engine/resolve/loader_hooks_test.go` must still pass.

---

## Task 1: Add per-result and per-loader error storage

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (`result` struct ~98, `Loader` struct ~152, `Init` ~224, `Free` ~212)

- [ ] **Step 1: Add `subgraphError` to the `result` struct**

In `loader.go`, inside `type result struct { ... }`, just after the `err error`
field (currently around line 117), add:

```go
	statusCode int
	err        error
	// subgraphError is THIS fetch's own subgraph error (errors.Join of res.err and
	// the rendered SubgraphError/RateLimitError). Accumulated across the merge path
	// because a single fetch may record more than once. Read by newResponseInfo so
	// OnFinished reports only this fetch's error, not the request-wide aggregate.
	subgraphError error
	ds         DataSourceInfo
```

- [ ] **Step 2: Add `subgraphErrors` to the `Loader` struct**

In `type Loader struct { ... }`, just after the `errors *astjson.Value` field block
(around line 159), add:

```go
	// subgraphErrors accumulates this Loader's subgraph errors, keyed by subgraph
	// name, mirroring Context.subgraphErrors. Written only from mergeResult (serial
	// within a Loader) and flushed into l.ctx.subgraphErrors once after the fetch
	// tree resolves (see flushSubgraphErrors). Keeps concurrent fetch execution off
	// the shared Context map.
	subgraphErrors map[string]error
```

- [ ] **Step 3: Document the no-concurrent-writes rule on the `ctx` field**

In `type Loader struct { ... }`, the `ctx` field is currently uncommented:

```go
	ctx  *Context
	info *GraphQLResponseInfo
```

Add:

```go
	// ctx is the shared request Context. It is safe to read, but must not be
	// written to: fetches run concurrently and the Context is not synchronized.
	ctx  *Context
	info *GraphQLResponseInfo
```

- [ ] **Step 4: Reset it in `Init`**

In `func (l *Loader) Init(...)`, add the reset alongside the existing ones:

```go
func (l *Loader) Init(ctx *Context, responseInfo *GraphQLResponseInfo) {
	l.errors = nil
	l.skipValueCompletion = false
	l.subgraphErrors = nil
	l.ctx = ctx
	l.info = responseInfo
	l.taintedObjs = make(taintedObjects)
}
```

- [ ] **Step 5: Clear it in `Free`**

In `func (l *Loader) Free()`, add:

```go
func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.taintedObjs = nil
	l.subgraphErrors = nil
}
```

- [ ] **Step 6: Verify the package still builds**

Run: `cd v2 && go build ./pkg/engine/resolve/...`
Expected: builds with no errors (fields are unused so far; Go allows unused struct fields).

---

## Task 2: Add `recordSubgraphError` and `flushSubgraphErrors`, with unit tests

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (add two methods near `callOnFinished` ~367)
- Test: `v2/pkg/engine/resolve/per_fetch_subgraph_errors_test.go` (create)

- [ ] **Step 1: Write the failing tests**

Create `v2/pkg/engine/resolve/per_fetch_subgraph_errors_test.go`:

```go
package resolve

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLoader() *Loader {
	return &Loader{ctx: NewContext(context.Background())}
}

func TestRecordSubgraphError_PerFetchIsolation(t *testing.T) {
	l := newTestLoader()
	resA := &result{ds: DataSourceInfo{Name: "Users"}}
	resB := &result{ds: DataSourceInfo{Name: "Users"}} // same subgraph, no error

	l.recordSubgraphError(resA, errors.New("boom"))

	// Per-fetch isolation: only resA carries the error.
	require.Error(t, resA.subgraphError)
	assert.NoError(t, resB.subgraphError)

	// newResponseInfo reads the per-result field.
	assert.Error(t, newResponseInfo(resA).Err)
	assert.NoError(t, newResponseInfo(resB).Err)
}

func TestRecordSubgraphError_Accumulates(t *testing.T) {
	l := newTestLoader()
	res := &result{ds: DataSourceInfo{Name: "Products"}}

	l.recordSubgraphError(res, errors.New("first"))
	l.recordSubgraphError(res, errors.New("second"))

	require.Error(t, res.subgraphError)
	msg := res.subgraphError.Error()
	assert.Contains(t, msg, "first")
	assert.Contains(t, msg, "second")
}

func TestFlushSubgraphErrors_AggregatesIntoContext(t *testing.T) {
	l := newTestLoader()
	resU := &result{ds: DataSourceInfo{Name: "Users"}}
	resP := &result{ds: DataSourceInfo{Name: "Products"}}

	l.recordSubgraphError(resU, errors.New("users-down"))
	l.recordSubgraphError(resP, errors.New("products-down"))

	// Before flush the Context is untouched.
	assert.NoError(t, l.ctx.SubgraphErrors())

	l.flushSubgraphErrors()

	joined := l.ctx.SubgraphErrors()
	require.Error(t, joined)
	assert.Contains(t, joined.Error(), "users-down")
	assert.Contains(t, joined.Error(), "products-down")
}

func TestRecordSubgraphError_NilIsNoOp(t *testing.T) {
	l := newTestLoader()
	res := &result{ds: DataSourceInfo{Name: "Users"}}

	l.recordSubgraphError(res, nil)

	assert.NoError(t, res.subgraphError)
	l.flushSubgraphErrors()
	assert.NoError(t, l.ctx.SubgraphErrors())
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run 'TestRecordSubgraphError|TestFlushSubgraphErrors'`
Expected: FAIL — does not compile: `l.recordSubgraphError undefined`, `l.flushSubgraphErrors undefined`, and `newResponseInfo` arity mismatch (still takes two args; fixed in Task 3).

> Note: until Task 3 changes `newResponseInfo`'s signature, the package will not
> compile. That is expected — these tests stay red across Task 2 and go green after
> Task 3. If you prefer a green checkpoint at the end of Task 2, temporarily call
> `newResponseInfo(res)` only after completing Task 3 Step 1.

- [ ] **Step 3: Implement the two methods**

In `loader.go`, immediately after `func (l *Loader) callOnFinished(...)` (ends ~371),
add:

```go
// recordSubgraphError is the Loader-local analog of Context.appendSubgraphErrors: it keeps
// the error on res (for OnFinished) and in l.subgraphErrors, which flushSubgraphErrors later
// merges into the Context.
func (l *Loader) recordSubgraphError(res *result, errs ...error) {
	res.subgraphError = errors.Join(res.subgraphError, errs...)
	if l.subgraphErrors == nil {
		l.subgraphErrors = make(map[string]error)
	}
	l.subgraphErrors[res.ds.Name] = errors.Join(l.subgraphErrors[res.ds.Name], errs...)
}

// flushSubgraphErrors merges this Loader's accumulated subgraph errors into the
// shared Context, preserving Context.appendSubgraphErrors' per-name errors.Join
// semantics. Call once after the fetch tree has resolved, on a single goroutine
// (or under the DataBuffer lock for concurrent defer groups).
func (l *Loader) flushSubgraphErrors() {
	for name, err := range l.subgraphErrors {
		l.ctx.appendSubgraphErrors(DataSourceInfo{Name: name}, err)
	}
}
```

(`errors` is already imported in `loader.go`.)

- [ ] **Step 4: (Implemented together with Task 3) Run tests after Task 3**

These tests compile and pass only once Task 3 fixes `newResponseInfo`. Proceed to
Task 3, then run:
`gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run 'TestRecordSubgraphError|TestFlushSubgraphErrors'`
Expected: PASS (4 tests).

---

## Task 3: Make `OnFinished` read the per-result error

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (`newResponseInfo` ~70, `callOnFinished` ~369)

- [ ] **Step 1: Change `newResponseInfo` to read `res.subgraphError`**

Replace the signature and the `Err` line. Current (loader.go:70-75):

```go
func newResponseInfo(res *result, subgraphErrors map[string]error) *ResponseInfo {
	responseInfo := &ResponseInfo{
		StatusCode:   res.statusCode,
		Err:          subgraphErrors[res.ds.Name],
		responseBody: res.out,
	}
```

New:

```go
func newResponseInfo(res *result) *ResponseInfo {
	responseInfo := &ResponseInfo{
		StatusCode:   res.statusCode,
		Err:          res.subgraphError,
		responseBody: res.out,
	}
```

(Leave the rest of the function — the `httpResponseContext` block — unchanged.)

- [ ] **Step 2: Update the `callOnFinished` call site**

Current (loader.go:369):

```go
		l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
```

New:

```go
		l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res))
```

- [ ] **Step 3: Build**

Run: `cd v2 && go build ./pkg/engine/resolve/...`
Expected: builds (no remaining references to the old `newResponseInfo` arity).

- [ ] **Step 4: Run the Task 2 unit tests — now green**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run 'TestRecordSubgraphError|TestFlushSubgraphErrors'`
Expected: PASS (4 tests).

---

## Task 4: Route the five append sites through `recordSubgraphError`

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (lines 722, 1101, 1121, 1153, 1198)

- [ ] **Step 1: `appendSubgraphError` (line 722)**

Current:

```go
	l.ctx.appendSubgraphErrors(res.ds, res.err, subgraphError)
```

New:

```go
	l.recordSubgraphError(res, res.err, subgraphError)
```

- [ ] **Step 2: `renderErrorsFailedToFetch` (line 1101)**

Current:

```go
	l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
```

New:

```go
	l.recordSubgraphError(res, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
```

- [ ] **Step 3: `renderErrorsStatusFallback` (line 1121)**

Current:

```go
	l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
```

New:

```go
	l.recordSubgraphError(res, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
```

- [ ] **Step 4: `renderAuthorizationRejectedErrors` (line 1153, inside the loop)**

Current:

```go
	for i := range res.authorizationRejectedReasons {
		l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, res.authorizationRejectedReasons[i], res.statusCode))
	}
```

New:

```go
	for i := range res.authorizationRejectedReasons {
		l.recordSubgraphError(res, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, res.authorizationRejectedReasons[i], res.statusCode))
	}
```

- [ ] **Step 5: `renderRateLimitRejectedErrors` (line 1198)**

Current:

```go
	l.ctx.appendSubgraphErrors(res.ds, res.err, NewRateLimitError(res.ds.Name, fetchItem.ResponsePath, res.rateLimitRejectedReason))
```

New:

```go
	l.recordSubgraphError(res, res.err, NewRateLimitError(res.ds.Name, fetchItem.ResponsePath, res.rateLimitRejectedReason))
```

- [ ] **Step 6: Verify no direct `l.ctx.appendSubgraphErrors` calls remain in loader.go**

Run: `grep -n "l.ctx.appendSubgraphErrors" v2/pkg/engine/resolve/loader.go`
Expected: no output (all five replaced).

- [ ] **Step 7: Build**

Run: `cd v2 && go build ./pkg/engine/resolve/...`
Expected: builds.

---

## Task 5: Flush the loader's errors into the Context after resolution

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (`LoadGraphQLResponseData` ~218)
- Modify: `v2/pkg/engine/resolve/resolve.go` (initial-defer loader ~476; `resolveDeferSingle` ~587)

- [ ] **Step 1: Flush in `LoadGraphQLResponseData` (covers all non-defer paths)**

`LoadGraphQLResponseData` is the entry for resolve.go:344, 406, 820. Current:

```go
func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse) (err error) {
	l.Init(ctx, response.Info)

	return l.ResolveFetchNode(response.Fetches)
}
```

New (use `defer` so prior errors are flushed even when a later fetch hard-fails,
matching the old write-during-merge behaviour):

```go
func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse) (err error) {
	l.Init(ctx, response.Info)
	defer l.flushSubgraphErrors()

	return l.ResolveFetchNode(response.Fetches)
}
```

- [ ] **Step 2: Flush the initial-defer loader via `defer` (resolve.go ~476)**

In the incremental/defer path the initial fetch drives the loader directly (not
`LoadGraphQLResponseData`). Add a `defer` right after `loader.Init(...)` so the flush
runs once at function exit — covering both the success path **and** the early
`return nil, err` on a hard fetch error — and single-threaded (it runs after the defer
groups have completed). Current (~476-480):

```go
		loader.Init(ctx, response.Response.Info)

		// fetch initial response
		if err := loader.ResolveFetchNode(response.Response.Fetches); err != nil {
			return nil, err
		}
```

New:

```go
		loader.Init(ctx, response.Response.Info)
		defer loader.flushSubgraphErrors()

		// fetch initial response
		if err := loader.ResolveFetchNode(response.Response.Fetches); err != nil {
			return nil, err
		}
```

(Leave the existing `resolvable.data = …` / `resolvable.errors = …` harvest lines
unchanged — the flush is independent of the harvest.)

- [ ] **Step 3: Flush inside `resolveDeferSingle`, under the DataBuffer lock (resolve.go ~587)**

Find the render phase in `resolveDeferSingle` (currently ~580-595):

```go
	// RENDER PHASE — serialised by the DataBuffer lock.
	dc.db.Lock()
	defer dc.db.Unlock()

	isLast := syncatomic.AddInt64(remaining, -1) == 0

	// Inject group-local state into Resolvable for this render.
	dc.resolvable.data = dc.db.Get()
	dc.resolvable.errors = groupLoader.errors
```

Add the flush right after `dc.resolvable.errors = groupLoader.errors`:

```go
	dc.resolvable.errors = groupLoader.errors
	groupLoader.flushSubgraphErrors()
```

Because this is inside the `dc.db.Lock()` critical section, concurrent defer groups
serialise their flushes into the shared `Context`. The flush must stay under the lock
(it writes the shared `Context`), so it cannot move to a `defer` at function top like
the initial loader does.

> NOTE: until Task 7 lands, the parallel-defer branch hands each group a cloned `ctx`
> (resolve.go ~644), so this flush writes into the clone and is discarded — behaviour
> unchanged at this point in the plan. Task 7 removes the clone, making this flush the
> mechanism that aggregates defer-group errors into `Context.SubgraphErrors()`, and
> also covers the early `return err` at resolve.go:575 (which skips this render-phase
> flush) with an error-path flush.

- [ ] **Step 4: Build and run the existing loader-hooks regression suite**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run TestLoaderHooks_FetchPipeline`
Expected: PASS — `OnFinished` still receives a non-nil `Err` for the single-fetch
subgraph-error case (its `responseInfo.Err` is now `res.subgraphError`, which equals
the old `subgraphErrors["Users"]` when only one fetch hits that subgraph).

---

## Task 6: End-to-end test — same-subgraph per-fetch isolation (the core bug)

**Files:**
- Test: `v2/pkg/engine/resolve/per_fetch_subgraph_errors_e2e_test.go` (create)

Two **sequential** fetches to the **same subgraph name** ("Users"): the first
returns subgraph errors, the second succeeds. With the old shared name-keyed map the
second fetch's `OnFinished.Err` would be non-nil (it would read
`subgraphErrors["Users"]`, already populated by fetch #1). With the per-`result`
error it must be `nil`. This is the exact regression the change fixes.

- [ ] **Step 1: Write the test**

Create `v2/pkg/engine/resolve/per_fetch_subgraph_errors_e2e_test.go`:

```go
package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// orderedLoaderHooks records, in OnFinished call order, whether each fetch's
// ResponseInfo.Err was set.
type orderedLoaderHooks struct {
	pre   atomic.Int64
	mu    sync.Mutex
	calls []bool // info.Err != nil, per OnFinished, in order
}

func (h *orderedLoaderHooks) OnLoad(ctx context.Context, ds DataSourceInfo) context.Context {
	h.pre.Add(1)
	return ctx
}

func (h *orderedLoaderHooks) OnFinished(ctx context.Context, ds DataSourceInfo, info *ResponseInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, info.Err != nil)
}

func TestOnFinished_SameSubgraphName_NoErrorInheritance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New(rCtx, ResolverOptions{MaxConcurrency: 1024})

	// Fetch #1: returns subgraph errors.
	failing := NewMockDataSource(ctrl)
	failing.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
			return []byte(`{"errors":[{"message":"boom"}]}`), nil
		})

	// Fetch #2: clean response, no errors array.
	clean := NewMockDataSource(ctrl)
	clean.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
			return []byte(`{}`), nil
		})

	hooks := &orderedLoaderHooks{}
	resolveCtx := NewContext(context.Background())
	resolveCtx.LoaderHooks = hooks

	resp := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: failing,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{DataSourceID: "Users", DataSourceName: "Users"},
			}, "query"),
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: clean,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{DataSourceID: "Users", DataSourceName: "Users"},
			}, "query"),
		),
		Data: &Object{
			Nullable: false,
			Fields: []*Field{
				{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
			},
		},
	}

	buf := &bytes.Buffer{}
	_, err := r.ResolveGraphQLResponse(resolveCtx, resp, nil, buf)
	require.NoError(t, err)

	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	require.Equal(t, int64(2), hooks.pre.Load(), "both fetches should load")
	require.Len(t, hooks.calls, 2, "OnFinished should fire once per fetch")
	assert.True(t, hooks.calls[0], "failing fetch's OnFinished must report its own error")
	assert.False(t, hooks.calls[1], "clean fetch's OnFinished must NOT inherit the failing fetch's error")

	// The aggregate Context error still reflects the (single) failing fetch.
	assert.Error(t, resolveCtx.SubgraphErrors())
}
```

- [ ] **Step 2: Run the test**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run TestOnFinished_SameSubgraphName_NoErrorInheritance`
Expected: PASS. (Reverting Tasks 3-4 makes `calls[1]` true — `assert.False` fails —
demonstrating the test catches the original over-reporting bug.)

- [ ] **Step 3: Full package run with the race detector**

Run: `gotestsum --format=short -- -race ./v2/pkg/engine/resolve/...`
Expected: PASS, no data races — confirms removing the shared-map write from the
concurrent merge path introduced no races and the loader-local map is touched serially.

---

## Task 7: Remove the parallel-defer Context clone (restore defer error aggregation)

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve.go` (`resolveDeferSingle` ~575; `resolveDeferTree` parallel branch ~623-656)
- Test: `v2/pkg/engine/resolve/resolve_defer_parallel_test.go` (append)

After Tasks 1-6 the loader no longer writes `ctx.subgraphErrors` during the
concurrent fetch phase — group loaders accumulate into their own `l.subgraphErrors`
and flush under `dc.db.Lock()`. The clone in the parallel defer branch existed only
to prevent races on that map (see the comment at resolve.go:633-643), so it can now
be removed. Removal is a **behaviour fix**: defer-group subgraph errors start
aggregating into `Context.SubgraphErrors()` (today they go to the discarded clone —
the documented KNOWN LIMITATION).

The other fields `clone()` deep-copies (`Variables`, `Files`, `Request.Header`,
`RenameTypeNames`, `RemapVariables`) are read-only during resolution (writes exist
only in `clone()`/`Free()`), so they don't need the clone either. The remaining
shared-`Context` writers during defer — `flushSubgraphErrors` (Task 5 Step 3) and
`Resolvable.addRejectFieldError` inside `ResolveDefer` — both run under
`dc.db.Lock()`, so they are serialised across groups.

- [ ] **Step 1: Write the failing aggregation test**

Append to `v2/pkg/engine/resolve/resolve_defer_parallel_test.go` (same harness as
`TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated` in that file; note
`SubgraphError.Error()` prints the subgraph *name*, not downstream messages, so the
fetches must carry named `FetchInfo` and assertions match on the names):

```go
// TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext:
// defer-group subgraph errors must reach Context.SubgraphErrors(). Before the
// Context clone is removed they are recorded on the discarded per-group clone
// and SubgraphErrors() returns nil.
func TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group A"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Info: &FetchInfo{DataSourceID: "subgraph-A", DataSourceName: "subgraph-A"},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group B"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Info: &FetchInfo{DataSourceID: "subgraph-B", DataSourceName: "subgraph-B"},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)

	joined := ctx.SubgraphErrors()
	require.Error(t, joined, "defer-group subgraph errors must aggregate into the Context")
	assert.Contains(t, joined.Error(), "subgraph-A")
	assert.Contains(t, joined.Error(), "subgraph-B")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext`
Expected: FAIL — `require.Error` fires because `SubgraphErrors()` is nil (errors
went to the discarded clone).

- [ ] **Step 3: Confirm the loader makes no direct Context writes (prerequisite check)**

Run: `grep -n "l.ctx.appendSubgraphErrors\|l\.ctx\.[A-Za-z]*\s*=" v2/pkg/engine/resolve/loader.go`
Expected: no output (Task 4 removed the appends; the loader never assigns ctx fields).

- [ ] **Step 4: Flush the group loader on the early-error path**

Once errors actually aggregate, the `return err` before the render phase would drop
a hard-erroring group's already-recorded errors. In `resolveDeferSingle`, current
(resolve.go ~575-577):

```go
	if err := groupLoader.ResolveFetchNode(group.Fetches); err != nil {
		return err
	}
```

New (the flush writes the shared Context, so it must take the lock):

```go
	if err := groupLoader.ResolveFetchNode(group.Fetches); err != nil {
		dc.db.Lock()
		groupLoader.flushSubgraphErrors()
		dc.db.Unlock()
		return err
	}
```

- [ ] **Step 5: Remove the clone and rewrite the stale comments**

In the `DeferTreeNodeKindParallel` branch of `resolveDeferTree`, current
(resolve.go ~624-654):

```go
		// Plain errgroup.Group (NOT errgroup.WithContext): a failed defer group
		// must not cancel its siblings, so we never let errgroup's error-driven
		// cancellation fire. errgroup is used only to spawn + wait + collect one
		// error. The client context (ctx.ctx) still cancels every in-flight fetch
		// on disconnect, because each group's fetch is passed a clone of it.
		var g errgroup.Group
		for _, child := range node.ChildNodes {
			child := child
			g.Go(func() error {
				// Clone the Context so this goroutine gets its OWN copy of the
				// mutable per-request fields — notably the subgraphErrors map,
				// which the fetch path writes via ctx.appendSubgraphErrors
				// (loader.go: mergeErrors / renderErrors*). Concurrent defer groups
				// sharing one *Context would race on that map. The underlying Go
				// context (ctx.ctx) is passed through unchanged, so a client
				// disconnect still cancels this group's fetch.
				// KNOWN LIMITATION: because the clone isolates subgraphErrors,
				// defer-group fetch subgraph errors do NOT aggregate into the
				// request ctx, so Context.SubgraphErrors() omits them. Accepted
				// for now.
				childCtx := ctx.clone(ctx.ctx)
				err := r.resolveDeferTree(dc, childCtx, child, remaining)
				// Surface the error only if the client context was cancelled
				// (disconnect). Ordinary defer-level subgraph errors are rendered
				// into the group's incremental frame, not propagated, so they
				// don't abort sibling groups.
				if err != nil && ctx.ctx.Err() != nil {
					return err
				}
				return nil
			})
		}
		return g.Wait()
```

New:

```go
		// Plain errgroup.Group (NOT errgroup.WithContext): a failed defer group
		// must not cancel its siblings, so we never let errgroup's error-driven
		// cancellation fire. errgroup is used only to spawn + wait + collect one
		// error. The client context (ctx.ctx) still cancels every in-flight fetch
		// on disconnect.
		var g errgroup.Group
		for _, child := range node.ChildNodes {
			child := child
			g.Go(func() error {
				// Groups share the request *Context. Its only mutation during defer
				// resolution is ctx.subgraphErrors, written exclusively under
				// dc.db.Lock() (flushSubgraphErrors and render-time
				// addRejectFieldError), so no per-goroutine clone is needed and
				// group subgraph errors aggregate into Context.SubgraphErrors().
				err := r.resolveDeferTree(dc, ctx, child, remaining)
				// Surface the error only if the client context was cancelled
				// (disconnect). Ordinary defer-level subgraph errors are rendered
				// into the group's incremental frame, not propagated, so they
				// don't abort sibling groups.
				if err != nil && ctx.ctx.Err() != nil {
					return err
				}
				return nil
			})
		}
		return g.Wait()
```

Also update the stale NOTE left by Task 5 Step 3 (the comment added there saying the
flush "writes into the cloned Context and is discarded") — the flush now targets the
shared Context.

- [ ] **Step 6: Run the test to verify it passes**

Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/ -run TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext`
Expected: PASS.

- [ ] **Step 7: Run the defer suites and the package with the race detector**

Run: `gotestsum --format=short -- -race ./v2/pkg/engine/resolve/...`
Expected: PASS, no data races — all `ctx.subgraphErrors` writes are serialised by
`dc.db.Lock()`; the read-only cloned fields are safe to share.

---

## Self-review checklist (performed)

- **Spec coverage:** OnFinished per-fetch error (Tasks 1,3,6); isolate writes to loader
  + single flush (Tasks 1,2,5); accumulation not overwrite (Task 2 test + helper);
  SubgraphErrors() preserved (Task 2 + Task 5 Step 4 + Task 6 final assertion);
  defer-group error aggregation restored by removing the Context clone (Task 7).
- **Type consistency:** `result.subgraphError error`, `Loader.subgraphErrors map[string]error`,
  `recordSubgraphError(res *result, errs ...error)`, `flushSubgraphErrors()`,
  `newResponseInfo(res *result)` used consistently across all tasks.
- **No placeholders:** all code is concrete and self-contained, including the Task 6
  two-fetch graph and hook and the Task 7 defer test.

## Risks / call-outs

- **Behaviour change (intended):** `OnFinished.Err` is now strictly per-fetch. Hook
  implementers relying on the aggregated value will see less — changelog note required.
- **Behaviour change (intended, Task 7):** defer-group subgraph errors now appear in
  `Context.SubgraphErrors()` (previously dropped with the clone) — consumers of that
  aggregate may observe more errors than before; changelog note required.
- **Hard-error path parity:** `LoadGraphQLResponseData` and the initial-defer loader
  flush via `defer`, so errors recorded before a later hard failure still reach the
  Context (matches old write-during-merge). Group loaders flush in the render phase
  and, after Task 7, also on the early-error return — both under `dc.db.Lock()`.
- **Two producers of `ctx.subgraphErrors`:** the loader flush and `Resolvable`'s
  render-time `addRejectFieldError` are independent and both intended; the flush is not
  the sole writer. During defer both run under `dc.db.Lock()`, which is what makes the
  Task 7 clone removal race-free.
- **Task 7 widens `Context` sharing:** external `authorizer`/`rateLimiter`
  implementations now receive the shared `*Context` from concurrent defer goroutines
  (previously per-group clones). The existing contract treats it as read-only during
  fetches; implementations violating that were already unsafe elsewhere.
