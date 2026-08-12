# Defer Populate Parent IDs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new normalization step that correctly derives `parentDeferId` for every `@__defer_internal`-stamped field after field merging, replacing the stale computation in the flatten step.

**Architecture:** The flatten step (`inlineFragmentExpandDefer`) stamps each field with its own defer ID but no longer assigns `parentDeferId`, because parent relationships are unknowable until after field deduplication runs. A new normalization step (`deferPopulateParentIds`) runs after the `cleanup` stage (which contains `deduplicateFields`) and walks the final merged AST, tracking the current enclosing defer ID as a stack; whenever it encounters a field whose defer ID differs from the enclosing group, it writes the correct `parentDeferId` into the field's `@__defer_internal` directive. This fixes a concrete bug where two parallel top-level `@defer` fragments referencing the same parent field are merged during deduplication — after merging, the "losing" group's fields are nested inside the "winning" group's tree, but their `parentDeferId` still reads `0` (top-level), causing `buildDeferTree` to put both groups in `DeferParallel` when it should build `DeferSequence`. To avoid running the new step on operations with no defers, a `skipCondition` hook is added to `walkerStage` and wired to `inlineDeferVisitor.hasDefers()`.

**Tech Stack:** Go, `v2/pkg/ast`, `v2/pkg/astnormalization`, `v2/pkg/astvisitor`

---

## Why each decision was made

**Why remove `parentDeferId` from the flatten step?**
The flatten step runs before field deduplication. At that point, the AST still has two separate `items` fields (one per parallel defer). The parent relationships visible to the walker at flatten time reflect the original `@defer` nesting, not the post-merge structure. After `MergeFieldsDefer` collapses both `items` fields into one (the lower-numbered defer group wins), the children of the "losing" group end up nested inside the "winning" group's subtree. Their `parentDeferId` — stamped before the merge — still points to `0` (or the wrong parent), making the relationship stale. Assigning parent IDs at flatten time is therefore inherently wrong for the parallel-defer case.

Example — given:
```graphql
{ ... @defer { items { id } }   ... @defer { items { name } } }
```

After flatten (before field merging):
```
items @__defer_internal(id: 1, parentDeferId: 0)   ← fragment 1
  id   @__defer_internal(id: 1, parentDeferId: 0)
items @__defer_internal(id: 2, parentDeferId: 0)   ← fragment 2
  name @__defer_internal(id: 2, parentDeferId: 0)
```

After `MergeFieldsDefer` (id:1 wins, both `items` collapsed into one):
```
items @__defer_internal(id: 1) {
  id   @__defer_internal(id: 1, parentDeferId: 0)
  name @__defer_internal(id: 2, parentDeferId: 0)  ← stale: should be parentDeferId: 1
}
```

`name`'s `parentDeferId` is still `0`. `deferInfoCollector` reads it and sets `DeferDescriptor[2].ParentID = 0`, so `buildDeferTree` produces `DeferParallel(group1, group2)` instead of `DeferSequence(group1, group2)` — parallel execution without group 1's `items` data available.

After the new populate step:
```
items @__defer_internal(id: 1) {
  id   @__defer_internal(id: 1)
  name @__defer_internal(id: 2, parentDeferId: 1)  ← correct
}
```

**Why a post-merge normalization step instead of fixing `MergeFieldsDefer` or `deferInfoCollector`?**
`MergeFieldsDefer` is called once per field pair and only sees two fields at a time — it has no visibility into the broader subtree context needed to recompute parent IDs for all descendants. `deferInfoCollector` lives in the planning phase: the normalization phase should produce a correct AST that the planner can trust without re-deriving it. A dedicated normalization step is the right layer of ownership.

**Why `skipCondition` on `walkerStage` instead of name-matching?**
A closure on the stage struct is self-contained — the condition lives next to the stage it gates, carries no string-matching fragility, and generalises to any future conditional stage without modifying the normalize loop logic.

**Spec compliance and deliberate deviation.**
The GraphQL incremental delivery spec says parallel `@defer` fragments are independent and may be delivered in any order. When two parallel `@defer` fragments reference the same parent field, the spec's own field-merging rules collapse them — the parent field appears once in the response, and each group's unique fields are delivered as incremental patches at sub-paths of that parent. The reference implementation (graphql-js) handles this by executing both groups concurrently: it works because it is in-process — both groups share the same in-memory resolver result for the parent field, so no second fetch is needed.

When the second defer group's fields are nested inside the first group's tree (after field merging), the gateway fetches both data paths — but the second group's fetch only **renders** the incremental fields that belong to it, not the full parent structure. The parent path is fetched for context (to resolve keys and navigate to the right node), but only the group-specific fields are delivered as an incremental chunk. This mirrors how root-node fetches work in federation: a second fetch to the same subgraph walks through parent data but renders only the fields it owns. See the "root nodes" integration test case in `execution/engine/execution_engine_defer_test.go` for a concrete example of this pattern.

The chosen approach makes group 2 sequentially dependent on group 1 when field merging places group 2's fields inside group 1's tree. This is **spec-compliant**: the spec does not mandate parallel execution, only that deferred fragments are not delivered before the initial response. The wire-format consequence is that group 2's `pending.path` becomes `["items"]` instead of `[]`, which correctly communicates to the client that group 2's data is anchored inside the `items` established by group 1. This is an honest and correct representation of the actual dependency.

Example — reference implementation output (both groups at `path: []`, can arrive in same chunk):
```json
{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":[]}],"hasNext":true}
{"incremental":[
  {"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"},
  {"data":{"name":"ItemOne"},"id":"2","subPath":["items",0]}
],"completed":[{"id":"1"},{"id":"2"}],"hasNext":false}
```

Gateway output (group 2 at `path: ["items"]`, delivered after group 1):
```json
{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]}],"completed":[{"id":"2"}],"hasNext":false}
```

The `path: ["items"]` tells the client that group 2's incremental data is anchored at the `items` array delivered by group 1. The client applies group 2's patches relative to that array — semantically equivalent to the reference output, without the double upstream fetch.

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `v2/pkg/ast/ast_field.go` | Add `FieldInternalDeferIDWithDirectiveRef` helper; remove stale TODO in `MergeFieldsDefer` |
| Modify | `v2/pkg/astnormalization/inline_fragment_expand_defer.go` | Remove `parentDeferId` assignment; reset visitor state in `EnterDocument`; expose `hasDefers()`; return visitor from registration func |
| Modify | `v2/pkg/astnormalization/inline_fragment_expand_defer_test.go` | Remove `parentDeferId` from expected outputs (flatten step no longer sets it) |
| Create | `v2/pkg/astnormalization/defer_populate_parent_ids.go` | New post-merge normalization visitor |
| Create | `v2/pkg/astnormalization/defer_populate_parent_ids_test.go` | Tests for the new visitor |
| Modify | `v2/pkg/astnormalization/astnormalization.go` | Add `skipCondition` to `walkerStage`; update normalize loops; store visitor ref; register new stage |

---

## Task 1 — Add `FieldInternalDeferIDWithDirectiveRef` AST helper

**Files:**
- Modify: `v2/pkg/ast/ast_field.go` (after `FieldInternalDeferID`, around line 265)

### Context

The populate visitor needs two things when it enters a field: the defer ID (to compare against the stack) and the directive ref (to append `parentDeferId` when the ID differs). The existing `FieldInternalDeferID` finds the directive ref internally but discards it. Rather than have the visitor do a second lookup, add a variant that returns both in a single pass.

`d.Directives[ref]` is an element of a `[]Directive` slice — it is addressable, so `Arguments.Refs` can be appended in place.

- [ ] **Add the helper**

```go
// FieldInternalDeferIDWithDirectiveRef is like FieldInternalDeferID but also returns the
// directive ref so the caller can mutate the directive without a second lookup.
func (d *Document) FieldInternalDeferIDWithDirectiveRef(fieldRef int) (directiveRef, id int, exists bool) {
	directiveRef, exists = d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !exists {
		return 0, 0, false
	}
	idValue, ok := d.DirectiveArgumentValueByName(directiveRef, []byte("id"))
	if !ok || idValue.Kind != ValueKindInteger {
		return 0, 0, false
	}
	return directiveRef, int(d.IntValueAsInt(idValue.Ref)), true
}
```

- [ ] **Remove the stale TODO in `MergeFieldsDefer`**

In `v2/pkg/ast/ast_field.go` around line 205, remove:
```go
// TODO: need to handle parent id too
```

This TODO existed because `MergeFieldsDefer` was expected to fix up `parentDeferId` after merging. That responsibility now belongs to the `deferPopulateParentIds` normalization step, which derives the correct parent IDs from the final merged AST. The TODO is fully resolved by this plan.

- [ ] **Run the ast package tests to confirm nothing is broken**

```
gotestsum --format=short -- ./v2/pkg/ast/...
```

Expected: all pass.

---

## Task 2 — Write failing tests for the new populate visitor

**Files:**
- Create: `v2/pkg/astnormalization/defer_populate_parent_ids_test.go`

### Context

The `run` and `runWithOptions` helpers (defined elsewhere in the `astnormalization` package) accept a visitor-registration function, parse the input query against `testDefinition`, run the walker, and compare the printed AST to the expected string. The input queries below already contain `@__defer_internal` directives as if the flatten step had already run — the populate step only adds `parentDeferId` arguments.

These tests will **fail** until Task 3 creates the visitor.

- [ ] **Create the test file**

```go
package astnormalization

import "testing"

func TestDeferPopulateParentIds(t *testing.T) {
	t.Run("no deferred fields - no change", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog {
					name
				}
			}`,
			`
			query dog {
				dog {
					name
				}
			}`, runOptions{indent: true})
	})

	t.Run("single top-level defer - no parent to assign", func(t *testing.T) {
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("genuinely nested defers - inner gets parentDeferId from outer", func(t *testing.T) {
		// Simulates: ... @defer { dog { ... @defer { name } } }
		// After flatten: dog(id:1), name(id:2) with no parentDeferId.
		// Populate step must derive name's parentDeferId=1 from the enclosing dog field.
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 2)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("parallel defers merged into one tree - sibling gets parentDeferId from winning group", func(t *testing.T) {
		// Simulates: ... @defer { dog { name } }  ... @defer { dog { nickname } }
		// After flatten+merge: dog(id:1) wins; nickname(id:2) ends up inside dog(id:1).
		// Populate step must assign nickname's parentDeferId=1.
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					name @__defer_internal(id: 1)
					nickname @__defer_internal(id: 2, parentDeferId: 1)
				}
			}`, runOptions{indent: true})
	})

	t.Run("parallel defers at depth - nested sibling gets correct parentDeferId", func(t *testing.T) {
		// Simulates: ... @defer { dog { extra { noString } } }  ... @defer { dog { extra { string } } }
		// After flatten+merge: dog(id:1), extra(id:1) win; string(id:2) lands inside extra(id:1).
		// string's parentDeferId must be 1 (the enclosing extra/dog group), not 0.
		runWithOptions(t, deferPopulateParentIds, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						noString @__defer_internal(id: 1)
						string @__defer_internal(id: 2)
					}
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						noString @__defer_internal(id: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`, runOptions{indent: true})
	})

}
```

- [ ] **Run tests to confirm they fail (visitor does not exist yet)**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/... -run TestDeferPopulateParentIds
```

Expected: compile error (`undefined: deferPopulateParentIds`).

---

## Task 3 — Implement the populate visitor

**Files:**
- Create: `v2/pkg/astnormalization/defer_populate_parent_ids.go`

### Context

The visitor registers `EnterField` and `LeaveField` hooks and maintains a `deferStack` of `(id, fieldRef)` pairs for ancestor fields that carry `@__defer_internal`, innermost last.

Storing `fieldRef` alongside `id` means `LeaveField` can decide whether to pop by comparing `ref` to the stack top's `fieldRef` — no AST lookup needed on leave.

When a field's defer ID differs from the current stack top's ID, the stack top's ID is the correct `parentDeferId`. The visitor appends the argument directly using the directive ref already returned by `FieldInternalDeferIDWithDirectiveRef` — no second scan needed.

- [ ] **Create the file**

```go
package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func deferPopulateParentIds(walker *astvisitor.Walker) {
	visitor := &deferPopulateParentIdsVisitor{Walker: walker}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
}

type deferStackEntry struct {
	id       int
	fieldRef int
}

type deferPopulateParentIdsVisitor struct {
	*astvisitor.Walker
	operation  *ast.Document
	deferStack []deferStackEntry // (id, fieldRef) of ancestor deferred fields, innermost last
}

func (v *deferPopulateParentIdsVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
	v.deferStack = v.deferStack[:0]
}

func (v *deferPopulateParentIdsVisitor) EnterField(ref int) {
	directiveRef, id, exists := v.operation.FieldInternalDeferIDWithDirectiveRef(ref)
	if !exists {
		return
	}

	// When the enclosing defer group differs from this field's group, the
	// enclosing group is the correct parent. Append parentDeferId directly
	// to the already-found directive ref — no second lookup needed.
	if len(v.deferStack) > 0 {
		if enclosing := v.deferStack[len(v.deferStack)-1].id; enclosing != id {
			argRef := v.operation.AddIntArgument("parentDeferId", enclosing)
			v.operation.Directives[directiveRef].Arguments.Refs = append(
				v.operation.Directives[directiveRef].Arguments.Refs, argRef,
			)
			v.operation.Directives[directiveRef].HasArguments = true
		}
	}

	v.deferStack = append(v.deferStack, deferStackEntry{id: id, fieldRef: ref})
}

func (v *deferPopulateParentIdsVisitor) LeaveField(ref int) {
	if len(v.deferStack) > 0 && v.deferStack[len(v.deferStack)-1].fieldRef == ref {
		v.deferStack = v.deferStack[:len(v.deferStack)-1]
	}
}
```

- [ ] **Run the new tests — they should pass now**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/... -run TestDeferPopulateParentIds
```

Expected: all pass.

---

## Task 4 — Fix the flatten step

**Files:**
- Modify: `v2/pkg/astnormalization/inline_fragment_expand_defer.go`

### Context

Three changes:

1. **Reset state in `EnterDocument`** — `currentDeferId` and `defers` are never reset between operations today. When the same `OperationNormalizer` instance is reused (common in hot-path usage), `currentDeferId` from a previous operation bleeds into the next one. Fix this in `EnterDocument`.

2. **Stop assigning `parentDeferId`** — `addInternalDeferDirective` currently reads `deferInfo.parentDeferId` from the active-defer stack and passes it to `AddDeferInternalDirectiveToField`. The stack reflects the original `@defer` nesting at flatten time, not the post-merge structure. Always pass `0`; the new populate step derives the correct value from the merged AST.

3. **Expose `hasDefers() bool` and return the visitor** — `setupOperationWalkers` needs to store the visitor so the skip condition for the populate stage can read it. Change the registration function to return `*inlineFragmentExpandDeferVisitor`. Add `hasDefers()` to expose whether any defer was activated in the last walk.

- [ ] **Apply all three changes**

```go
// EnterDocument — add resets
func (f *inlineFragmentExpandDeferVisitor) EnterDocument(operation, _ *ast.Document) {
	f.operation = operation
	f.currentDeferId = 0
	f.defers = f.defers[:0]
}

// addInternalDeferDirective — always pass parentDeferId=0
func (f *inlineFragmentExpandDeferVisitor) addInternalDeferDirective(fieldRef int) {
	deferInfo := f.defers[len(f.defers)-1]
	f.operation.AddDeferInternalDirectiveToField(fieldRef, deferInfo.id, deferInfo.label, 0)
}

// hasDefers — new method
func (f *inlineFragmentExpandDeferVisitor) hasDefers() bool {
	return f.currentDeferId > 0
}

// inlineFragmentExpandDefer — return the visitor
func inlineFragmentExpandDefer(walker *astvisitor.Walker) *inlineFragmentExpandDeferVisitor {
	visitor := &inlineFragmentExpandDeferVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)
	return visitor
}
```

- [ ] **Run astnormalization tests to see which flatten tests now fail**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/... -run TestInlineFragmentExpandDefer
```

Expected: failures on any test whose expected output contains `parentDeferId`.

---

## Task 5 — Update flatten step tests

**Files:**
- Modify: `v2/pkg/astnormalization/inline_fragment_expand_defer_test.go`

### Context

The flatten step no longer assigns `parentDeferId`. Tests that expected `@__defer_internal(id: 2, parentDeferId: 1)` in flatten-step output must be updated to expect `@__defer_internal(id: 2)`. The correct `parentDeferId` will now be verified by the populate-step tests (Task 2).

The test case with genuinely nested defers (the `with interface type` test) has `barkVolume @__defer_internal(id: 2, parentDeferId: 1)` — this must become `barkVolume @__defer_internal(id: 2)`.

- [ ] **Remove all `parentDeferId` from flatten test expected outputs**

In `TestInlineFragmentExpandDefer` / `with interface type`, change:
```
barkVolume @__defer_internal(id: 2, parentDeferId: 1)
```
to:
```
barkVolume @__defer_internal(id: 2)
```

Search for any other occurrences of `parentDeferId` in `inline_fragment_expand_defer_test.go` and apply the same removal.

- [ ] **Run the flatten tests — they should all pass again**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/... -run TestInlineFragmentExpandDefer
```

Expected: all pass.

---

## Task 6 — Add `skipCondition` to `walkerStage` and update normalize loops

**Files:**
- Modify: `v2/pkg/astnormalization/astnormalization.go`

### Context

`walkerStage` is an internal struct used only within this file. Adding a `skipCondition func() bool` field requires no public API change. Both `NormalizeOperation` and `NormalizeNamedOperation` iterate the same `o.operationWalkers` slice — both loops must be updated identically.

- [ ] **Add `skipCondition` to `walkerStage`**

```go
type walkerStage struct {
	name          string
	walker        *astvisitor.Walker
	skipCondition func() bool // optional; stage is skipped when this returns true
}
```

- [ ] **Update both normalize loops**

In `NormalizeOperation`:
```go
for i := range o.operationWalkers {
	if sc := o.operationWalkers[i].skipCondition; sc != nil && sc() {
		continue
	}
	o.operationWalkers[i].walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}
}
```

Apply the identical change in `NormalizeNamedOperation`.

- [ ] **Run the full astnormalization suite to confirm nothing broke**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/...
```

Expected: all pass (no stage has a skipCondition yet, so behaviour is unchanged).

---

## Task 7 — Wire the new stage into `setupOperationWalkers`

**Files:**
- Modify: `v2/pkg/astnormalization/astnormalization.go`

### Context

`OperationNormalizer` needs to store the `inlineFragmentExpandDeferVisitor` pointer so the skip closure can call `hasDefers()` after the inlineDefer stage has run. The closure captures the pointer at setup time; it reads `currentDeferId` at call time (after the inlineDefer walker has already walked the operation), so it reflects the current operation correctly.

The new stage must be placed **after** the `cleanup` stage (which contains `deduplicateFields`) and **before** `variablesProcessing`. Both stages are gated on `o.options.inlineDefer`.

- [ ] **Add the visitor field to `OperationNormalizer`**

```go
type OperationNormalizer struct {
	operationWalkers []walkerStage

	removeOperationDefinitionsVisitor *removeOperationDefinitionsVisitor
	inlineDeferVisitor                *inlineFragmentExpandDeferVisitor // non-nil when WithInlineDefer() is set

	options              options
	definitionNormalizer *DefinitionNormalizer
}
```

- [ ] **Capture the visitor and register the new stage in `setupOperationWalkers`**

Change the existing `inlineDefer` block from:
```go
if o.options.inlineDefer {
	inlineDefer := astvisitor.NewWalkerWithID(8, "Inline defer")
	inlineFragmentExpandDefer(&inlineDefer)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "inlineDefer",
		walker: &inlineDefer,
	})
}
```

to:
```go
if o.options.inlineDefer {
	inlineDefer := astvisitor.NewWalkerWithID(8, "Inline defer")
	o.inlineDeferVisitor = inlineFragmentExpandDefer(&inlineDefer)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "inlineDefer",
		walker: &inlineDefer,
	})
}
```

Then, after the `cleanup` stage append (after line ~301 in the current file), add:

```go
if o.options.inlineDefer {
	populateParentIds := astvisitor.NewWalkerWithID(8, "PopulateDeferParentIds")
	deferPopulateParentIds(&populateParentIds)
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:          "populateDeferParentIds",
		walker:        &populateParentIds,
		skipCondition: func() bool { return !o.inlineDeferVisitor.hasDefers() },
	})
}
```

- [ ] **Run the full astnormalization suite**

```
gotestsum --format=short -- ./v2/pkg/astnormalization/...
```

Expected: all pass.

- [ ] **Run the full engine plan and resolve suites** (the planning phase reads `parentDeferId` via `deferInfoCollector`)

```
gotestsum --format=short -- ./v2/pkg/engine/...
```

Expected: all pass. If any test fails, the expected `parentDeferId` values in those tests need updating to match the now-correct values.

- [ ] **Run the execution engine integration tests**

```
gotestsum --format=short -- ./execution/engine/... -run TestExecutionEngine
```

Expected: all pass, including the parallel defer test at line 2094.

---

## Self-review checklist

**Spec coverage:**
- ✅ Flatten step stops assigning `parentDeferId` — Task 4
- ✅ State reset per document in flatten step — Task 4
- ✅ `hasDefers()` exposed — Task 4
- ✅ `FieldInternalDeferIDWithDirectiveRef` AST helper — Task 1
- ✅ Stale `MergeFieldsDefer` TODO removed — Task 1
- ✅ New populate visitor with correct stack logic — Task 3
- ✅ Tests for populate visitor (no-defer, single, genuinely nested, parallel-merged) — Task 2
- ✅ `skipCondition` on `walkerStage` — Task 6
- ✅ Both normalize loops updated — Task 6
- ✅ New stage wired after `cleanup`, gated on `inlineDefer` option — Task 7
- ✅ Visitor reference stored in normalizer — Task 7
- ✅ Flatten tests updated — Task 5

**Placeholder scan:** None found.

**Type consistency:**
- `FieldInternalDeferIDWithDirectiveRef(fieldRef int) (directiveRef, id int, exists bool)` — defined Task 1, called Task 3 ✅
- `deferPopulateParentIds(walker *astvisitor.Walker)` — defined Task 3, used Task 7 ✅
- `inlineFragmentExpandDefer(walker *astvisitor.Walker) *inlineFragmentExpandDeferVisitor` — defined Task 4, used Task 7 ✅
- `hasDefers() bool` — defined Task 4, called in closure Task 7 ✅
- `walkerStage.skipCondition func() bool` — defined Task 6, set Task 7 ✅
