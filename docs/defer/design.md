# GraphQL `@defer`

This document describes how the engine executes `@defer`.
It covers every stage, from validation to the bytes on the wire.

- [1. Overview](#1-overview)
- [2. Pipeline](#2-pipeline)
- [3. Rules for clients](#3-rules-for-clients)
- [4. Deliberate differences from the incremental delivery draft](#4-deliberate-differences-from-the-incremental-delivery-draft)
- [5. Known limitations](#5-known-limitations)
- [6. Configuration and file reference](#6-configuration-and-file-reference)

---

## 1. Overview

A client puts `@defer` on an inline fragment or on a named fragment spread.
The engine sends the fields of that fragment in a later payload.
The initial payload does not wait for them.

The base schema defines the directive in `v2/pkg/asttransform/base.graphql`:

```graphql
directive @defer(
    label: String
    if: Boolean! = true
) on FRAGMENT_SPREAD | INLINE_FRAGMENT
```

`@defer` is valid in a query operation.
It is also valid below the root selection of a mutation operation.
The validator rejects it on a mutation root selection and on a subscription root selection.
The validator also rejects an enabled `@defer` in a nested selection of a subscription.
See [Placement](#placement).

The three fragment forms:

```graphql
# anonymous inline fragment
query {
  user(id: "1") {
    name
    ... @defer { expensiveField }
  }
}

# inline fragment with a type condition
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

### Response shape

The response is a stream of JSON payloads.
The engine uses the `pending`, `incremental` and `completed` format of the incremental delivery draft.

- The initial payload contains every field that is not deferred.
  It lists each deferred fragment in `pending`, with an `id` and a `path`.
- Each later payload delivers one deferred fragment.
  The `incremental` entry carries the data and the `id`.
  The `completed` entry tells the client that the fragment is done.
- `hasNext: true` tells the client that more payloads follow.
  The payload that completes the last outstanding fragment has `hasNext: false`.

```jsonc
// initial payload
{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}

// incremental payload
{"incremental":[{"data":{"expensiveField":"..."},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
```

When no deferred fragment survives the initial render, the engine sends one ordinary execution result.
That result has no `pending` and no `hasNext`.
See [Cancelled defers](#cancelled-defers).

### Terms

- **Defer ID**: an integer that identifies one `@defer` directive after normalization.
  The normalizer assigns the IDs in document order, from `1`.
  The ID `0` means "not deferred".
  The wire format writes the ID as a JSON string, for example `"1"`.
- **Defer descriptor** (`resolve.DeferDescriptor`): the ID, the parent defer ID, the label and the path of one defer.
- **Anchor**: the object that a deferred fragment is placed on.
  The descriptor path points to the anchor.
- **Fetch anchor**: the node where the planner starts the deferred fetch.
  It is a query root field or an entity with a key.

---

## 2. Pipeline

An operation with `@defer` passes through these stages:

1. [Validation](#21-validation): the prevalidation rules check placement and labels.
2. [Normalization](#22-normalization): the normalizer coerces `if` and replaces `@defer` with a per-field `@__defer_internal` directive.
3. [Planning](#23-planning): the planner assigns each field to a fetch and to a defer scope.
4. [Postprocess](#24-postprocess): the postprocess steps split the fetches by defer ID and build the defer tree.
5. [Resolver](#25-resolver): the resolver sends the initial payload, then runs the defer tree.
6. [Wire format](#26-wire-format): the router writes the payloads as multipart parts.

### 2.1 Validation

**Files:**
- `v2/pkg/astvalidation/operation_rule_defer_stream_on_root_fields.go`
- `v2/pkg/astvalidation/operation_rule_defer_stream_unique_labels.go`
- `execution/engine/execution_engine.go`

Two rules check `@defer` and `@stream`:

- `DeferStreamOnValidOperations` checks the placement.
- `DeferStreamHaveUniqueLabels` checks the labels.

The caller registers them with `astnormalization.WithPrevalidationRules`.
The normalizer runs them on the `directivesIncludeSkip` walker.
That walker runs before fragment inlining and before defer expansion.
So the rules see every `@defer` and `@stream`, also a directive that the normalizer later removes because `if` is `false`.

The execution engine registers both rules in `ExecutionEngine.Execute`.
A caller that normalizes without these rules gets no placement check and no label check.

When the request names an operation, `execution/graphql/normalization.go` adds `WithRemoveNotMatchingOperationDefinitions`.
The normalizer then removes the other operations in an earlier walk, before the rules run.
The nested subscription check in [Placement](#placement) and the label scope in [Labels](#labels) depend on this removal.

`ExecutionEngine.Execute` sets missing or `null` request variables to `{}`.
Then it runs variables validation after normalization, for all variables that are a JSON object.
So a request without variables also gets variables validation.
The cosmo router also sets missing or `null` variables to `{}` before variables validation.
The normalizer keeps a directive with an unresolved required variable in the operation.
Variables validation is the only check that reports that variable.
Example: `@defer(if: $d)` with `$d: Boolean!` and no variables fails with `Variable "$d" of required type "Boolean!" was not provided.`
`@skip` and `@include` get the same result.

Tests:
- `TestExecutionEngine_Execute_VariablesValidation` in `execution/engine/execution_engine_variables_validation_test.go`
- `[E4] request without variables misses a required variable` in `execution/engine/execution_engine_defer_if_test.go`

The rules are described in [Placement](#placement) and [Labels](#labels).

### 2.2 Normalization

**Files:**
- `v2/pkg/astnormalization/astnormalization.go`
- `v2/pkg/astnormalization/defer_expand_into_internal.go`
- `v2/pkg/astnormalization/defer_ensure_typename.go`
- `v2/pkg/astnormalization/defer_align_typename_scope.go`
- `v2/pkg/astnormalization/defer_populate_parent_ids.go`
- `v2/pkg/ast/ast_directive_if_argument.go`
- `v2/pkg/ast/ast_field.go`

The option `WithEnableDefer()` enables defer normalization.
Without it, the normalizer still coerces `if` and removes `@defer`, but it stamps no field.
The operation then executes as an ordinary synchronous operation.

The normalizer runs the defer stages in this order:

1. Fragment inlining turns a named fragment spread into an inline fragment.
   The inline fragment keeps the directives of the spread.
2. Defer expansion replaces `@defer` with `@__defer_internal` on each field.
3. The cleanup stage merges duplicate fields and adds the `__typename` placeholder.
4. Typename alignment moves each deferred `__typename` to the scope of its object.
5. Parent repair sets the final `parentDeferId` of each deferred field.

Stages 4 and 5 run only when the document contains a defer.

#### Defer expansion

`deferExpandIntoInternal` visits each inline fragment with `@defer`.
For each fragment it does these steps:

1. It coerces `if` with `ast.Document.CoerceIfArgument`.
   The result is `IfArgumentTrue`, `IfArgumentFalse`, `IfArgumentNull` or `IfArgumentInvalid`.
   - For `IfArgumentNull`, the normalizer reports `Argument "if" of non-null type "Boolean!" must not be null.` and stops.
   - For `IfArgumentInvalid`, the normalizer keeps the directive and does not expand the fragment.
     Operation validation or variables validation then reports the value.
2. It removes `@defer` from the fragment.
3. When `if` is `false` or defer is disabled, it stops here.
   The fields of the fragment then go to the initial response.
4. It assigns the next defer ID.
   It records the ID of the enclosing defer as the parent ID, or `0` at the top level.
5. It stamps each field of the fragment with `@__defer_internal(id, parentDeferId, label)`.

The normalizer reads the label only when it is a string literal.
So `label: null` gives a defer without a label.

After expansion, `... @defer { title }` becomes `... { title @__defer_internal(id: 1) }`.
A nested defer becomes:

```graphql
... {
  profile @__defer_internal(id: 1) {
    ... { bio @__defer_internal(id: 2, parentDeferId: 1) }
  }
}
```

The engine stamps each field because of field merging.
A field can occur inside a defer fragment and outside it in the same selection set.
`MergeFieldsDefer` in `v2/pkg/ast/ast_field.go` compares the two copies when the cleanup stage merges them:

- When one copy is not deferred, the merged field is not deferred.
  The field goes to the initial response.
- When both copies are deferred, the copy with the smaller defer ID wins.

#### Typename placeholder

After expansion, every child field of a selection set can be deferred.
The initial subgraph query would then contain an empty selection set, which is not valid.
`deferEnsureTypename` adds a `__typename` field with the alias `__internal_typename` to such a selection set.
The planner keeps this field out of the client response.

The defer scope of the placeholder depends on the enclosing field:

- The enclosing field is not deferred: the placeholder is not deferred.
- The enclosing field is deferred, and no child has its defer ID: the placeholder gets the defer ID of the enclosing field.
- The enclosing field is deferred, and a child has its defer ID: the stage adds no placeholder.

#### `__typename` scope alignment

`deferAlignTypenameScope` gives each deferred `__typename` the defer scope of its enclosing object field.
The innermost `@defer` around the `__typename` does not decide its scope.

- The enclosing object field is not deferred, or there is no enclosing field: the stage removes `@__defer_internal` from the `__typename`.
- The enclosing object field is deferred with another ID: the stage stamps the `__typename` with the ID of the object field.
- The `__typename` already has the scope of the object field: the stage changes nothing.

The resolver reads the literal `__typename` key to find the type of an object.
That type selects the fields with type conditions, the entity representations and the abstract type.
So the `__typename` must be in the fetch that creates the object.
The value of `__typename` is known when the object exists, so a later delivery saves no time.

Example:

```graphql
{ article { id ... @defer { __typename title } reviews { id } } }
```

`article` is not deferred.
So the normalizer moves `__typename` to the initial response, and `title` stays deferred.
The initial subgraph fetch then contains a plain `__typename`.
The `Article` representation for the `reviews` entity fetch uses that value.

A `__typename` inside a deferred entity subtree stays deferred, because its object is deferred too.

The stage runs after field merging, so it sees the final defer ID of the enclosing field.
It runs as a separate walker stage before parent repair, so parent repair sees the new IDs.

#### Parent repair

`deferPopulateParentIds` sets the final `parentDeferId` of each deferred field.
The parent of a deferred field must be the nearest enclosing deferred object.
The resolver needs this parent to walk into the field.
Field merging can remove the defer of an ancestor, so a recorded parent can become stale.

The stage first collects the defer IDs that are still in the operation.
Then it does one of these for each deferred field:

- It adds a missing parent from the nearest enclosing deferred ancestor.
- It keeps a parent that is still in the operation.
- It replaces a stale parent with the nearest enclosing deferred ancestor.
  It removes the parent when there is no such ancestor.

### 2.3 Planning

**Files:**
- `v2/pkg/engine/plan/planner.go`
- `v2/pkg/engine/plan/defer_info_collector.go`
- `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go`
- `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`
- `v2/pkg/engine/plan/visitor.go`

The planner decides which data source fetches each field, and in which defer scope.
It builds one planner for each pair of data source and defer ID.

#### Defer descriptors

`deferInfoCollector` runs during operation preparation.
It reads each `@__defer_internal` field and records one `DeferDescriptor{ID, ParentID, Label, Path}` for each defer ID.
The path is the response path of the selection set that holds the first field of the defer.
The collector cuts the path after the outermost list field.
See [IDs, paths and `subPath`](#ids-paths-and-subpath).

A defer ID that no field carries gets no descriptor.

#### Defer context on node suggestions

The collect nodes visitor attaches defer data to each `NodeSuggestion`:

- `deferInfo` holds the defer ID, the label and the parent ID of a deferred field.
- `descendantDeferIDs` holds the IDs of deferred descendants that use this node as a path to their fetch anchor.
  `ProcessDefer` fills it.

#### `ProcessDefer`

`Planner.Plan` calls `ProcessDefer` after node selection.
It passes the defer descriptors and the name of the mutation root type.

For each selected deferred field, `propagateDeferParentsUpToRootNode` walks up the ancestors on the same data source.
The walk stops at the fetch anchor.
A fetch anchor is a query root field, or an entity node with a key where an `_entities` fetch can start.
The walk adds the defer ID to the `descendantDeferIDs` of each ancestor on the path.
These ancestors become **defer parents**.
An ancestor in the same defer gets no mark, and the walk continues above it.
The walk writes the IDs in a second pass, because the mutation root check can discard the whole path.

**Mutation root fields.**
When the walk ends at a top-level field of the mutation root type, that mutation root field is the fetch anchor.
A deferred fetch from that anchor would execute the mutation a second time.
So the walk marks no parent, and `ProcessDefer` sets the `deferInfo` of the field to `nil`.
The field then belongs to the initial response.
The decision is per field, so an entity-backed field of the same defer stays deferred.
The decision uses the generated fetch anchor.
A declared entity key on the mutation subgraph therefore does not keep the field deferred.

After the pass, `deleteEmptyDeferDescriptors` deletes each descriptor that has no deferred field left.
It gives the children of a deleted descriptor the parent of that descriptor.
The other defer IDs keep their values.

#### Required fields

`required_fields_visitor.go` adds the fields that entity resolution needs.
`applyDeferInternalDirective` stamps them with the correct defer scope:

- A `@requires` field gets the defer ID of the field that needs it.
  It must arrive in the same deferred fetch.
- A `@key` field gets the defer ID of the parent field, or no defer ID at the top level.
  The key must exist before the deferred entity fetch starts.
  The visitor reuses an existing copy of the key in the same scope.

The visitor records each added field in `skipFieldRefs`.
The client response does not contain these fields.

#### Path builder

The path builder plans each field in one of three modes:

1. **Deferred field** (`deferInfo` is set): the planner for the pair of data source and defer ID plans it.
   The path builder creates that planner when it does not exist.
2. **Defer parent** (`descendantDeferIDs` is not empty): the path builder plans the field once for each descendant defer ID.
   Each copy is a path that is not deferred, on the planner that owns the descendant.
   This copy starts the deferred fetch at the fetch anchor and extends the query down to the deferred fields.
3. **Normal field**: the planner for the initial response plans it.

A field can be in mode 1 and mode 2 at the same time.

`path_builder_visitor.go` keeps the scopes apart:

```go
if plannerConfig.DeferID() != 0 && field.deferID == 0 { continue } // a deferred planner rejects a field that is not deferred
if field.deferID != 0 && plannerConfig.DeferID() != field.deferID { continue } // a deferred field joins only the planner of its defer
```

Without this check, a planner for the initial response could take a deferred field on the same data source and path.
One defer ID can produce more than one planner when its fields have different fetch anchors.

#### Plan output

`visitor.go` writes the defer data into the plan:

- `assignDefer` sets `resolve.Field.Defer` on a deferred response field.
  The resolver uses it to select the fields of each payload.
- `configureFetch` sets `FetchDependencies.DeferID` on each fetch.
  The postprocess steps use it to split the fetches.
- When a planner has a defer ID, the plan is a `DeferResponsePlan`.
  Otherwise it is a `SynchronousResponsePlan` or a `SubscriptionResponsePlan`.
  A mutation whose deferred fields all moved to the initial response gets a `SynchronousResponsePlan`.

### 2.4 Postprocess

**Files:**
- `v2/pkg/engine/postprocess/postprocess.go`
- `v2/pkg/engine/postprocess/extract_defer_fetches.go`
- `v2/pkg/engine/postprocess/build_defer_tree.go`

For a `DeferResponsePlan`, `Processor.Process` runs these steps:

1. `mergeFields` merges duplicate field nodes in the response tree.
2. `createFetchTree` puts all fetches of all defer IDs into one flat sequence.
3. `processFlatFetchTree` collects the authorization coordinates, removes duplicate fetches, assigns fetch IDs and adds the missing nested dependencies.
   These steps need the full flat list.
4. `extractDeferFetches` splits the flat sequence by `FetchDependencies.DeferID`.
   The fetches with ID `0` stay in the initial tree.
   Each other ID gets one `DeferFetchGroup{DeferID, Fetches}`, in ascending ID order.
5. `organizeFetchTree` and `processOrganizedFetchTree` run on the initial tree and on each group separately.
   They merge fetches, order the fetches by their dependencies, render the subgraph inputs and resolve the input templates.
6. `buildDeferTree` builds the defer tree from the descriptors.
   Then `Process` sets `GraphQLDeferResponse.Defers` to `nil`, because the tree holds the groups.

#### The defer tree

`buildDeferTree` builds a `DeferTreeNode` tree that follows the nesting of the defers:

1. It groups the fetch groups by the `ParentID` of their descriptor.
   It sorts each sibling list by defer ID.
2. The roots are the groups with `ParentID == 0`.
3. A group without children becomes a `Single` node.
4. A group with children becomes `Sequence(Single(group), subtree)`.
   The subtree is the chain of the one child, or a `Parallel` node over the chains of all children.
5. Two or more roots go into a top-level `Parallel` node.

A `Sequence` node runs a parent before its children.
A `Parallel` node runs independent siblings at the same time.

#### Dependencies between groups

`organizeFetchTree` orders the fetches in one tree.
A deferred entity fetch can need a key from the initial tree.
The structure of the execution satisfies this dependency:

- The initial tree finishes before any group starts.
- The defer tree runs a child group after its parent group.

### 2.5 Resolver

**Files:**
- `v2/pkg/engine/resolve/resolve.go`
- `v2/pkg/engine/resolve/resolvable.go`
- `v2/pkg/engine/resolve/defer_tree.go`
- `v2/pkg/engine/resolve/data_buffer.go`
- `v2/pkg/engine/resolve/loader.go`

#### `ResolveGraphQLDeferResponse`

`Resolver.ResolveGraphQLDeferResponse` writes the payloads in this order:

1. It takes one arena from the pool.
   It creates the `Resolvable` and the initial `Loader` on that arena.
   It wraps the response tree in a `DataBuffer`.
2. It runs the initial fetch tree.
3. It renders the initial payload in defer mode.
   The render skips each field with `Field.Defer`.
   The payload lists the top-level defers whose anchor survived in `pending`, and it has `"hasNext":true`.
   When no defer survives, the payload has no `pending` and no `hasNext`.
4. It flushes the initial payload.
   An error before this flush returns to the caller, and the router formats it as an ordinary error response.
5. It registers `writer.Complete()`.
   After the first flush, every exit closes the stream.
6. `pruneDeadDefers` removes the subtrees of the top-level defers that did not survive.
   When no defer survives, no deferred fetch runs and `Complete()` closes the stream.
7. It sets the `outstanding` counter to the number of surviving top-level defers.
   Then it runs the defer tree.

#### Phase barrier

The resolver starts deferred work only after these events:

- The full initial fetch tree finished.
- The resolver rendered and flushed the initial payload.

For a mutation, all mutation root fetches finish before any deferred fetch starts.
A later mutation root field never waits for the deferred work of an earlier mutation root field.

#### Anchor survival

A deferred fragment is delivered only when its anchor survived the render of its parent payload.

- `deferAnchorAlive(path)` reads the anchor from the response data.
  The anchor is dead when it is absent or `null`.
- `liveChildDescriptors(parentID)` returns the child descriptors of `parentID` whose anchor is alive.
  The ID `0` selects the top-level defers.

Null propagation in the render sets a nullable object to `null` when a non-null child is `null`.
So the anchor of a defer below that object reads as dead.
When null propagation reaches the root, the render does not change the stored data.
So `Resolvable.Resolve` stores `null` as the data in defer mode.
Then every anchor reads as dead.

#### Running the defer tree

`resolveDeferTree` walks the tree:

- **Single**: `resolveDeferSingle` fetches and renders one group.
- **Sequence**: it resolves the parent first.
  Then it runs only the children that the parent announced.
  The other children never run.
- **Parallel**: it runs each child on a plain `errgroup.Group`.
  A failed group does not cancel its siblings.
  A branch returns its error only when the client context is cancelled.

#### Resolving one group

`resolveDeferSingle` creates a new `Loader` for each group.
All loaders and the resolvable share one arena and one `DataBuffer`.

- **Fetch phase**: the loader runs the fetches of the group without the `DataBuffer` lock.
  So the fetches of sibling groups overlap.
  The network phase allocates nothing from the arena.
- **Render phase**: the resolver takes the `DataBuffer` lock.
  The loader merges the results into the shared tree.
  The resolvable renders the payload, and the writer flushes it.
  The lock prevents two groups from writing interleaved payloads or allocating from the arena at the same time.

Each group loader collects only the errors of its own fetches.
So a payload contains only the errors of its own group.

A fetch-phase error, for example from a pre-fetch authorizer or a rate limiter, calls `ResolveDeferError`.
The payload completes the defer with the error in `completed.errors` and has no `incremental` entry.
The children of that defer are never announced.
The other defers continue.

#### Incremental render

`ResolveDeferBatch` renders the payload of one group in two passes over the response tree.
The resolver must know the shape of the payload before it writes the first byte.

1. **Pre-walk** (no output): `collectDeferFields` sorts the fields of each object.
   - A field with the current defer ID is rendered.
   - A field without a defer ID, or with a smaller defer ID, is walked to reach the matching fields below it.
   - A field with a larger defer ID is skipped, because its data is not fetched yet.

   The pre-walk runs the authorization checks and detects null propagation.
   Each of them can mark the fragment as having no data to deliver.
2. **Render**: the resolver renders the deferred fields into a scratch buffer.
   When the render fails, the resolver discards the buffer.
   The error then goes to `completed.errors`, and no partial payload reaches the wire.

The resolver then writes the payload:

- `incremental: [{data, id, subPath?, errors?}]`, only when the fragment has data to deliver.
  `errors` holds the recoverable errors of the group.
- `completed: [{id, errors?}]`, always.
  `errors` is present only when the fragment has no data to deliver.
- `pending: [...]`, with the children of this defer whose anchor survived.
  The resolver announces a nested defer in the payload of its parent, not in the initial payload.
- `hasNext`, from the `outstanding` counter.

#### The `outstanding` counter

`outstanding` counts the announced defers that are not completed.
It starts at the number of surviving top-level defers.
Each payload adds the number of announced children and subtracts one for the completed defer.
The payload that sets the counter to `0` has `"hasNext":false`.
All changes and all writes happen under the `DataBuffer` lock, so the counter is a plain `int64`.

#### Client cancellation

Deferred execution handles client cancellation in the same way as synchronous execution.
The client context cancels every running fetch.
A parallel branch returns its error only after the client context is cancelled.
The resolver can still prepare a payload after the cancellation, but the client does not receive it.

### 2.6 Wire format

This repository writes no HTTP header for a defer response.
The router implements `resolve.DeferResponseWriter`:

```go
type DeferResponseWriter interface {
    ResponseWriter
    Flush() error
    Complete()
}
```

`Flush()` sends the current payload.
Each payload causes exactly one `Flush()`.
`Complete()` closes the stream.

The cosmo router sets `Content-Type: multipart/mixed; deferSpec=20220824; boundary="graphql"` in `router/core/defer_response_writer.go`.
The `deferSpec=20220824` parameter names an older format.
The body uses the `pending`, `incremental` and `completed` format of the current draft.

**Initial payload.**
It contains every field that is not deferred.
It has one `pending` entry for each surviving top-level defer.
The entry has the `id`, the `path` of the anchor and the `label` when the defer has one.

```json
{"data":{"user":{"id":"1","name":"Alice"}},"pending":[{"id":"1","path":["user"],"label":"details"}],"hasNext":true}
```

**Initial payload without a surviving defer.**
It is an ordinary execution result, and the stream ends after it.

```json
{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}
```

**Incremental payload with data.**

```json
{"incremental":[{"data":{"expensiveField":"..."},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
```

**Incremental payload without data.**
The fragment was nulled by null propagation, failed authorization or failed in the render.

```json
{"completed":[{"id":"1","errors":[{"message":"..."}]}],"hasNext":true}
```

`hasNext` rules:

- `hasNext: true` on every payload of a stream with a surviving defer, except the last payload.
- `hasNext: false` on the payload that completes the last outstanding defer.
- No `hasNext` on an initial payload without a surviving defer.

---

## 3. Rules for clients

### Placement

`DeferStreamOnValidOperations` compares the parent type of the directive location with the root types.

- The rule rejects `@defer` and `@stream` when the parent type is the mutation root type.
  The error is `directive "@defer" is not allowed on root fields of mutation operations`.
- The rule rejects `@defer` and `@stream` when the parent type is the subscription root type.
  The error is `directive "@defer" is not allowed on subscription operations`.
- The `if` argument does not change these results.
  `if: false`, `if: null`, `if: $var` with any value, and an absent `if` all fail.
- The rule has no depth check.
  A directive in a nested inline fragment on the root type fails.
  A directive below a field that returns the mutation root type also fails.
- A directive below a field of a type that is not a root type is valid.
- Only an object type definition can match a root type.
  The walker pushes an invalid node with an empty name for an unknown field.
  A schema without a mutation type has an empty mutation type name.
  The object type check keeps these two empty names apart.
  So a directive below an unknown field gets no false mutation root error.

The rule also rejects a nested `@defer` or `@stream` in a subscription operation when `if` coerces to `true`.
This includes an absent `if` argument and an absent nullable variable without a default.
When `if` coerces to `false`, the rule accepts the directive, and the normalizer removes it.

The nested subscription check also covers fragment definitions.
When the document has one operation, `EnterDocument` takes the operation type and the operation ref from that operation.
So a fragment definition before a subscription operation gets the subscription type.
A fragment definition after the operation gets the type from `EnterOperationDefinition`.
The check depends on the operation removal in [Validation](#21-validation).
See [Known limitations](#5-known-limitations) for a document that keeps several operations.

Tests:
- `TestDeferStreamOnValidOperations` in `v2/pkg/astvalidation/operation_rule_defer_stream_on_root_fields_test.go`
- `TestDeferStreamOnValidOperationsIfCoercion` in `v2/pkg/astvalidation/operation_rule_defer_stream_if_coercion_test.go`
- `TestExecutionEngine_Execute_DeferRootValidation` in `execution/engine/execution_engine_defer_root_validation_test.go`

### The `if` argument

The argument type is `Boolean! = true`.
`ast.Document.CoerceIfArgument` coerces the value in the order of CoerceVariableValues and then CoerceArgumentValues of the GraphQL specification.

| `if` value | Result |
|------------|--------|
| no `if` argument | defer |
| literal `true` / `false` | defer / no defer |
| nullable variable, absent, no default | defer, from the argument default `true` |
| variable, absent, Boolean default | the variable default |
| variable with a Boolean value | that value |
| nullable variable, explicit `null` | error `Argument "if" of non-null type "Boolean!" must not be null.` |
| nullable variable, absent, `null` default | the same error |
| non-null variable, absent without a default, or explicit `null` | the directive stays, and variables validation reports the value |
| variable value of the wrong JSON type | the directive stays, and variables validation reports the value |
| literal `null`, or a literal of the wrong type | the directive stays, and operation validation reports the value |
| undefined variable, or a variable of the wrong type | the directive stays, and operation validation reports the value |

- The normalizer reports the `null` error itself.
  No validator reports a `null` value of a nullable variable in a location with a default.
- The nested subscription check in [Placement](#placement) uses the same coercion.
- The coercion also runs without `WithEnableDefer`.

Tests:
- `TestDeferDirectiveArguments` in `v2/pkg/astnormalization/defer_directive_arguments_test.go`
- `TestExecutionEngine_Execute_DeferIfArgument` in `execution/engine/execution_engine_defer_if_test.go`

### Labels

`DeferStreamHaveUniqueLabels` checks the labels:

- A label is a static string literal.
  A variable or another literal kind fails with `directive "@defer" label argument must be a static string value`.
- `label: null` means no label.
  It reserves nothing.
- Each static label reserves its value regardless of `if`.
  This applies to `if: false`, to `if: $var`, and to a selection that `@skip` or `@include` removes.
- `@defer` and `@stream` share one label namespace.
- A duplicate fails with `directive "@defer" label "<label>" must be unique, but was already used on "@defer" directive`.
- A labeled directive inside a fragment definition is one directive.
  Two spreads of that fragment are valid.
  Two spreads that each carry `@defer(label: "a")` are a duplicate.
- A directive is never a duplicate of itself.
  The walker visits a directive again when a `@skip` or `@include` removal changes its selection set.
  The rule compares the directive refs, so the second visit is valid.

The rule keeps one label set for each document.
When the request names an operation, the normalizer removes the other operations before the rule runs.
So the label scope is the selected operation.
When the request names no operation, the rule checks every operation in the document.
Every fragment definition in the document takes part in the check, also a fragment that the selected operation does not use.

The engine writes the label on the `pending` entry only.

Tests:
- `TestDeferStreamHaveUniqueLabels` in `v2/pkg/astvalidation/operation_rule_defer_stream_unique_labels_test.go`
- `TestExecutionEngine_Execute_DeferLabels` in `execution/engine/execution_engine_defer_labels_test.go`

### IDs, paths and `subPath`

- One defer ID represents one planned defer directive.
- One ID can cover more than one list item.
- The `pending` path stops at the outermost list field.
- Each `incremental` entry has a `subPath` when its position is deeper than the `pending` path.
  The `subPath` holds the list indices and the field names below the `pending` path.
- The `completed` entry applies to the full defer, with all its list items.
- The IDs can have gaps.
  A defer that loses all its fields keeps no ID in the response, and the other IDs keep their values.

Example: `{ items { ... @defer { name } } }` with two items gives this stream:

```json
{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"1","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
```

`deferInfoCollector` in `v2/pkg/engine/plan/defer_info_collector.go` cuts the path.
`printDeferSubPathIfAny` in `v2/pkg/engine/resolve/resolvable.go` writes the `subPath`.
The test `nested list entities` in `execution/engine/execution_engine_defer_test.go` covers the shape.

### Overlapping defers

The planning pipeline selects one delivery group for each merged field.
The resolver does not choose between overlapping defers.

- When a deferred copy and a copy that is not deferred merge, the field goes to the initial response.
- When two deferred copies of one field merge, the copy with the smaller defer ID wins.
  For sibling defers, the smaller ID is the defer that comes first in the document.
- The unique fields of the other defer stay in that defer.
- When the other defer has no fields left, it gets no descriptor.
  The client receives no `pending` entry and no `completed` entry for it.

Example:

```graphql
query Q {
  thing {
    id
    ... @defer(label: "first") {
      slow
    }
    ... @defer(label: "second") {
      slow
    }
  }
}
```

The engine plans one deferred fetch and one descriptor, with the label `first`.
The label `second` does not appear in the response.

`MergeFieldsDefer` in `v2/pkg/ast/ast_field.go` implements the rule.
The test `with internal defer` in `v2/pkg/astnormalization/field_deduplication_test.go` covers the merge.

### `__typename` scope

The engine delivers a `__typename` in the payload that creates its object.
A client can write `__typename` inside a defer fragment, and the engine can send it in an earlier payload.
When a defer fragment contains only such a `__typename`, the defer has no field left, and the client receives no `pending` entry for it.
See [`__typename` scope alignment](#__typename-scope-alignment).

### Mutations

- Each mutation root field executes once.
  The planner never creates a second upstream mutation for a deferred field.
- A deferred field whose fetch anchor is a mutation root field goes to the initial response.
- An entity-backed deferred field stays deferred.
  Its `_entities` fetch depends on the mutation root fetch that returns the key.
- One defer can split.
  Its fields that need the mutation root go to the initial response, and its entity-backed fields stay deferred.
  The engine announces the defer and delivers the remaining fields, with a `subPath` when they are deeper than the descriptor path.
- A defer that loses all its fields leaves nothing in the response.
  It has no deferred fetch, no descriptor, no `pending` entry and no `completed` entry.
- When all deferred fields move, the plan is a `SynchronousResponsePlan`, and the client receives one ordinary JSON response.

Example, where `createThing` returns no usable entity key:

```graphql
mutation CreateThing {
  a: createThing {
    id
    ... @defer {
      slow
    }
  }
}
```

The engine sends one upstream request, `mutation{a: createThing {id slow}}`.
The response is `{"data":{"a":{"id":"t1","slow":"done"}}}`.

Tests:
- `TestGraphQLDataSourceDeferMutation` in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_mutation_test.go`
- `TestExecutionEngine_Execute_DeferMutation` in `execution/engine/execution_engine_defer_mutation_test.go`

### Cancelled defers

- A defer whose anchor becomes `null` in the render of its parent payload is never announced.
  Its deferred work never runs, and the work of its children never runs.
- When some top-level defers survive, the initial `pending` lists only those defers, and the payload has `"hasNext":true`.
- When the initial render cancels every defer, the initial payload is an ordinary execution result.
  It has `data` and optional `errors`, with no `pending` and no `hasNext`.
  No deferred fetch runs, and the stream ends after this payload.
- When null propagation reaches the root, the payload has `"data":null` and no `pending`.
- The transport framing does not change.
  The router still writes one multipart part and the closing boundary.

Example, where `name` is non-null and returns `null`:

```graphql
{ user { name ... @defer { title } } }
```

```json
{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}
```

Tests:
- `TestDefer_AllDefersCancelledBeforeInitialResult` in `v2/pkg/engine/resolve/resolve_defer_cancel_shape_test.go`
- `TestExecutionEngine_Execute_DeferCancelledBeforeInitialResult` in `execution/engine/execution_engine_defer_cancel_test.go`

### Ordering

- The initial payload comes before every incremental payload.
- A parent defer comes before its children.
- Sibling defers run at the same time, and their payloads can arrive in any order.
- A failed sibling does not cancel the other siblings.

---

## 4. Deliberate differences from the incremental delivery draft

The reference draft is GraphQL specification pull request 1110 at commit `045e19363c2b55f127960bd3b5e8072a15b29aec`.
The reference implementation is GraphQL.js `17.x.x` at commit `ee5ce41d4b68d1852306d3b56dba2cbbb6c43fea`.
The draft is at RFC Stage 2, so its requirements can change.

The engine differs from the draft in these points:

1. **Overlapping defers.**
   The draft announces and completes each overlapping defer.
   The engine delivers a shared field in one defer, and it drops a defer without fields.
   See [Overlapping defers](#overlapping-defers).
2. **One ID for each planned defer.**
   The draft creates one `pending` entry for each runtime application of a defer, with its own ID and an indexed path.
   The engine uses one ID for all list items and puts the indices in `subPath`.
   See [IDs, paths and `subPath`](#ids-paths-and-subpath).
3. **Query root re-execution.**
   When a deferred field below a query root field has no entity boundary, the planner starts the deferred fetch at the query root field.
   The engine then sends one upstream query for the initial fields and another one for the deferred fields.
   Both queries execute the same root field, so the subgraph resolver of that field runs twice for each request.
   The working group solution criteria require one execution for a deduplicated field.
   The second fetch can also see newer upstream data than the first fetch.
   The test `basic/on root query node/defer User.title` in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go` plans `{user {name}}` and `{user {title}}`.
4. **Label scope.**
   The draft defines one label set for the full document.
   The engine checks the labels of the selected operation.
   See [Labels](#labels).
5. **`__typename` scope.**
   The engine can deliver a deferred `__typename` in an earlier payload.
   See [`__typename` scope](#__typename-scope).
6. **Nested subscription directives.**
   GraphQL.js accepts a nested directive with `if: $var` for every runtime value.
   The engine rejects it when the variable coerces to `true`.
   See [Placement](#placement).

These points follow the draft or GraphQL.js:

- **Root placement.**
  The placement check is the check of the GraphQL.js `DeferStreamDirectiveOnRootFieldRule`.
- **`label: null`.**
  GraphQL.js also treats a `null` label as no label.
- **Phase barrier.**
  The draft permits parallel execution of deferred work, but it does not require it.
  GraphQL.js also waits for the initial result by default.
  The engine does not implement the GraphQL.js option `enableEarlyExecution`.

---

## 5. Known limitations

- **Query plan output.**
  The postprocess sets `GraphQLDeferResponse.Defers` to `nil` after it builds the defer tree.
  `GraphQLDeferResponse.QueryPlanString` reads `Defers`, so the printed `Deferred` section is empty.
- **Actual cost.**
  The execution engine computes the actual cost after a synchronous response only.
  A defer operation has no actual cost.
- **Router cache key for absent and `null` values.**
  The cosmo router writes the same key byte for an absent `if` variable and for a `null` value in `writeSkipIncludeCacheKeyToKeyGen`.
  The `if` coercion gives different results for these two values.
  So a cached result for an absent value can serve a `null` request and skip the error.
- **`@skip` and `@include` with an explicit `null`.**
  `directiveIncludeSkip` reads the value with `GetBooleanValue`, not with the `if` coercion of `@defer`.
  For an explicit `null`, `GetVariableBooleanValue` uses the variable default.
  Without a default, it reports no valid value, and the normalizer keeps the directive without a `null` error.
- **Error location after fragment inlining.**
  `CopyArgument` in `v2/pkg/ast/ast_argument.go` copies the name and the value of an argument, but not its position.
  Fragment inlining copies directives with it.
  So the `if: null` error for a defer inside a fragment has no location.
- **Nested subscription check with several operations.**
  In a document that keeps several operations, a fragment definition takes the type of the last operation that the walker entered before it.
- **Abstract fragments on the mutation root.**
  The engine does not support abstract fragment paths from a mutation root selection.
  This includes interface conditions that the mutation root implements, and union conditions that contain the mutation root type.
  Behavior outside direct mutation root selections and fragments on the concrete mutation root type is not specified.
  Two tests record the current validator result:
  - `mutation { ... on Node @defer { id } }` fails, because the parent type of the directive is the mutation root type (`[P6]`).
  - `mutation { ... on Node { ... @defer { id } } }` passes validation (`[P7]`).
- **Planning without prevalidation.**
  The planner has no guard for a `@defer` on a mutation root selection, because the placement rule rejects it.
  A caller that plans without the prevalidation rules gets a deferred fetch that executes the mutation again.
- **Transport parameter.**
  The cosmo router advertises `deferSpec=20220824`, but the body uses the format of the current draft.
  See [Wire format](#26-wire-format).
- **Parity tests.**
  No test compares the complex overlap cases with GraphQL.js.
  No test covers the dropped label in [Overlapping defers](#overlapping-defers).

---

## 6. Configuration and file reference

### Options

| Option | Location | Purpose |
|--------|----------|---------|
| `WithEnableDefer()` | `v2/pkg/astnormalization/astnormalization.go` | Enables defer normalization. Without it, the normalizer removes `@defer` and stamps no field. |
| `WithPrevalidationRules(...)` | `v2/pkg/astnormalization/astnormalization.go` | Runs the placement rule and the label rule before defer expansion. |
| `DisableExtractDeferFetches()` | `v2/pkg/engine/postprocess/postprocess.go` | Skips the split of the fetches by defer ID. Tests use it to check the planner output. |
| `DisableBuildDeferTree()` | `v2/pkg/engine/postprocess/postprocess.go` | Skips the defer tree. Tests use it to check the split. |

### Data structures

```go
// v2/pkg/engine/resolve/response.go
type GraphQLDeferResponse struct {
    Response         *GraphQLResponse        // fields and fetches of the initial payload
    Defers           []*DeferFetchGroup      // postprocess input; nil after buildDeferTree
    DeferDescriptors map[int]DeferDescriptor // one descriptor for each defer ID
    DeferTree        *DeferTreeNode          // execution tree
}

type DeferDescriptor struct {
    ID       int      // starts at 1
    ParentID int      // 0 for a top-level defer
    Label    string   // "" when the defer has no label
    Path     []string // response path of the anchor
}

type DeferFetchGroup struct {
    DeferID int
    Fetches *FetchTreeNode
}

// v2/pkg/engine/resolve/defer_tree.go
type DeferTreeNode struct {
    Kind       DeferTreeNodeKind // Single, Sequence or Parallel
    Item       *DeferFetchGroup  // set only for Single
    ChildNodes []*DeferTreeNode  // children of Sequence and Parallel
}

// v2/pkg/engine/resolve/node_object.go
type DeferField struct { DeferID int } // on resolve.Field; the initial render skips the field

// v2/pkg/engine/resolve/fetch.go
type FetchDependencies struct {
    FetchID           int
    DependsOnFetchIDs []int
    DeferID           int // not 0 for a deferred fetch
}
```

### Files

| Area | File |
|------|------|
| Directive definition | `v2/pkg/asttransform/base.graphql` |
| Directive name constants | `v2/pkg/lexer/literal/literal.go` |
| `if` coercion | `v2/pkg/ast/ast_directive_if_argument.go` |
| Field helpers: merge, stamp, read the defer ID | `v2/pkg/ast/ast_field.go` |
| Placement rule | `v2/pkg/astvalidation/operation_rule_defer_stream_on_root_fields.go` |
| Label rule | `v2/pkg/astvalidation/operation_rule_defer_stream_unique_labels.go` |
| Normalizer stages | `v2/pkg/astnormalization/astnormalization.go` |
| Defer expansion | `v2/pkg/astnormalization/defer_expand_into_internal.go` |
| Typename placeholder | `v2/pkg/astnormalization/defer_ensure_typename.go` |
| `__typename` scope alignment | `v2/pkg/astnormalization/defer_align_typename_scope.go` |
| Parent repair | `v2/pkg/astnormalization/defer_populate_parent_ids.go` |
| Request normalization and operation removal | `execution/graphql/normalization.go` |
| Engine normalization options | `execution/engine/execution_engine.go` |
| Planner entry and `ProcessDefer` call | `v2/pkg/engine/plan/planner.go` |
| Defer descriptors | `v2/pkg/engine/plan/defer_info_collector.go` |
| Collect nodes visitor | `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go` |
| Node suggestions and `ProcessDefer` | `v2/pkg/engine/plan/datasource_filter_node_suggestions.go` |
| Node selection and `skipFieldRefs` | `v2/pkg/engine/plan/node_selection_visitor.go` |
| Required fields and their defer scope | `v2/pkg/engine/plan/required_fields_visitor.go` |
| Path builder | `v2/pkg/engine/plan/path_builder_visitor.go` |
| `assignDefer`, `configureFetch` and the plan type | `v2/pkg/engine/plan/visitor.go` |
| Plan types | `v2/pkg/engine/plan/plan.go` |
| Postprocess steps | `v2/pkg/engine/postprocess/postprocess.go` |
| Fetch split by defer ID | `v2/pkg/engine/postprocess/extract_defer_fetches.go` |
| Defer tree build | `v2/pkg/engine/postprocess/build_defer_tree.go` |
| Response types and `DeferResponseWriter` | `v2/pkg/engine/resolve/response.go` |
| Defer tree and `pruneDeadDefers` | `v2/pkg/engine/resolve/defer_tree.go` |
| `DeferField` | `v2/pkg/engine/resolve/node_object.go` |
| `FetchDependencies` | `v2/pkg/engine/resolve/fetch.go` |
| Shared response tree and lock | `v2/pkg/engine/resolve/data_buffer.go` |
| Resolver entry and defer tree walk | `v2/pkg/engine/resolve/resolve.go` |
| Initial and incremental render | `v2/pkg/engine/resolve/resolvable.go` |

### Tests

| Area | File |
|------|------|
| `if` coercion | `v2/pkg/astnormalization/defer_directive_arguments_test.go` |
| Defer expansion | `v2/pkg/astnormalization/defer_expand_into_internal_test.go` |
| Typename placeholder | `v2/pkg/astnormalization/defer_ensure_typename_test.go` |
| `__typename` scope alignment | `v2/pkg/astnormalization/defer_align_typename_scope_test.go` |
| Parent repair | `v2/pkg/astnormalization/defer_populate_parent_ids_test.go` |
| Placement | `v2/pkg/astvalidation/operation_rule_defer_stream_on_root_fields_test.go`, `v2/pkg/astvalidation/operation_rule_defer_stream_if_coercion_test.go` |
| Labels | `v2/pkg/astvalidation/operation_rule_defer_stream_unique_labels_test.go` |
| Required fields | `v2/pkg/engine/plan/required_fields_visitor_test.go` |
| Planner | `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go` |
| Planner, mutations | `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_mutation_test.go` |
| Defer tree | `v2/pkg/engine/resolve/defer_tree_test.go` |
| Resolver, cancelled defers | `v2/pkg/engine/resolve/resolve_defer_cancel_shape_test.go` |
| Resolver, errors | `v2/pkg/engine/resolve/resolve_defer_errors_test.go` |
| Resolver, parallel groups | `v2/pkg/engine/resolve/resolve_defer_parallel_test.go` |
| Engine | `execution/engine/execution_engine_defer_test.go` |
| Engine, contract cases | `execution/engine/execution_engine_defer_root_validation_test.go`, `execution/engine/execution_engine_defer_if_test.go`, `execution/engine/execution_engine_defer_labels_test.go`, `execution/engine/execution_engine_defer_mutation_test.go`, `execution/engine/execution_engine_defer_cancel_test.go` |
| Engine, variables validation | `execution/engine/execution_engine_variables_validation_test.go` |
