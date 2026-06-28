# Defer Label Refactor: Single Source of Truth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current "label-threaded-everywhere" implementation (`pathConfiguration.deferLabel`, `objectFetchConfiguration.deferLabel`, `currentFieldInfo.deferLabel`, `parentFieldDeferLabel` on multiple structs, `DeferField.Label`, `FetchDependencies.Label`, `DeferFetchGroup.Label`) with a single `map[int]string` (defer id → label) collected once and stored on `GraphQLDeferResponse.DeferLabels`. The resolver looks up by defer id when entering a defer group.

**Why now:** Labels are metadata bound to a defer id. Plumbing them through plan/fetch/resolve structs duplicates state, requires a workaround in `planWithExistingPlanners` to "rewrite" the label when a deferred field reuses a parent-path planner, and makes synthesized directives (`__typename` placeholders, key/requires fields) carry labels they don't need.

**Approach:**
1. Most files only contain label-threading diffs against `HEAD` — those get reverted in bulk via `git checkout HEAD -- ...`. No careful Edit gymnastics needed.
2. A small new visitor walks `@__defer_internal` directives once on `Planner.prepareOperationWalker` and produces `map[int]string`.
3. Stash on `Planner`, copy to `planningVisitor`, attach to `GraphQLDeferResponse.DeferLabels` in `LeaveDocument`.
4. Resolver reads `response.DeferLabels[deferGroup.DeferID]` and sets `r.deferLabel`.

**Tech Stack:** Go 1.21+, graphql-go-tools v2, gotestsum.

---

## Task 1: Bulk-revert files whose only uncommitted changes are label threading

These files were verified (see chat) to contain only label-threading diffs against `HEAD`. Reverting via git is cleaner than reverse-Edits.

- [ ] Run from repo root:

```
git checkout HEAD -- \
  v2/pkg/astnormalization/defer_ensure_typename.go \
  v2/pkg/engine/plan/abstract_selection_rewriter.go \
  v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go \
  v2/pkg/engine/plan/abstract_selection_rewriter_info.go \
  v2/pkg/engine/plan/node_selection_visitor.go \
  v2/pkg/engine/plan/path_builder_visitor.go \
  v2/pkg/engine/plan/planner_configuration.go \
  v2/pkg/engine/plan/required_fields_visitor.go \
  v2/pkg/engine/postprocess/extract_defer_fetches.go \
  v2/pkg/engine/resolve/fetch.go \
  v2/pkg/engine/resolve/node_object.go \
  v2/pkg/engine/resolve/response.go
```

- [ ] Verify with `git status` that the listed files are clean and the remaining `M` set is exactly: `v2/pkg/ast/ast_field.go`, `v2/pkg/engine/plan/visitor.go`, `v2/pkg/engine/resolve/const.go`, `v2/pkg/engine/resolve/resolve.go`, `v2/pkg/engine/resolve/resolvable.go`, `execution/engine/execution_engine_defer_test.go`.

After this task, the codebase is in the "post-int-migration, pre-label-support" state for the reverted files, but the kept files (above) still carry the `deferLabel` plumbing on the resolve side that we want to preserve, plus the integration tests we want to keep.

**Note:** The build will be broken at this point — `visitor.go` still writes `Label:` into structs that no longer have the field. Fixed in Task 4. Don't run tests until then.

---

## Task 2: Resolve -- expose `DeferLabels` on `GraphQLDeferResponse`

**File:** `v2/pkg/engine/resolve/response.go`

After Task 1 this file is back to its pre-label-work state. Add the central map.

### Step 2.1: Add the map field

- [ ] Add `DeferLabels map[int]string` to `GraphQLDeferResponse`. The map only contains entries for defer ids whose user-supplied label is non-empty; missing key → empty label.

**Edit:**

`old_string`:
```
type GraphQLDeferResponse struct {
	Response *GraphQLResponse
	Defers   []*DeferFetchGroup
}
```

`new_string`:
```
type GraphQLDeferResponse struct {
	Response    *GraphQLResponse
	Defers      []*DeferFetchGroup
	DeferLabels map[int]string
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/response.go`

---

## Task 3: Plan -- add a defer-label collector visitor

**File (new):** `v2/pkg/engine/plan/defer_label_collector.go`

### Step 3.1: Create the visitor

- [ ] Create the file:

```go
package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// deferLabelCollector walks all fields of an operation and records the
// user-supplied defer label keyed by defer id. It reads from the
// @__defer_internal directives that the inline_fragment_expand_defer
// normalizer placed on every field of a deferred fragment.
//
// Multiple fields share the same (id, label) pair (the normalizer applies
// the same directive to every field in a deferred fragment); the collector
// records the first non-empty label it encounters and ignores subsequent
// duplicates. Fields with empty labels are skipped — a missing entry in
// the resulting map means "no label was supplied".
type deferLabelCollector struct {
	operation *ast.Document
	labels    map[int]string
}

func registerDeferLabelCollector(walker *astvisitor.Walker) *deferLabelCollector {
	c := &deferLabelCollector{}
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterFieldVisitor(c)
	return c
}

func (c *deferLabelCollector) EnterDocument(operation, _ *ast.Document) {
	c.operation = operation
	c.labels = nil
}

func (c *deferLabelCollector) EnterField(ref int) {
	id, ok := c.operation.FieldInternalDeferID(ref)
	if !ok || id == 0 { // NOTE: enough to check for zero id,
		return
	}
	if _, seen := c.labels[id]; seen {
		return
	}
	label, ok := c.operation.FieldInternalDeferLabel(ref)
	if !ok || label == "" { // NOTE: I believe it is enough to check for empty label,
		return
	}
	if c.labels == nil { // NOTE: we can do it in enterDocument, to properly reset in case walker was reused
		c.labels = make(map[int]string)
	}
	c.labels[id] = label
}

// Labels returns the collected map. May be nil if no labels were supplied.
func (c *deferLabelCollector) Labels() map[int]string { // NOTE: no need for helper we are in the same package so have access to the fields
	return c.labels
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/defer_label_collector.go`

---

## Task 4: Plan -- wire the collector through `Planner` → `Visitor` → `DeferResponsePlan`

### Step 4.1: Register the collector and stash it on `Planner`

**File:** `v2/pkg/engine/plan/planner.go`

- [ ] Add a `deferLabelCollector *deferLabelCollector` field to the `Planner` struct, register it on `prepareOperationWalker` during construction, and copy `Labels()` to the planning visitor in `Plan()` after `prepareOperation` returns.

**Edit A (struct field):**

`old_string`:
```
	prepareOperationWalker *astvisitor.Walker
}
```

`new_string`:
```
	prepareOperationWalker *astvisitor.Walker
	deferLabelCollector    *deferLabelCollector
}
```

**Edit B (registration):**

`old_string`:
```
	prepareOperationWalker := astvisitor.NewWalkerWithID(48, "PrepareOperationWalker")
	astnormalization.InlineFragmentAddOnType(&prepareOperationWalker)
```

`new_string`:
```
	prepareOperationWalker := astvisitor.NewWalkerWithID(48, "PrepareOperationWalker")
	astnormalization.InlineFragmentAddOnType(&prepareOperationWalker)
	deferLabelCollector := registerDeferLabelCollector(&prepareOperationWalker)
```

**Edit C (Planner literal):**

`old_string`:
```
		prepareOperationWalker: &prepareOperationWalker,
	}
```

`new_string`:
```
		prepareOperationWalker: &prepareOperationWalker,
		deferLabelCollector:    deferLabelCollector,
	}
```

**Edit D (in `Plan()` after `prepareOperation`):**

`old_string`:
```
	p.prepareOperation(operation, definition, report)
	if report.HasErrors() {
		return
	}
```

`new_string`:
```
	p.prepareOperation(operation, definition, report)
	if report.HasErrors() {
		return
	}

	p.planningVisitor.deferLabels = p.deferLabelCollector.Labels()
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/planner.go`

### Step 4.2: Carry labels through the visitor

**File:** `v2/pkg/engine/plan/visitor.go`

- [ ] Add `deferLabels map[int]string` to the `Visitor` struct. Locate the struct (`grep -n "^type Visitor struct" visitor.go`) and add the field near other planner-input fields. Use a unique single-line anchor.

- [ ] Drop the two stale `Label:` writes that reference fields that no longer exist after Task 1.

**Edit A (`DeferField`, anchor on `currentField.Defer = &resolve.DeferField{`):**

`old_string`:
```
			currentField.Defer = &resolve.DeferField{
				DeferID: fieldPathConfiguration.deferID,
				Label:   fieldPathConfiguration.deferLabel,
			}
```

`new_string`:
```
			currentField.Defer = &resolve.DeferField{
				DeferID: fieldPathConfiguration.deferID,
			}
```

**Edit B (`FetchDependencies`):**

`old_string`:
```
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           internal.fetchID,
			DependsOnFetchIDs: internal.dependsOnFetchIDs,
			DeferID:           internal.deferID,
			Label:             internal.deferLabel,
		},
```

`new_string`:
```
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           internal.fetchID,
			DependsOnFetchIDs: internal.dependsOnFetchIDs,
			DeferID:           internal.deferID,
		},
```

### Step 4.3: Attach labels to `GraphQLDeferResponse` when finalizing the plan

- [ ] In `LeaveDocument`, where `DeferResponsePlan` is built (around line 1024), set `DeferLabels: v.deferLabels`.

**Edit:**

`old_string`:
```
		v.plan = &DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				Response: v.response,
			},
		}
```

`new_string`:
```
		v.plan = &DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				Response:    v.response,
				DeferLabels: v.deferLabels,
			},
		}
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/visitor.go`

---

## Task 5: Resolve -- look up the label by id when entering a defer group

**File:** `v2/pkg/engine/resolve/resolve.go`

### Step 5.1: Replace `deferGroup.Label` with `response.DeferLabels[...]`

`deferGroup.Label` no longer exists after Task 1. Switch the source.

- [ ] Edit (anchor on `t.resolvable.deferID = deferGroup.DeferID`):

`old_string`:
```
			t.resolvable.deferID = deferGroup.DeferID
			t.resolvable.deferLabel = deferGroup.Label

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
```

`new_string`:
```
			t.resolvable.deferID = deferGroup.DeferID
			t.resolvable.deferLabel = response.DeferLabels[deferGroup.DeferID]

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
```

`Resolvable.deferLabel`, the reset to `""`, `literalLabel`, and `printDeferPathAndErrors` stay as they are — only the source of the value changed.

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/resolve.go`

---

## Task 6: Build verification

### Step 6.1: Confirm the package compiles

- [ ] Run: `go build ./v2/pkg/...`

If the build fails with references to removed fields (`pathConfiguration.deferLabel`, `objectFetchConfiguration.deferLabel`, `parentFieldDeferLabel`, `DeferField.Label`, `DeferFetchGroup.Label`, `FetchDependencies.Label`, `selectionSetInfo.typenameFieldDeferLabel`, etc.), the cause is a missed reference outside the files we touched. Grep for the field name and remove the offending line — there should be none, but verify.

### Step 6.2: Sanity-grep for stale references

- [ ] Run: `grep -rn "deferLabel\|parentFieldDeferLabel\|typenameFieldDeferLabel\|FetchDependencies\.Label\|DeferFetchGroup\.Label\|DeferField\.Label" v2/ execution/ | grep -v "_test.go" | grep -v "docs/"`

Expected matches:
- `v2/pkg/engine/plan/visitor.go` — `v.deferLabels` (the field on the Visitor)
- `v2/pkg/engine/plan/planner.go` — `p.planningVisitor.deferLabels = ...`
- `v2/pkg/engine/plan/defer_label_collector.go` — internal use
- `v2/pkg/engine/resolve/resolvable.go` — the `deferLabel` field, `r.deferLabel = ""` reset, `r.deferLabel` read in `printDeferPathAndErrors`
- `v2/pkg/engine/resolve/resolve.go` — the reset and the `response.DeferLabels[...]` lookup

If any other file shows up, it's a stale reference that needs removal.

---

## Task 7: Run the test suite

### Step 7.1: All affected packages

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... ./v2/pkg/astnormalization/... ./v2/pkg/engine/plan/... ./v2/pkg/engine/postprocess/... ./v2/pkg/engine/resolve/... ./execution/engine/... -count=1`

### Step 7.2: Spot-check the four labeled cases

The integration tests in `execution/engine/execution_engine_defer_test.go` already cover:
- single defer with label
- multiple deferred fields with label
- nested defers with labels (id 1 outer, id 2 inner)
- labeled+unlabeled sibling defers (id 1 labeled, id 2 unlabeled)

All four must pass — they're the ground truth that the central-map approach produces the same envelope as the threaded approach. Particular attention to the sibling case: id 1 must emit `"label":"a"` and id 2 must NOT emit a `"label":` key.

If any case fails, the most likely cause is a missing entry in the collected `DeferLabels` map. Drop a print into `deferLabelCollector.EnterField` to verify it sees every `@__defer_internal` directive that carries a label.

### Step 7.3: Sanity-check non-label defers

- [ ] Confirm pre-existing non-label defer tests still pass: with no label supplied, `DeferLabels[id]` is the zero value `""`, `r.deferLabel = ""`, and `printDeferPathAndErrors` skips the `"label":...` emission.

---

## Verification checklist

- [ ] `go build ./...` succeeds.
- [ ] All four labeled integration tests in `execution/engine/execution_engine_defer_test.go` pass.
- [ ] All four pre-existing unlabeled defer tests still pass.
- [ ] `git diff --stat` shows the expected file set: 1 new file (`defer_label_collector.go`), 4 modified (`planner.go`, `visitor.go`, `response.go`, `resolve.go`), plus the previously kept (`ast_field.go`, `resolvable.go`, `const.go`, `execution_engine_defer_test.go`). The 12 reverted files do not appear.
- [ ] No stale `deferLabel` / `parentFieldDeferLabel` / `Label` references outside the files listed in Step 6.2.
