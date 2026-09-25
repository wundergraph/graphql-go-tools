# Design: Per-Defer-Group Error Isolation in Parallel Defer Execution

**Date:** 2026-05-26  
**Status:** Draft  
**Branch:** feat/eng-7770-add-defer-support-additional

---

## Key Design Decisions

The following choices were made explicitly during design and must not be revisited without
good reason.

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | **No shallow copies of `Loader` or `Resolvable`** | Shallow copies share arena, maps (`authorizationAllow`, `authorizationDeny`, `actualListSizes`), and other pointer fields by reference — silent data races and hard-to-audit ownership. The value copy of `Loader` in `resolveDeferSingle` is safe because the Loader no longer holds `*Resolvable`; the only shared pointer is `*DataBuffer`, which is intentional. |
| 2 | **Decouple `Loader` from `Resolvable`** | The Loader is a fetch+merge engine; the Resolvable is a render context. Their coupling is a design flaw: the Loader only uses four things from Resolvable (`data`, `errors`, two option flags, one bool write), none of which require knowledge of the full rendering context. Decoupling makes each independently testable and makes error ownership structural. |
| 3 | **`DataBuffer` — mutex lives with the data it protects** | Passing `renderMu` as a loose pointer (the previous approach) lets callers access `data` without the lock. Embedding the mutex in `DataBuffer` makes unprotected access impossible by construction. The `enableLock` flag preserves zero locking overhead on the non-enableLock path. |
| 4 | **Errors on `Loader` are a plain `*astjson.Value` — no wrapper type** | A dedicated error wrapper type was considered but rejected: since errors cannot own their arena (not thread-safe; all allocations happen under `DataBuffer.Lock()`), a wrapper would add a type with no behaviour beyond the lazy-init pattern already present on `Loader`. The `errors *astjson.Value` field with `ensureErrorsInitialized()` is sufficient. |

---

## Problem Statement

When GraphQL `@defer` groups execute in enableLock, they share a single `*Resolvable` and a
single `*Loader` instance. The shared `data` buffer is correctly protected by `renderMu`.
However, `Resolvable.errors` is conceptually per-defer-group state — each incremental
response frame must only contain errors belonging to **that** defer group — yet the field
is shared and reset bluntly at the start of every render phase.

### The Race

```
goroutine A: loader.ResolveFetchNode(groupA)
                └─ lockResolvable()
                └─ mergeErrors() → resolvable.errors = [errA]
                └─ unlockResolvable()

goroutine B: loader.ResolveFetchNode(groupB)          (concurrent with A)
                └─ lockResolvable()
                └─ mergeErrors() → resolvable.errors = [errA, errB]
                └─ unlockResolvable()

goroutine A  →  renderMu.Lock()
                resolvable.errors = nil          ← ✗ errA AND errB both lost
                render A's fragment
                renderMu.Unlock()

goroutine B  →  renderMu.Lock()
                resolvable.errors = nil
                render B's fragment              ← B's fetch-time errors gone
                renderMu.Unlock()
```

### Root Cause: a Design Flaw

`Loader` holds `*Resolvable` and writes directly into it:

| Access | Purpose |
|--------|---------|
| `l.resolvable.data` | path navigation + root data merge |
| `l.resolvable.errors` | append fetch-time errors (write-only, never read back) |
| `l.resolvable.options.XXX` | two Apollo-compat flags, read-only |
| `l.resolvable.skipValueCompletion` | one flag write |

The `Loader` is a **fetch + merge** engine. `Resolvable` is a **render** context. There is
no structural reason for the Loader to hold a full `*Resolvable`. The coupling is a design
flaw that makes per-defer-group error isolation impossible without complex routing
workarounds.

---

## Solution: Decouple `Loader` from `Resolvable`

`data` and `errors` are initialised **outside** both `Loader` and `Resolvable` and passed
into each separately. Options are copied at construction time. With the coupling removed,
each enableLock defer group gets its own `Loader` value copy that holds group-local `errors`
while sharing the same `DataBuffer`.

### Guiding principles

1. **`data` is shared** — all defers write into the same response tree; access is
   serialised by `DataBuffer`'s embedded lock.
2. **`errors` is per-defer-group** — each enableLock group's Loader starts with `errors = nil`
   and accumulates only its own errors; no mutex needed for the pointer itself.
3. **Arena writes stay under the lock** — the `jsonArena` is not thread-safe; all
   allocations (including error node creation) happen inside `DataBuffer.Lock()`.
4. **`Resolvable` is a pure render context** — `data` and `errors` are injected into it
   from outside at render time (caller holds the lock); it never reaches into the Loader.
5. **`Loader` is a pure fetch + merge engine** — it writes to its own `dataBuffer` and
   `errors`; it knows nothing about rendering.

---

## New Type: `DataBuffer`

`DataBuffer` owns the shared response tree and its concurrency protection. Only `Loader`
holds `*DataBuffer`. `Resolvable` receives `data *astjson.Value` directly at render time;
the lock is acquired by the caller in `resolve.go` before handing data to Resolvable.

```go
// DataBuffer is the shared response data tree with its concurrency guard.
// enableLock is false during normal single-threaded execution (no lock overhead)
// and set to true when enableLock defer groups are active.
type DataBuffer struct {
    mu       sync.Mutex
    enableLock bool
    data     *astjson.Value
}

func (d *DataBuffer) Lock()                { if d.enableLock { d.mu.Lock() } }
func (d *DataBuffer) Unlock()              { if d.enableLock { d.mu.Unlock() } }
func (d *DataBuffer) Get() *astjson.Value  { return d.data }
func (d *DataBuffer) Set(v *astjson.Value) { d.data = v }
```

`DataBuffer` replaces:
- `Loader.mu *sync.Mutex` (the `renderMu` pointer)
- `Loader.lockResolvable()` / `Loader.unlockResolvable()`
- The `renderMu` variable in `resolve.go`

---

## Errors on `Loader` — plain `*astjson.Value`

No wrapper type is introduced. The Loader holds `errors *astjson.Value` directly with
the same lazy-init pattern already used in Resolvable:

```go
// On Loader:
errors *astjson.Value

func (l *Loader) ensureErrorsInitialized() {
    if l.errors == nil {
        l.errors = astjson.ArrayValue(l.jsonArena)
    }
}
```

All existing `l.resolvable.ensureErrorsInitialized()` + `AppendToArray(..., l.resolvable.errors, ...)` 
calls become `l.ensureErrorsInitialized()` + `AppendToArray(..., l.errors, ...)`.

---

## Changes

### 1. `Loader` struct

```go
type Loader struct {
    // dataBuffer is the shared response tree with its concurrency guard.
    // Shared with Resolvable via the same *DataBuffer pointer.
    dataBuffer *DataBuffer

    // errors is the per-group error accumulator.
    // For enableLock defer groups each Loader value copy starts with nil
    // and accumulates only its own group's errors.
    // All writes happen under dataBuffer.Lock() (arena is not thread-safe).
    errors *astjson.Value

    // Apollo-compat flags — copied from ResolvableOptions at Loader
    // construction time in newTools; never read from Resolvable at fetch time.
    apolloCompatibilitySuppressFetchErrors          bool
    apolloCompatibilityValueCompletionInExtensions  bool

    // skipValueCompletion is set by the Loader when a response has errors
    // but no data and valueCompletionInExtensions is enabled.
    // The caller reads this back after ResolveFetchNode and sets
    // resolvable.skipValueCompletion before rendering.
    skipValueCompletion bool

    // --- unchanged ---
    ctx        *Context
    info       *GraphQLResponseInfo
    jsonArena  arena.Arena
    // ... propagation flags, singleFlight, taintedObjs, etc.
    // mu and lockResolvable/unlockResolvable are removed
}
```

### 2. `Loader.Init()` — no longer takes `*Resolvable` or `errors`

The Loader always starts with `errors = nil` and initialises lazily on first error.
The caller reads `loader.errors` back after `ResolveFetchNode` completes.
Passing an external `errors` value into `Init` would imply shared ownership — which is
exactly what is being eliminated.

```go
func (l *Loader) Init(ctx *Context, info *GraphQLResponseInfo, db *DataBuffer) {
    l.dataBuffer = db
    l.errors     = nil
    l.ctx        = ctx
    l.info       = info
    l.taintedObjs = make(taintedObjects)
}
```

### 3. Internal Loader method changes

| Before | After |
|--------|-------|
| `l.resolvable.data` | `l.dataBuffer.Get()` |
| `l.resolvable.data = v` | `l.dataBuffer.Set(v)` |
| `l.resolvable.errors` | `l.errors` |
| `l.resolvable.ensureErrorsInitialized()` | `l.ensureErrorsInitialized()` |
| `l.resolvable.options.ApolloCompatibilitySuppressFetchErrors` | `l.apolloCompatibilitySuppressFetchErrors` |
| `l.resolvable.options.ApolloCompatibilityValueCompletionInExtensions` | `l.apolloCompatibilityValueCompletionInExtensions` |
| `l.resolvable.skipValueCompletion = true` | `l.skipValueCompletion = true` |
| `l.lockResolvable()` / `l.unlockResolvable()` | `l.dataBuffer.Lock()` / `l.dataBuffer.Unlock()` |
| `l.mu = renderMu` | `l.dataBuffer.enableLock = true` |

### 4. `Resolvable` struct

`Resolvable.data` remains a plain `*astjson.Value`. It is set by `resolve.go` from
`loader.dataBuffer.Get()` while the caller already holds `DataBuffer.Lock()`.
`Resolvable.errors` is similarly set from `groupLoader.errors` before each render.
`Resolvable` gains no knowledge of `DataBuffer`.

```go
type Resolvable struct {
    data   *astjson.Value // set from DataBuffer.Get() before rendering, under lock
    errors *astjson.Value // set from groupLoader.errors before rendering, under lock
    // ... all other fields unchanged
}
```

### 5. `LoadGraphQLResponseData`

```go
func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, db *DataBuffer) error {
    l.Init(ctx, response.Info, db)
    return l.ResolveFetchNode(response.Fetches)
}
```

### 6. `resolve.go` — construction and parallel defer execution

```go
// At tools construction (newTools or equivalent):
db := &DataBuffer{}  // enableLock = false, no lock overhead on the normal path
loader.dataBuffer = db

// Non-defer path:
loader.LoadGraphQLResponseData(ctx, response, db)
resolvable.data   = db.Get()      // pick up root replacement if it happened
resolvable.errors = loader.errors // errors accumulated during fetch

// Before parallel defer tree resolution:
db.enableLock = true  // arm the lock for concurrent access

// resolveDeferSingle — per-goroutine Loader value copy:
func (r *Resolver) resolveDeferSingle(group, response, t, writer, remaining) error {
    groupLoader := *t.loader    // value copy — own errors field
    groupLoader.errors = nil    // group-local, starts empty

    if err := groupLoader.ResolveFetchNode(group.Fetches); err != nil {
        return err
    }

    t.loader.dataBuffer.Lock()
    defer t.loader.dataBuffer.Unlock()

    // Inject group-local state into Resolvable for this render.
    t.resolvable.data   = t.loader.dataBuffer.Get()
    t.resolvable.errors = groupLoader.errors

    // TODO: skipValueCompletion is set to true inside mergeResult when a fetch
    // response has errors but no data and apolloCompatibilityValueCompletionInExtensions
    // is enabled. Setting it here propagates the flag from this group's fetch phase to
    // the render phase, which is correct for this group. However, within a single
    // group's fetch tree, resolveParallel spawns concurrent sub-fetches that share the
    // same Loader — if one sub-fetch sets skipValueCompletion=true, it contaminates all
    // other in-flight sub-fetches in that group. This is a pre-existing issue that
    // should be addressed separately.
    t.resolvable.skipValueCompletion = groupLoader.skipValueCompletion

    descriptor := t.resolvable.deferDescriptors[group.DeferID]
    t.resolvable.currentDefer = &descriptor
    if err := t.resolvable.ResolveDefer(writer, !isLast); err != nil {
        return err
    }
    return writer.Flush()
}
```

---

## What is NOT changing

- Locking protocol for data and arena — all writes still serialised; `DataBuffer.Lock()`
  is the same critical section as `renderMu.Lock()` was.
- `Resolvable` rendering logic — field resolution, null propagation, render methods are
  all untouched.
- Non-defer path — `DataBuffer.enableLock` is false; no lock overhead.
- Error lazy-init pattern — preserved on the Loader (`ensureErrorsInitialized`).

---

## Benefits

- **Correctness** — errors are structurally owned by the group that produced them; no maps,
  no DeferID routing, no `errors = nil` timing hazards.
- **Loader is decoupled from Resolvable** — testable without constructing a Resolvable;
  clear single responsibility.
- **Lock lives with the data it protects** — `DataBuffer` makes it impossible to access
  `data` without going through the type; the `enableLock` flag preserves zero-overhead on
  the non-enableLock path.
- **No new types for errors** — a plain `*astjson.Value` with a two-line lazy init is
  sufficient; no wrapper needed.

## Drawbacks

- Broader diff than a targeted patch: `Loader.Init`, `LoadGraphQLResponseData`, all
  `l.resolvable.X` accesses in `loader.go`, and all `r.data` accesses in `resolvable.go`
  change. Mechanically straightforward but touches many lines.
- `resolveDeferSingle` gains two explicit handoff assignments
  (`resolvable.errors = groupLoader.errors`, `resolvable.skipValueCompletion = ...`).
  These should be grouped into a small helper to make the contract obvious and
  hard to forget.
