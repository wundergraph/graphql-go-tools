package astnormalization

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// deferEnsureTypename registers a visitor that adds a __typename placeholder to a
// field whose child fields are all deferred, so the planner never produces an empty
// selection set. It runs after defer expansion, so deferred children are identified
// by their @__defer_internal directive.
//
// What gets added depends on the enclosing parent field:
//   - parent not deferred:                     plain placeholder.
//   - parent deferred, no child shares its id: placeholder tagged with the parent's defer id.
//   - parent deferred, a child shares its id:  no placeholder needed.
//
// Only nested selection sets are considered; the operation root is skipped.
func deferEnsureTypename(walker *astvisitor.Walker) {
	visitor := deferEnsureTypenameVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type deferEnsureTypenameVisitor struct {
	*astvisitor.Walker

	operation *ast.Document
}

func (f *deferEnsureTypenameVisitor) EnterDocument(operation, _ *ast.Document) {
	f.operation = operation
}

func (f *deferEnsureTypenameVisitor) EnterSelectionSet(ref int) {
	// skip root-level selection sets: we need at least depth > 2
	// and a field ancestor to be inside a field's selection set
	if len(f.Ancestors) <= 2 {
		return
	}
	hasFieldAncestor := false
	for _, v := range slices.Backward(f.Ancestors) {
		if v.Kind == ast.NodeKindField {
			hasFieldAncestor = true
			break
		}
	}
	if !hasFieldAncestor {
		return
	}

	fieldSelectionRefs := f.operation.SelectionSetFieldSelections(ref)
	if len(fieldSelectionRefs) == 0 {
		return
	}

	// single pass over field selections to gather:
	// - whether all fields carry @__defer_internal
	// - whether any field's defer id matches the parent field's defer id (intersection)
	parentDeferID := f.parentFieldDeferID()
	allDeferred := true
	hasDeferIntersection := false

	for _, selectionRef := range fieldSelectionRefs {
		fieldRef := f.operation.Selections[selectionRef].Ref
		directiveRef, exists := f.operation.Fields[fieldRef].Directives.HasDirectiveByNameBytes(f.operation, literal.DEFER_INTERNAL)
		if !exists {
			allDeferred = false
			break
		}
		if parentDeferID != 0 && !hasDeferIntersection {
			idValue, ok := f.operation.DirectiveArgumentValueByName(directiveRef, []byte("id"))
			if ok && idValue.Kind == ast.ValueKindInteger && int(f.operation.IntValueAsInt32(idValue.Ref)) == parentDeferID {
				hasDeferIntersection = true
			}
		}
	}

	// If at least one field is not deferred, do not add the typename placeholder.
	if !allDeferred {
		return
	}

	if parentDeferID == 0 {
		// the enclosing field is not deferred; add a plain placeholder so the
		// selection set has at least one non-deferred field selection
		addInternalTypeNamePlaceholder(f.operation, ref)
		return
	}

	// the enclosing field is deferred; if at least one child shares the same
	// defer id there is an intersection and no placeholder is needed
	if hasDeferIntersection {
		return
	}

	// no intersection: add a placeholder annotated with the parent's defer id
	// so it is planned in the parent field defer scope
	fieldRef := addInternalTypeNamePlaceholder(f.operation, ref)
	f.operation.AddDeferInternalDirectiveToField(fieldRef, parentDeferID, "", 0)
}

// parentFieldDeferID returns the defer id of the nearest enclosing field that
// carries a @__defer_internal directive, or an empty string if there is none.
func (f *deferEnsureTypenameVisitor) parentFieldDeferID() int {
	for _, v := range slices.Backward(f.Ancestors) {
		ancestor := v
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
