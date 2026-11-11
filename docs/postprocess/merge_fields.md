# `mergeFields` postprocessor — Design Document

Code: `v2/pkg/engine/postprocess/merge_fields.go`.
Resolver side: `shouldSkipFieldByTypeCondition` in `v2/pkg/engine/resolve/resolvable.go`.

## 1. Where it runs

`Processor.Process` in `v2/pkg/engine/postprocess/postprocess.go` runs `mergeFields` first, before the fetch tree exists.
It runs on the response tree of every plan kind: synchronous, defer and subscription.
The option `DisableMergeFields` turns it off, for tests that assert the raw planner output.

The input is the response tree that `plan.Visitor` built: `resolve.Object`, `resolve.Array`, `resolve.Field` and scalar nodes.
The output is the same tree with the same shape, but with fewer `resolve.Field` nodes per object.

## 2. The problem

The planner emits one `resolve.Field` per field selection in the normalized operation.
A field that appears in several places of one selection set appears several times as a sibling.
This happens when a client selects the same field through several inline fragments.

```graphql
query {
  products {        # [Product], an interface
    category { id }
    ... on ProductA { category { owner } }
    ... on ProductB { category { owner } }
  }
}
```

The planner output for the `products` item object has three sibling fields named `category`.
The response can hold the key `category` once per object.
The postprocessor has to merge these siblings into one field, and the merged field has to render the right subfields for each runtime type.

Each sibling carries where it came from.
A field directly under an inline fragment has the fragment's type condition in `Field.OnTypeNames`.
`Visitor.resolveOnTypeNames` in `v2/pkg/engine/plan/visitor.go` sets it.
A concrete condition gives one name.
An interface condition gives every object type that implements it.
A field whose fragment normalization flattened has no `OnTypeNames`, see `v2/pkg/astnormalization/inline_selections_from_inline_fragments.go`.

The resolver reads these conditions per response object.
`walkObject` pushes the runtime `__typename` of each object on a stack.
`shouldSkipFieldByTypeCondition` compares the stack with the field conditions and skips a field that does not match.
A skipped field is absent from the response.

### 2.1 Problems the merge has to solve

1. **One key, one field.**
   Same-named siblings must become one field, because the resolver writes one key per field.
2. **The fragment path must survive the merge.**
   In the example, `owner` under `... on ProductA` must render only when the product is a `ProductA`.
   After the merge, `owner` is a child of the one merged `category` field.
   The merged `category` has no condition, so `owner` has to carry the condition of its former parent itself.
   This is `Field.ParentOnTypeNames` with a `Depth`.
3. **Several paths select one field.**
   In the example, `owner` is selected under `ProductA` and under `ProductB`.
   The two `owner` fields merge into one.
   The merged `owner` must render when the product is a `ProductA` OR a `ProductB`.
   A merge that keeps only the first condition drops `owner` for `ProductB`, and the result depends on the fragment order.
4. **An unconditional selection wins.**
   `category { id }` has no condition, so `id` renders for every product.
   A conditional `id` that merges into it must not restrict it.
5. **Conditions at different depths must not mix.**
   A path through `... on A { x { y { z } } }` and a path through `x { ... on C { y { z } } }` select `z` under different conditions at different depths.
   The merged `z` must render when the first path matches OR the second path matches.
   A merge that unions names per depth produces `(A at depth 2) AND (C at depth 1)`, which is wrong.
6. **A multi-type condition must merge with its single-type twins.**
   An interface condition expands to several names.
   A sibling under a concrete fragment has one name.
   The two can merge per type only when the multi-type field is split into one field per type.

## 3. The data model

```go
type Field struct {
    // ...
    OnTypeNames       [][]byte              // the field's own fragment condition, OR over names
    ParentOnTypeNames [][]ParentOnTypeNames // OR over groups
}

type ParentOnTypeNames struct {
    Depth int      // 0 = the field's containing object, 1 = its parent, ...
    Names [][]byte // OR over names
}
```

The rule is:

- A field renders when at least one group in `ParentOnTypeNames` matches, AND `OnTypeNames` matches.
- A group matches when every entry in it matches.
- An entry matches when the runtime `__typename` at its depth is one of its names.
- An empty `ParentOnTypeNames` means no parent condition.
- A nil `OnTypeNames` means no own condition.

Each group is one fragment path that selected the field.
One group is the common case.
Several groups appear only when paths differ at more than one depth.

`Depth` counts object layers.
An array adds no depth, because the resolver pushes on the stack per object, not per array.

## 4. The algorithm

`traverseNode` runs four steps on each object, then recurses into each field value.
An object with one field skips the steps and recurses, because nothing can merge there.

### Step 1: split multi-type fields

A field with several `OnTypeNames` becomes one field per name.
`Field.Copy` clones the subtree, with the `ParentOnTypeNames` groups.
After this step every field has at most one name in `OnTypeNames`.
This solves problem 6.

### Step 2: propagate the own condition to the descendants

`propagateParentTypeNames` writes the field's `OnTypeNames` into every descendant as a `ParentOnTypeNames` entry.
The direct children get `Depth: 1`, the grandchildren get `Depth: 2`, and so on.
`appendConditionToGroups` ANDs the entry into every existing group of the descendant.
A descendant without groups gets one new group.

A descendant already has groups when a step 2 at a deeper level ran first.
That does not happen in one top-down traversal.
It does happen after a merge, when a merged subtree is traversed again at the next level.

Step 2 solves problem 2.
After it, a field does not need its ancestor to keep the condition.

### Step 3: an unconditional field absorbs the conditional siblings

For every field without `OnTypeNames`, every same-named sibling with `OnTypeNames` merges into it.
`mergeTypeConditions` leaves the unconditional field without conditions.
`mergeValues` appends the children of the absorbed field to the unconditional one.
The absorbed children keep their own conditions from step 2.

This solves problem 4.
In the example, `category { id }` absorbs both `category { owner }` fields.
The merged `category` has the children `id`, `owner` with `[1: ProductA]`, `owner` with `[1: ProductB]`.

### Step 4: merge same-named siblings with equal `OnTypeNames`

`fieldsCanMerge` requires the same name, the same node kind and the same `OnTypeNames`.
Fields with different `OnTypeNames` stay separate, so the field order in the response stays stable.
`mergeTypeConditions` ORs the parent conditions.
`mergeValues` appends the children.
This applies to objects, arrays and scalars alike.

In the example, the two `owner` fields merge at the next recursion level.
The result is one `owner` with `[1: ProductA, ProductB]`.
This solves problems 1, 3 and 5.

## 5. Merging the conditions

`mergeTypeConditions(left, right)` ORs the conditions of `right` into `left`.

1. `left` has no condition at all: keep `left` as it is.
2. `right` has no condition at all: clear both `OnTypeNames` and `ParentOnTypeNames` on `left`.
3. Both have conditions:
   - Collect the groups of both sides.
     A side without groups gives one empty group.
   - When the two sides have different `OnTypeNames`, the merged field cannot keep either.
     Each side's `OnTypeNames` moves into its groups as an entry at `Depth: 0`, and the merged `OnTypeNames` becomes nil.
     This case occurs only in step 3, where `fieldsCanMerge` is not the gate.
   - An empty group means one path had no parent condition.
     The merged field gets no parent condition, because that path always renders.
   - Otherwise every group is added with `addConditionGroup`.

`addConditionGroup` folds a group into an existing group when the two differ at one depth at most.
`(A AND B) OR (A AND C)` becomes `A AND (B OR C)`.
Groups that differ at two depths or more stay separate.
This keeps the issue query at one group, and keeps the plan size small for the common shapes.

Example of two groups that stay separate:

```graphql
... on A { x { ... on C { y { z } } } }
... on B { x { ... on D { y { z } } } }
```

`z` ends with `[2: A, 1: C] OR [2: B, 1: D]`.
A fold would produce `[2: A, B] AND [1: C, D]`, which also renders `z` for `A` with `D`.

### 5.1 What is not minimized

`[1: B] OR [1: B, 0: D]` stays as two groups, although it equals `[1: B]`.
The resolver result is the same, and a subsumption pass is out of scope.
The test row `[S8]` documents this as a characterization, not as a required result.

`deduplicateOnTypeNames` reads names out of a map, so the name order after a fold is not deterministic.
The test harness prints names sorted for this reason.

## 6. The resolver contract

`walkObject` pushes the `__typename` of each object on `typeNames`.
A concrete object without `__typename` pushes nil.
An abstract object without `__typename` is an error, or null when the field is nullable.

`shouldSkipFieldByTypeCondition`:

1. When `ParentOnTypeNames` is not empty and no group matches, skip.
2. When `OnTypeNames` is not nil and the top of the stack is not one of the names, skip.

`parentTypeNamesMatch` indexes the stack with `Depth` from the top.
A depth where the stack holds nil never matches.

The merge relies on the upstream operation to request `__typename` where a condition needs it.
The GraphQL data source planner adds `__typename` in every selection set whose enclosing type is abstract, and in every selection set that contains an inline fragment on a different type.
See `EnterSelectionSet` and `EnterInlineFragment` in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go`.

## 7. Known limits

- `setParentTypeNames` and `walkArray` handle one array level between two objects.
  A nested list `[[Interface]]` is not verified beyond the one-level fixtures.
- The split in step 1 copies the subtree once per type name.
  A fragment on an interface with many implementations under a large selection produces many copies.
  Step 4 merges them again when their subtrees are equal, but the copies exist in between.
- Groups are not minimized, see section 5.1.

## 8. Tests

- `v2/pkg/engine/postprocess/merge_fields_test.go`: the merge rules that predate the groups.
- `v2/pkg/engine/postprocess/merge_fields_parent_types_test.go`: one structural row per shape, with the GraphQL above each row, and a render table for the rows that depend on the runtime type.
- `v2/pkg/engine/postprocess/response_tree_harness_test.go`: `assertResponseTreeEqual` prints both trees and shows a line diff, and `renderResponse` resolves a tree against one JSON input.
- `v2/pkg/engine/resolve/parent_type_groups_test.go`: the resolver side, keyed by runtime type names.
- `execution/engine/parent_type_conditions_test.go`: gateway queries on the accounts fixtures in `execution/federationtesting`, full JSON equality.
