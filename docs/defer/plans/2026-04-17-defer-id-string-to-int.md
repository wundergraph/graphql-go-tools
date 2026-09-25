# Defer ID String to Int Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change all defer ID representations from string to int, eliminating all strconv.Atoi/fmt.Sprintf conversions.

**Architecture:** The refactor is bottom-up: first update AST helpers, then normalization, then plan, then resolve. Each layer depends on the layer below it.

**Tech Stack:** Go 1.21+, graphql-go-tools v2 codebase, gotestsum for running tests

---

## Task 1: AST Package -- Add `AddIntArgument`, Update `AddDeferInternalDirectiveToField`, Update `FieldInternalDeferID`

**Files:**
- `v2/pkg/ast/ast_argument.go`
- `v2/pkg/ast/ast_field.go`

### Step 1.1: Add `AddIntArgument` helper

- [ ] In `v2/pkg/ast/ast_argument.go`, add a new method after the existing `AddStringArgument` (line 209). This is analogous to `AddStringArgument` but creates an integer-valued argument:

```go
func (d *Document) AddIntArgument(name string, value int) int {
	intValueRef := d.AddIntValue(IntValue{
		Raw: d.Input.AppendInputString(strconv.Itoa(value)),
	})

	arg := Argument{
		Name:  d.Input.AppendInputString(name),
		Value: Value{Kind: ValueKindInteger, Ref: intValueRef},
	}

	return d.AddArgument(arg)
}
```

- [ ] Add `"strconv"` to the import block at the top of `ast_argument.go` (currently imports `"bytes"`, `"io"`, and two internal packages).

- [ ] Run: `gofmt -w v2/pkg/ast/ast_argument.go`

### Step 1.2: Update `AddDeferInternalDirectiveToField` signature from string to int

- [ ] In `v2/pkg/ast/ast_field.go`, change the function signature and body at lines 222-251:

**Old code (lines 222-251):**
```go
// AddDeferInternalDirectiveToField attaches @__defer_internal(id: id, label: label, parentID: parentID) to the given field.
func (d *Document) AddDeferInternalDirectiveToField(fieldRef int, id, label, parentID string) {
	if id == "" {
		return
	}

	var argRefs []int

	argRefs = append(argRefs, d.AddStringArgument("id", id))

	if label != "" {
		argRefs = append(argRefs, d.AddStringArgument("label", label))
	}
	if parentID != "" {
		argRefs = append(argRefs, d.AddStringArgument("parentDeferId", parentID))
	}

	directiveRef := d.AddDirective(Directive{
		Name:         d.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: len(argRefs) > 0,
		Arguments: ArgumentList{
			Refs: argRefs,
		},
	})

	d.AddDirectiveToNode(directiveRef, Node{
		Kind: NodeKindField,
		Ref:  fieldRef,
	})
}
```

**New code:**
```go
// AddDeferInternalDirectiveToField attaches @__defer_internal(id: id, label: label, parentDeferId: parentID) to the given field.
func (d *Document) AddDeferInternalDirectiveToField(fieldRef int, id int, label string, parentID int) {
	if id == 0 {
		return
	}

	var argRefs []int

	argRefs = append(argRefs, d.AddIntArgument("id", id))

	if label != "" {
		argRefs = append(argRefs, d.AddStringArgument("label", label))
	}
	if parentID != 0 {
		argRefs = append(argRefs, d.AddIntArgument("parentDeferId", parentID))
	}

	directiveRef := d.AddDirective(Directive{
		Name:         d.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: len(argRefs) > 0,
		Arguments: ArgumentList{
			Refs: argRefs,
		},
	})

	d.AddDirectiveToNode(directiveRef, Node{
		Kind: NodeKindField,
		Ref:  fieldRef,
	})
}
```

### Step 1.3: Update `FieldInternalDeferID` return type from string to int

- [ ] In `v2/pkg/ast/ast_field.go`, change lines 253-263:

**Old code:**
```go
func (d *Document) FieldInternalDeferID(fieldRef int) (id string, exists bool) {
	directiveRef, exists := d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !exists {
		return "", false
	}
	idValue, exists := d.DirectiveArgumentValueByName(directiveRef, []byte("id"))
	if !exists {
		return "", false
	}
	return d.StringValueContentString(idValue.Ref), true
}
```

**New code:**
```go
func (d *Document) FieldInternalDeferID(fieldRef int) (id int, exists bool) {
	directiveRef, exists := d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !exists {
		return 0, false
	}
	idValue, exists := d.DirectiveArgumentValueByName(directiveRef, []byte("id"))
	if !exists {
		return 0, false
	}
	return int(d.IntValueAsInt(idValue.Ref)), true
}
```

### Step 1.4: Update field-merge logic that reads defer ID as string

- [ ] In `v2/pkg/ast/ast_field.go`, change lines 200-204 which read the defer ID during field merging:

**Old code (lines 200-204):**
```go
		leftDeferIdValue, _ := d.DirectiveArgumentValueByName(leftDeferDirectiveRef, []byte("id"))
		rightDeferIdValue, _ := d.DirectiveArgumentValueByName(rightDeferDirectiveRef, []byte("id"))

		leftId, _ := strconv.Atoi(d.StringValueContentString(leftDeferIdValue.Ref))
		rightId, _ := strconv.Atoi(d.StringValueContentString(rightDeferIdValue.Ref))
```

**New code:**
```go
		leftDeferIdValue, _ := d.DirectiveArgumentValueByName(leftDeferDirectiveRef, []byte("id"))
		rightDeferIdValue, _ := d.DirectiveArgumentValueByName(rightDeferDirectiveRef, []byte("id"))

		leftId := int(d.IntValueAsInt(leftDeferIdValue.Ref))
		rightId := int(d.IntValueAsInt(rightDeferIdValue.Ref))
```

- [ ] Remove `"strconv"` from imports of `ast_field.go` if it is no longer used after this change (check with the compiler).
- [ ] Run: `gofmt -w v2/pkg/ast/ast_field.go`

### Step 1.5: Verify compilation

- [ ] Run: `go build ./v2/pkg/ast/...` -- this will fail because callers still pass strings. That is expected; we proceed to fix callers.

---

## Task 2: astnormalization Package -- Update `inline_fragment_expand_defer.go` and `defer_ensure_typename.go`

**Files:**
- `v2/pkg/astnormalization/inline_fragment_expand_defer.go`
- `v2/pkg/astnormalization/defer_ensure_typename.go`

### Step 2.1: Update `deferInfo` struct and `inlineFragmentExpandDeferVisitor`

- [ ] In `v2/pkg/astnormalization/inline_fragment_expand_defer.go`, change the `deferInfo` struct (lines 30-35):

**Old code:**
```go
type deferInfo struct {
	parentDeferId string
	id            string
	label         string
	fragmentRef   int
}
```

**New code:**
```go
type deferInfo struct {
	parentDeferId int
	id            int
	label         string
	fragmentRef   int
}
```

### Step 2.2: Update `EnterInlineFragment` to use int IDs

- [ ] Change lines 89-96 in `EnterInlineFragment`:

**Old code:**
```go
	parentDeferId := ""
	if len(f.defers) > 0 {
		parentDeferId = f.defers[len(f.defers)-1].id
	}

	deferInfo := deferInfo{
		parentDeferId: parentDeferId,
		id:            fmt.Sprintf("%d", f.currentDeferId),
		label:         label,
		fragmentRef:   ref,
	}
```

**New code:**
```go
	parentDeferId := 0
	if len(f.defers) > 0 {
		parentDeferId = f.defers[len(f.defers)-1].id
	}

	deferInfo := deferInfo{
		parentDeferId: parentDeferId,
		id:            f.currentDeferId,
		label:         label,
		fragmentRef:   ref,
	}
```

### Step 2.3: Simplify `addInternalDeferDirective` to delegate to `Document.AddDeferInternalDirectiveToField`

- [ ] Replace the `addInternalDeferDirective` method body (lines 132-162) with a single delegation to the AST helper — all the directive-building logic is now in `AddDeferInternalDirectiveToField`:

**Old code (lines 132-162):**
```go
func (f *inlineFragmentExpandDeferVisitor) addInternalDeferDirective(fieldRef int) {
	var argRefs []int

	deferInfo := f.defers[len(f.defers)-1]

	if deferInfo.id != "" {
		argRefs = append(argRefs, f.addStringArgument("id", deferInfo.id))
	}

	if deferInfo.parentDeferId != "" {
		argRefs = append(argRefs, f.addStringArgument("parentDeferId", deferInfo.parentDeferId))
	}

	if deferInfo.label != "" {
		argRefs = append(argRefs, f.addStringArgument("label", deferInfo.label))
	}

	directive := ast.Directive{
		Name:         f.operation.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: len(argRefs) > 0,
		Arguments: ast.ArgumentList{
			Refs: argRefs,
		},
	}
	directiveRef := f.operation.AddDirective(directive)

	f.operation.AddDirectiveToNode(directiveRef, ast.Node{
		Kind: ast.NodeKindField,
		Ref:  fieldRef,
	})
}
```

**New code:**
```go
func (f *inlineFragmentExpandDeferVisitor) addInternalDeferDirective(fieldRef int) {
	deferInfo := f.defers[len(f.defers)-1]
	f.operation.AddDeferInternalDirectiveToField(fieldRef, deferInfo.id, deferInfo.label, deferInfo.parentDeferId)
}
```

### Step 2.4: Update imports

- [ ] Remove `"fmt"` from the import block — `fmt.Sprintf` is gone and no replacement import is needed (the delegation in Step 2.3 uses no new imports):

**Old:**
```go
import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
```

**New:**
```go
import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
```

- [ ] Run: `gofmt -w v2/pkg/astnormalization/inline_fragment_expand_defer.go`

### Step 2.6: Update `defer_ensure_typename.go`

- [ ] In `v2/pkg/astnormalization/defer_ensure_typename.go`, update `parentFieldDeferID()` return type and body (lines 112-125):

**Old code:**
```go
func (f *deferEnsureTypenameVisitor) parentFieldDeferID() string {
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
	return ""
}
```

**New code:**
```go
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

- [ ] Update `EnterSelectionSet` method in the same file. The local variable `parentDeferID` and its comparisons change (lines 67-107):

**Line 78** -- change guard from `!= ""` to `!= 0`:
```go
		if parentDeferID != 0 && !hasDeferIntersection {
```

**Line 80** -- change from reading string value to reading int value:

**Old (line 80):**
```go
			if ok && f.operation.StringValueContentString(idValue.Ref) == parentDeferID {
```

**New:**
```go
			if ok && int(f.operation.IntValueAsInt32(idValue.Ref)) == parentDeferID {
```

**Line 91** -- change empty check:

**Old:**
```go
	if parentDeferID == "" {
```

**New:**
```go
	if parentDeferID == 0 {
```

**Line 107** -- update call to `AddDeferInternalDirectiveToField`:

**Old:**
```go
	f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, "", "")
```

**New:**
```go
	f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, "", 0)
```

- [ ] Run: `gofmt -w v2/pkg/astnormalization/defer_ensure_typename.go`

### Step 2.7: Verify compilation and run tests

- [ ] Run: `go build ./v2/pkg/astnormalization/...`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... -count=1` (these tests should still pass if no test uses the changed functions directly)

---

## Task 3: Plan Package Structs -- Update Type Declarations

**Files:**
- `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`
- `v2/pkg/engine/plan/planner_configuration.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`

### Step 3.1: Update `DeferInfo` struct

- [ ] In `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`, change lines 48-52:

**Old:**
```go
type DeferInfo struct {
	ID       string
	Label    string
	ParentID string
}
```

**New:**
```go
type DeferInfo struct {
	ID       int
	Label    string
	ParentID int
}
```

### Step 3.2: Update `NodeSuggestion.deferIDs` field

- [ ] In `v2/pkg/engine/plan/datasource_filter_node_suggestions.go`, change line 43:

**Old:**
```go
	deferIDs        []string
```

**New:**
```go
	deferIDs        []int
```

### Step 3.3: Update `objectFetchConfiguration.deferID`

- [ ] In `v2/pkg/engine/plan/path_builder_visitor.go`, change line 126:

**Old:**
```go
	deferID            string
```

**New:**
```go
	deferID            int
```

### Step 3.4: Update `currentFieldInfo.deferID`

- [ ] In `v2/pkg/engine/plan/path_builder_visitor.go`, change line 139:

**Old:**
```go
	deferID             string
```

**New:**
```go
	deferID             int
```

### Step 3.5: Update `pathConfiguration.deferID`

- [ ] In `v2/pkg/engine/plan/planner_configuration.go`, change line 252:

**Old:**
```go
	deferID       string
```

**New:**
```go
	deferID       int
```

### Step 3.6: Update `PlannerConfiguration.DeferID()` interface return type

- [ ] In `v2/pkg/engine/plan/planner_configuration.go`, change line 31:

**Old:**
```go
	DeferID() string
```

**New:**
```go
	DeferID() int
```

### Step 3.7: Update `plannerConfiguration[T].DeferID()` implementation

- [ ] In `v2/pkg/engine/plan/planner_configuration.go`, change lines 65-67:

**Old:**
```go
func (p *plannerConfiguration[T]) DeferID() string {
	return p.objectFetchConfiguration.deferID
}
```

**New:**
```go
func (p *plannerConfiguration[T]) DeferID() int {
	return p.objectFetchConfiguration.deferID
}
```

### Step 3.8: Update debug format string

- [ ] In `v2/pkg/engine/plan/planner_configuration.go`, change line 266 (the `String()` method of `pathConfiguration`):

**Old:**
```go
	return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"isRootNode":%t,"pathType":"field","deferID":"%s"}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.isRootNode, p.deferID)
```

**New:**
```go
	return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"isRootNode":%t,"pathType":"field","deferID":%d}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.isRootNode, p.deferID)
```

### Step 3.9: Update `pendingKeyRequirementExistsKey.deferID`

- [ ] In `v2/pkg/engine/plan/node_selection_visitor.go`, change line 85:

**Old:**
```go
	deferID string
```

**New:**
```go
	deferID int
```

### Step 3.10: Update `keyRequirements.parentFieldDeferID`

- [ ] In `v2/pkg/engine/plan/node_selection_visitor.go`, change line 103:

**Old:**
```go
	parentFieldDeferID   string
```

**New:**
```go
	parentFieldDeferID   int
```

### Step 3.11: Update `fieldRequirements.parentFieldDeferID`

- [ ] In `v2/pkg/engine/plan/node_selection_visitor.go`, change line 113:

**Old:**
```go
	parentFieldDeferID           string
```

**New:**
```go
	parentFieldDeferID           int
```

### Step 3.12: Update `pendingFieldRequirementExistsKey.deferID`

- [ ] In `v2/pkg/engine/plan/node_selection_visitor.go`, change line 125:

**Old:**
```go
	deferID                      string
```

**New:**
```go
	deferID                      int
```

### Step 3.13: Update `fieldRequirementsContext.parentFieldDeferID`

- [ ] In `v2/pkg/engine/plan/node_selection_visitor.go`, change line 235:

**Old:**
```go
	parentFieldDeferID string
```

**New:**
```go
	parentFieldDeferID int
```

- [ ] Run `gofmt -w` on all modified files.

---

## Task 4: Plan Package Logic -- Update All Method Bodies

**Files:**
- `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/abstract_selection_rewriter.go`
- `v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go`
- `v2/pkg/engine/plan/visitor.go`

### Step 4.1: Update `datasource_filter_collect_nodes_visitor.go` -- `deferInfo()` method

- [ ] In `v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go`, change lines 629-641 to read int values instead of string values:

**Old code (lines 629-641):**
```go
	idValue, _ := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("id"))
	info.ID = f.operation.StringValueContentString(idValue.Ref)

	parentIdValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("parentDeferId"))
	if exists {
		info.ParentID = f.operation.StringValueContentString(parentIdValue.Ref)
	}

	labelValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("label"))
	if exists {
		info.Label = f.operation.StringValueContentString(labelValue.Ref)
	}
```

**New code:**
```go
	idValue, _ := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("id"))
	info.ID = int(f.operation.IntValueAsInt(idValue.Ref))

	parentIdValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("parentDeferId"))
	if exists {
		info.ParentID = int(f.operation.IntValueAsInt(parentIdValue.Ref))
	}

	labelValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("label"))
	if exists {
		info.Label = f.operation.StringValueContentString(labelValue.Ref)
	}
```

### Step 4.2: Update `path_builder_visitor.go` -- string guards to int guards

- [ ] **Line 578**: Change `field.deferID = ""` to `field.deferID = 0`

**Old:**
```go
			field.deferID = ""
```

**New:**
```go
			field.deferID = 0
```

- [ ] **Line 634**: Change `field.deferID == ""` to `field.deferID == 0`

**Old:**
```go
		if field.deferID == "" {
```

**New:**
```go
		if field.deferID == 0 {
```

- [ ] **Line 842**: Change `plannerConfig.DeferID() != ""` to `plannerConfig.DeferID() != 0`

**Old:**
```go
		if plannerConfig.DeferID() != "" && field.deferID == "" {
```

**New:**
```go
		if plannerConfig.DeferID() != 0 && field.deferID == 0 {
```

- [ ] **Line 847**: Change `field.deferID != ""` to `field.deferID != 0`

**Old:**
```go
		if field.deferID != "" && plannerConfig.DeferID() != field.deferID {
```

**New:**
```go
		if field.deferID != 0 && plannerConfig.DeferID() != field.deferID {
```

### Step 4.3: Update `node_selection_visitor.go` -- `wrappingFieldDeferID()`

- [ ] Change lines 294-311:

**Old code:**
```go
func (c *nodeSelectionVisitor) wrappingFieldDeferID() string {
	for i := len(c.walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := c.walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		directiveRef, exists := c.operation.Fields[ancestor.Ref].Directives.HasDirectiveByNameBytes(c.operation, literal.DEFER_INTERNAL)
		if !exists {
			return ""
		}
		idValue, ok := c.operation.DirectiveArgumentValueByName(directiveRef, []byte("id"))
		if !ok {
			return ""
		}
		return c.operation.StringValueContentString(idValue.Ref)
	}
	return ""
}
```

**New code:**
```go
func (c *nodeSelectionVisitor) wrappingFieldDeferID() int {
	for i := len(c.walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := c.walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		directiveRef, exists := c.operation.Fields[ancestor.Ref].Directives.HasDirectiveByNameBytes(c.operation, literal.DEFER_INTERNAL)
		if !exists {
			return 0
		}
		idValue, ok := c.operation.DirectiveArgumentValueByName(directiveRef, []byte("id"))
		if !ok {
			return 0
		}
		return int(c.operation.IntValueAsInt(idValue.Ref))
	}
	return 0
}
```

### Step 4.4: Update `node_selection_visitor.go` -- `pendingFieldRequirementExistsKey` and `pendingKeyRequirementExistsKey` usage

- [ ] **Lines 505-509**: Change defer ID extraction for field requirements:

**Old:**
```go
	deferID := ""
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingFieldRequirementExistsKey{fieldCtx.dsConfig.Hash(), fieldConfiguration.SelectionSet, isTypenameForEntityInterface, deferID}
```

**New:**
```go
	deferID := 0
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingFieldRequirementExistsKey{fieldCtx.dsConfig.Hash(), fieldConfiguration.SelectionSet, isTypenameForEntityInterface, deferID}
```

- [ ] **Lines 550-554**: Change defer ID extraction for key requirements:

**Old:**
```go
	deferID := ""
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingKeyRequirementExistsKey{dsHash: fieldCtx.dsConfig.Hash(), deferID: deferID}
```

**New:**
```go
	deferID := 0
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingKeyRequirementExistsKey{dsHash: fieldCtx.dsConfig.Hash(), deferID: deferID}
```

### Step 4.5: Update `required_fields_visitor.go` -- `parentFieldDeferID` type and related logic

- [ ] **Line 63**: Change `parentFieldDeferID` field type in `addRequiredFieldsConfiguration`:

**Old:**
```go
	parentFieldDeferID           string
```

**New:**
```go
	parentFieldDeferID           int
```

### Step 4.6: Update `required_fields_visitor.go` -- `fieldDeferID()`

- [ ] Change lines 220-233:

**Old code:**
```go
func (v *requiredFieldsVisitor) fieldDeferID(fieldRef int) string {
	for _, dirRef := range v.config.operation.Fields[fieldRef].Directives.Refs {
		if !bytes.Equal(v.config.operation.DirectiveNameBytes(dirRef), literal.DEFER_INTERNAL) {
			continue // not the right directive
		}
		// found @__defer_internal -- extract the "id" argument
		val, ok := v.config.operation.DirectiveArgumentValueByName(dirRef, []byte("id"))
		if !ok || val.Kind != ast.ValueKindString {
			continue
		}
		return v.config.operation.StringValueContentString(val.Ref)
	}
	return ""
}
```

**New code:**
```go
func (v *requiredFieldsVisitor) fieldDeferID(fieldRef int) int {
	for _, dirRef := range v.config.operation.Fields[fieldRef].Directives.Refs {
		if !bytes.Equal(v.config.operation.DirectiveNameBytes(dirRef), literal.DEFER_INTERNAL) {
			continue // not the right directive
		}
		// found @__defer_internal -- extract the "id" argument
		val, ok := v.config.operation.DirectiveArgumentValueByName(dirRef, []byte("id"))
		if !ok || val.Kind != ast.ValueKindInteger {
			continue
		}
		return int(v.config.operation.IntValueAsInt(val.Ref))
	}
	return 0
}
```

### Step 4.7: Update `required_fields_visitor.go` -- `effectiveDeferID()`

- [ ] Change lines 251-256:

**Old code:**
```go
func (v *requiredFieldsVisitor) effectiveDeferID() string {
	if v.config.isKey && v.config.parentFieldDeferID != "" {
		return v.config.parentFieldDeferID
	}
	return v.config.deferInfo.ID
}
```

**New code:**
```go
func (v *requiredFieldsVisitor) effectiveDeferID() int {
	if v.config.isKey && v.config.parentFieldDeferID != 0 {
		return v.config.parentFieldDeferID
	}
	return v.config.deferInfo.ID
}
```

### Step 4.8: Update `required_fields_visitor.go` -- alias format string

- [ ] Change line 283 in `resolveDeferredAlias`:

**Old:**
```go
	conflictAlias := fmt.Appendf(nil, "__internal_%s_%s", effectiveID, fieldName)
```

**New:**
```go
	conflictAlias := fmt.Appendf(nil, "__internal_%d_%s", effectiveID, fieldName)
```

- [ ] Change line 555 in `addRequiredField`:

**Old:**
```go
			fullAliasName = fmt.Appendf(nil, "__internal_%s_%s", v.effectiveDeferID(), fieldName)
```

**New:**
```go
			fullAliasName = fmt.Appendf(nil, "__internal_%d_%s", v.effectiveDeferID(), fieldName)
```

### Step 4.9: Update `required_fields_visitor.go` -- `applyDeferInternalDirective()` parentFieldDeferID guard

- [ ] Change line 613:

**Old:**
```go
	if v.config.parentFieldDeferID != "" {
```

**New:**
```go
	if v.config.parentFieldDeferID != 0 {
```

- [ ] Change line 617 -- the call already passes `v.config.parentFieldDeferID` which is now int, and the third arg `""` must become `0`:

**Old:**
```go
		v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.parentFieldDeferID, "", "")
```

**New:**
```go
		v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.parentFieldDeferID, "", 0)
```

- [ ] Also update line 607 where `AddDeferInternalDirectiveToField` is called with `deferInfo` fields (these are now ints):

**Old:**
```go
		v.config.operation.AddDeferInternalDirectiveToField(fieldRef, v.config.deferInfo.ID, v.config.deferInfo.Label, v.config.deferInfo.ParentID)
```

This line already works since `deferInfo.ID` and `deferInfo.ParentID` are now `int`, and `deferInfo.Label` remains `string` -- no change needed here beyond what the struct change provides. Verify it compiles.

### Step 4.10: Update `abstract_selection_rewriter.go` and `abstract_selection_rewriter_helpers.go`

- [ ] In `v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go`, change line 440:

**Old:**
```go
func (r *fieldSelectionRewriter) typeNameSelection(deferID string) (selectionRef int, fieldRef int) {
```

**New:**
```go
func (r *fieldSelectionRewriter) typeNameSelection(deferID int) (selectionRef int, fieldRef int) {
```

- [ ] Change the guard on line 445:

**Old:**
```go
	if deferID != "" {
```

**New:**
```go
	if deferID != 0 {
```

- [ ] Change line 446 -- the `AddDeferInternalDirectiveToField` call, the empty string args become 0:

**Old:**
```go
		r.operation.AddDeferInternalDirectiveToField(field.Ref, deferID, "", "")
```

**New:**
```go
		r.operation.AddDeferInternalDirectiveToField(field.Ref, deferID, "", 0)
```

- [ ] In `v2/pkg/engine/plan/abstract_selection_rewriter.go`, lines 277 and 584: `FieldInternalDeferID` now returns `int` -- no code change needed since the return value is passed directly to `typeNameSelection` which also now takes `int`. Verify that these call sites compile.

### Step 4.11: Update `visitor.go`

- [ ] **Line 989**: Change the empty check:

**Old:**
```go
		if v.planners[i].DeferID() != "" {
```

**New:**
```go
		if v.planners[i].DeferID() != 0 {
```

- [ ] Lines 611-612 (`DeferID: fieldPathConfiguration.deferID`) -- this is assigning to `resolve.DeferField.DeferID` which will be changed to `int` in Task 5. Leave this for now; it will be fixed in Task 5.

### Step 4.12: Run gofmt and verify

- [ ] Run: `gofmt -w v2/pkg/engine/plan/datasource_filter_collect_nodes_visitor.go v2/pkg/engine/plan/datasource_filter_node_suggestions.go v2/pkg/engine/plan/path_builder_visitor.go v2/pkg/engine/plan/planner_configuration.go v2/pkg/engine/plan/node_selection_visitor.go v2/pkg/engine/plan/required_fields_visitor.go v2/pkg/engine/plan/abstract_selection_rewriter.go v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go v2/pkg/engine/plan/visitor.go`
- [ ] Run: `go build ./v2/pkg/engine/plan/...` -- this may still fail due to resolve package types not yet updated. That is expected.

---

## Task 5: Resolve Package -- Update All Defer ID Types

**Files:**
- `v2/pkg/engine/resolve/response.go`
- `v2/pkg/engine/resolve/fetch.go`
- `v2/pkg/engine/resolve/node_object.go`
- `v2/pkg/engine/resolve/resolvable.go`
- `v2/pkg/engine/resolve/resolve.go`

### Step 5.1: Update `DeferFetchGroup.DeferID`

- [ ] In `v2/pkg/engine/resolve/response.go`, change line 91:

**Old:**
```go
type DeferFetchGroup struct {
	DeferID string
	Fetches *FetchTreeNode
}
```

**New:**
```go
type DeferFetchGroup struct {
	DeferID int
	Fetches *FetchTreeNode
}
```

### Step 5.2: Update `FetchDependencies.DeferID`

- [ ] In `v2/pkg/engine/resolve/fetch.go`, change line 113:

**Old:**
```go
	DeferID           string
```

**New:**
```go
	DeferID           int
```

### Step 5.3: Update `DeferField.DeferID`

- [ ] In `v2/pkg/engine/resolve/node_object.go`, change lines 182-184:

**Old:**
```go
type DeferField struct {
	DeferID string
}
```

**New:**
```go
type DeferField struct {
	DeferID int
}
```

### Step 5.4: Update `resolvable.deferID` and all guards

- [ ] In `v2/pkg/engine/resolve/resolvable.go`, change the field type (line 58):

**Old:**
```go
	deferID             string
```

**New:**
```go
	deferID             int
```

- [ ] **Line 124**: Change reset value:

**Old:**
```go
	r.deferID = ""
```

**New:**
```go
	r.deferID = 0
```

- [ ] **Line 817**: Change guard:

**Old:**
```go
			if r.deferID != "" {
```

**New:**
```go
			if r.deferID != 0 {
```

- [ ] **Line 834** (approximate -- the second guard in the same function):

**Old:**
```go
			if r.deferID != "" {
```

**New:**
```go
			if r.deferID != 0 {
```

- [ ] **Line 850**: Change guard:

**Old:**
```go
	if r.deferID != "" && len(seekFiels) > 0 {
```

**New:**
```go
	if r.deferID != 0 && len(seekFiels) > 0 {
```

- [ ] **Line 878**: Change guard:

**Old:**
```go
	if r.deferID == "" {
```

**New:**
```go
	if r.deferID == 0 {
```

- [ ] **Lines 908-913**: Remove `strconv.Atoi` calls -- the fields are now int, direct comparison:

**Old:**
```go
		fieldDeferId, _ := strconv.Atoi(obj.Fields[i].Defer.DeferID)
		currentDeferIDInt, _ := strconv.Atoi(r.deferID)

		// TODO: it is a temporary solution,
		// because defer could be parallel
		if currentDeferIDInt < fieldDeferId {
```

**New:**
```go
		// TODO: it is a temporary solution,
		// because defer could be parallel
		if r.deferID < obj.Fields[i].Defer.DeferID {
```

- [ ] Remove `"strconv"` from imports if it is no longer used in `resolvable.go`.

### Step 5.5: Update `resolve.go`

- [ ] **Line 465**: Change reset value:

**Old:**
```go
		t.resolvable.deferID = ""
```

**New:**
```go
		t.resolvable.deferID = 0
```

- [ ] Line 489 (`t.resolvable.deferID = deferGroup.DeferID`) -- both sides are now `int`, no change needed.

### Step 5.6: Run gofmt and verify

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/response.go v2/pkg/engine/resolve/fetch.go v2/pkg/engine/resolve/node_object.go v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/resolve.go`
- [ ] Run: `go build ./v2/pkg/engine/resolve/...`

---

## Task 6: Postprocess Package -- Update `extract_defer_fetches.go`

**File:** `v2/pkg/engine/postprocess/extract_defer_fetches.go`

### Step 6.1: Update `fetchGroups` map key type and sort

- [ ] Change the `fetchGroups` function return type and internal map from `map[string]` to `map[int]` (lines 50-63):

**Old code (lines 50-63):**
```go
func (d *extractDeferFetches) fetchGroups(deferPlan *plan.DeferResponsePlan) (root []*resolve.FetchTreeNode, deffered map[string][]*resolve.FetchTreeNode) {
	fetchGroups := make(map[string][]*resolve.FetchTreeNode)

	for _, fetch := range deferPlan.Response.Response.Fetches.ChildNodes {
		deferID := fetch.Item.Fetch.Dependencies().DeferID
		if deferID == "" {
			root = append(root, fetch)
			continue
		}

		fetchGroups[deferID] = append(fetchGroups[deferID], fetch)
	}

	return root, fetchGroups
}
```

**New code:**
```go
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

### Step 6.2: Update `Process` method sorting and iteration

- [ ] Change lines 30-36 in `Process`:

**Old code:**
```go
	// sort defer ids in direct natural order
	deferIds := slices.SortedFunc(maps.Keys(fetchGroups), func(a, b string) int {
		an, _ := strconv.Atoi(a)
		bn, _ := strconv.Atoi(b)
		return cmp.Compare(an, bn)
	})

	for _, deferID := range deferIds {
```

**New code:**
```go
	// sort defer ids in direct natural order
	deferIds := slices.SortedFunc(maps.Keys(fetchGroups), func(a, b int) int {
		return cmp.Compare(a, b)
	})

	for _, deferID := range deferIds {
```

### Step 6.3: Remove unused imports

- [ ] Remove `"strconv"` from the import block (line 7) as it is no longer needed.

### Step 6.4: Run gofmt and verify

- [ ] Run: `gofmt -w v2/pkg/engine/postprocess/extract_defer_fetches.go`
- [ ] Run: `go build ./v2/pkg/engine/postprocess/...`

---

## Task 7: Update Test Files

**Files:**
- `v2/pkg/astnormalization/inline_fragment_expand_defer_test.go`
- `v2/pkg/astnormalization/defer_ensure_typename_test.go`
- `v2/pkg/astnormalization/inline_fragment_selection_merging_test.go`
- `v2/pkg/engine/plan/required_fields_visitor_test.go`
- `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go`

### Step 7.1: Update normalization test files -- change `id: "N"` to `id: N` in expected GraphQL strings

In these test files, all occurrences of `@__defer_internal(id: "N"` and `parentDeferId: "N"` in expected GraphQL output strings must be updated from quoted string values to unquoted integer values.

- [ ] In `v2/pkg/astnormalization/inline_fragment_expand_defer_test.go`, perform a global find-and-replace:
  - `id: "1"` -> `id: 1`
  - `id: "2"` -> `id: 2`
  - `id: "3"` -> `id: 3`
  - `id: "4"` -> `id: 4`
  - `id: "5"` -> `id: 5`
  - `id: "6"` -> `id: 6`
  - `parentDeferId: "1"` -> `parentDeferId: 1`
  - `parentDeferId: "2"` -> `parentDeferId: 2`

  NOTE: Only replace occurrences inside `@__defer_internal(...)` argument strings. Do NOT replace occurrences like `user(id: "1")` which are regular GraphQL field arguments.

- [ ] In `v2/pkg/astnormalization/defer_ensure_typename_test.go`, perform the same replacements for all `@__defer_internal` id/parentDeferId arguments.

- [ ] In `v2/pkg/astnormalization/inline_fragment_selection_merging_test.go`, perform the same replacements.

### Step 7.2: Update `required_fields_visitor_test.go` -- change `DeferInfo` struct literals and `parentFieldDeferID` values

- [ ] Change all `DeferInfo{ID: "N", ...}` struct literals from string to int:
  - `&DeferInfo{ID: "1"}` -> `&DeferInfo{ID: 1}`
  - `&DeferInfo{ID: "2", ParentID: "2"}` -> `&DeferInfo{ID: 2, ParentID: 2}`
  - `&DeferInfo{ID: "2", Label: "myLabel", ParentID: "1"}` -> `&DeferInfo{ID: 2, Label: "myLabel", ParentID: 1}`
  - `&DeferInfo{ID: "2", ParentID: "1"}` -> `&DeferInfo{ID: 2, ParentID: 1}`
  - `&DeferInfo{ID: "3"}` -> `&DeferInfo{ID: 3}`
  - `&DeferInfo{ID: "5"}` -> `&DeferInfo{ID: 5}`

- [ ] Change all `parentFieldDeferID: "N"` values from string to int:
  - `parentFieldDeferID: "1"` -> `parentFieldDeferID: 1`
  - `parentFieldDeferID: "2"` -> `parentFieldDeferID: 2`

- [ ] Change all `@__defer_internal(id: "N"` patterns in expected operation strings within this file to use int values (same replacements as Step 7.1).

- [ ] Update comments that reference string values (e.g. `parentFieldDeferID="1"` -> `parentFieldDeferID=1`).

### Step 7.3: Update `graphql_datasource_defer_test.go` -- change `DeferID: "N"` struct literals

- [ ] In `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go`, change all `DeferID: "1"` to `DeferID: 1`. Specifically:
  - Line 106: `DeferID: "1"` -> `DeferID: 1`
  - Line 148: `DeferID: "1"` -> `DeferID: 1`
  - Line 220: `DeferID: "1"` -> `DeferID: 1`
  - Line 234: `DeferID: "1"` -> `DeferID: 1`
  - Line 239: `DeferID: "1"` -> `DeferID: 1`
  - Line 448: `DeferID: "1"` -> `DeferID: 1`
  - Line 510: `DeferID: "1"` -> `DeferID: 1`
  - Line 623: `DeferID: "1"` -> `DeferID: 1`
  - Line 637: `DeferID: "1"` -> `DeferID: 1`
  - Line 643: `DeferID: "1"` -> `DeferID: 1`

### Step 7.4: Run gofmt on all test files

- [ ] Run: `gofmt -w v2/pkg/astnormalization/inline_fragment_expand_defer_test.go v2/pkg/astnormalization/defer_ensure_typename_test.go v2/pkg/astnormalization/inline_fragment_selection_merging_test.go v2/pkg/engine/plan/required_fields_visitor_test.go v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_defer_test.go`

### Step 7.5: Full compilation check

- [ ] Run: `go build ./v2/...`

If this fails, there may be additional test files or callers that need updating. Fix any remaining compilation errors by searching for the specific error messages and applying the same pattern (string to int, `""` to `0`, `%s` to `%d`, `strconv.Atoi` removal).

### Step 7.6: Run all affected package tests

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... -count=1`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/... -count=1`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/plan/... -count=1`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/resolve/... -count=1`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/postprocess/... -count=1`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... -count=1`

### Step 7.7: Run full test suite

- [ ] Run: `gotestsum --format=short -- ./v2/... -count=1`

Fix any remaining failures. Common issues:
- Missed `DeferID: "N"` literals in other test files (search with `grep -rn 'DeferID:.*"' v2/`)
- Missed `id: "N"` in GraphQL string expectations inside `@__defer_internal` (search with `grep -rn '__defer_internal(id: "' v2/`)
- Missed `parentDeferId: "N"` patterns (search with `grep -rn 'parentDeferId: "' v2/`)

---

## Post-Implementation Verification Checklist

- [ ] No remaining `strconv.Atoi` calls related to defer IDs (search: `grep -rn 'strconv.Atoi' v2/pkg/engine/resolve/resolvable.go v2/pkg/ast/ast_field.go v2/pkg/engine/postprocess/extract_defer_fetches.go`)
- [ ] No remaining `fmt.Sprintf("%d"` for defer ID formatting in normalization (search: `grep -rn 'fmt.Sprintf.*currentDeferId' v2/pkg/astnormalization/`)
- [ ] No remaining `StringValueContentString` calls for defer ID reading (search: `grep -rn 'StringValueContentString.*id' v2/pkg/engine/plan/ v2/pkg/astnormalization/defer_ensure_typename.go`)
- [ ] All `DeferInfo` struct fields `ID` and `ParentID` are `int` (verify in `datasource_filter_node_suggestions.go`)
- [ ] All `DeferID` fields in resolve package are `int` (verify in `response.go`, `fetch.go`, `node_object.go`, `resolvable.go`)
- [ ] `go vet ./v2/...` passes cleanly
- [ ] `gotestsum --format=short -- ./v2/... -count=1` passes