package astnormalization

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

var parentDeferIDArgName = []byte("parentDeferId")

// deferPopulateParentIds finalizes the parentDeferId of every
// @__defer_internal-stamped field after field merging.
//
// parentDeferId is physical tree-ancestry: a deferred field must be parented to
// the nearest enclosing deferred object so the resolver can traverse into it
// (see resolve.isDeferAncestor). Field merging (deduplicateFields) can invalidate
// the value stamped at expand time by removing the defer directive of an
// ancestor, so this rule:
//   - adds a missing parent from the nearest enclosing deferred ancestor,
//   - keeps an existing parent that still references a live defer (this also
//     preserves the delivery ordering of genuinely-nested defers),
//   - repairs a stale parent (its defer id no longer exists in the document) to
//     the nearest enclosing deferred ancestor, or removes it when there is none.
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
//
// This runs in EnterDocument as a one-shot pre-scan, so it observes the tree as
// it is when this stage starts. Any rule that rewrites defer ids (e.g.
// deferAlignTypenameScope) must therefore run as an earlier, separate walker
// stage — not on this walker — so its rewrites are visible here.
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
	for _, v0 := range slices.Backward(v.deferStack) {
		if v0.id != currentID {
			return v0.id, true
		}
	}
	return 0, false
}

// setParentDeferID sets parentDeferId to parentID, replacing any existing value.
func (v *deferPopulateParentIdsVisitor) setParentDeferID(directiveRef, parentID int) {
	v.removeParentDeferID(directiveRef)
	argRef := v.operation.AddIntArgument("parentDeferId", parentID)
	v.operation.Directives[directiveRef].Arguments.AddArgumentRef(argRef)
	v.operation.Directives[directiveRef].HasArguments = true
}

// removeParentDeferID drops the parentDeferId argument from the directive if present.
func (v *deferPopulateParentIdsVisitor) removeParentDeferID(directiveRef int) {
	v.operation.Directives[directiveRef].Arguments.RemoveArgumentByName(v.operation, "parentDeferId")
	v.operation.Directives[directiveRef].HasArguments = len(v.operation.Directives[directiveRef].Arguments.Refs) > 0
}
