# Defer Group Error Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple `Loader` from `Resolvable` so that parallel defer groups each own their fetch-time errors, eliminating the bug where `errors = nil` in the render phase discards every group's accumulated errors.

**Architecture:** Introduce `DataBuffer` to carry the shared response JSON tree and its concurrency guard; the `DataBuffer` owns its root object (created with the arena, not derived from `Resolvable`). `Loader` holds `*DataBuffer` and its own `errors *astjson.Value`; it no longer holds `*Resolvable`. Before each render (initial or deferred), `resolve.go` injects `data` and `errors` into `Resolvable` from the loader(s). Each parallel defer group gets its **own** `Loader` instance (via `NewLoader`), sharing only the parent's `*DataBuffer` (the response tree). It uses a **nil arena** — since the arena is not thread-safe and groups run concurrently, per-group error and merge allocations go on the heap; the shared tree is mutated only under `DataBuffer.Lock()`. Each group also gets a fresh `errors` accumulator and `taintedObjs` map — structural per-group ownership with no shared mutable state (no value copy, which would alias the `taintedObjs` map and race). The defer helpers receive a `*deferContext` bundling the request-scoped shared state (response, info, db, resolvable, writer), plus `ctx` as a separate arg since it's cloned per parallel goroutine.

**Tech Stack:** Go, `github.com/wundergraph/astjson`, `sync.Mutex`, `gotestsum`

---

## File Map

| File | Change |
|------|--------|
| `v2/pkg/engine/resolve/data_buffer.go` | **Create** — `DataBuffer` type |
| `v2/pkg/engine/resolve/data_buffer_test.go` | **Create** — `DataBuffer` unit tests |
| `v2/pkg/engine/resolve/resolve_defer_parallel_test.go` | **Modify** — add failing parallel-errors test |
| `v2/pkg/engine/resolve/loader.go` | **Modify** — remove `*Resolvable`/`*sync.Mutex`; add `*DataBuffer`, `errors`, `skipValueCompletion`, Apollo flags; migrate all internals; remove `lockResolvable`/`unlockResolvable` |
| `v2/pkg/engine/resolve/resolve.go` | **Modify** — replace `newTools` in place with `NewLoader` (returns `*Loader`, takes shared `*DataBuffer`); remove `tools` struct; update all 4 call sites to create `db`/`resolvable`/`loader` + post-fetch injection; arm `dataBuffer.enableLock` instead of `loader.mu`; introduce `deferContext`; rewrite `resolveDeferTree`/`resolveDeferSingle` to take `*deferContext` and build a per-group nil-arena `Loader` |
| `v2/pkg/engine/resolve/loader_test.go` | **Modify** — give each `&Loader{}` a `DataBuffer`; drop `resolvable` arg from `LoadGraphQLResponseData`; read data/errors back from loader |

---

## Task 1: Write the Failing Test

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve_defer_parallel_test.go`

- [ ] **Add the test at the bottom of the file**

```go
func TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pass-through mode so raw subgraph error messages appear verbatim in the
	// incremental frames, making them easy to assert on.
	r := New(rCtx, ResolverOptions{
		MaxConcurrency:               1024,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
	})

	// Each group's data source returns errors alongside data. PostProcessing
	// must select the "data" and "errors" paths — without SelectResponseErrorsPath
	// the loader never extracts the errors array (loader.go gates extraction on
	// res.postProcessing.SelectResponseErrorsPath != nil), and the errors would be
	// silently dropped regardless of the fix.
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
	require.Len(t, writer.payloads, 3, "expected 1 initial + 2 incremental frames")

	// Each error must appear exactly once across all incremental payloads.
	// Before the fix both errors are discarded (errors=nil wipes them); this
	// assertion fails with count 0 for both.
	all := strings.Join(writer.payloads[1:], " ")
	require.Equal(t, 1, strings.Count(all, "error from group A"),
		"error from group A must appear in exactly one incremental frame")
	require.Equal(t, 1, strings.Count(all, "error from group B"),
		"error from group B must appear in exactly one incremental frame")

	// Isolation: no single incremental frame must contain both errors.
	for _, p := range writer.payloads[1:] {
		if strings.Contains(p, "error from group A") {
			require.NotContains(t, p, "error from group B",
				"group A frame must not contain group B error")
		}
	}
}
```

- [ ] **Run the test to confirm it fails**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/... -run TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated
```

Expected: FAIL — the `strings.Count` assertions produce 0 (errors are discarded).

---

## Task 2: Create `DataBuffer`

**Files:**
- Create: `v2/pkg/engine/resolve/data_buffer.go`
- Create: `v2/pkg/engine/resolve/data_buffer_test.go`

- [ ] **Create `data_buffer.go`**

```go
package resolve

import (
	"sync"

	"github.com/wundergraph/astjson"
)

// DataBuffer holds the shared response JSON tree and its concurrency guard.
//
// enableLock is false during normal single-threaded execution (no locking
// overhead) and set to true before parallel defer groups are launched.
//
// Only the Loader holds a *DataBuffer. The Loader reads/writes the tree during
// the fetch phase; resolve.go reads via Get and injects the value into
// Resolvable.data before each render. Resolvable never references the DataBuffer.
type DataBuffer struct {
	mu         sync.Mutex
	enableLock bool
	data       *astjson.Value
}

// Lock acquires the mutex when parallel execution is active.
func (d *DataBuffer) Lock() {
	if d.enableLock {
		d.mu.Lock()
	}
}

// Unlock releases the mutex when parallel execution is active.
func (d *DataBuffer) Unlock() {
	if d.enableLock {
		d.mu.Unlock()
	}
}

// Get returns the current data value.
func (d *DataBuffer) Get() *astjson.Value { return d.data }

// Set replaces the data value (root-level merge case in Loader.mergeResult).
func (d *DataBuffer) Set(v *astjson.Value) { d.data = v }
```

- [ ] **Create `data_buffer_test.go`**

```go
package resolve

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"
)

func TestDataBuffer_LockDisabled(t *testing.T) {
	d := &DataBuffer{}
	// Must not panic or deadlock when enableLock is false.
	d.Lock()
	d.Unlock()
}

func TestDataBuffer_LockEnabled(t *testing.T) {
	// Run with `go test -race` to have the race detector verify serialisation.
	// Two goroutines each write a distinct field on the shared object under the lock;
	// afterwards the object must contain both fields.
	obj := astjson.ObjectValue(nil)
	d := &DataBuffer{enableLock: true, data: obj}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.Lock()
		astjson.SetValue(nil, d.Get(), astjson.StringValue(nil, "a"), "a")
		d.Unlock()
	}()
	go func() {
		defer wg.Done()
		d.Lock()
		astjson.SetValue(nil, d.Get(), astjson.StringValue(nil, "b"), "b")
		d.Unlock()
	}()
	wg.Wait()

	got := d.Get().String()
	assert.Contains(t, got, `"a":"a"`, "field a must be present")
	assert.Contains(t, got, `"b":"b"`, "field b must be present")
}

func TestDataBuffer_GetSet(t *testing.T) {
	d := &DataBuffer{}
	assert.Nil(t, d.Get())
	// Set accepts nil without panic.
	d.Set(nil)
	assert.Nil(t, d.Get())
}
```

- [ ] **Run the new tests**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/... -run TestDataBuffer
```

Expected: PASS (3 tests).

---

## Task 3: Decouple Loader from Resolvable; Replace `newTools` with `NewLoader`; Per-Group Defer Loaders

One atomic refactor that also lands the fix. The Loader changes shape, all internals migrate to the new fields, `newTools` is replaced **in place** by `NewLoader` (returns `*Loader`, takes the shared `*DataBuffer`), the `tools` struct is deleted, every caller creates `resolvable`/`loader`/`db` directly, and the loader tests are updated. The defer helpers are rewritten to take a `*deferContext` and give **each defer group its own `Loader`** (via `NewLoader` with a nil arena, sharing the parent `*DataBuffer`) — the per-group error isolation that makes the Task 1 test pass. The defer helpers must be rewritten anyway once `tools` is removed, so doing it once in the final form avoids churn. No bridge/intermediate state — everything compiles together and the Task 1 test goes green at the end.

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go`
- Modify: `v2/pkg/engine/resolve/resolve.go`
- Modify: `v2/pkg/engine/resolve/loader_test.go`

### Loader struct & methods (loader.go)

- [ ] **Replace the `resolvable *Resolvable` and `mu *sync.Mutex` fields** in the Loader struct with the new fields:

```go
// dataBuffer holds the shared response tree and its concurrency guard.
dataBuffer *DataBuffer

// errors accumulates fetch-time errors for this Loader instance.
// Each parallel defer group gets its own Loader (via NewLoader) and so its
// own errors. All writes happen under dataBuffer.Lock() (arena not thread-safe).
errors *astjson.Value

// skipValueCompletion is set when a response has errors but no data
// and apolloCompatibilityValueCompletionInExtensions is enabled.
// Read back by the caller after ResolveFetchNode.
skipValueCompletion bool

// Apollo compatibility flags — copied from ResolvableOptions in NewLoader.
// Replaces reading l.resolvable.options at fetch time.
apolloCompatibilitySuppressFetchErrors         bool
apolloCompatibilityValueCompletionInExtensions bool
```

- [ ] **Update `Loader.Free()`** — remove the `resolvable` and `mu` nil assignments:

```go
func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.taintedObjs = nil
}
```

- [ ] **Update `Loader.Init()`** — remove `*Resolvable` parameter; reset only loader-owned fields (`taintedObjs` gets a fresh map so per-group loaders never alias it):

```go
func (l *Loader) Init(ctx *Context, responseInfo *GraphQLResponseInfo) {
	l.errors = nil
	l.skipValueCompletion = false
	l.ctx = ctx
	l.info = responseInfo
	l.taintedObjs = make(taintedObjects)
}
```

- [ ] **Update `LoadGraphQLResponseData()`** — remove `*Resolvable` parameter:

```go
func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse) (err error) {
	l.Init(ctx, response.Info)
	return l.ResolveFetchNode(response.Fetches)
}
```

- [ ] **Add `ensureErrorsInitialized()` method to `Loader`:**

```go
func (l *Loader) ensureErrorsInitialized() {
	if l.errors == nil {
		l.errors = astjson.ArrayValue(l.jsonArena)
	}
}
```

### Loader internals (loader.go)

- [ ] **Replace `l.resolvable.data` reads and writes:**

| Old | New |
|-----|-----|
| `items[0] = l.resolvable.data` | `items[0] = l.dataBuffer.Get()` |
| `l.resolvable.data = responseData` | `l.dataBuffer.Set(responseData)` |

- [ ] **Replace `l.resolvable.options` reads:**

| Old | New |
|-----|-----|
| `l.resolvable.options.ApolloCompatibilitySuppressFetchErrors` | `l.apolloCompatibilitySuppressFetchErrors` |
| `l.resolvable.options.ApolloCompatibilityValueCompletionInExtensions` | `l.apolloCompatibilityValueCompletionInExtensions` |

- [ ] **Replace `l.resolvable.skipValueCompletion` write:**

| Old | New |
|-----|-----|
| `l.resolvable.skipValueCompletion = true` | `l.skipValueCompletion = true` |

- [ ] **Replace every `l.resolvable.errors` write + `l.resolvable.ensureErrorsInitialized()` call** — 8 `ensureErrorsInitialized` call sites across **7 methods**: `mergeErrors` (twice — once in append mode, once in pass-through mode), `addApolloRouterCompatibilityError`, `renderErrorsFailedDeps`, `renderErrorsFailedToFetch`, `renderErrorsStatusFallback`, `renderAuthorizationRejectedErrors`, `renderRateLimitRejectedErrors` (the rate-limit method has 4 `AppendToArray` calls under a single `ensureErrorsInitialized`).

Replace the standard pair:

```go
// Old:
l.resolvable.ensureErrorsInitialized()
astjson.AppendToArray(l.jsonArena, l.resolvable.errors, errorObject)

// New:
l.ensureErrorsInitialized()
astjson.AppendToArray(l.jsonArena, l.errors, errorObject)
```

And the pass-through path of `mergeErrors`:

```go
// Old:
l.resolvable.ensureErrorsInitialized()
l.resolvable.errors.AppendArrayItems(l.jsonArena, value)

// New:
l.ensureErrorsInitialized()
l.errors.AppendArrayItems(l.jsonArena, value)
```

- [ ] **Replace `l.lockResolvable()` / `l.unlockResolvable()` calls with `l.dataBuffer.Lock()` / `l.dataBuffer.Unlock()`.**

- [ ] **Remove the `lockResolvable` and `unlockResolvable` methods** (dead code):

```go
// remove:
func (l *Loader) lockResolvable()   { if l.mu != nil { l.mu.Lock() } }
func (l *Loader) unlockResolvable() { if l.mu != nil { l.mu.Unlock() } }
```

### Replace `newTools` with `NewLoader` (resolve.go)

- [ ] **Replace `newTools` in place with `NewLoader`** (same location, lines ~294–316). It returns `*Loader` (no more `tools` wrapper, no `NewResolvable` call) and takes the shared `*DataBuffer` so per-group defer loaders can share it. Copies the two Apollo compat flags:

```go
func NewLoader(options ResolverOptions, allowedExtensionFields map[string]struct{}, allowedErrorFields map[string]struct{}, sf *SubgraphRequestSingleFlight, a arena.Arena, db *DataBuffer) *Loader {
	return &Loader{
		dataBuffer:                                     db,
		apolloCompatibilitySuppressFetchErrors:         options.ResolvableOptions.ApolloCompatibilitySuppressFetchErrors,
		apolloCompatibilityValueCompletionInExtensions: options.ResolvableOptions.ApolloCompatibilityValueCompletionInExtensions,
		propagateSubgraphErrors:                      options.PropagateSubgraphErrors,
		propagateSubgraphStatusCodes:                 options.PropagateSubgraphStatusCodes,
		subgraphErrorPropagationMode:                 options.SubgraphErrorPropagationMode,
		rewriteSubgraphErrorPaths:                    options.RewriteSubgraphErrorPaths,
		omitSubgraphErrorLocations:                   options.OmitSubgraphErrorLocations,
		omitSubgraphErrorExtensions:                  options.OmitSubgraphErrorExtensions,
		allowedErrorExtensionFields:                  allowedExtensionFields,
		attachServiceNameToErrorExtension:            options.AttachServiceNameToErrorExtensions,
		defaultErrorExtensionCode:                    options.DefaultErrorExtensionCode,
		allowedSubgraphErrorFields:                   allowedErrorFields,
		allowAllErrorExtensionFields:                 options.AllowAllErrorExtensionFields,
		apolloRouterCompatibilitySubrequestHTTPError: options.ApolloRouterCompatibilitySubrequestHTTPError,
		propagateFetchReasons:                        options.PropagateFetchReasons,
		validateRequiredExternalFields:               options.ValidateRequiredExternalFields,
		singleFlight:                                 sf,
		jsonArena:                                    a,
	}
}
```

- [ ] **Delete the `tools` struct from `resolve.go`:**

```go
// remove:
type tools struct {
	resolvable *Resolvable
	loader     *Loader
}
```

### Call sites (resolve.go)

Each call site creates the `DataBuffer` (which owns its empty root object — `astjson.ObjectValue(a)`, **not** derived from Resolvable), a `Resolvable`, and a `Loader`. No sync from Resolvable to DataBuffer is needed before fetching. After the fetch, inject `data`/`errors`/`skipValueCompletion` back into Resolvable for the render phase. (`Resolvable.Init`/`InitSubscription` still set their own `r.data`, used only on the `SkipLoader` path where no fetch happens.)

- [ ] **Update `ResolveGraphQLResponse`** (~line 337):

```go
db := &DataBuffer{data: astjson.ObjectValue(nil)}
resolvable := NewResolvable(nil, r.options.ResolvableOptions)
loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil, db)

err := resolvable.Init(ctx, data, response.Info.OperationType)
if err != nil { return nil, err }

if !ctx.ExecutionOptions.SkipLoader {
	err = loader.LoadGraphQLResponseData(ctx, response)
	if err != nil { return nil, err }
	// Inject loader output into Resolvable before rendering.
	resolvable.data = loader.dataBuffer.Get()
	resolvable.errors = loader.errors
	resolvable.skipValueCompletion = loader.skipValueCompletion
}

err = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, writer)
...
ctx.ActualListSizes = resolvable.actualListSizes
```

- [ ] **Update `ArenaResolveGraphQLResponse`** (~line 391) — same pattern with the pool arena:

```go
resolveArena := r.resolveArenaPool.Acquire(ctx.Request.ID)
db := &DataBuffer{data: astjson.ObjectValue(resolveArena.Arena)}
resolvable := NewResolvable(resolveArena.Arena, r.options.ResolvableOptions)
loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena, db)

err = resolvable.Init(ctx, nil, response.Info.OperationType)
if err != nil { ... }

if !ctx.ExecutionOptions.SkipLoader {
	err = loader.LoadGraphQLResponseData(ctx, response)
	if err != nil { ... }
	resolvable.data = loader.dataBuffer.Get()
	resolvable.errors = loader.errors
	resolvable.skipValueCompletion = loader.skipValueCompletion
}
```

- [ ] **Update `executeSubscriptionUpdate`** (~line 746):

```go
resolveArena := r.resolveArenaPool.Acquire(resolveCtx.Request.ID)
db := &DataBuffer{data: astjson.ObjectValue(resolveArena.Arena)}
resolvable := NewResolvable(resolveArena.Arena, r.options.ResolvableOptions)
loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena, db)

if err := resolvable.InitSubscription(resolveCtx, input, sub.resolve.Trigger.PostProcessing); err != nil { ... }
if err := loader.LoadGraphQLResponseData(resolveCtx, sub.resolve.Response); err != nil { ... }
resolvable.data = loader.dataBuffer.Get()
resolvable.errors = loader.errors
resolvable.skipValueCompletion = loader.skipValueCompletion
```

- [ ] **Update `ResolveGraphQLDeferResponse`** (~line 442) — create the three locals; injection for the initial fetch; arm `dataBuffer.enableLock = true` instead of setting `loader.mu`; build a `deferContext` and pass it to `resolveDeferTree`:

```go
db := &DataBuffer{data: astjson.ObjectValue(nil)}
resolvable := NewResolvable(nil, r.options.ResolvableOptions)
loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil, db)

err := resolvable.Init(ctx, nil, response.Response.Info.OperationType)
if err != nil { return nil, err }

if !ctx.ExecutionOptions.SkipLoader {
	loader.Init(ctx, response.Response.Info)

	if err := loader.ResolveFetchNode(response.Response.Fetches); err != nil { return nil, err }

	resolvable.data = loader.dataBuffer.Get()
	resolvable.errors = loader.errors
	resolvable.skipValueCompletion = loader.skipValueCompletion

	resolvable.deferMode = true
	resolvable.currentDefer = nil
	resolvable.deferDescriptors = response.DeferDescriptors

	err = resolvable.Resolve(ctx.ctx, response.Response.Data, response.Response.Fetches, writer)
	if err != nil { return nil, err }
	err = writer.Flush()
	if err != nil { return nil, err }
	if resolvable.hasErrors() { return resolveInfo, nil }

	if response.DeferTree != nil {
		// Arm the DataBuffer lock so that concurrent defer-group goroutines
		// serialise their data merges and render phases.
		db.enableLock = true
		dc := &deferContext{
			response:   response,
			info:       response.Response.Info,
			db:         db,
			resolvable: resolvable,
			writer:     writer,
		}
		remaining := int64(countDeferLeaves(response.DeferTree))
		if err := r.resolveDeferTree(dc, ctx, response.DeferTree, &remaining); err != nil {
			return nil, err
		}
	}

	writer.Complete()
}
```

- [ ] **Introduce `deferContext` and rewrite `resolveDeferTree` / `resolveDeferSingle`** — these helpers previously took `t *tools` + `renderMu`. They now take a `*deferContext` (the request-scoped state shared by every node in the walk) plus `ctx` (kept separate because each parallel branch hands each goroutine its own clone), the per-node `node`/`group`, and the shared `remaining` counter. The parallel branch uses a **plain `errgroup.Group`** (not `errgroup.WithContext`) so a failed defer group never cancels its siblings; the client context still flows through to each fetch via the clone. Each defer group builds its **own** `Loader` via `NewLoader` with a **nil arena**: groups run concurrently and the arena is not thread-safe, so this group's error and merge allocations land on the heap (concurrency-safe); structural mutation of the shared response tree stays serialised by `db.Lock()`. This is the actual per-group error-isolation fix.

```go
// deferContext bundles the request-scoped state shared by every node in a
// defer tree walk. ctx is NOT included — it varies per goroutine (cloned with
// the errgroup context in the parallel branch) and is passed as its own arg.
type deferContext struct {
	response   *GraphQLDeferResponse
	info       *GraphQLResponseInfo
	db         *DataBuffer
	resolvable *Resolvable
	writer     DeferResponseWriter
}

func (r *Resolver) resolveDeferTree(dc *deferContext, ctx *Context, node *DeferTreeNode, remaining *int64) error {
	switch node.Kind {
	case DeferTreeNodeKindSingle:
		return r.resolveDeferSingle(dc, ctx, node.Item, remaining)

	case DeferTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			if err := r.resolveDeferTree(dc, ctx, child, remaining); err != nil {
				return err
			}
		}
		return nil

	case DeferTreeNodeKindParallel:
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
				// sharing one *Context would race on that map (concurrent write +
				// lazy make()). The underlying Go context (ctx.ctx) is passed
				// through unchanged, so a client disconnect still cancels this
				// group's fetch.
				// KNOWN LIMITATION: because the clone isolates subgraphErrors,
				// defer-group fetch subgraph errors do NOT aggregate into the
				// request ctx, so Context.SubgraphErrors() omits them. Accepted
				// for now; see the note below this function.
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
	}
	return nil
}

func (r *Resolver) resolveDeferSingle(dc *deferContext, ctx *Context, group *DeferFetchGroup, remaining *int64) error {
	// FETCH PHASE — runs outside the DataBuffer lock.
	// Each goroutine gets its OWN Loader with a NIL arena. Defer groups run
	// concurrently and the arena is not thread-safe, so this group's errors and
	// merge allocations go on the heap (concurrency-safe). The shared response
	// tree (dc.db) is mutated only under dc.db.Lock() inside the merge phase.
	groupLoader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil, dc.db)
	groupLoader.Init(ctx, dc.info) // fresh taintedObjs; errors=nil

	if err := groupLoader.ResolveFetchNode(group.Fetches); err != nil {
		return err
	}

	// RENDER PHASE — serialised by the DataBuffer lock.
	dc.db.Lock()
	defer dc.db.Unlock()

	isLast := syncatomic.AddInt64(remaining, -1) == 0

	// Inject group-local state into Resolvable for this render.
	dc.resolvable.data = dc.db.Get()
	dc.resolvable.errors = groupLoader.errors

	// TODO: skipValueCompletion is set inside mergeResult when a fetch response
	// has errors but no data and apolloCompatibilityValueCompletionInExtensions
	// is enabled. Within a single group's fetch tree, resolveParallel spawns
	// concurrent sub-fetches that share this group's Loader — if one sub-fetch
	// sets skipValueCompletion=true it contaminates the others in that group.
	// This is a pre-existing issue to be addressed separately.
	dc.resolvable.skipValueCompletion = groupLoader.skipValueCompletion

	descriptor := dc.resolvable.deferDescriptors[group.DeferID]
	dc.resolvable.currentDefer = &descriptor
	if err := dc.resolvable.ResolveDefer(dc.response.Response.Data, dc.writer, !isLast); err != nil {
		return err
	}
	return dc.writer.Flush()
}
```

> **Known limitation (accepted for now):** the per-goroutine `ctx.clone(ctx.ctx)` isolates each defer group's `ctx.subgraphErrors` map (needed to avoid a concurrent-write race). As a consequence, defer-group **fetch** subgraph errors accumulate in the clone and are **not** merged back into the request-level `*Context` — so the exported `Context.SubgraphErrors()` aggregate (consumed externally, e.g. by the router) will be missing defer-group subgraph errors. This differs from the non-defer path, where subgraph errors land on the request ctx. The per-fetch `LoaderHooks.OnFinished` reader is unaffected (it reads the group's own clone). Leaving this as-is for now; a follow-up can merge each group's `subgraphErrors` back into the original ctx under `db.Lock()` after its fetch.

### Loader tests (loader_test.go)

- [ ] **Update all 8 `loader := &Loader{}` constructions** (lines ~289, 376, 465, 745, 1018, 1117, 1411, 2080) to give the loader a `DataBuffer` that owns an empty root object (nil arena = heap, matching these tests' `NewResolvable(nil, ...)`):

```go
// Old:
loader := &Loader{}
// New:
loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
```

- [ ] **Update all 7 `LoadGraphQLResponseData` call sites** (lines ~292, 379, 468, 748, 1030, 1122, 1414) — drop the `resolvable` parameter and add a post-load read from the loader so existing `fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)` assertions still see populated values (no pre-load sync needed — the DataBuffer already owns its root):

```go
// Old:
err = loader.LoadGraphQLResponseData(ctx, response, resolvable)

// New:
err = loader.LoadGraphQLResponseData(ctx, response)
resolvable.data = loader.dataBuffer.Get()
resolvable.errors = loader.errors
```

### Wrap-up

- [ ] **Format the modified files**

```
gofmt -w v2/pkg/engine/resolve/loader.go v2/pkg/engine/resolve/resolve.go v2/pkg/engine/resolve/loader_test.go
```

- [ ] **Compile**

```
cd v2 && go build ./pkg/engine/resolve/...
```

Expected: no errors.

- [ ] **Run the previously-failing test — it must now pass**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/... -run TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated
```

Expected: PASS — each defer group's fetch errors are now isolated in its own per-group Loader.

- [ ] **Run all resolve tests**

```
gotestsum --format=short -- ./v2/pkg/engine/resolve/...
```

Expected: all tests PASS.

---

## Task 4: Full Test Suite

- [ ] **Run the full engine test suite** (includes resolve, datasource, plan, postprocess):

```
gotestsum --format=short -- ./v2/pkg/engine/...
```

- [ ] **Run the execution/engine test suite** (separate module):

```
cd execution && gotestsum --format=short -- ./engine/...
```

- [ ] **Run the full v2 module test suite**

```
gotestsum --format=short -- ./v2/...
```

Expected: all tests pass. If any test fails, the failure is a pre-existing issue unrelated to this change — check by running the same test on `master`.

- [ ] **Verify the spec is satisfied:**

  - `TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated` passes ✅
  - `Loader` no longer holds `*Resolvable` ✅
  - `DataBuffer` carries the shared data tree (owns its root object) and its lock ✅
  - Each parallel defer group gets its own `Loader` instance (via `NewLoader`) sharing the parent `*DataBuffer`, with a fresh `errors` accumulator and `taintedObjs` map ✅
  - `DataBuffer.enableLock` is false on non-defer paths (zero locking overhead) ✅
