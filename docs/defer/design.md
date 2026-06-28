# GraphQL `@defer` — Design Document

## Overview

The `@defer` directive lets a client mark fragments — inline fragments or named fragment spreads — whose fields are delivered as separate incremental payloads instead of blocking the primary response. It is valid on queries only; mutations (serial execution) and subscriptions (already streaming) are excluded by design.

```graphql
# anonymous inline fragment
query {
  user(id: "1") {
    name
    ... @defer { expensiveField }
  }
}

# inline fragment with type condition
query {
  user(id: "1") {
    name
    ... on User @defer { expensiveField }
  }
}

# named fragment spread
fragment UserDetails on User { expensiveField }
query {
  user(id: "1") {
    name
    ...UserDetails @defer
  }
}
```

The directive accepts two optional arguments:
- `if: Boolean` — when `false`, the fragment is not deferred and its fields appear in the primary response. Defaults to `true`. Both literal booleans and variables are evaluated.
- `label: String` — a client-supplied identifier, surfaced on the `pending` entry of the incremental stream.

The client receives the primary response immediately with all non-deferred fields, plus a `pending` list announcing the deferred fragments. Each deferred fragment is then delivered in its own incremental payload that carries a `completed` marker correlated back by `id`. `hasNext: true` signals more payloads are coming; the payload that delivers the last outstanding fragment writes `hasNext: false`.

```jsonc
// initial response — announces the deferred fragment as pending
{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}

// incremental payload — delivers it, correlated by id, and marks it completed
{"incremental":[{"data":{"expensiveField":"..."},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
```

> **Spec note:** the wire format follows the `pending`/`incremental`/`completed` model with opaque IDs from the current incremental-delivery spec draft. (An earlier iteration of this engine used a `path`-keyed `incremental` shape without `pending`/`completed`; that is no longer the case.)

---

## High-Level Pipeline

A query with `@defer` travels through four phases, each producing a richer representation for the next. Defer IDs are **sequential integers** assigned in document order; ID `0` means "not deferred".

### 1 · Normalization
- Every `@defer` on a fragment is removed and each field in the fragment is stamped with `@__defer_internal(id, parentDeferId, label)`.
- A `__internal_typename` placeholder is injected into any selection set whose children are *all* deferred, so the downstream subgraph query is never empty.
- Two follow-up passes realign `__typename` defer scope and repair `parentDeferId` after field merging.

### 2 · Planning
- Fields are mapped to datasources; `@key`/`@requires` fields are injected in the correct defer scope.
- `ProcessDefer` propagates each deferred field's ID up to the nearest root anchor (root query node or entity-with-key node).
- The path builder plans each field in one of three modes (deferred field, defer parent, normal) and produces planners keyed by `(datasource, deferID)`.
- `assignDefer` stamps `resolve.Field.Defer`; `configureFetch` stamps `FetchDependencies.DeferID`. If any fetch carries a non-zero deferID, a `DeferResponsePlan` is produced; the `DeferDescriptors` map is populated here.

### 3 · Post-Processing
- The flat fetch tree is deduplicated, templated, and typed, then partitioned by `FetchDependencies.DeferID` into the primary tree and one `DeferFetchGroup` per ID.
- Each tree is organised (dependency-ordered, parallelised) independently.
- `buildDeferTree` turns `DeferDescriptors` (parent/child relationships) into a `DeferTreeNode` execution tree of Single/Sequence/Parallel nodes.

### 4 · Execution
- Primary fetches run, the initial response is rendered (deferred fields skipped) with its `pending` list, and flushed.
- The `DeferTree` is walked: parallel branches run concurrently, sequence branches run parent-before-child. Each group fetches off-lock, then merges + renders its incremental payload under a shared lock and flushes it.

---

## Phase 1: Normalization

**Files:**
- `v2/pkg/astnormalization/defer_expand_into_internal.go`
- `v2/pkg/astnormalization/defer_align_typename_scope.go`
- `v2/pkg/astnormalization/defer_populate_parent_ids.go`
- `v2/pkg/astnormalization/defer_ensure_typename.go`
- `v2/pkg/astnormalization/astnormalization.go` (opt-in via `WithEnableDefer()`)

Normalization is enabled with `WithEnableDefer()` (sets `options.enableDefer`). Without it the `inlineDefer` walker is registered in a *disabled* state (see below) and `@defer` is effectively stripped to a no-op, so the rest of the pipeline treats the query as synchronous. The three follow-up stages additionally carry a `skipCondition` of `!inlineDeferVisitor.hasDefers()`, so they cost nothing when the document contains no `@defer`.

Throughout, the internal directive is `@__defer_internal` (`literal.DEFER_INTERNAL`) and all defer IDs are **integers** assigned sequentially from `1`.

### Stage 1 — Defer expansion (`deferExpandIntoInternal`)

Converts user-facing `@defer` on a fragment into a per-field `@__defer_internal` directive the planner consumes directly. It handles inline fragments (`... on User @defer`, `... @defer`) and named fragment spreads (`...Frag @defer`).

For each `@defer` fragment it:
1. Evaluates `@defer(if: ...)` via `GetBooleanValue` (handles a literal `true`/`false` **and** a variable). Disabled when `if` is `false` or unresolvable.
2. Removes `@defer` from the fragment node.
3. Assigns the next sequential integer ID and records the enclosing defer's ID as `parentDeferId` (0 at top level).
4. Stamps every field in the selection set with `@__defer_internal(id, parentDeferId?, label?)`.

**Soft-disable:** when the fragment is disabled — `@defer(if:false)`, or the visitor's `disable` flag (`!enableDefer`), or an `ignore` scope — the directive is still *removed* from the fragment but no fields are stamped, so the selection resolves inline as an ordinary (non-deferred) query.

After expansion `... @defer { title }` becomes `... { title @__defer_internal(id: 1) }`, and a nested defer:

```graphql
... {
  profile @__defer_internal(id: 1) {
    ... { bio @__defer_internal(id: 2, parentDeferId: 1) }
  }
}
```

**Why stamp individual fields rather than keep `@defer` on the fragment?** Field merging. A field can occur both inside a `@defer` fragment and outside it in the same selection set. Stamping `@__defer_internal` per field lets `MergeFieldsDefer` (`ast_field.go`) compare the two copies directly: if a non-deferred copy exists, it wins and the deferred annotation is discarded, so the field lands in the primary response.

### Stage 2 — Typename placeholder (`deferEnsureTypename`, runs in cleanup)

After expansion a selection set can have *all* its children deferred — none would appear in the primary response, leaving an empty selection set that is an invalid subgraph query. To keep the object materialisable (it must appear as `{}` in the initial response), this stage injects a `__typename` field aliased to **`__internal_typename`** (`literal.INTERNAL_TYPENAME`). The alias marks it as engine-internal so the planner excludes it from the client-visible response shape.

Placement depends on the enclosing field's defer scope:
- Enclosing field **not deferred** → a plain placeholder (lands in the primary fetch).
- Enclosing field **deferred**, no child shares its defer ID → placeholder stamped with the parent's defer ID, so it is fetched in the parent's scope.
- Enclosing field **deferred**, a child already shares its defer ID → no placeholder needed.

### Stage 3 — Align typename scope (`deferAlignTypenameScope`)

A `__typename` is a meta-field available exactly when its enclosing object is materialised, so it must belong to that object's defer scope — not the innermost defer it was textually written under. This pass re-stamps `__typename`'s `@__defer_internal` to match its enclosing object: removes the directive when the object is not deferred, leaves it when already aligned, and re-stamps it with the enclosing object's ID otherwise. It runs after field deduplication (so the enclosing field's final defer ID is known) and before Stage 4.

### Stage 4 — Repair parent IDs (`deferPopulateParentIds`)

Finalises every field's `parentDeferId` after field merging. A deferred field must be parented to the nearest enclosing deferred object so the resolver can traverse into it; field merging can remove a defer directive from an ancestor and invalidate a recorded `parentDeferId`. A document pre-scan collects the live defer IDs, then per field this pass: adds a missing parent from the current defer stack, keeps a still-valid parent, or repairs a stale one to the nearest enclosing deferred ancestor (removing it if none remains).

---

## Phase 2: Planning

**Files:**
- `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go`
- `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`
- `v2/pkg/engine/plan/defer_info_collector.go`
- `v2/pkg/engine/plan/visitor.go`

Planning decides which datasource fetches each field, in which defer scope, and builds one planner per `(datasource, deferID)` pair.

### Defer context on node suggestions

Collecting nodes attaches defer context to each `NodeSuggestion`:

```go
type DeferInfo struct {
    ID       int
    Label    string
    ParentID int
}
```

- `deferInfo *DeferInfo` — set when the field itself is deferred (read from its `@__defer_internal`).
- `descendantDeferIDs []int` — defer IDs of deferred descendants that route through this node; populated by `ProcessDefer`.

### Planners scoped by deferID

A planner is identified by its datasource **and** its deferID. Path assignment (`path_builder_visitor.go`) enforces the separation:

```go
if plannerConfig.DeferID() != 0 && field.deferID == 0 { continue } // deferred planner rejects non-deferred field
if field.deferID != 0 && plannerConfig.DeferID() != field.deferID { continue } // field only joins its matching planner
```

Without this scoping a deferred field could be claimed by a non-deferred planner serving the same datasource and path, producing a primary-scope fetch instead of a deferred one. A single deferID can produce **multiple** planners when its fields are reachable from different root anchors (e.g. one from the root query node, another from an entity node elsewhere in the tree).

### `ProcessDefer` — propagate defer parents

`ProcessDefer` (`datasource_filter_node_suggestions.go`) runs once over the selected suggestions. For each deferred field, `propagateDeferParentsUpToRootNode` walks ancestors **on the same datasource** up to the nearest root anchor — a root query node, or an entity node that requires a key (where an `_entities` fetch can branch) — adding the field's ID to each ancestor's `descendantDeferIDs`. Child nodes that cannot start a fetch on their own thus become **defer parents** that anchor the deferred fetch at a real root. Mutations are applied in a second pass so the walk never observes IDs it just wrote.

### Required fields (`@key`, `@requires`)

`required_fields_visitor.go` injects fields needed for entity resolution and computed fields, stamping them with `@__defer_internal` in the correct scope (`applyDeferInternalDirective`):
- **`@requires`** fields use the requesting field's own defer ID (`deferInfo.ID`) — they must arrive in the same deferred fetch.
- **`@key`** fields use the *parent* defer ID (`parentFieldDeferID`), or are left in primary scope when there is no enclosing defer — the key must be available *before* the deferred entity fetch runs, so it cannot be deferred to the same scope. A pre-existing plain copy of the key is reused. (`ast.Document.FieldInternalDeferID` reads the ID back from the directive.)

All injected fields are recorded in `skipFieldRefs` so they never appear in the client response shape.

### Path builder — three modes

For each field the path builder plans one of:

1. **Deferred field** (`deferInfo != nil`): planned under its own deferID on a planner with a matching `(datasource, deferID)` pair, creating one if none exists.
2. **Defer parent** (`descendantDeferIDs` non-empty): planned once per descendant deferID, each as a *non-deferred* path on the planner that owns that descendant — anchoring the deferred fetch at the correct root and extending the query down to the deferred fields.
3. **Normal field** (neither): planned once on the primary-scope planner.

A field can be modes 1 **and** 2 at once: deferred under its own group while also anchoring fields deferred under other groups.

### Emit the plan (`visitor.go`)

- **`assignDefer`** sets `resolve.Field.Defer = &resolve.DeferField{DeferID: ...}` when the field's path configuration is `deferredField`. This is the signal the renderer uses to classify fields at execution time.
- **`configureFetch`** writes the fetch's deferID onto `FetchDependencies.DeferID` — the signal post-processing uses to partition the fetch tree.
- If any planner has a non-zero `DeferID()`, the plan is a `DeferResponsePlan`; otherwise `SynchronousResponsePlan` (or `SubscriptionResponsePlan`).
- The `DeferDescriptors` map is assembled by `deferInfoCollector` during operation preparation: for each `@__defer_internal` field it records `DeferDescriptor{ID, ParentID, Label, Path}` (path = where the fragment is mounted), keyed by ID.

---

## Phase 3: Post-Processing

**Files:**
- `v2/pkg/engine/postprocess/postprocess.go`
- `v2/pkg/engine/postprocess/extract_defer_fetches.go`
- `v2/pkg/engine/postprocess/build_defer_tree.go`

For a `DeferResponsePlan` the steps run in this order:

1. **Merge fields** (`mergeFields`) — merge duplicate field nodes in the response object tree.
2. **Build flat fetch tree** (`createFetchTree`) — promote the planners' raw fetches into a flat sequence (one root, one child per fetch), regardless of deferID.
3. **Process the flat tree** (`processFlatFetchTree`) — deduplicate, assign fetch IDs, resolve input templates (substitute entity-representation variables with references to fetched data), add missing nested dependencies, and create concrete fetch types.
4. **Extract deferred fetches** (`extractDeferFetches`) — partition the flat tree by `FetchDependencies.DeferID`: ID `0` stays in the primary tree; each non-zero ID is grouped (in sorted ID order) into a `DeferFetchGroup{DeferID, Fetches}` appended to `GraphQLDeferResponse.Defers`.
5. **Organise fetch trees** (`organizeFetchTree`) — run `orderSequenceByDependencies` + `createParallelNodes` **independently** on the primary tree and on each group's tree, so each is a self-contained, dependency-ordered, parallelised unit.
6. **Build the defer tree** (`buildDeferTree`) — turn `DeferDescriptors` into the `DeferTree` execution structure (below), then clear the now-redundant `Defers` slice (the groups are referenced from the tree's Single nodes).

The split in step 4 must happen after dedup/templating (which need the full flat list) but before step 5 (which orders primary and deferred trees separately).

### Building the defer tree (`buildDeferTree`)

The execution tree mirrors the parent/child structure of the `@defer` fragments:

1. Group each `DeferFetchGroup` by its descriptor's `ParentID` (`childrenOf[parentID]`), sorting siblings by ID for a deterministic shape.
2. Roots are the groups with `ParentID == 0`.
3. `buildChain` recursively builds each subtree:
   - A group with no children → a `Single` node wrapping it.
   - A group with children → `Sequence(Single(group), subtree)`, where `subtree` is the single child's chain or a `Parallel` of multiple children's chains.
4. A single root yields that root's chain; multiple independent roots are wrapped in a top-level `Parallel`.

So **Sequence** encodes a parent→child dependency (the parent's anchor must exist before its nested children can be delivered) and **Parallel** encodes independence (siblings, or independent top-level defers, run concurrently).

### Cross-group dependencies

`organizeFetchTree` resolves dependencies *within* each group. Dependencies *between* groups — e.g. a deferred entity fetch needing a `@key` produced by the primary response — are satisfied structurally: the primary tree always completes before any deferred group runs, and the `DeferTree` sequences a child group after its parent. `FetchDependencies.DependsOnFetchIDs` is used for ordering within a tree and otherwise serves as query-plan metadata.

### Feature flags

`DisableExtractDeferFetches()` skips step 4 and `DisableBuildDeferTree()` skips step 6 — both useful for testing earlier stages in isolation.

---

## Phase 4: Execution

**Files:**
- `v2/pkg/engine/resolve/resolve.go` — `ResolveGraphQLDeferResponse`, `resolveDeferTree`, `resolveDeferSingle`
- `v2/pkg/engine/resolve/resolvable.go` — `ResolveDeferBatch`, `ResolveDeferError`, anchor-survival helpers
- `v2/pkg/engine/resolve/defer_tree.go` — `DeferTreeNode`, `pruneDeadDefers`
- `v2/pkg/engine/resolve/data_buffer.go` — `DataBuffer`

### `ResolveGraphQLDeferResponse`

The streaming entry point. Unlike `ResolveGraphQLResponse` it emits multiple frames over time:

1. Acquire one arena from the pool; build the `Resolvable` and the primary `Loader` on it, wrapping the base tree in a `DataBuffer`.
2. Fetch the primary tree (`response.Response.Fetches`) into the `DataBuffer`.
3. Render the initial response with `deferMode=true` and no current defer: deferred fields (`Field.Defer != nil`) are skipped, and the **surviving** top-level defers are announced as `pending`. Write `hasNext:true` and **flush**.
4. Register `writer.Complete()` only *after* the first successful flush — a pre-flush error can still return cleanly to the caller for top-level formatting; once the initial frame is on the wire every exit must terminate the stream.
5. Prune dead top-level defers (`pruneDeadDefers(DeferTree, liveTop)`), seed `outstanding := len(liveTop)`, and walk the tree.

### Anchor-survival gating

A `@defer` fragment only deserves delivery if the object it is mounted on survived the initial render. `deferAnchorAlive(path)` checks whether `r.data.Get(path...)` is non-null — the validation walk sets a nullable object to `null` when a non-null child null-propagated, so a dead anchor reads back as null/absent. `liveChildDescriptors(parentID)` returns the descriptors whose anchor survived. Dead top-level defers are pruned before scheduling; dead nested children are never announced.

### Walking the tree (`resolveDeferTree`)

- **Single** → `resolveDeferSingle`.
- **Sequence** → resolve the parent (`ChildNodes[0]`), then schedule only the children it announced as live (`pruneDeadDefers(child, liveChildren)`); dead children are cancelled.
- **Parallel** → spawn each child on a plain `errgroup.Group` (not `WithContext`): a failed group must not cancel its siblings, so the group's error is surfaced only if the client context was cancelled (disconnect). Sibling fetch I/O overlaps.

### Resolving one group (`resolveDeferSingle`)

Each group gets its **own** `Loader`, but all loaders (and the resolvable) share **one arena** and the **one** `DataBuffer`. Safety comes from the lock discipline, not from per-group isolation:

- **Fetch phase** runs *outside* `dc.db.Lock()` so sibling fetches overlap. The network load writes raw bytes; it allocates nothing from the arena.
- **Merge + render + flush** run *under* `dc.db.Lock()`. The loader parses and merges into the shared tree, the resolvable renders the incremental frame, and the frame is flushed — all while holding the lock, so concurrent groups cannot interleave their frames or their arena allocations.

A hard fetch-phase error (e.g. a pre-fetch authorizer/rate-limiter error) takes the lock and calls `ResolveDeferError`, which completes the announced pending with the error and terminates.

### Incremental rendering (`ResolveDeferBatch`)

One frame per group, rendered in **two passes** over the object tree (the HTTP stream is already open, so the frame's shape must be decided before the first byte):

- **Pass 1 — pre-walk** (`enableRender=false`): no bytes written. `collectDeferFields` classifies each object's fields — `Defer.DeferID == currentDefer.ID` → render set; no `Defer`, or a smaller ID → seek set (traverse into outer/earlier objects to reach matching fields); a larger ID → skipped (not yet fetched). Authorization runs and null-bubbling is detected; either sets `deferItemDataNull`, meaning there is no deliverable data.
- **Pass 2 — render** (`enableRender=true`): the deferred fields are rendered into an arena-backed **scratch buffer**, so a render-phase error discards the partial frame and is scoped to this defer's `completed.errors` instead of leaving a torn frame on the wire.

The frame is then assembled:
- `incremental:[{data, id [,subPath] [,errors]}]` — only when there is deliverable data. `subPath` is the runtime path minus the descriptor path (list indices and any deeper segments); recoverable subgraph errors ride in the item's `errors`.
- `completed:[{id [,errors]}]` — always; errors are attached here only when the fragment had no deliverable data (null-bubble / auth / render failure).
- `pending:[...]` — the surviving direct children, announced **lazily** in their parent's release frame.
- `hasNext` — driven by the `outstanding` counter.

### The `outstanding` counter

`outstanding` is the count of announced-but-not-completed defers. It starts at the live top-level count; each frame adjusts it by `len(liveChildren) - 1` (announce this defer's live children, complete this defer). The frame that drives it to `0` writes `hasNext:false`. Because every frame's counter mutation and writes happen under `dc.db.Lock()`, a plain `int64` is sufficient — no atomics.

### `DeferResponseWriter`

```go
type DeferResponseWriter interface {
    ResponseWriter // io.Writer
    Flush() error
    Complete()
}
```

`Flush()` commits the current frame to the client; each group produces exactly one `Flush()`. `Complete()` closes the stream.

---

## Key Data Structures

### `GraphQLDeferResponse` (resolve/response.go)

```go
type GraphQLDeferResponse struct {
    Response         *GraphQLResponse          // primary (non-deferred) fields
    Defers           []*DeferFetchGroup        // post-processing intermediate; nil after buildDeferTree
    DeferDescriptors map[int]DeferDescriptor   // every @defer fragment, keyed by ID
    DeferTree        *DeferTreeNode            // execution tree; nil until buildDeferTree runs
}

type DeferDescriptor struct {
    ID       int      // valid IDs start at 1
    ParentID int      // enclosing @defer ID (0 for top-level)
    Label    string   // user-supplied label ("" when none)
    Path     []string // response path where the fragment is mounted
}

type DeferFetchGroup struct {
    DeferID int
    Fetches *FetchTreeNode
}
```

### `DeferTreeNode` (resolve/defer_tree.go)

```go
type DeferTreeNode struct {
    Kind       DeferTreeNodeKind // Single | Sequence | Parallel
    Item       *DeferFetchGroup  // set only for Single
    ChildNodes []*DeferTreeNode  // children for Sequence/Parallel
}
```

`Sequence(Single(parent), subtree)` runs the parent before its children; `Parallel` runs independent branches concurrently.

### `DeferField` (resolve/node_object.go)

```go
type DeferField struct { DeferID int }
```

Attached to a `Field` in the response object tree. The renderer skips fields with a non-zero `DeferField.DeferID` in the primary pass and emits them only in the matching defer's incremental pass.

### `FetchDependencies.DeferID` (resolve/fetch.go)

```go
type FetchDependencies struct {
    FetchID           int
    DependsOnFetchIDs []int
    DeferID           int // non-zero → belongs to a deferred group
}
```

---

## Wire Format

`Content-Type: multipart/mixed; deferSpec=20220824`. IDs are rendered as JSON strings (e.g. `"1"`) though they are integers internally.

**Initial response** — all non-deferred fields, plus a `pending` entry per surviving deferred fragment (`path` = where it is mounted, optional `label`).

```json
{"data":{"user":{"id":"1","name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
```

**Incremental — with data** — one per delivered `@defer` group. The item carries the deferred `data`, its `id`, an optional `subPath` (list indices / deeper segments relative to the descriptor path), and optional recoverable `errors`. A `completed` entry marks the fragment done; surviving nested children are announced as `pending` in the same frame.

```json
{"incremental":[{"data":{"expensiveField":"..."},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
```

**Incremental — no deliverable data** — when the fragment null-bubbled, failed authorization, or failed during render, no `incremental` item is emitted; the error is attached to the `completed` entry.

```json
{"completed":[{"id":"1","errors":[{"message":"..."}]}],"hasNext":true}
```

- `hasNext: true` on every frame except the one that completes the last outstanding defer.
- `hasNext: false` on that final frame, ending the stream.

---

## Concurrency & Memory

- **One shared response tree.** Every fetch (primary and deferred) merges into a single `DataBuffer` — the accumulated document plus the mutex guarding it. The lock is held across a group's whole merge → render → flush region so concurrent groups never interleave frames.
- **Parallel delivery.** Independent defers (Parallel nodes) run concurrently via `errgroup`; their fetches overlap off-lock, and only the merge/render/flush is serialised.
- **One shared arena.** The resolvable, the primary loader, and every group's loader allocate from a single pooled arena, released when `ResolveGraphQLDeferResponse` returns. This is safe despite concurrency because **every arena allocation happens under the `DataBuffer` lock** (the loader allocates only in its prepare/merge phases, the off-lock network phase touches no arena, and the resolvable's defer renders run under the lock). See `data_buffer.go` for the locking contract.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| **`@defer` → per-field `@__defer_internal`** | Enables field merging: `MergeFieldsDefer` compares deferred and non-deferred copies of a field directly and discards the deferred annotation when a non-deferred copy exists. Fragment-level `@defer` would make this far harder. |
| **Sequential integer defer IDs** | Assigned in document order; lets post-processing and the `pending` list order groups deterministically. `0` is the natural "not deferred" sentinel. |
| **`__internal_typename` placeholder** | When all children of a selection set are deferred, the parent must still appear as `{}` in the primary response. The placeholder keeps the subgraph query valid; the internal alias excludes it from the client response shape. |
| **Separate align/repair normalization passes** | Field merging and `__typename`'s object-scoped availability invalidate naive per-field defer scoping. `deferAlignTypenameScope` and `deferPopulateParentIds` run after merge to realign typename scope and repair `parentDeferId`. |
| **Planners scoped by `(datasource, deferID)`** | Prevents a non-deferred planner from claiming a deferred field on the same datasource/path; each defer scope generates its query independently. |
| **Fetch split in post-processing, after dedup** | Dedup and template resolution need the full flat fetch list; the split must precede per-tree organisation. |
| **`DeferTree` of Single/Sequence/Parallel** | Encodes parent→child ordering (a child defer can only be delivered once its parent's anchor exists) and sibling independence (parallel delivery), derived from `DeferDescriptor.ParentID`. |
| **Anchor-survival gating** | A defer whose mount point null-propagated has nothing to deliver, so it is pruned (top-level) or never announced (nested) rather than delivered empty. |
| **Lazy nested announcement** | A nested defer is announced as `pending` only in its parent's release frame — its survival is unknown until the parent renders, so eager announcement could promise a fragment that never arrives. |
| **Two-pass incremental render + scratch buffer** | The frame shape (data vs. error-only) must be decided before any byte is written to the open stream; rendering into a scratch buffer lets a render error be redirected to `completed.errors` without a torn frame. |
| **Shared arena under the `DataBuffer` lock** | Gives deferred delivery the same arena benefits as the synchronous path without per-group arenas, because all arena allocation is already serialised by the lock. |
| **`@defer` is query-only** | Mutations require serial field execution; subscriptions already stream. Neither is compatible with incremental delivery. |

---

## Configuration & Feature Flags

| Option | Location | Purpose |
|--------|----------|---------|
| `WithEnableDefer()` | `astnormalization/astnormalization.go` | Enables defer normalization; without it `@defer` is stripped to a no-op. |
| `DisableExtractDeferFetches()` | `postprocess/postprocess.go` | Skips fetch-tree partitioning (test the planner in isolation). |
| `DisableBuildDeferTree()` | `postprocess/postprocess.go` | Skips building the `DeferTree` (test extraction in isolation). |

---

## Known Limitations / TODOs

- **Mutations and subscriptions are not supported** — only queries produce a `DeferResponsePlan`.
- **`@defer(label:)`** is surfaced on `pending` entries but is not otherwise used for correlation beyond the numeric ID.

---

## File Reference

| Area | File |
|------|------|
| Defer directive constants | `v2/pkg/lexer/literal/literal.go` |
| Field AST helpers (merge, stamp, read deferID) | `v2/pkg/ast/ast_field.go` |
| Defer expansion (normalization) | `v2/pkg/astnormalization/defer_expand_into_internal.go` |
| Align typename scope (normalization) | `v2/pkg/astnormalization/defer_align_typename_scope.go` |
| Repair parent IDs (normalization) | `v2/pkg/astnormalization/defer_populate_parent_ids.go` |
| Typename placeholder (normalization) | `v2/pkg/astnormalization/defer_ensure_typename.go` |
| Collect nodes visitor | `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go` |
| NodeSuggestions + ProcessDefer | `v2/pkg/engine/plan/datasource_filter_node_suggestions.go` |
| Node selection + skipFieldRefs | `v2/pkg/engine/plan/node_selection_visitor.go` |
| Required fields visitor (defer scope) | `v2/pkg/engine/plan/required_fields_visitor.go` |
| Path builder (three planning modes) | `v2/pkg/engine/plan/path_builder_visitor.go` |
| Defer descriptor collection | `v2/pkg/engine/plan/defer_info_collector.go` |
| assignDefer + configureFetch + plan type | `v2/pkg/engine/plan/visitor.go` |
| Plan type definitions | `v2/pkg/engine/plan/plan.go` |
| Post-processing pipeline | `v2/pkg/engine/postprocess/postprocess.go` |
| Extract deferred fetches | `v2/pkg/engine/postprocess/extract_defer_fetches.go` |
| Build defer execution tree | `v2/pkg/engine/postprocess/build_defer_tree.go` |
| Response types (GraphQLDeferResponse, DeferDescriptor, DeferFetchGroup) | `v2/pkg/engine/resolve/response.go` |
| Defer execution tree (DeferTreeNode, pruneDeadDefers) | `v2/pkg/engine/resolve/defer_tree.go` |
| Field defer annotation (DeferField) | `v2/pkg/engine/resolve/node_object.go` |
| Fetch dependencies (DeferID) | `v2/pkg/engine/resolve/fetch.go` |
| Shared response buffer + lock | `v2/pkg/engine/resolve/data_buffer.go` |
| Execution entry point | `v2/pkg/engine/resolve/resolve.go` |
| Incremental rendering (ResolveDeferBatch, collectDeferFields) | `v2/pkg/engine/resolve/resolvable.go` |
| Integration tests (planner) | `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go` |
| Integration tests (engine) | `execution/engine/execution_engine_defer_test.go` |
| Normalization tests | `v2/pkg/astnormalization/defer_expand_into_internal_test.go` |
| Typename placeholder tests | `v2/pkg/astnormalization/defer_ensure_typename_test.go` |
| Required fields defer tests | `v2/pkg/engine/plan/required_fields_visitor_test.go` |
