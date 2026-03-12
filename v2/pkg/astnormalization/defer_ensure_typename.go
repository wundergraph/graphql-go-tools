package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// deferEnsureTypename registers a visitor that ensures a non-deferred field always
// has at least one non-deferred field selection (a __typename placeholder) when all
// of its child fields carry @__defer_internal. This runs after defer expansion, so
// only the expanded field form with @__defer_internal is considered.
//
// This placeholder is necessary for the planner to not produce an empty selection set,
// when all nested fields are deffered
//
// When the enclosing parent field is not deferred, a plain placeholder is added.
//
// When the enclosing parent field is itself deferred, a placeholder is added only if
// none of the child fields share the same defer id as the parent (no intersection).
// In that case the placeholder is annotated with the parent's defer id so it lands
// in the correct defer scope. If there is an intersection (at least one child field
// has the same defer id as the parent), no placeholder is needed.
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
	for i := len(f.Ancestors) - 1; i >= 0; i-- {
		if f.Ancestors[i].Kind == ast.NodeKindField {
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
		if parentDeferID != "" && !hasDeferIntersection {
			idValue, ok := f.operation.DirectiveArgumentValueByName(directiveRef, []byte("id"))
			if ok && f.operation.StringValueContentString(idValue.Ref) == parentDeferID {
				hasDeferIntersection = true
			}
		}
	}

	// if at least one field is not deffered we do not need to add the typename placeholder
	if !allDeferred {
		return
	}

	if parentDeferID == "" {
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
	f.addDeferInternalDirective(fieldRef, parentDeferID)
}

// parentFieldDeferID returns the defer id of the nearest enclosing field that
// carries a @__defer_internal directive, or an empty string if there is none.
func (f *deferEnsureTypenameVisitor) parentFieldDeferID() string {
	for i := len(f.Ancestors) - 1; i >= 0; i-- {
		ancestor := f.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		directiveRef, exists := f.operation.Fields[ancestor.Ref].Directives.HasDirectiveByNameBytes(f.operation, literal.DEFER_INTERNAL)
		if !exists {
			return ""
		}
		idValue, ok := f.operation.DirectiveArgumentValueByName(directiveRef, []byte("id"))
		if !ok {
			return ""
		}
		return f.operation.StringValueContentString(idValue.Ref)
	}
	return ""
}

// addDeferInternalDirective attaches @__defer_internal(id: deferID) to the given field.
func (f *deferEnsureTypenameVisitor) addDeferInternalDirective(fieldRef int, deferID string) {
	strValueRef := f.operation.AddStringValue(ast.StringValue{
		Content: f.operation.Input.AppendInputString(deferID),
	})
	argRef := f.operation.AddArgument(ast.Argument{
		Name:  f.operation.Input.AppendInputString("id"),
		Value: ast.Value{Kind: ast.ValueKindString, Ref: strValueRef},
	})
	directive := ast.Directive{
		Name:         f.operation.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: true,
		Arguments:    ast.ArgumentList{Refs: []int{argRef}},
	}
	directiveRef := f.operation.AddDirective(directive)
	f.operation.AddDirectiveToNode(directiveRef, ast.Node{
		Kind: ast.NodeKindField,
		Ref:  fieldRef,
	})
}
