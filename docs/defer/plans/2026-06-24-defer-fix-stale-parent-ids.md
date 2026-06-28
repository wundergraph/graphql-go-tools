# Fix Stale `parentDeferId` After Field Merging

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **User preferences:** No `git add`/`git commit` steps — the user manages commits. Run Go tests with `gotestsum --format=short -- <pkg> -run <Test>`. Use the Edit tool for all file edits; run `gofmt -w` after editing Go files. Avoid novel Bash command shapes (see project memory).

**Goal:** Make `deferPopulateParentIds` repair `parentDeferId` values that field
merging has invalidated, so nested `@defer` fields under a merged-away or
discarded parent get a correct (reachable) parent — or none. Fixes the two
failing engine subtests under `merged/discarded defer parent`.

**Builds on:** `docs/superpowers/plans/2026-05-20-defer-populate-parent-ids.md`
(original rule). This plan extends that rule from "add when missing" to also
"repair when stale".

**Tech Stack:** Go; package
`github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization`; AST helpers in
`v2/pkg/ast`.

---

## Background

`@__defer_internal(id, parentDeferId)` is stamped during the **expand** stage
(`deferExpandIntoInternal`) from lexical `@defer` nesting. The **cleanup** stage
runs `deduplicateFields`, which merges duplicate fields via
`ast.MergeFieldsDefer`. A merge can strip a defer directive off a field (it
collapses into a non-deferred sibling, or loses to a lower-id defer), so that
defer **id disappears from the document** — but descendant fields relocated by
the merge still carry a `parentDeferId` pointing at the vanished id.

`deferPopulateParentIds` runs after merging. Today it only **adds** a missing
parent and otherwise leaves stamped values untouched; it never **repairs** a
stale value, leaving dangling parent ids on nested defers.

### What `parentDeferId` means at runtime (verified — do not re-derive)

`parentDeferId` is **physical tree-ancestry in the final merged tree**, not
lexical `@defer` nesting:

- `v2/pkg/engine/resolve/resolvable.go` (`isDeferAncestor` / `collectDeferFields`):
  to render a nested defer, the resolver traverses *into* a deferred object only
  if that object's id is in the current defer's **parent chain**. A field whose
  enclosing deferred object is not in its chain is **unreachable**.
- `v2/pkg/engine/plan/defer_info_collector.go`: a defer's
  `DeferDescriptor.ParentID` is taken from the **first** field (document order)
  carrying that id, and feeds both `isDeferAncestor` (reachability) and
  `v2/pkg/engine/postprocess/build_defer_tree.go` (delivery order / dependency).

Implications that fix the design:
- The nearest enclosing deferred ancestor in the merged tree (the existing
  `deferStack`) is the correct parent for reachability.
- A *valid* stamped parent (id still present) must be **kept** — its chain already
  contains the physical ancestors and encodes delivery ordering for
  genuinely-nested defers.
- Recomputing from the *lexical* parent graph is **wrong**: two top-level sibling
  `@defer`s whose objects merge leave a surviving leaf under the merged object;
  that leaf must be re-parented to the merged object (`0 → 1`), which a lexical
  model cannot produce and which would make the leaf unreachable.

## Rule

For each `@__defer_internal` field, classify its `parentDeferId`:

| state | action |
|---|---|
| **unset** | add nearest enclosing deferred ancestor (deferStack top, if its id differs) — *unchanged current behavior* |
| **set, id still present in the document** | **keep** (valid chain / ordering) |
| **set, id absent (stale)** | **repair** → nearest enclosing deferred ancestor with a different id; **remove** the argument if there is none |

### Case validation

- **test 1 (discarded parent):** `info`/`user` non-deferred,
  `phone @id2(parent 1)`, no field carries id 1 → stale, no enclosing → **remove**
  → `phone @id2`.
- **test 2 (merged parent):** `info @id1 { …, phone @id3(parent 2) }`, id 2 absent
  → stale, enclosing `info@1` → **repair to 1**.
- **two top-level sibling defers merge:** surviving leaf `b @id2` is *unset* and
  sits in `nestedInfo @id1` → unset branch adds `1` (reachable).
- **fully-nested / nested-defers tests:** parents still present → kept.
- **defer spanning depths:** valid parent kept; chain still contains the physical
  ancestor, so deep fields stay reachable.

No changes to the expand stage, `MergeFieldsDefer`, or the planner. Public rule
signature unchanged, so `astnormalization.go` wiring and the unit-test harness are
untouched.

---

## Tasks

### Task 1 — Add failing rule-level unit tests (TDD)

File: `v2/pkg/astnormalization/defer_populate_parent_ids_test.go`. Uses the
existing `run(t, deferPopulateParentIds, testDefinition, in, out, withIndent())`.
`testDefinition` has `Dog { name, nickname, extra: DogExtra }` and
`DogExtra { string, noString, … }`.

- [ ] **stale parent removed** (collapsed into non-deferred path):
  ```graphql
  # in
  query dog { dog { extra { string @__defer_internal(id: 2, parentDeferId: 1) } } }
  # out
  query dog { dog { extra { string @__defer_internal(id: 2) } } }
  ```
- [ ] **stale parent repaired** (merged into enclosing defer):
  ```graphql
  # in
  query dog { dog @__defer_internal(id: 1) { extra @__defer_internal(id: 1) {
      string @__defer_internal(id: 3, parentDeferId: 2) } } }
  # out
  query dog { dog @__defer_internal(id: 1) { extra @__defer_internal(id: 1) {
      string @__defer_internal(id: 3, parentDeferId: 1) } } }
  ```
- [ ] **valid parent preserved** (sibling defer still present), in == out:
  ```graphql
  query dog { dog { name @__defer_internal(id: 1)
      nickname @__defer_internal(id: 2, parentDeferId: 1) } }
  ```
- [ ] **unset parent still added** (lock current behavior): `extra @id1 { string @id2 }`
  → `string` gains `parentDeferId: 1`.
- [ ] Keep the 5 existing tests unchanged.
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/... -run 'TestDeferPopulateParentIds'` — new cases fail, existing pass.

### Task 2 — Implement repair in the rule

File: `v2/pkg/astnormalization/defer_populate_parent_ids.go`. Full replacement:

```go
package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

var parentDeferIDArgName = []byte("parentDeferId")

// deferPopulateParentIds finalizes the parentDeferId of every
// @__defer_internal-stamped field after field merging.
//
// parentDeferId is physical tree-ancestry: a deferred field must be parented to
// the nearest enclosing deferred object so the resolver can traverse into it
// (see resolve.isDeferAncestor). Field merging can invalidate the value stamped
// at expand time by removing the defer directive of an ancestor, so this rule:
//   - adds a missing parent from the nearest enclosing deferred ancestor,
//   - keeps an existing parent that still references a live defer (this also
//     preserves the delivery ordering of genuinely-nested defers),
//   - repairs a stale parent (its defer id no longer exists) to the nearest
//     enclosing deferred ancestor, or removes it when there is none.
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

	operation        *ast.Document
	deferStack       []deferStackEntry
	existingDeferIds map[int]struct{}
}

func (v *deferPopulateParentIdsVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
	v.deferStack = v.deferStack[:0]
	v.existingDeferIds = make(map[int]struct{})
	v.collectExistingDeferIds()
}

// collectExistingDeferIds records every defer id still reachable in the live
// operation tree. It traverses selection sets rather than scanning d.Fields,
// because field merging leaves orphaned Field entries that still carry a
// now-removed defer directive and would otherwise look "alive".
func (v *deferPopulateParentIdsVisitor) collectExistingDeferIds() {
	for i := range v.operation.RootNodes {
		node := v.operation.RootNodes[i]
		if node.Kind != ast.NodeKindOperationDefinition {
			continue
		}
		def := v.operation.OperationDefinitions[node.Ref]
		if !def.HasSelections {
			continue
		}
		v.collectFromSelectionSet(def.SelectionSet)
	}
}

func (v *deferPopulateParentIdsVisitor) collectFromSelectionSet(setRef int) {
	for _, selectionRef := range v.operation.SelectionSets[setRef].SelectionRefs {
		selection := v.operation.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			if id, exists := v.operation.FieldInternalDeferID(selection.Ref); exists {
				v.existingDeferIds[id] = struct{}{}
			}
			if ssRef, ok := v.operation.FieldSelectionSet(selection.Ref); ok {
				v.collectFromSelectionSet(ssRef)
			}
		case ast.SelectionKindInlineFragment:
			if ssRef, ok := v.operation.InlineFragmentSelectionSet(selection.Ref); ok {
				v.collectFromSelectionSet(ssRef)
			}
		}
	}
}

func (v *deferPopulateParentIdsVisitor) EnterField(ref int) {
	id, directiveRef, exists := v.operation.FieldInternalDeferIDWithDirectiveRef(ref)
	if !exists {
		return
	}

	parentValue, parentSet := v.operation.DirectiveArgumentValueByName(directiveRef, parentDeferIDArgName)

	switch {
	case !parentSet:
		// derive a missing parent from the nearest enclosing deferred ancestor
		if len(v.deferStack) > 0 {
			if enclosing := v.deferStack[len(v.deferStack)-1].id; enclosing != id {
				v.setParentDeferID(directiveRef, enclosing)
			}
		}
	case parentValue.Kind == ast.ValueKindInteger:
		parentID := int(v.operation.IntValueAsInt(parentValue.Ref))
		if _, live := v.existingDeferIds[parentID]; live {
			break // still valid; keep as-is
		}
		// stale: parent defer was merged away or discarded during field merging
		if enclosing, ok := v.nearestEnclosingDeferID(id); ok {
			v.setParentDeferID(directiveRef, enclosing)
		} else {
			v.removeParentDeferID(directiveRef)
		}
	}

	v.deferStack = append(v.deferStack, deferStackEntry{id: id, fieldRef: ref})
}

func (v *deferPopulateParentIdsVisitor) LeaveField(ref int) {
	if len(v.deferStack) > 0 && v.deferStack[len(v.deferStack)-1].fieldRef == ref {
		v.deferStack = v.deferStack[:len(v.deferStack)-1]
	}
}

// nearestEnclosingDeferID returns the closest ancestor on the defer stack whose
// id differs from currentID.
func (v *deferPopulateParentIdsVisitor) nearestEnclosingDeferID(currentID int) (int, bool) {
	for i := len(v.deferStack) - 1; i >= 0; i-- {
		if v.deferStack[i].id != currentID {
			return v.deferStack[i].id, true
		}
	}
	return 0, false
}

// setParentDeferID sets parentDeferId to parentID, replacing any existing value.
func (v *deferPopulateParentIdsVisitor) setParentDeferID(directiveRef, parentID int) {
	v.removeParentDeferID(directiveRef)
	argRef := v.operation.AddIntArgument("parentDeferId", parentID)
	v.operation.Directives[directiveRef].Arguments.Refs = append(v.operation.Directives[directiveRef].Arguments.Refs, argRef)
	v.operation.Directives[directiveRef].HasArguments = true
}

// removeParentDeferID drops the parentDeferId argument if present.
func (v *deferPopulateParentIdsVisitor) removeParentDeferID(directiveRef int) {
	refs := v.operation.Directives[directiveRef].Arguments.Refs
	filtered := refs[:0]
	for _, argRef := range refs {
		if bytes.Equal(v.operation.ArgumentNameBytes(argRef), parentDeferIDArgName) {
			continue
		}
		filtered = append(filtered, argRef)
	}
	v.operation.Directives[directiveRef].Arguments.Refs = filtered
	v.operation.Directives[directiveRef].HasArguments = len(filtered) > 0
}
```

Notes:
- `setParentDeferID` removes first so it serves both add and repair; on the unset
  path the remove is a harmless no-op preserving `id`/`label` args.
- `refs[:0]` reuses the backing array; `AddIntArgument` appends to separate
  slices, so the following append to `Arguments.Refs` is safe.

- [ ] Apply the replacement with the Edit/Write tool, then `gofmt -w v2/pkg/astnormalization/defer_populate_parent_ids.go`.
- [ ] Run Task 1 tests — all pass now.

### Task 3 — Engine acceptance

- [ ] `gotestsum --format=short -- ./execution/engine/... -run 'TestExecutionEngine_Execute_Defer'` — the two subtests under `merged/discarded defer parent` pass. Expected normalized forms: test 1 → `phone @__defer_internal(id: 2)`; test 2 → `phone @__defer_internal(id: 3, parentDeferId: 1)`.

### Task 4 — Full regression

- [ ] `gotestsum --format=short -- ./v2/pkg/astnormalization/... ./execution/engine/...` — green (includes fully-nested / parallel / labeled-sibling defer tests).
- [ ] `gofmt -w` on any edited Go files.

---

## Known limitation (pre-existing, out of scope)
The "keep when still present" rule could leave a physically-unreachable parent if
a defer id *survives* yet one of its fields is relocated under an unrelated
surviving deferred object not in its chain (one defer spanning two non-nested
deferred objects after merge). Not exercised by current tests; the repair path
triggers only on staleness, so this is no worse than today.