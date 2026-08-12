# Defer Label Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Thread defer labels end-to-end from normalization through planning, postprocessing, resolution, and into the incremental response JSON envelope.

**Architecture:** Labels are already stored in `@__defer_internal` and in `DeferInfo.Label`. The work is purely additive: add label fields to intermediate structs, thread the value through, write it in the envelope. Three call sites that create `@__defer_internal` from parent defer context also need to look up and forward the parent label.

**Tech Stack:** Go 1.21+, graphql-go-tools v2, gotestsum for running tests

---

## Task 1: AST Package -- Add `FieldInternalDeferLabel`

**File:** `v2/pkg/ast/ast_field.go`

### Step 1.1: Add `FieldInternalDeferLabel` helper

- [ ] In `v2/pkg/ast/ast_field.go`, add a new method directly after `FieldInternalDeferID` (after line 265). This method reads the `label` string argument from the `@__defer_internal` directive attached to the field. Unlike `FieldInternalDeferID`, a missing or empty label is a valid outcome — `exists` is `true` when the `label` argument is present on the directive.

Use a unique anchor inside the last line of `FieldInternalDeferID` for the Edit (start from inside the line, not at column 1, to avoid tab/space mismatch).

**Anchor (start of `old_string`):** `return int(d.IntValueAsInt(idValue.Ref)), true`

**Edit:**

`old_string`:
```
return int(d.IntValueAsInt(idValue.Ref)), true
}
```

`new_string`:
```
return int(d.IntValueAsInt(idValue.Ref)), true
}

// FieldInternalDeferLabel returns the label argument on the @__defer_internal directive
// attached to the given field. `exists` is true when the directive is present and carries
// a `label` argument of string kind; the returned label may be an empty string if the user
// wrote `@defer(label: "")` (unlikely in practice). Callers that only care whether a label
// was supplied should check `exists && label != ""`.
func (d *Document) FieldInternalDeferLabel(fieldRef int) (label string, exists bool) {
	directiveRef, exists := d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !exists {
		return "", false
	}
	labelValue, exists := d.DirectiveArgumentValueByName(directiveRef, []byte("label"))
	if !exists {
		return "", false
	}
	if labelValue.Kind != ValueKindString {
		return "", false
	}
	return d.StringValueContentString(labelValue.Ref), true
}
```

- [ ] Run: `gofmt -w v2/pkg/ast/ast_field.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... -count=1`

---

## Task 2: Plan -- Add `deferLabel` to `pathConfiguration`

**File:** `v2/pkg/engine/plan/planner_configuration.go`

### Step 2.1: Extend `pathConfiguration` struct

- [ ] In `v2/pkg/engine/plan/planner_configuration.go`, add a `deferLabel` field next to the existing `deferID` field (currently on line 252). Anchor on the unique line `deferID       int`.

**Edit:**

`old_string`:
```
deferredField bool
	deferID       int
}
```

`new_string`:
```
deferredField bool
	deferID       int
	deferLabel    string
}
```

### Step 2.2: Update `String()` debug output to include label

- [ ] Update the `PathTypeField` case of the `String()` method (line 266) to include `deferLabel`. Anchor on the unique format string start.

**Edit:**

`old_string`:
```
return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"isRootNode":%t,"pathType":"field","deferID":%d}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.isRootNode, p.deferID)
```

`new_string`:
```
return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"isRootNode":%t,"pathType":"field","deferID":%d,"deferLabel":"%s"}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.isRootNode, p.deferID, p.deferLabel)
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/planner_configuration.go`

---

## Task 3: Plan -- Add `deferLabel` to internal field/fetch configs in path builder

**File:** `v2/pkg/engine/plan/path_builder_visitor.go`

### Step 3.1: Extend `currentFieldInfo` and `objectFetchConfiguration`

- [ ] In `v2/pkg/engine/plan/path_builder_visitor.go`, add `deferLabel` alongside `deferID` in `currentFieldInfo` (currently line 139 range) and in `objectFetchConfiguration` (currently line 126 range).

**Edit for `objectFetchConfiguration`:**

`old_string`:
```
operationType      ast.OperationType
	deferID            int
}
```

`new_string`:
```
operationType      ast.OperationType
	deferID            int
	deferLabel         string
}
```

**Edit for `currentFieldInfo`:**

`old_string`:
```
shareable           bool
	deferID             int
	deferField          bool
}
```

`new_string`:
```
shareable           bool
	deferID             int
	deferLabel          string
	deferField          bool
}
```

### Step 3.2: Populate `field.deferLabel` from `suggestion.deferInfo.Label`

- [ ] Update the three defer-setting branches inside the suggestion loop (near line 560–580) to also set `field.deferLabel`. The previous block only set `field.deferID`.

**Edit (anchor on the unique block around lines 559–581):**

`old_string`:
```
if isDeferParent {
			for _, deferID := range suggestion.deferIDs {
				field.deferID = deferID
				field.deferField = false
				// defer parent path planning - should be planned as a deferred path
				c.handlePlanningField(field)
			}
		}

		// plan deferred field
		if hasDeferInfo {
			field.deferID = suggestion.deferInfo.ID
			field.deferField = true
			// should be planned only as a deferred path
			c.handlePlanningField(field)
		}

		// normal field planning is handled if the field itself is not deferred
		if !hasDeferInfo {
			field.deferID = 0
			field.deferField = false
			c.handlePlanningField(field)
		}
```

`new_string`:
```
if isDeferParent {
			for _, deferID := range suggestion.deferIDs {
				field.deferID = deferID
				field.deferLabel = ""
				field.deferField = false
				// defer parent path planning - should be planned as a deferred path
				c.handlePlanningField(field)
			}
		}

		// plan deferred field
		if hasDeferInfo {
			field.deferID = suggestion.deferInfo.ID
			field.deferLabel = suggestion.deferInfo.Label
			field.deferField = true
			// should be planned only as a deferred path
			c.handlePlanningField(field)
		}

		// normal field planning is handled if the field itself is not deferred
		if !hasDeferInfo {
			field.deferID = 0
			field.deferLabel = ""
			field.deferField = false
			c.handlePlanningField(field)
		}
```

Note: `isDeferParent` sets `deferID` from `suggestion.deferIDs` which is just a list of IDs without labels attached (it represents "paths through which deferred descendants traverse"). That path configuration is not the one whose defer fetch is ultimately emitted — only the path configuration produced for the deferred field itself (the `hasDeferInfo` branch) drives the fetch. Therefore setting `deferLabel = ""` for the parent branch is correct: label belongs with the terminal deferred field.

### Step 3.3: Propagate `field.deferLabel` into `pathConfiguration` entries

- [ ] There are four call sites that build a `pathConfiguration` with `deferID: field.deferID`. Add `deferLabel: field.deferLabel` to each.

Currently located at lines 895, 947, 1370 (pathConfiguration) and line 1038 (`objectFetchConfiguration`). Use unique surrounding-line anchors for each edit.

**Edit A (around line 884-897):**

`old_string`:
```
c.addPath(plannerIdx, pathConfiguration{
					parentPath:       field.parentPath,
					path:             field.currentPath,
					shouldWalkFields: true,
					typeName:         field.typeName,
					fieldRef:         field.fieldRef,
					fragmentRef:      ast.InvalidRef,
					enclosingNode:    c.walker.EnclosingTypeDefinition,
					dsHash:           currentPlannerDSHash,
					isRootNode:       isRootNode,
					pathType:         PathTypeField,
					deferID:          field.deferID,
					deferredField:    field.deferField,
				})
```

`new_string`:
```
c.addPath(plannerIdx, pathConfiguration{
					parentPath:       field.parentPath,
					path:             field.currentPath,
					shouldWalkFields: true,
					typeName:         field.typeName,
					fieldRef:         field.fieldRef,
					fragmentRef:      ast.InvalidRef,
					enclosingNode:    c.walker.EnclosingTypeDefinition,
					dsHash:           currentPlannerDSHash,
					isRootNode:       isRootNode,
					pathType:         PathTypeField,
					deferID:          field.deferID,
					deferLabel:       field.deferLabel,
					deferredField:    field.deferField,
				})
```

**Edit B (around line 936-949, `currentPathConfiguration`):**

`old_string`:
```
currentPathConfiguration := pathConfiguration{
		parentPath:       field.parentPath,
		path:             field.currentPath,
		shouldWalkFields: true,
		typeName:         field.typeName,
		fieldRef:         field.fieldRef,
		fragmentRef:      ast.InvalidRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           field.ds.Hash(),
		isRootNode:       true,
		pathType:         PathTypeField,
		deferID:          field.deferID,
		deferredField:    field.deferField,
	}
```

`new_string`:
```
currentPathConfiguration := pathConfiguration{
		parentPath:       field.parentPath,
		path:             field.currentPath,
		shouldWalkFields: true,
		typeName:         field.typeName,
		fieldRef:         field.fieldRef,
		fragmentRef:      ast.InvalidRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           field.ds.Hash(),
		isRootNode:       true,
		pathType:         PathTypeField,
		deferID:          field.deferID,
		deferLabel:       field.deferLabel,
		deferredField:    field.deferField,
	}
```

**Edit C (around line 1033-1044, `objectFetchConfiguration`):**

`old_string`:
```
fetchConfiguration := &objectFetchConfiguration{
		isSubscription:     isSubscription,
		fieldRef:           field.fieldRef,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
		deferID:            field.deferID,
		fetchItem:          c.fetchItem(),
		sourceID:           field.ds.Id(),
		sourceName:         field.ds.Name(),
		operationType:      c.resolveRootFieldOperationType(field.typeName),
		filter:             c.resolveSubscriptionFilterCondition(field.typeName, field.fieldName),
	}
```

`new_string`:
```
fetchConfiguration := &objectFetchConfiguration{
		isSubscription:     isSubscription,
		fieldRef:           field.fieldRef,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
		deferID:            field.deferID,
		deferLabel:         field.deferLabel,
		fetchItem:          c.fetchItem(),
		sourceID:           field.ds.Id(),
		sourceName:         field.ds.Name(),
		operationType:      c.resolveRootFieldOperationType(field.typeName),
		filter:             c.resolveSubscriptionFilterCondition(field.typeName, field.fieldName),
	}
```

**Edit D (around line 1361-1372, `addPlannerPathForTypename`):**

`old_string`:
```
c.addPath(plannerIndex, pathConfiguration{
		parentPath:       field.parentPath,
		path:             field.currentPath,
		shouldWalkFields: true,
		typeName:         field.typeName,
		fieldRef:         field.fieldRef,
		fragmentRef:      ast.InvalidRef,
		dsHash:           c.planners[plannerIndex].DataSourceConfiguration().Hash(),
		pathType:         PathTypeField,
		deferID:          field.deferID,
		deferredField:    field.deferField,
	})
```

`new_string`:
```
c.addPath(plannerIndex, pathConfiguration{
		parentPath:       field.parentPath,
		path:             field.currentPath,
		shouldWalkFields: true,
		typeName:         field.typeName,
		fieldRef:         field.fieldRef,
		fragmentRef:      ast.InvalidRef,
		dsHash:           c.planners[plannerIndex].DataSourceConfiguration().Hash(),
		pathType:         PathTypeField,
		deferID:          field.deferID,
		deferLabel:       field.deferLabel,
		deferredField:    field.deferField,
	})
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/path_builder_visitor.go`

---

## Task 4: Plan -- Build `DeferField` and `FetchDependencies` with `Label`

**File:** `v2/pkg/engine/plan/visitor.go`

### Step 4.1: Include `Label` in `DeferField`

- [ ] Update `assignDefer` (currently building `resolve.DeferField{DeferID: fieldPathConfiguration.deferID}` at line 611-613) to pass `Label`.

**Edit (anchor on `currentField.Defer = &resolve.DeferField{`):**

`old_string`:
```
currentField.Defer = &resolve.DeferField{
				DeferID: fieldPathConfiguration.deferID,
			}
```

`new_string`:
```
currentField.Defer = &resolve.DeferField{
				DeferID: fieldPathConfiguration.deferID,
				Label:   fieldPathConfiguration.deferLabel,
			}
```

### Step 4.2: Include `Label` on `FetchDependencies`

- [ ] Update the `configureFetch` method (currently at lines 1257–1265) to include `Label` alongside `DeferID` when constructing `resolve.FetchDependencies`.

**Edit:**

`old_string`:
```
FetchDependencies: resolve.FetchDependencies{
			FetchID:           internal.fetchID,
			DependsOnFetchIDs: internal.dependsOnFetchIDs,
			DeferID:           internal.deferID,
		},
```

`new_string`:
```
FetchDependencies: resolve.FetchDependencies{
			FetchID:           internal.fetchID,
			DependsOnFetchIDs: internal.dependsOnFetchIDs,
			DeferID:           internal.deferID,
			Label:             internal.deferLabel,
		},
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/visitor.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/plan/... -count=1` — expect this to still pass because existing tests do not exercise labels yet; fix any accidental regressions before proceeding.

---

## Task 5: Resolve Structs -- Add `Label` to `DeferField`, `DeferFetchGroup`, `FetchDependencies`

### Step 5.1: `DeferField.Label`

**File:** `v2/pkg/engine/resolve/node_object.go`

- [ ] Add `Label string` to the `DeferField` struct at lines 182–184.

**Edit:**

`old_string`:
```
type DeferField struct {
	DeferID int
}
```

`new_string`:
```
type DeferField struct {
	DeferID int
	Label   string
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/node_object.go`

### Step 5.2: `DeferFetchGroup.Label`

**File:** `v2/pkg/engine/resolve/response.go`

- [ ] Add `Label string` to the `DeferFetchGroup` struct at lines 90–93.

**Edit:**

`old_string`:
```
type DeferFetchGroup struct {
	DeferID int
	Fetches *FetchTreeNode
}
```

`new_string`:
```
type DeferFetchGroup struct {
	DeferID int
	Label   string
	Fetches *FetchTreeNode
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/response.go`

### Step 5.3: `FetchDependencies.Label`

**File:** `v2/pkg/engine/resolve/fetch.go`

- [ ] Add `Label string` to the `FetchDependencies` struct at lines 110–114.

**Edit:**

`old_string`:
```
type FetchDependencies struct {
	FetchID           int
	DependsOnFetchIDs []int
	DeferID           int
}
```

`new_string`:
```
type FetchDependencies struct {
	FetchID           int
	DependsOnFetchIDs []int
	DeferID           int
	Label             string
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/fetch.go`

---

## Task 6: Postprocess -- Copy `Label` from `FetchDependencies` into `DeferFetchGroup`

**File:** `v2/pkg/engine/postprocess/extract_defer_fetches.go`

### Step 6.1: Track label alongside fetches per defer id, then write it onto the `DeferFetchGroup`

Because multiple fetches can share the same defer id (and the plan ensures they all share the same label — label travels with `@__defer_internal(id, label)` and is attached identically by all contributors), it suffices to pick the label from the first fetch in the group. But to be safe, we look up the label from any fetch in the group (they are all equal).

- [ ] Rewrite the two functions to collect labels and forward them.

**Edit (whole-file replacement; anchor on `package postprocess`):**

`old_string`:
```
package postprocess

import (
	"maps"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferFetches struct {
	disable bool
}

func (d *extractDeferFetches) Process(deferPlan *plan.DeferResponsePlan) {
	if d.disable {
		return
	}

	root, fetchGroups := d.fetchGroups(deferPlan)

	deferPlan.Response.Response.Fetches = &resolve.FetchTreeNode{
		Kind:       resolve.FetchTreeNodeKindSequence,
		ChildNodes: root,
	}

	// sort defer ids in direct natural order
	deferIds := slices.Sorted(maps.Keys(fetchGroups))

	for _, deferID := range deferIds {
		fetches := fetchGroups[deferID]
		deferResponse := &resolve.DeferFetchGroup{
			DeferID: deferID,

			Fetches: &resolve.FetchTreeNode{
				Kind:       resolve.FetchTreeNodeKindSequence,
				ChildNodes: fetches,
			},
		}
		deferPlan.Response.Defers = append(deferPlan.Response.Defers, deferResponse)
	}
}

func (d *extractDeferFetches) fetchGroups(deferPlan *plan.DeferResponsePlan) (root []*resolve.FetchTreeNode, deffered map[int][]*resolve.FetchTreeNode) {
	fetchGroups := make(map[int][]*resolve.FetchTreeNode)

	for _, fetch := range deferPlan.Response.Response.Fetches.ChildNodes {
		deferID := fetch.Item.Fetch.Dependencies().DeferID
		if deferID == 0 {
			root = append(root, fetch)
			continue
		}

		fetchGroups[deferID] = append(fetchGroups[deferID], fetch)
	}

	return root, fetchGroups
}
```

`new_string`:
```
package postprocess

import (
	"maps"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferFetches struct {
	disable bool
}

func (d *extractDeferFetches) Process(deferPlan *plan.DeferResponsePlan) {
	if d.disable {
		return
	}

	root, fetchGroups, deferLabels := d.fetchGroups(deferPlan)

	deferPlan.Response.Response.Fetches = &resolve.FetchTreeNode{
		Kind:       resolve.FetchTreeNodeKindSequence,
		ChildNodes: root,
	}

	// sort defer ids in direct natural order
	deferIds := slices.Sorted(maps.Keys(fetchGroups))

	for _, deferID := range deferIds {
		fetches := fetchGroups[deferID]
		deferResponse := &resolve.DeferFetchGroup{
			DeferID: deferID,
			Label:   deferLabels[deferID],

			Fetches: &resolve.FetchTreeNode{
				Kind:       resolve.FetchTreeNodeKindSequence,
				ChildNodes: fetches,
			},
		}
		deferPlan.Response.Defers = append(deferPlan.Response.Defers, deferResponse)
	}
}

func (d *extractDeferFetches) fetchGroups(deferPlan *plan.DeferResponsePlan) (root []*resolve.FetchTreeNode, deffered map[int][]*resolve.FetchTreeNode, labels map[int]string) {
	fetchGroups := make(map[int][]*resolve.FetchTreeNode)
	deferLabels := make(map[int]string)

	for _, fetch := range deferPlan.Response.Response.Fetches.ChildNodes {
		deps := fetch.Item.Fetch.Dependencies()
		deferID := deps.DeferID
		if deferID == 0 {
			root = append(root, fetch)
			continue
		}

		fetchGroups[deferID] = append(fetchGroups[deferID], fetch)
		// Label is identical across all fetches sharing the same defer id (the
		// normalizer writes the same @__defer_internal(id, label) on all fields
		// of the same deferred fragment). Keep the first non-empty label we see;
		// fetches added by requires/key chains may be unlabeled.
		if deps.Label != "" {
			if _, ok := deferLabels[deferID]; !ok {
				deferLabels[deferID] = deps.Label
			}
		}
	}

	return root, fetchGroups, deferLabels
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/postprocess/extract_defer_fetches.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/postprocess/... -count=1`

---

## Task 7: Resolvable -- Thread `deferLabel` through rendering

**File:** `v2/pkg/engine/resolve/resolvable.go`

### Step 7.1: Add `deferLabel` field

- [ ] Add `deferLabel string` next to `deferID int` (line 58) on the `Resolvable` struct. Anchor on the unique `deferID             int` line.

**Edit:**

`old_string`:
```
deferMode           bool
	deferID             int
```

`new_string`:
```
deferMode           bool
	deferID             int
	deferLabel          string
```

### Step 7.2: Reset `deferLabel` in `Reset()`

- [ ] Find the `Reset()` line `r.deferID = 0` (line 124) and add the label reset directly after it.

**Edit:**

`old_string`:
```
r.deferMode = false
	r.deferID = 0
	r.enableDeferRender = false
```

`new_string`:
```
r.deferMode = false
	r.deferID = 0
	r.deferLabel = ""
	r.enableDeferRender = false
```

### Step 7.3: Add `literalLabel` constant

**File:** `v2/pkg/engine/resolve/const.go`

- [ ] Add `literalLabel` in the literal block. Anchor on `literalHasNext            = []byte("hasNext")`.

**Edit:**

`old_string`:
```
literalHasNext            = []byte("hasNext")
```

`new_string`:
```
literalHasNext            = []byte("hasNext")
literalLabel              = []byte("label")
```

### Step 7.4: Emit label in `printDeferPathAndErrors`

The response-format rule is: each incremental item is `{"data":<obj>,"path":[...],"label":"<l>","errors":[...]}`. Since `printDeferPathAndErrors` is the single place that writes `"path":[...]` followed by optional `"errors":[...]`, we inject the label after path and before errors.

- [ ] In `v2/pkg/engine/resolve/resolvable.go`, update `printDeferPathAndErrors` (line 356) to write the label after the path when `deferLabel` is non-empty.

**Edit (anchor on the unique `func (r *Resolvable) printDeferPathAndErrors()` signature):**

`old_string`:
```
func (r *Resolvable) printDeferPathAndErrors() {
	r.printBytes(quote)
	r.printBytes(literalPath)
	r.printBytes(quote)
	r.printBytes(colon)
	r.renderPath()
	if r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
}
```

`new_string`:
```
func (r *Resolvable) printDeferPathAndErrors() {
	r.printBytes(quote)
	r.printBytes(literalPath)
	r.printBytes(quote)
	r.printBytes(colon)
	r.renderPath()
	if r.deferLabel != "" {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalLabel)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(quote)
		r.printBytes([]byte(r.deferLabel))
		r.printBytes(quote)
	}
	if r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
}
```

Note: The label is user-supplied but originates from the parsed operation, already validated as a GraphQL string by the parser. For defense-in-depth, a future iteration could escape control characters and embedded quotes; for now the label surface matches upstream data source strings written elsewhere via `printNode`. Since GraphQL string values cannot contain bare newlines or `"` without escaping, and the parser's `StringValueContentString` returns the unescaped content, we intentionally mirror the simpler approach used in `const.go` literal emissions and leave JSON-escaping to a later hardening pass. Callers providing adversarial labels are constrained by the GraphQL grammar at parse time.

Since `"` or `\` could still appear in `StringValueContentString` output, apply proper JSON escaping. Use `strconv.AppendQuote`:

- [ ] Revise the label emission to JSON-escape via `strconv.AppendQuote`. The `strconv` import already exists in `resolvable.go` (line 9).

Replace the sub-block introduced above:

**Edit:**

`old_string`:
```
if r.deferLabel != "" {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalLabel)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(quote)
		r.printBytes([]byte(r.deferLabel))
		r.printBytes(quote)
	}
```

`new_string`:
```
if r.deferLabel != "" {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalLabel)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(strconv.AppendQuote(nil, r.deferLabel))
	}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/const.go`

---

## Task 8: Resolve -- Set `deferLabel` on `Resolvable` when entering a defer group

**File:** `v2/pkg/engine/resolve/resolve.go`

### Step 8.1: Reset and set `deferLabel` in `ResolveGraphQLIncrementalResponse`

- [ ] Update the two sites around lines 464-490:
  - Reset to empty string right after `t.resolvable.deferID = 0` (line 465)
  - Assign `deferGroup.Label` right after `t.resolvable.deferID = deferGroup.DeferID` (line 489)

**Edit A (reset, anchor on `t.resolvable.deferMode = true`):**

`old_string`:
```
t.resolvable.deferMode = true
		t.resolvable.deferID = 0
```

`new_string`:
```
t.resolvable.deferMode = true
		t.resolvable.deferID = 0
		t.resolvable.deferLabel = ""
```

**Edit B (defer group set, anchor on `t.resolvable.deferID = deferGroup.DeferID`):**

`old_string`:
```
t.resolvable.deferID = deferGroup.DeferID

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
```

`new_string`:
```
t.resolvable.deferID = deferGroup.DeferID
			t.resolvable.deferLabel = deferGroup.Label

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/resolve.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/... -count=1`

---

## Task 9: Fix label-loss call site 1 -- `defer_ensure_typename.go`

**File:** `v2/pkg/astnormalization/defer_ensure_typename.go`

### Step 9.1: Widen `parentFieldDeferID` to return both id and label

- [ ] Rename and widen the helper to return `(id int, label string)` so the placeholder typename directive preserves the parent label. Anchor on the unique line `func (f *deferEnsureTypenameVisitor) parentFieldDeferID() int {`.

**Edit:**

`old_string`:
```
// parentFieldDeferID returns the defer id of the nearest enclosing field that
// carries a @__defer_internal directive, or an empty string if there is none.
func (f *deferEnsureTypenameVisitor) parentFieldDeferID() int {
	for i := len(f.Ancestors) - 1; i >= 0; i-- {
		ancestor := f.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}

		id, exist := f.operation.FieldInternalDeferID(ancestor.Ref)
		if exist {
			return id
		}
	}
	return 0
}
```

`new_string`:
```
// parentFieldDeferInfo returns the defer id and label of the nearest enclosing
// field that carries a @__defer_internal directive. Returns (0, "") if there is
// no deferred ancestor.
func (f *deferEnsureTypenameVisitor) parentFieldDeferInfo() (id int, label string) {
	for i := len(f.Ancestors) - 1; i >= 0; i-- {
		ancestor := f.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}

		id, exist := f.operation.FieldInternalDeferID(ancestor.Ref)
		if !exist {
			continue
		}
		label, _ = f.operation.FieldInternalDeferLabel(ancestor.Ref)
		return id, label
	}
	return 0, ""
}
```

### Step 9.2: Update callers of the renamed helper

- [ ] In `EnterSelectionSet` (line 67), update the call.

**Edit A (anchor on `parentDeferID := f.parentFieldDeferID()`):**

`old_string`:
```
parentDeferID := f.parentFieldDeferID()
```

`new_string`:
```
parentDeferID, parentDeferLabel := f.parentFieldDeferInfo()
```

- [ ] Update the directive write (line 107) to pass the label.

**Edit B (anchor on `f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, "", 0)`):**

`old_string`:
```
f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, "", 0)
```

`new_string`:
```
f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, parentDeferLabel, 0)
```

- [ ] Run: `gofmt -w v2/pkg/astnormalization/defer_ensure_typename.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/... -count=1`

---

## Task 10: Fix label-loss call site 2 -- `required_fields_visitor.go` key path

**Files:**
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`

### Step 10.1: Add `parentFieldDeferLabel` to `addRequiredFieldsConfiguration`

- [ ] In `v2/pkg/engine/plan/required_fields_visitor.go`, add the new field next to `parentFieldDeferID` on the struct at lines 54–68.

**Edit (anchor on `parentFieldDeferID           int`):**

`old_string`:
```
deferInfo                    *DeferInfo
	parentFieldDeferID           int
```

`new_string`:
```
deferInfo                    *DeferInfo
	parentFieldDeferID           int
	parentFieldDeferLabel        string
```

### Step 10.2: Use the label in `applyDeferInternalDirective` key path

- [ ] At line 617, replace the hard-coded empty label with `v.config.parentFieldDeferLabel`.

**Edit (anchor on the unique `v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.parentFieldDeferID, "", 0)`):**

`old_string`:
```
v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.parentFieldDeferID, "", 0)
```

`new_string`:
```
v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.parentFieldDeferID, v.config.parentFieldDeferLabel, 0)
```

### Step 10.3: Thread `parentFieldDeferLabel` through `fieldRequirementsContext`, `keyRequirements`, `fieldRequirements`

**File:** `v2/pkg/engine/plan/node_selection_visitor.go`

- [ ] Add `parentFieldDeferLabel string` next to `parentFieldDeferID int` in all three structs: `keyRequirements` (line 102), `fieldRequirements` (line 112), `fieldRequirementsContext` (line 234).

**Edit A (`keyRequirements`):**

`old_string`:
```
typeName             string
	deferInfo            *DeferInfo
	parentFieldDeferID   int
}
```

`new_string`:
```
typeName              string
	deferInfo             *DeferInfo
	parentFieldDeferID    int
	parentFieldDeferLabel string
}
```

**Edit B (`fieldRequirements`):**

`old_string`:
```
isTypenameForEntityInterface bool
	deferInfo                    *DeferInfo
	parentFieldDeferID           int
}
```

`new_string`:
```
isTypenameForEntityInterface bool
	deferInfo                    *DeferInfo
	parentFieldDeferID           int
	parentFieldDeferLabel        string
}
```

**Edit C (`fieldRequirementsContext`, line 234 range):**

`old_string`:
```
dsConfig           DataSource
	deferInfo          *DeferInfo
	parentFieldDeferID int
}
```

`new_string`:
```
dsConfig              DataSource
	deferInfo             *DeferInfo
	parentFieldDeferID    int
	parentFieldDeferLabel string
}
```

### Step 10.4: Populate the new fields when building contexts

- [ ] Add a helper `wrappingFieldDeferLabel()` next to `wrappingFieldDeferID()` (line 293) that walks ancestors and returns the label from the nearest wrapping `@__defer_internal` field.

**Edit (anchor on `// wrappingFieldDeferID walks the walker ancestors in reverse`):**

`old_string`:
```
// wrappingFieldDeferID walks the walker ancestors in reverse to find the nearest wrapping field
// that has a @__defer_internal directive and returns its "id" argument value.
func (c *nodeSelectionVisitor) wrappingFieldDeferID() int {
	for i := len(c.walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := c.walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		id, exists := c.operation.FieldInternalDeferID(ancestor.Ref)
		if !exists {
			return 0
		}
		return id
	}
	return 0
}
```

`new_string`:
```
// wrappingFieldDeferID walks the walker ancestors in reverse to find the nearest wrapping field
// that has a @__defer_internal directive and returns its "id" argument value.
func (c *nodeSelectionVisitor) wrappingFieldDeferID() int {
	for i := len(c.walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := c.walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		id, exists := c.operation.FieldInternalDeferID(ancestor.Ref)
		if !exists {
			return 0
		}
		return id
	}
	return 0
}

// wrappingFieldDeferLabel walks the walker ancestors in reverse to find the nearest wrapping
// field with a @__defer_internal directive and returns its "label" argument value (empty
// string when no label was specified on the @defer).
func (c *nodeSelectionVisitor) wrappingFieldDeferLabel() string {
	for i := len(c.walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := c.walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		if _, exists := c.operation.FieldInternalDeferID(ancestor.Ref); !exists {
			return ""
		}
		label, _ := c.operation.FieldInternalDeferLabel(ancestor.Ref)
		return label
	}
	return ""
}
```

- [ ] In `handleEnterField` (line 263), populate `parentFieldDeferLabel` on the new `fieldRequirementsContext`.

**Edit (anchor on `fieldCtx := fieldRequirementsContext{`):**

`old_string`:
```
fieldCtx := fieldRequirementsContext{
			fieldRef:           fieldRef,
			parentPath:         parentPath,
			typeName:           typeName,
			fieldName:          fieldName,
			currentPath:        currentPath,
			dsConfig:           c.dataSources[dsIdx],
			deferInfo:          suggestion.deferInfo,
			parentFieldDeferID: c.wrappingFieldDeferID(),
		}
```

`new_string`:
```
fieldCtx := fieldRequirementsContext{
			fieldRef:              fieldRef,
			parentPath:            parentPath,
			typeName:              typeName,
			fieldName:             fieldName,
			currentPath:           currentPath,
			dsConfig:              c.dataSources[dsIdx],
			deferInfo:             suggestion.deferInfo,
			parentFieldDeferID:    c.wrappingFieldDeferID(),
			parentFieldDeferLabel: c.wrappingFieldDeferLabel(),
		}
```

- [ ] In `addPendingFieldRequirements` (around line 506-514), copy `parentFieldDeferLabel` into `fieldRequirements`.

**Edit (anchor on `parentFieldDeferID:           fieldCtx.parentFieldDeferID,`):**

`old_string`:
```
deferInfo:                    fieldCtx.deferInfo,
			parentFieldDeferID:           fieldCtx.parentFieldDeferID,
		}

		requirements.existsTracker[existsKey] = struct{}{}
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	} else {
		for i := range requirements.requirementConfigs {
			if requirements.requirementConfigs[i].selectionSet == fieldConfiguration.SelectionSet && requirements.requirementConfigs[i].dsHash == fieldCtx.dsConfig.Hash() && requirements.requirementConfigs[i].isTypenameForEntityInterface == isTypenameForEntityInterface {
```

`new_string`:
```
deferInfo:                    fieldCtx.deferInfo,
			parentFieldDeferID:           fieldCtx.parentFieldDeferID,
			parentFieldDeferLabel:        fieldCtx.parentFieldDeferLabel,
		}

		requirements.existsTracker[existsKey] = struct{}{}
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	} else {
		for i := range requirements.requirementConfigs {
			if requirements.requirementConfigs[i].selectionSet == fieldConfiguration.SelectionSet && requirements.requirementConfigs[i].dsHash == fieldCtx.dsConfig.Hash() && requirements.requirementConfigs[i].isTypenameForEntityInterface == isTypenameForEntityInterface {
```

- [ ] In `addPendingKeyRequirements` (around line 551-560), copy `parentFieldDeferLabel` into `keyRequirements`.

**Edit (anchor on `parentFieldDeferID:   fieldCtx.parentFieldDeferID,`):**

`old_string`:
```
deferInfo:            fieldCtx.deferInfo,
			parentFieldDeferID:   fieldCtx.parentFieldDeferID,
		}
```

`new_string`:
```
deferInfo:             fieldCtx.deferInfo,
			parentFieldDeferID:    fieldCtx.parentFieldDeferID,
			parentFieldDeferLabel: fieldCtx.parentFieldDeferLabel,
		}
```

- [ ] In `addFieldRequirementsToOperation` (around line 593-605), forward label into `addRequiredFieldsConfiguration`.

**Edit (anchor on `parentFieldDeferID:            requirements.parentFieldDeferID,`):**

`old_string`:
```
parentFieldDeferID:            requirements.parentFieldDeferID,
		addTypenameInNestedSelections: c.addTypenameInNestedSelections,
	}
```

`new_string`:
```
parentFieldDeferID:            requirements.parentFieldDeferID,
		parentFieldDeferLabel:         requirements.parentFieldDeferLabel,
		addTypenameInNestedSelections: c.addTypenameInNestedSelections,
	}
```

- [ ] In `addKeyRequirementsToOperation` (around line 684-694), forward label into `addRequiredFieldsConfiguration`.

**Edit (anchor on `parentFieldDeferID:       pendingKey.parentFieldDeferID,`):**

`old_string`:
```
parentFieldDeferID:       pendingKey.parentFieldDeferID,
		}
```

`new_string`:
```
parentFieldDeferID:       pendingKey.parentFieldDeferID,
			parentFieldDeferLabel:    pendingKey.parentFieldDeferLabel,
		}
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/required_fields_visitor.go v2/pkg/engine/plan/node_selection_visitor.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/plan/... -count=1 -run RequiredFields` — existing required-field tests should still pass. If any test struct literal needs the new field, add the zero value `parentFieldDeferLabel: ""` to keep tests compiling (see `required_fields_visitor_test.go` near lines 27 / 1134 / 1144).

### Step 10.5: Keep `required_fields_visitor_test.go` compiling

- [ ] Inspect test at lines 27–30 (struct field declarations for the test table) and at the instantiation point (line 1134). If adding `parentFieldDeferLabel` to `addRequiredFieldsConfiguration` breaks any named-field struct literal, add the missing field with zero-value (`parentFieldDeferLabel: ""`). Named struct literals should still compile; positional literals do not exist here.

Verify compilation:

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/plan/... -count=1`

---

## Task 11: Fix label-loss call site 3 -- abstract selection rewriter `typeNameSelection`

**File:** `v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go`, `v2/pkg/engine/plan/abstract_selection_rewriter.go`, `v2/pkg/engine/plan/abstract_selection_rewriter_info.go`

### Step 11.1: Widen `typeNameSelection` signature

- [ ] Change the signature of `typeNameSelection(deferID int)` (line 440) to take an additional `label string` argument.

**Edit (anchor on `func (r *fieldSelectionRewriter) typeNameSelection(deferID int)`):**

`old_string`:
```
func (r *fieldSelectionRewriter) typeNameSelection(deferID int) (selectionRef int, fieldRef int) {
	field := r.operation.AddField(ast.Field{
		Name: r.operation.Input.AppendInputString(typeNameField),
	})

	if deferID != 0 {
		r.operation.AddDeferInternalDirectiveToField(field.Ref, deferID, "", 0)
	}

	return r.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	}), field.Ref
}
```

`new_string`:
```
func (r *fieldSelectionRewriter) typeNameSelection(deferID int, label string) (selectionRef int, fieldRef int) {
	field := r.operation.AddField(ast.Field{
		Name: r.operation.Input.AppendInputString(typeNameField),
	})

	if deferID != 0 {
		r.operation.AddDeferInternalDirectiveToField(field.Ref, deferID, label, 0)
	}

	return r.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	}), field.Ref
}
```

### Step 11.2: Update `preserveTypeNameSelection` to pass the label

- [ ] In `preserveTypeNameSelection` (line 455-463), update the call and read the label from `selectionSetInfo`.

**Edit (anchor on `selectionRef, _ := r.typeNameSelection(selectionSetInfo.typenameFieldDeferId)`):**

`old_string`:
```
selectionRef, _ := r.typeNameSelection(selectionSetInfo.typenameFieldDeferId)
```

`new_string`:
```
selectionRef, _ := r.typeNameSelection(selectionSetInfo.typenameFieldDeferId, selectionSetInfo.typenameFieldDeferLabel)
```

### Step 11.3: Extend `selectionSetInfo` to carry the typename label

**File:** `v2/pkg/engine/plan/abstract_selection_rewriter_info.go`

- [ ] Add `typenameFieldDeferLabel string` on the struct (line 21).

**Edit (anchor on `typenameFieldDeferId           int`):**

`old_string`:
```
typenameFieldDeferId           int
}
```

`new_string`:
```
typenameFieldDeferId           int
	typenameFieldDeferLabel        string
}
```

### Step 11.4: Read and return the label in `selectionSetFieldSelections`

- [ ] Update the signature and body (lines 66-85) to additionally return the label.

**Edit:**

`old_string`:
```
func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int) (fieldSelections []fieldSelection, hasTypename bool, typeNameFieldDeferID int) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)

		if fieldName == typeNameField {
			hasTypename = true
			typeNameFieldDeferID, _ = r.operation.FieldInternalDeferID(fieldRef)
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename, typeNameFieldDeferID
}
```

`new_string`:
```
func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int) (fieldSelections []fieldSelection, hasTypename bool, typeNameFieldDeferID int, typeNameFieldDeferLabel string) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)

		if fieldName == typeNameField {
			hasTypename = true
			typeNameFieldDeferID, _ = r.operation.FieldInternalDeferID(fieldRef)
			typeNameFieldDeferLabel, _ = r.operation.FieldInternalDeferLabel(fieldRef)
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename, typeNameFieldDeferID, typeNameFieldDeferLabel
}
```

### Step 11.5: Update the single caller in `collectSelectionSetInformation`

- [ ] Update the assignment on line 190 and struct literal on line 208.

**Edit (anchor on `fieldSelections, hasSharedTypename, typenameFieldDeferId := r.selectionSetFieldSelections(selectionSetRef)`):**

`old_string`:
```
fieldSelections, hasSharedTypename, typenameFieldDeferId := r.selectionSetFieldSelections(selectionSetRef)
```

`new_string`:
```
fieldSelections, hasSharedTypename, typenameFieldDeferId, typenameFieldDeferLabel := r.selectionSetFieldSelections(selectionSetRef)
```

- [ ] And the struct literal at line 204-215.

**Edit (anchor on `typenameFieldDeferId:           typenameFieldDeferId,`):**

`old_string`:
```
typenameFieldDeferId:           typenameFieldDeferId,
		inlineFragmentsOnObjects:       inlineFragmentSelectionsOnObjects,
```

`new_string`:
```
typenameFieldDeferId:           typenameFieldDeferId,
		typenameFieldDeferLabel:        typenameFieldDeferLabel,
		inlineFragmentsOnObjects:       inlineFragmentSelectionsOnObjects,
```

### Step 11.6: Update the two other callers of `typeNameSelection`

**File:** `v2/pkg/engine/plan/abstract_selection_rewriter.go`

- [ ] Line 279 (empty-selection fallback) and line 585 (interface-object with typename) both call `r.typeNameSelection(deferID)`. Read the label alongside the id and pass it.

**Edit A (line ~277-279):**

`old_string`:
```
if len(newSelectionRefs) == 0 {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		// we have to add __typename selection in case there is no other selections
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID)
```

`new_string`:
```
if len(newSelectionRefs) == 0 {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		deferLabel, _ := r.operation.FieldInternalDeferLabel(fieldRef)
		// we have to add __typename selection in case there is no other selections
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID, deferLabel)
```

**Edit B (line ~583-585):**

`old_string`:
```
if fieldInfo.isInterfaceObject && !fieldInfo.hasTypeNameSelection && fieldInfo.hasInlineFragmentsOnObjects {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID)
```

`new_string`:
```
if fieldInfo.isInterfaceObject && !fieldInfo.hasTypeNameSelection && fieldInfo.hasInlineFragmentsOnObjects {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		deferLabel, _ := r.operation.FieldInternalDeferLabel(fieldRef)
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID, deferLabel)
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/abstract_selection_rewriter.go v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go v2/pkg/engine/plan/abstract_selection_rewriter_info.go`

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/plan/... -count=1 -run AbstractSelectionRewriter`

---

## Task 12: Unit Test -- envelope rendering includes label

**File:** `v2/pkg/engine/resolve/resolvable_test.go`

### Step 12.1: Write a failing test that exercises the label path

- [ ] Locate an existing test that exercises `printDeferPathAndErrors` / `ResolveDefer` (e.g., search for `ResolveDefer(` in `*_test.go`). Add a new test `TestResolvable_DeferEnvelope_WithLabel` that:
  - Constructs a `Resolvable` directly (using test utilities already in the file)
  - Sets `r.deferID = 1`, `r.deferLabel = "myLabel"`, `r.deferMode = true`, a trivial path
  - Calls `printDeferEnvelopeOpen` + some trivial data + `printDeferEnvelopeClose`
  - Asserts output contains `"label":"myLabel"` positioned after `"path":[...]`

If `resolvable_test.go` does not have a suitable harness, prefer adding the coverage at the integration-test level instead (Task 13) and skip this unit-test step.

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/... -count=1 -run TestResolvable_Defer`

---

## Task 13: Integration test -- end-to-end label in incremental response

**File:** `execution/engine/execution_engine_defer_test.go`

### Step 13.1: Add labeled variants of existing tests (TDD -- they fail pre-implementation)

For each change surface, add a subtest right after the corresponding unlabeled subtest (so reviewers can compare diffs):

- [ ] After `t.Run("single deffered field", ...)` at line 381, add:

```go
t.Run("single deffered field with label", runWithoutError(ExecutionEngineTestCase{
    schema: schema,
    operation: func(t *testing.T) graphql.Request {
        return graphql.Request{
            OperationName: "DeferUserTitle",
            Query: `
            query DeferUserTitle {
                user {
                    name
                    ... @defer(label: "titleLabel") {
                        title
                    }
                }
            }`,
        }
    },
    dataSources: tc.dataSources,
    expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"],"label":"titleLabel"}],"hasNext":false}
`,
}, withStreamingResponse()))
```

- [ ] After `t.Run("multiple deffered fields", ...)` at line 426, add:

```go
t.Run("multiple deffered fields with label", runWithoutError(ExecutionEngineTestCase{
    schema: schema,
    operation: func(t *testing.T) graphql.Request {
        return graphql.Request{
            OperationName: "DeferUserTitle",
            Query: `
            query DeferUserTitle {
                user {
                    name
                    ... @defer(label: "detailsLabel") {
                        title
                        id
                    }
                }
            }`,
        }
    },
    dataSources: tc.dataSources,
    expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat","id":"1"},"path":["user"],"label":"detailsLabel"}],"hasNext":false}
`,
}, withStreamingResponse()))
```

- [ ] After `t.Run("nested defers", ...)` at line 472, add a variant verifying that labels are distinct between inner and outer groups:

```go
t.Run("nested defers with labels", runWithoutError(ExecutionEngineTestCase{
    schema: schema,
    operation: func(t *testing.T) graphql.Request {
        return graphql.Request{
            OperationName: "DeferUserTitle",
            Query: `
            query DeferUserTitle {
                user {
                    name
                    ... @defer(label: "outer") {
                        title
                        ... @defer(label: "inner") {
                            id
                        }
                    }
                }
            }`,
        }
    },
    dataSources: tc.dataSources,
    expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"],"label":"inner"}],"hasNext":false}
`,
}, withStreamingResponse()))
```

- [ ] Add one more case that mixes labeled and unlabeled defers in a single operation, immediately after the test above:

```go
t.Run("labeled and unlabeled sibling defers", runWithoutError(ExecutionEngineTestCase{
    schema: schema,
    operation: func(t *testing.T) graphql.Request {
        return graphql.Request{
            OperationName: "DeferUserTitle",
            Query: `
            query DeferUserTitle {
                user {
                    ... @defer(label: "a") { title }
                    ... @defer { id }
                }
            }`,
        }
    },
    dataSources: tc.dataSources,
    // The order of incremental items follows defer id allocation; both items share path ["user"].
    // The labeled one must include "label":"a"; the unlabeled one must NOT include a "label" key.
    expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"],"label":"a"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":false}
`,
}, withStreamingResponse()))
```

Note: The exact ordering of `title` vs `id` in this sibling case may depend on defer id allocation order in the normalizer. If the expected order is reversed, simply flip the two `incremental` lines in `expectedResponse`. Do this by running the test first with one ordering, reading the actual output, and pinning the correct order.

### Step 13.2: Run the integration suite

- [ ] Run: `gotestsum --format=short -- ./execution/engine/... -count=1 -run TestExecutionEngine_Execute_Defer`

- [ ] If the failing assertion reveals an ordering mismatch in "labeled and unlabeled sibling defers", swap the two incremental lines and re-run.

---

## Task 14: Full verification

### Step 14.1: Run every affected package

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... ./v2/pkg/astnormalization/... ./v2/pkg/engine/plan/... ./v2/pkg/engine/postprocess/... ./v2/pkg/engine/resolve/... ./execution/engine/... -count=1`

- [ ] Address any regressions before considering the plan complete.

### Step 14.2: Sanity-check label is absent when not supplied

- [ ] Confirm that the pre-existing non-label defer tests (line 381 "single deffered field", line 472 "nested defers", etc.) still pass without modification — the envelope for a defer without a label must still produce `"path":[...]` with no `"label"` key.

### Step 14.3: Commit boundary

- [ ] Suggested commit breakdown (do not create the commits automatically unless asked):
  1. `ast: add FieldInternalDeferLabel`
  2. `plan: thread deferLabel through pathConfiguration, fetch configs, DeferField`
  3. `resolve: add Label to DeferField, DeferFetchGroup, FetchDependencies and write it in defer envelope`
  4. `postprocess: copy label from FetchDependencies into DeferFetchGroup`
  5. `normalization/plan: preserve defer label when re-emitting @__defer_internal for placeholders and key fields`
  6. `test: add defer label integration coverage`

---

## Verification checklist (from `superpowers:verification-before-completion`)

- [ ] All changed Go files have been run through `gofmt -w`.
- [ ] `gotestsum --format=short -- ./... -count=1` from the `v2/` root and from the `execution/` root both pass.
- [ ] At least one new assertion exercises a `@defer(label:"...")` end-to-end and observes `"label":"..."` in the streamed JSON envelope.
- [ ] At least one pre-existing non-label defer test remains green, confirming no `"label"` key is emitted when the user did not supply one.
