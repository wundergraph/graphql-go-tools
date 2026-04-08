# GraphQL `@defer` — Design Document

## Overview

The `@defer` directive allows clients to mark fragments — both inline fragments and named fragment spreads — whose fields should be delivered as separate incremental payloads rather than blocking the primary response. It is only valid on queries; mutations and subscriptions cannot support incremental delivery by design.

```graphql
# anonymous inline fragment
query {
  user(id: "1") {
    name
    ... @defer {
      expensiveField
    }
  }
}

# inline fragment with type condition
query {
  user(id: "1") {
    name
    ... on User @defer {
      expensiveField
    }
  }
}

# named fragment spread
fragment UserDetails on User {
  expensiveField
}

query {
  user(id: "1") {
    name
    ...UserDetails @defer
  }
}
```

The directive accepts two optional arguments:
- `if: Boolean` — when `false`, the fragment is not deferred and its fields appear in the primary response. Defaults to `true`.
- `label: String` — a client-supplied identifier. **Not yet passed through to incremental responses** — this will be documented once implemented.

The client receives the primary response immediately with all non-deferred fields, followed by one incremental chunk per deferred group. `hasNext: true` signals more chunks are coming; `hasNext: false` on the final chunk signals the stream is complete.

```json
// Primary response
{"data": {"user": {"name": "Alice"}}, "hasNext": true}

// Incremental chunk
{"incremental": [{"data": {"expensiveField": "..."}, "path": ["user"]}], "hasNext": false}
```

> **Spec note:** This implementation follows an earlier version of the incremental delivery spec. The current spec draft introduces `pending`/`completed` entries and opaque IDs for correlating chunks — those are not yet implemented.

---

## High-Level Pipeline

A query with `@defer` travels through four phases, each producing a richer representation for the next.

### 1 · Normalization

- `@defer` is removed from every fragment (inline or named spread)
- Every field inside is stamped with `@__defer_internal(id, parentDeferId, label)`
- defer IDs are assigned sequentially in AST walk order
- A `___typename` placeholder is injected into any selection set where all children are deferred

### 2 · Planning

- Fields are mapped to one or more datasources; required fields (`@key`, `@requires`) are added to the operation in the correct defer scope
- `ProcessDefer` propagates deferIDs up through parent nodes to identify root anchor nodes
- The path builder plans each field in one of three modes (deferred field, defer parent, or normal)
- `assignDefer` stamps `resolve.Field.Defer` on each deferred field in the response object tree — consumed by the renderer to classify fields at render time
- `configureFetch` writes the deferID onto `FetchDependencies` of each `SingleFetch` — consumed by post-processing to partition the fetch tree
- If any fetch carries a deferID, a `DeferResponsePlan` is produced; otherwise a `SynchronousResponsePlan`

### 3 · Post-Processing

- Fields are merged and the fetch tree is built and ordered by dependency
- The fetch tree is partitioned by `FetchDependencies.DeferID`: empty deferID goes into the primary fetch group; each non-empty deferID forms its own `DeferFetchGroup`
- Groups are sorted numerically, preserving AST definition order

### 4 · Execution

- Primary fetches are executed and the initial response is rendered (deferred fields skipped) and flushed to the client
- For each `DeferFetchGroup` in order:
  - Deferred fetches are executed
  - A two-pass render runs: pre-walk validates auth and detects null-bubbling, then the render pass emits the incremental chunk
  - The chunk is flushed immediately


---

## Phase 1: Normalization

**Files:**
- `v2/pkg/astnormalization/inline_fragment_expand_defer.go`
- `v2/pkg/astnormalization/defer_ensure_typename.go`
- `v2/pkg/astnormalization/astnormalization.go` (opt-in via `WithInlineDefer()`)

Normalization is enabled by passing `WithInlineDefer()` to the normalizer. Without it, `@defer` is left untouched and the rest of the pipeline treats the query as a normal synchronous request.

### Defer Expansion (`inlineFragmentExpandDefer`)

This visitor converts user-facing `@defer` on fragments into a per-field internal directive the planner can consume directly. It handles both inline fragments (`... on User @defer { ... }`, `... @defer { ... }`) and named fragment spreads (`...MyFragment @defer`).

**What it does:**

When it encounters `@defer` on a fragment:

1. Checks `@defer(if: false)` — if disabled, removes `@defer` from the fragment but does not stamp any fields (they are treated as non-deferred).
2. Removes `@defer` from the fragment node itself.
3. Assigns a sequential integer ID to this defer group. IDs are assigned in AST walk order, so they reflect the order in which `@defer` fragments appear in the document.
4. Records `parentDeferId` pointing to the enclosing defer group's ID, if any (for nested `@defer`).
5. Stamps every field in the selection set with `@__defer_internal(id: "N", parentDeferId: "M", label: "...")`.

After expansion a fragment like `... @defer { title }` becomes:

```graphql
... {
  title @__defer_internal(id: "1")
}
```

And a nested defer like `... @defer { profile { ... @defer { bio } } }` becomes:

```graphql
... {
  profile @__defer_internal(id: "1") {
    ... {
      bio @__defer_internal(id: "2", parentDeferId: "1")
    }
  }
}
```

**Why stamp individual fields rather than keeping `@defer` on the fragment?**

The primary motivation is **field merging**. GraphQL query can have duplicate field occurrences — for example, the same field may appear both inside a `@defer` fragment and outside it in the same selection set. By stamping `@__defer_internal` on individual fields, the merge step (`MergeFieldsDefer` in `ast_field.go`) can compare them directly: if a non-deferred version of a field exists alongside a deferred version, the non-deferred version wins and the deferred annotation is discarded. The field ends up in the primary response. If `@defer` remained on the fragment, this field-level merge would be much harder to reason about.

**Why sequential integer IDs?**

IDs are assigned in AST walk order, which matches document definition order. Post-processing sorts defer groups numerically, so incremental chunks are always streamed to the client in the order the `@defer` fragments appear in the query.

### Typename Placeholder (`deferEnsureTypename`)

After defer expansion, a field's selection set can end up with *all* of its child fields carrying `@__defer_internal`. This means all children are deferred — none of them will appear in the primary response. The client must still receive the parent object as an empty `{}` in the initial response so it knows the object exists and where deferred data will be inserted later. To produce that empty object the planner must send a query to the subgraph that selects *something* from it — otherwise the selection set is invalid.

To solve this, `deferEnsureTypename` injects a `___typename` placeholder (triple-underscore alias) into any selection set where all fields are deferred. The triple-underscore alias distinguishes it from a user-requested `__typename`. The `nodeSelectionVisitor` adds it to `skipFieldRefs` so it never appears in the response shape seen by the client — it exists purely to keep the downstream query valid.

The placeholder is placed in the correct defer scope depending on context:

- If the enclosing parent field is **not deferred**: a plain `___typename` with no `@__defer_internal` annotation is added. It lands in the primary fetch.
- If the enclosing parent field **is deferred** and no child shares the parent's defer ID: `___typename` is annotated with the parent's `@__defer_internal` ID so it is fetched in the parent's defer scope, not the children's scope.
- If at least one child already shares the parent's defer ID: no placeholder is needed — that child is effectively "in scope" for the parent's fetch.

---

## Phase 2: Planning

**Files:**
- `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go`
- `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`
- `v2/pkg/engine/plan/visitor.go`

Planning is the most involved phase for defer. Its job is to determine which datasource fetches each field, in which defer scope, and to build a set of planner instances — one per `(datasource, deferID)` pair — that will generate the downstream queries.

### Why planners are scoped by deferID

A planner is identified by its datasource hash **and** its deferID. Two planners can share the same datasource but serve different defer scopes. This separation is enforced during path assignment:

- A planner whose `DeferID` is non-empty refuses to accept non-deferred fields.
- A field with a `deferID` is only accepted by a planner whose `DeferID` matches exactly.

Without this scoping, a deferred field could be picked up by a non-deferred planner that happens to serve the same datasource and path — producing a primary-scope fetch instead of a deferred one.

A single deferID can produce **multiple** planners if the deferred fields are reachable from different root anchors in the query tree — for example, one starting from the root query node and another starting from an entity node in a different part of the tree.

### Step 1 — Collect nodes (`datasource_filter_collect_nodes_visitor.go`)

Builds a `NodeSuggestion` tree that maps every field to one or more candidate datasources. For each field it reads `@__defer_internal` and attaches a `DeferInfo` struct (`id`, `parentDeferId`, `label`) to the suggestion, making the defer context of every field available to all subsequent steps.

### Step 2 — Node selection and required fields (`node_selection_visitor.go`, `required_fields_visitor.go`)

Resolves which datasource(s) handle each field. Also detects fields that require additional data to be fetched — `@key` fields for entity resolution and `@requires` fields for computed fields — and injects them directly into the operation AST in the correct defer scope.

**`@requires` fields** are stamped with the same `@__defer_internal` as the field that needs them. They must be present in the same deferred fetch so the field resolver has the data it depends on.

**`@key` fields** are placed in the *parent* defer scope, or left plain (primary scope) if there is no enclosing defer. The key must already be available before the entity fetch runs, so it cannot be deferred to the same scope as the field that depends on it. When a plain (non-deferred) copy of the key already exists in the selection set, it is reused directly — no annotation needed.

All injected fields are recorded in `skipFieldRefs` so they never appear in the client response shape.

### Step 3 — Propagate defer parents (`datasource_filter_node_suggestions.go` — `ProcessDefer`)

After node selection, `ProcessDefer` runs once over all selected suggestions. For every deferred field it walks up the `NodeSuggestion` tree through ancestors **on the same datasource**, searching for the nearest root anchor:

- A **root query node** (e.g. `Query.user`) — a natural starting point for a full query.
- An **entity node that requires a key to be provided** — meaning an entity fetch (`_entities`) will branch from it.

Child nodes (fields that are neither root query fields nor entity fields with a key requirement) cannot independently start a fetch. They must always be included as part of an ancestor's query. The propagation therefore walks all the way up to the first ancestor that *can* start a fetch, adding the deferred field's ID to the `deferIDs` list of every node on that path. Those nodes become **defer parents**.

### Step 4 — Path building (`path_builder_visitor.go`)

For each field, the path builder uses the node suggestion results to plan fetch paths. A field is handled in one of three modes:

**1. Deferred field** (`deferField=true`, `deferID=<id>`)

The field carries `@__defer_internal`. It is planned as a deferred path under its own deferID. The path builder looks for an existing planner with a matching `(datasource, deferID)` pair. If none exists, a new planner is created for a new `objectFetchConfiguration` with `deferID` set to the field's ID.

**2. Defer parent** (`deferField=false`, `deferID=<child-id>`)

The field has one or more child deferIDs in its `deferIDs` list from `ProcessDefer`. It is planned **once per child deferID** it covers, each time as a non-deferred path on the planner that owns that child deferID. This anchors the deferred fetch at the correct root node and ensures the planner's generated query contains the full path down to the deferred fields. Without this, the child fields — which cannot start a fetch on their own — would have no root to attach to.

**3. Normal field** (`deferField=false`, `deferID=""`)

No defer involvement. Planned once on the primary-scope planner for its datasource.

A field can be in modes 1 and 2 simultaneously: it may carry its own `deferID` (deferred under one group) while also appearing as a parent anchor for fields deferred under other groups.

### Step 5 — Assign defer annotations and emit the plan (`visitor.go`)

**`assignDefer`** runs for every field in the response object tree. When a field's `pathConfiguration.deferredField` is true, it sets `resolve.Field.Defer = &resolve.DeferField{DeferID: ...}`. This annotation is the signal the **renderer** uses at execution time: fields with `Defer != nil` are skipped during the primary response pass and included only during the incremental pass whose `deferID` matches.

**`configureFetch`** writes `objectFetchConfiguration.deferID` onto `FetchDependencies.DeferID` of the resulting `SingleFetch`. This is the signal **post-processing** uses to partition the fetch tree into primary and deferred groups.

After all planners are built, if any planner exposes a non-empty `DeferID()`, the plan is a `DeferResponsePlan`. Otherwise it is a `SynchronousResponsePlan`.

---

## Phase 3: Post-Processing

**Files:**
- `v2/pkg/engine/postprocess/postprocess.go`
- `v2/pkg/engine/postprocess/extract_defer_fetches.go`

Post-processing takes the raw plan produced by the visitor and turns it into an executable form. For a `DeferResponsePlan` the steps run in this order:

**1. Merge fields** (`mergeFields`)

Merges duplicate field nodes in the response object tree. This can leave behind fields from different query branches that happen to resolve to the same path.

**2. Build flat fetch tree** (`createFetchTree`)

Promotes `RawFetches` from the planner into a flat sequence node — a single root with one child per fetch. At this point all fetches are in one list regardless of their deferID.

**3. Process flat fetch tree** (`processFlatFetchTree`)

Three sub-steps run over the flat list:
- **Resolve input templates** — substitutes variable placeholders in fetch inputs (e.g. entity representation variables) with concrete references to previously fetched data.
- **Deduplication** — removes identical fetches that would otherwise query the same data twice.
- **Create concrete fetch types** — converts generic fetch nodes into concrete typed nodes (single fetch, batch fetch, parallel fetch) based on their shape and dependencies.

**4. Extract deferred fetches** (`extractDeferFetches`)

This is the defer-specific step. The flat fetch tree is partitioned by `FetchDependencies.DeferID`:

- Fetches with an empty `DeferID` stay in the primary response fetch tree.
- Fetches with a non-empty `DeferID` are grouped by ID into `DeferFetchGroup` structs and stored in `GraphQLDeferResponse.Defers`.

The split must happen before step 5 because each group is organised independently. Groups are sorted numerically by ID, preserving AST definition order so chunks stream to the client in the order the `@defer` fragments appear in the query.

**5. Organise fetch trees**

`organizeFetchTree` runs separately on the primary fetch tree and on each `DeferFetchGroup`'s fetch tree. It reorders fetch nodes so that a fetch always executes after all fetches it depends on, and wraps independent fetches in parallel nodes where possible. Each group is organised as a self-contained tree. `DependsOnFetchIDs` is used during this ordering step to sequence fetches correctly within a tree; after organisation it serves only as metadata for query plan display. Cross-group dependencies (e.g. a deferred entity fetch that depends on a key from the primary response) are not re-checked at runtime — they are satisfied structurally because the execution loop always completes the primary response before running any deferred group.

---

## Phase 4: Execution

**Files:**
- `v2/pkg/engine/resolve/resolve.go` (`ResolveGraphQLDeferResponse`, line 439)
- `v2/pkg/engine/resolve/resolvable.go` (`ResolveDefer`, line 266)

### 1. `ResolveGraphQLDeferResponse`

`ResolveGraphQLDeferResponse` is the entry point for defer execution. It differs from the regular `ResolveGraphQLResponse` in that it does not produce a single response — it drives a streaming loop that emits multiple chunks to the client over time.

The loop runs as follows:

1. **Initialise** the resolvable state with the operation type.
2. **Fetch primary data** — the loader executes all fetches in the primary fetch tree (`response.Response.Fetches`), populating the shared JSON data buffer.
3. **Render the primary response** — `resolvable.Resolve` is called with `deferMode=true` and `deferID=""`. In this mode `collectDeferFields` skips all fields where `Field.Defer != nil`, rendering only non-deferred fields. Because `deferMode=true` and no errors occurred, `hasNext: true` is written unconditionally at the end of the primary response.
4. **Flush** — the primary chunk is sent to the client immediately.
5. If any errors occurred during primary rendering, the loop stops.
6. **For each `DeferFetchGroup`** in definition order:
   - The loader executes the group's fetch tree, appending deferred data into the shared buffer alongside the already-fetched primary data.
   - `resolvable.deferID` is set to the group's ID.
   - `ResolveDefer` is called to render the incremental chunk.
   - The chunk is flushed to the client immediately.
   - If errors occurred, the loop stops.
7. **`writer.Complete()`** signals the end of the stream.

The same shared data buffer (`response.Response.Data`) is reused across all passes. Each deferred fetch appends its results into the buffer at the correct paths, so the renderer can find them during the incremental pass.

### 2. `DeferResponseWriter`

```go
type DeferResponseWriter interface {
    io.Writer
    Flush() error
    Complete()
}
```

`Flush()` commits the current chunk to the client. `Complete()` closes the stream. Each group produces exactly one `Flush()` call.

### 3. `ResolveDefer` — Incremental Rendering

`ResolveDefer` generates one incremental chunk for a given `deferID`. Unlike a regular response render, it cannot simply walk the entire object tree — it must find only the fields belonging to the current defer group, which may be scattered at different depths and paths within the tree. It also cannot start writing to the client until it knows there are no authorization errors, since the HTTP stream is already open and partially sent.

For these reasons rendering runs in **two passes** over the same object tree.

#### **Pass 1 — pre-walk** (`enableRender=false`)

The tree is walked without writing any bytes. For each object node, `collectDeferFields` classifies its fields:

- Fields whose `Defer.DeferID` matches `r.deferID` → **render set** (will be written in pass 2)
- Fields with no `Defer` annotation, or whose `Defer.DeferID` is numerically smaller than `r.deferID` → **seek set** (traversed to find nested defer content)
- Fields whose `Defer.DeferID` is numerically larger than `r.deferID` → **skipped** (not yet fetched)

The seek set exists because after normalization the same object can appear in both deferred and non-deferred contexts, producing response tree nodes whose outer object has no deferID but whose nested fields do. The walker must traverse into those outer objects to reach the matching fields inside them. Similarly, an already-completed earlier defer group (smaller ID) may contain nested fields belonging to the current group — the walker seeks into it.

During this pass, authorization checks run and null-bubbling through non-nullable chains is detected. If a non-nullable field fails authorization, `deferItemDataNull` is set, signalling that the entire incremental item for this object must render as `{"data": null, ...}`.

#### **Pass 2 — render** (`enableRender=true`)

The same walk runs again. For each object node that has matching deferred fields, the render pass must decide which incremental item envelope to produce. This decision is made before writing any bytes for that item, based on `deferItemDataNull` set during pass 1.

###### Normal envelope (`deferItemDataNull=false`)

The outer `{"incremental": [` wrapper is opened, then `printDeferEnvelopeOpen` writes `{"data": {`, the deferred fields are rendered inside, then `printDeferEnvelopeClose` appends `}, "path": [...]}`. The result is:

```json
{"incremental": [{"data": {"expensiveField": "..."}, "path": ["user"]}], "hasNext": ...}
```

###### Null envelope (`deferItemDataNull=true`)

`printDeferEnvelopeNullData` writes the entire item as `{"data": null, "path": [...], "errors": [...]}` in one shot — the normal `{"data": {` opener and the outer `{"incremental": [` wrapper are never written. The walker returns immediately without descending further.

This is exactly why the pre-walk is necessary. Each chunk is assembled in an intermediate buffer before being flushed to the client. Once bytes have been written into that buffer you cannot go back and modify them — for example you cannot change `{"data": {field: "value"}` into `{"data": null, ...}` after the fact. The pre-walk ensures the render pass knows which shape to produce before it writes the first byte.

The `"path"` value in both envelopes is taken from `r.path` — the path stack at the moment the object is entered, pointing to the location in the response tree where the client should merge the incremental data.

A single `ResolveDefer` call can produce **multiple incremental items** within the one envelope — one per object node that owns matching deferred fields, found by the seeker as it traverses the tree. Array items also produce separate entries: each element of a list that contains deferred fields gets its own incremental item with an index in its path.

`hasNext: true` is written on all chunks except the last. The last chunk in the loop writes `hasNext: false`.

---

## Key Data Structures

### `GraphQLDeferResponse` (resolve/response.go)

```go
type GraphQLDeferResponse struct {
    Response *GraphQLResponse     // primary (non-deferred) fields
    Defers   []*DeferFetchGroup   // one per @defer group, in order
}

type DeferFetchGroup struct {
    DeferID string
    Fetches *FetchTreeNode
}
```

### `DeferField` (resolve/node_object.go)

```go
type DeferField struct {
    DeferID string
}
```

Attached to `Field` in the response object tree. During rendering, fields with a
non-empty `DeferField.DeferID` are skipped in the primary pass and included only
when `deferID` matches.

### `FetchDependencies.DeferID` (resolve/fetch.go)

```go
type FetchDependencies struct {
    FetchID           int
    DependsOnFetchIDs []int
    DeferID           string   // non-empty → belongs to a deferred group
}
```

---

## Wire Format

**Primary response** — sent immediately, contains all non-deferred fields. `hasNext: true` signals more chunks are coming.

```json
{"data": {"user": {"id": "1", "name": "Alice"}}, "hasNext": true}
```

**Incremental response — normal envelope** — one per `@defer` group when all fields resolved successfully. Each item carries the deferred data and the path where the client should merge it.

```json
{"incremental": [{"data": {"expensiveField": "..."}, "path": ["user"]}], "hasNext": true}
```

**Incremental response — null data envelope** — emitted when a non-nullable field in the deferred group fails (authorization error or null-bubbling). The data is null and errors are included.

```json
{"incremental": [{"data": null, "path": ["user"], "errors": [{"message": "..."}]}], "hasNext": true}
```

- `hasNext: true` on all chunks except the last.
- `hasNext: false` on the final chunk, signalling the stream is complete.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| **`@defer` → `@__defer_internal` stamped on individual fields** | The primary motivation is field merging. Stamping at field level lets `MergeFieldsDefer` compare deferred and non-deferred copies of the same field directly and discard the deferred annotation when a non-deferred counterpart exists. Fragment-level `@defer` would make this merge much harder to reason about. |
| **Sequential integer defer IDs** | IDs are assigned in AST walk order, matching document definition order. This lets post-processing sort groups numerically so chunks stream to the client in the order `@defer` fragments appear in the query. |
| **`parentDeferId` tracking** | Required fields (`@key`, `@requires`) and `___typename` placeholders must land in the correct defer scope. `parentDeferId` lets the required fields visitor determine that scope without re-walking the tree. |
| **`___typename` placeholder** | When all children of a selection set are deferred, the parent object must still appear as `{}` in the primary response so the client knows where to insert deferred data. The placeholder keeps the downstream query valid. The triple-underscore alias ensures it is excluded from the client response shape via `skipFieldRefs`. |
| **Planners scoped by `(datasource, deferID)` pair** | Prevents non-deferred planners from claiming deferred fields that share the same datasource and path. Each defer group gets its own dedicated set of planners so downstream queries are generated in the correct scope. |
| **Fetch tree split happens in post-processing, after deduplication** | Deduplication and template resolution must see the full flat fetch list to work correctly. The split must happen after those steps but before `organizeFetchTree`, which must run independently per group since primary and deferred trees are ordered separately. |
| **Two-pass rendering in `ResolveDefer`** | The pre-walk determines the correct envelope shape before any bytes are written to the intermediate chunk buffer. Two failure cases require the null envelope (`{"data": null, "path": [...], "errors": [...]}`): unauthorized fields (values must not leak) and null-bubbling from non-nullable field errors. Without the pre-walk, the render pass would open the normal `{"data": {` envelope and then be unable to change it once bytes have already been written into the buffer. |
| **Sequential deferred group delivery** | Groups are fetched and flushed one at a time in definition order. Parallel fetch-and-stream is not implemented. |
| **`@defer` is query-only** | Mutations require serial field execution; subscriptions already stream continuously. Neither is compatible with incremental delivery semantics. |

---

## Configuration & Feature Flags

| Option | Location | Purpose |
|--------|----------|---------|
| `WithInlineDefer()` | `astnormalization/astnormalization.go` | Enables defer normalization; without this the `@defer` directive is left untouched. |
| `DisableExtractDeferFetches()` | `postprocess/postprocess.go` | Skips the fetch-splitting step (useful for testing the planner in isolation). |

---

## Known Limitations / TODOs

- **Mutations and subscriptions not supported.** The planner detects `isDefer` and
  creates `DeferResponsePlan` only for queries.
- **Sequential delivery only.** Deferred groups are fetched one after another; there
  is no parallel fetching across groups.
- **`MergeFieldsDefer` TODO** (`ast_field.go:206`): When merging two fields that both
  carry `@__defer_internal`, the merge logic does not yet fully account for `parentId`
  reconciliation.

---

## File Reference

| Area | File |
|------|------|
| Defer directive constants | `v2/pkg/lexer/literal/literal.go` |
| Field AST helpers (merge, stamp, read deferID) | `v2/pkg/ast/ast_field.go` |
| Defer expansion (normalization) | `v2/pkg/astnormalization/inline_fragment_expand_defer.go` |
| Typename placeholder (normalization) | `v2/pkg/astnormalization/defer_ensure_typename.go` |
| Collect nodes visitor | `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go` |
| NodeSuggestions + ProcessDefer | `v2/pkg/engine/plan/datasource_filter_node_suggestions.go` |
| Node selection + skipFieldRefs | `v2/pkg/engine/plan/node_selection_visitor.go` |
| Required fields visitor (defer scope logic) | `v2/pkg/engine/plan/required_fields_visitor.go` |
| Path builder (three planning modes) | `v2/pkg/engine/plan/path_builder_visitor.go` |
| assignDefer + configureFetch + plan type | `v2/pkg/engine/plan/visitor.go` |
| Plan type definitions | `v2/pkg/engine/plan/plan.go` |
| Post-processing pipeline | `v2/pkg/engine/postprocess/postprocess.go` |
| Extract deferred fetches | `v2/pkg/engine/postprocess/extract_defer_fetches.go` |
| Response types (GraphQLDeferResponse, DeferFetchGroup) | `v2/pkg/engine/resolve/response.go` |
| Field defer annotation (DeferField) | `v2/pkg/engine/resolve/node_object.go` |
| Fetch dependencies (DeferID) | `v2/pkg/engine/resolve/fetch.go` |
| Execution entry point | `v2/pkg/engine/resolve/resolve.go` |
| Incremental rendering (ResolveDefer, collectDeferFields) | `v2/pkg/engine/resolve/resolvable.go` |
| Integration tests (planner) | `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go` |
| Integration tests (engine) | `execution/engine/execution_engine_defer_test.go` |
| Normalization tests | `v2/pkg/astnormalization/inline_fragment_expand_defer_test.go` |
| Typename placeholder tests | `v2/pkg/astnormalization/defer_ensure_typename_test.go` |
| Required fields defer tests | `v2/pkg/engine/plan/required_fields_visitor_test.go` |