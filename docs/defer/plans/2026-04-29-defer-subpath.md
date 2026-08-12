# Defer subPath + Collector Path Fix

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to drive task-by-task execution.

**Goal:** Make the resolver emit `"subPath":[...]` on each `incremental` item that lands at a runtime position deeper than the defer's mount path. Cover both:
- list-iteration cases (`subPath: [0]`, `[0, 1]`, …) when the defer is mounted inside list field(s)
- nested-object cases (`subPath: ["info"]`, …) when the deferred fragment merges with non-deferred siblings and the resolver splits the deferred subtree into multiple incremental envelopes

This in turn requires correcting the collector: it currently records `descriptor.path` from the *first deferred field's enclosing-field chain*, which is wrong whenever the deferred fragment merges with non-deferred siblings (e.g. `info { email }` + deferred `info { phone }` → merged `info` is non-deferred, only `phone` carries the directive — collector wrongly anchors descriptor.path at `["user","info"]` instead of `["user"]`).

## Three problems, one combined plan

1. **Collector path bug.** The current `EnterField`-based deferInfoCollector reaches into the field-walker's `Path`, which gives the path of the *first deferred field*, not the path of the inline fragment. After @defer expansion + field merging, those can differ.

2. **subPath rule too narrow.** The on-disk `printDeferSubPathIfAny` (premature implementation) only emits list indices. The correct rule is `subPath = runtime_path − descriptor.path` — strip the descriptor's named segments from the runtime path; everything left (names AND indices) goes in subPath.

3. **Test expectations.** A subset of integration tests in `execution_engine_defer_test.go` currently hard-code:
   - wrong `pending[].path` values (because descriptor.path is wrong)
   - missing `subPath` entries on incremental items

   Both will surface as test failures once #1 and #2 land. Test rewrites come last.

## Design — the collector fix

Normalization and planning are separate phases; we cannot pass state from `inline_fragment_expand_defer.go` (which knows the inline-fragment path at expansion time) into the planner. So the collector keeps reading `@__defer_internal` directives from the AST, but it switches the callback it uses and applies a list-anchor truncation rule.

**Callback switch.**

- **Today:** `EnterField` → reads the directive on the current field, takes `walker.Path` (path *to* the field's parent) as descriptor.path. Wrong because the first field encountered in document order isn't necessarily a direct child of the defer fragment's selection set after merging.
- **Fix:** `EnterSelectionSet` → manually iterate the selection set's `FieldSelections`. For each field, read its `@__defer_internal` id. Record the descriptor for any unseen id, with descriptor.path computed from the SELECTION SET's path (the inline fragment's mount), then post-processed by the list-anchor truncation rule below.

**List-anchor truncation rule (deviation from spec).**

When the inline fragment's mount path crosses a list field (e.g. `users { friends { ...@defer { name } } }`, mount = `["users","friends"]` — both `users` and `friends` are lists), the spec's reference implementation would allocate a fresh defer id per runtime list iteration. We don't do that — one AST defer = one id — so a single pending entry has to anchor somewhere meaningful while subPath disambiguates across iterations.

Anchor at the **outermost list-field ancestor** of the inline fragment (inclusive). Everything past that goes into subPath when the data is rendered. If the mount path has no list ancestors, descriptor.path is the full mount path.

Algorithm in collector:
1. Compute the candidate descriptor.path from `walker.Path` (selection-set path), stripping the operation-type root segment.
2. Walk the ancestor chain (or the candidate path with a parallel field-type lookup) to identify which segments correspond to list fields. We need the schema definition to determine list-ness.
3. Find the outermost list segment's index. Truncate the candidate path to end at that index (inclusive).
4. If no list ancestors: keep the full candidate path.

Worked through:

- `user { name info { email } ... @defer { info { phone }, title } }` after normalization:
  - user.selectionSet: { name (no dir), info (no dir — merged), title (id=1) }
  - user.info.selectionSet: { email (no dir), phone (id=1) }
  - Collector visits user.selectionSet first → finds title with id=1. Candidate path = `["user"]`. No list ancestors → descriptor.path = `["user"]`. ✓
  - Then visits user.info.selectionSet → finds phone with id=1, seen → skips. ✓

- `... @defer { hero { ... @defer { name } } }` after normalization:
  - root.selectionSet: { hero (id=1) }
  - hero.selectionSet: { name (id=2) }
  - root → finds hero with id=1. Candidate = `[]`. No list ancestors → descriptor.path[1] = `[]`. ✓
  - hero.selectionSet → finds name with id=2. Candidate = `["hero"]`. No list ancestors → descriptor.path[2] = `["hero"]`. ✓

- `users { ...@defer { name } }`:
  - root.selectionSet: { users (no dir) }
  - users.selectionSet (per-item): { name (id=1) }
  - users.selectionSet → finds name with id=1. Candidate = `["users"]`. `users` is a list → outermost list ancestor = `users` (the only candidate segment) → truncate at `users` (inclusive) → descriptor.path = `["users"]`. ✓

- `users { friends { ...@defer { name } } }`:
  - friends.selectionSet → finds name with id=1. Candidate = `["users","friends"]`. Both `users` and `friends` are lists → outermost = `users` → truncate at `users` → descriptor.path = `["users"]`. ✓ (deviation in action)

- `a { users { b { ...@defer { name } } } }`:
  - b.selectionSet → finds name with id=1. Candidate = `["a","users","b"]`. `users` is the only list → outermost = `users` → truncate at `users` (inclusive) → descriptor.path = `["a","users"]`. ✓

The "first unseen id wins" rule is robust: every `@__defer_internal` in the same fragment carries the same id, and the OUTERMOST selection set containing any of them yields the candidate path. The list-anchor truncation then folds list-iteration ambiguity into subPath rather than into pending.

## Design — subPath rule

`subPath = runtime_path − descriptor.path`

Implementation: walk runtime path; track a cursor into descriptor.path; whenever the current runtime segment matches the cursor's name in descriptor.path, advance the cursor and skip the segment (don't add to subPath). Everything else (unmatched names, list indices) goes into subPath.

Worked through:

| Case | descriptor.path | runtime path at envelope-close | subPath |
|---|---|---|---|
| `hero { ...@defer { name } }` | `["hero"]` | `["hero"]` | (omitted) |
| `... @defer { hero { name } }` | `[]` | `[]` | (omitted) |
| `users { ...@defer { name } }`, user[0] | `["users"]` (anchored at outermost list) | `["users", 0]` | `[0]` |
| `users { friends { ...@defer { name } } }`, user[0] friend[1] | `["users"]` (anchored at outermost list `users`, not the inner `friends` list) | `["users",0,"friends",1]` | `[0,"friends",1]` |
| `a { users { b { ...@defer { name } } } }`, user[0] | `["a","users"]` (anchored at outermost list) | `["a","users",0,"b"]` | `[0,"b"]` |
| `user { ...@defer { info { phone }, title } }`, title item | `["user"]` (no list ancestors) | `["user"]` | (omitted) |
| `user { ...@defer { info { phone }, title } }`, phone item | `["user"]` (no list ancestors) | `["user","info"]` | `["info"]` |

Emit `,"subPath":[...]` only when the result is non-empty. JSON segments preserve type: integer indices render as bare numbers, names as quoted strings.

## What's already on disk

- `v2/pkg/engine/resolve/const.go` — `literalSubPath = []byte("subPath")` added.
- `v2/pkg/engine/resolve/resolvable.go` — `printDeferSubPathIfAny()` added with the **wrong** rule (indices only). To be replaced in Task 2 below.

The const literal is fine and stays. The function body is rewritten in Task 2.

## Tasks

### Task 1: Collector — switch to `EnterSelectionSet`, fix `descriptor.path`, apply list-anchor truncation

**File:** `v2/pkg/engine/plan/defer_info_collector.go`

The collector needs the schema definition for list-ness lookup, so add `definition *ast.Document` to the struct and capture it in `EnterDocument`.

- [ ] Add `definition *ast.Document` field to `deferInfoCollector`.
- [ ] In `EnterDocument(operation, definition *ast.Document)` capture both.
- [ ] Replace the `RegisterEnterFieldVisitor` registration with `RegisterEnterSelectionSetVisitor`.
- [ ] Implement `EnterSelectionSet(ref int)`:
  - Iterate the selection set's field selections via `c.operation.SelectionSetFieldSelections(ref)`.
  - For each `fieldSelectionRef`, get the field ref via `c.operation.Selections[fieldSelectionRef].Ref`.
  - Call `c.operation.FieldDeferInfo(fieldRef)` to get `(id, label, parentID, ok)`.
  - On `ok && id != 0`:
    - If `c.descriptors[id]` already exists, continue (first-occurrence wins).
    - Otherwise build descriptor with `ID/ParentID/Label` from the directive and `Path: c.deferPath()`.
- [ ] Drop the `EnterField` callback. The "first field" heuristic becomes "first selection set containing a deferred direct child."

#### List-anchor truncation in `deferPath()`

Walker.Path does NOT contain array indices during operation walks (verified at `astvisitor/visitor.go:1446–1493` — only `FieldName` and `InlineFragmentName` items are pushed). So list-ness must be derived from the schema.

- [ ] Rewrite `deferPath()` to:
  1. Build the candidate path by iterating `c.Walker.Path[1:]` (skipping the operation-type root segment) and collecting items where `Kind == ast.FieldName`.
  2. Walk `c.Walker.Ancestors` in parallel — for each `NodeKindField` ancestor, determine whether that field's type is a list. The first list-typed field marks the truncation point: descriptor.path keeps everything up to and INCLUDING that field; nothing after.
  3. If no list-typed Field ancestor exists, return the full candidate path.

- [ ] Implement the list-ness check using the existing AST helpers:
  ```go
  // For each Field ancestor at index i in c.Walker.Ancestors:
  //   - find the parent type's field definition matching this field's name
  //   - read the field definition's TypeRef
  //   - check whether it's a list (possibly wrapped in NonNull)
  ```
  Concretely:
  - Use `c.Walker.Ancestors` to iterate. For each `ancestor.Kind == ast.NodeKindField`:
    - Determine the parent type at that position. The walker tracks `w.TypeDefinitions` — use `c.Walker.TypeDefinitions[fieldDepthIndex]` to get the enclosing type.
    - Use `c.definition.NodeFieldDefinitions(parentType)` and match by name (`c.definition.FieldDefinitionNameBytes(fieldDefRef)` against `c.operation.FieldNameBytes(ancestor.Ref)`).
    - Get the TypeRef via `c.definition.FieldDefinitionType(fieldDefRef)` and check via `c.definition.TypeIsList(typeRef)` (or walk `TypeKind` chain — `TypeKindNonNull` may wrap a `TypeKindList`).
  - The FIRST (outermost) Field ancestor whose type is a list is the truncation anchor. descriptor.path = candidate-path[: index_of_that_field + 1] (inclusive).
  - Verify the existing `TypeIsList` helper exists or use `definition.Types[ref].TypeKind == ast.TypeKindList` plus a NonNull-unwrap loop.

- [ ] Verify by reading the collector and reasoning through the worked-through cases above (especially `users { friends { ...@defer { name } } }` truncating at `users`, and `a { users { b { ...@defer { name } } } }` truncating at `a.users`).

- [ ] Run resolver/plan/normalization unit tests:
  ```
  gotestsum --format=short -- ./v2/pkg/engine/plan/... ./v2/pkg/astnormalization/... -count=1
  ```

### Task 2: Resolver — rewrite `printDeferSubPathIfAny` with the correct rule

**File:** `v2/pkg/engine/resolve/resolvable.go`

- [ ] Replace `printDeferSubPathIfAny()` with a version that takes the current defer's descriptor (looked up via `r.deferDescriptors[r.deferID]`) and computes `subPath = runtime_path − descriptor.path`.
- [ ] Algorithm:
  1. Look up `descriptor := r.deferDescriptors[r.deferID]`. If missing, no subPath (defensive).
  2. Walk `r.path` once; maintain a cursor `descIdx` into `descriptor.Path`.
  3. For each `r.path[i]`:
     - If `descIdx < len(descriptor.Path)` AND `r.path[i].Name == descriptor.Path[descIdx]` → advance `descIdx`, do not add to subPath.
     - Otherwise → mark "have subPath", remember the segment.
  4. If "have subPath" is false, return without emitting.
  5. Otherwise emit `,"subPath":[...]` listing each remembered segment in order, with names quoted and indices bare.
- [ ] Reuse the segment-emission pattern from `renderPath` for consistency (`unsafebytes.StringToBytes` for indices, quoted bytes for names).
- [ ] Keep the call site inside `printDeferIdAndErrors` between `id` and `errors`.
- [ ] Run resolver unit tests:
  ```
  gotestsum --format=short -- ./v2/pkg/engine/resolve/... -count=1
  ```

### Task 3: Run integration suite, capture failures

- [ ] Run:
  ```
  gotestsum --format=short -- ./execution/engine/... -count=1 -run TestExecutionEngine_Execute_Defer
  ```
- [ ] Two classes of failures expected:
  1. **Wrong `pending[].path`** — cases where descriptor.path was previously misanchored (e.g., `["user","info"]` instead of `["user"]`).
  2. **Missing `subPath`** — incremental items that now get a subPath they didn't have before.
- [ ] Confirm the failure count is roughly within the order of "tens of subtests" (precise number depends on how many tests hit either class). If it's something dramatic like the entire suite, stop and re-evaluate.

### Task 4: Update failing `expectedResponse` literals

For each failing test case:

1. Read the test case's `expectedResponse` block.
2. Capture the `actual:` JSON from the test diff (resolver is correct after Tasks 1–2; tests are stale).
3. Apply an Edit using the existing literal as `old_string` and the new (correctly-anchored, subPath-included) literal as `new_string`.
4. Convert escaped JSON in the diff back to literal characters: `\"` → `"`, `\n` → real newline.

**Hard rules during the test sweep:**

- **Use the Edit tool only.** No `python`, `sed`, `awk`, `echo >`. Applies to subagents too.
- **No commits.** No `git commit` from me; the user reviews diffs.
- **Use `gotestsum` not `go test`.**
- **Do one test category at a time** for a reviewable audit trail. Suggested order: `nested_list_entities` → `named_fragments_with_defer` → list-touching cases inside `defer_on_non_entity_field` / `entity_-_distributed_fields` → object-split cases (e.g., `defer nested object with duplicated non defered object`).
- **Do not modify any other test file.**

### Task 5: Add nested-list integration test case

We don't currently exercise the `users.friends` deeply-nested-list scenario in the integration suite. Add one new subtest to lock in the truncation rule and the mixed-type subPath.

**File:** `execution/engine/execution_engine_defer_test.go`

- [ ] Locate a suitable parent block (e.g. inside `nested_list_entities` if it has a fitting schema, or alongside it). Reuse an existing schema/datasource fixture that exposes a list-of-lists shape (e.g. items → subItems).
- [ ] Add a subtest with a query of the shape:
  ```graphql
  {
    items {
      subItems {
        ... @defer { description }
      }
    }
  }
  ```
- [ ] Expected response shape (with our deviation):
  - Initial: `{"data":{"items":[{"subItems":[{},{}]},{"subItems":[{}]}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}`
  - Subsequent: `{"incremental":[{"data":{"description":"..."},"id":"1","subPath":[0,"subItems",0]}, {"data":{"description":"..."},"id":"1","subPath":[0,"subItems",1]}, {"data":{"description":"..."},"id":"1","subPath":[1,"subItems",0]}],"completed":[{"id":"1"}],"hasNext":false}`
  - (Adjust counts and field names to match the existing fixture's data.)
- [ ] Verify that:
  - `pending[0].path` is anchored at the OUTERMOST list field (`["items"]`), not at the inner list (`["items","subItems"]`).
  - Each `incremental[i].subPath` is a mixed-type array starting with the outer list's index and including the inner list's field name and index.

### Task 6: Verify

- [ ] Run:
  ```
  gotestsum --format=short -- ./execution/engine/... -count=1 -run TestExecutionEngine_Execute_Defer
  ```
  Expected: all subtests green, including the new nested-list case.
- [ ] Run the full execution/engine suite:
  ```
  gotestsum --format=short -- ./execution/engine/... -count=1
  ```
- [ ] Run the lower-level packages to confirm no regression:
  ```
  gotestsum --format=short -- ./v2/pkg/engine/plan/... ./v2/pkg/engine/resolve/... ./v2/pkg/astnormalization/... ./v2/pkg/ast/... -count=1
  ```

## Out of scope

- **Per-list-item id allocation** (graphql-js's `_ensureId` per runtime DeliveryGroup). Still one AST defer = one id. subPath is the disambiguator.
- **Spec-lazy `pending`** — nested defers all go in initial `pending`.
- **Comprehensive spec-fixture parity tests.** Those come in a later iteration per agreement.

## Verification checklist

- [ ] Collector uses `EnterSelectionSet`; descriptor.path is truncated at the outermost list-field ancestor when one exists (verified via the worked-through table).
- [ ] `printDeferSubPathIfAny` emits both names and indices per the `runtime − descriptor` rule, only when the result is non-empty.
- [ ] All `TestExecutionEngine_Execute_Defer/*` subtests pass.
- [ ] **New nested-list test case passes** with `pending[0].path` anchored at the outer list and `subPath` carrying mixed-type segments.
- [ ] Full `execution/engine` package green.
- [ ] No regression in `v2/pkg/engine/plan/...`, `v2/pkg/engine/resolve/...`, `v2/pkg/astnormalization/...`, `v2/pkg/ast/...`.
- [ ] `git status` shows changes only in: `defer_info_collector.go`, `resolvable.go`, `const.go` (unchanged from premature impl), `execution_engine_defer_test.go`. Nothing else.
- [ ] No commits.
