package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// deferEnsureTypename registers a visitor that
// adds internal typename to a selection set of non deferred field
// if all it's fields are deferred
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
	fieldSelectionRefs := f.operation.SelectionSetFieldSelections(ref)
	// if there are some fields in the current selection set, nothing to do
	if len(fieldSelectionRefs) > 0 {
		return
	}

	inlineFragmentSelectionsRefs := f.operation.SelectionSetInlineFragmentSelections(ref)

	allFragmentsHasDefer := true
	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionsRefs {
		fragmentRef := f.operation.Selections[inlineFragmentSelectionRef].Ref
		// fragment has directives?
		if !f.operation.InlineFragmentHasDirectives(fragmentRef) {
			allFragmentsHasDefer = false
			break
		}

		// has defer directive?
		_, exists := f.operation.InlineFragmentDirectiveByName(fragmentRef, literal.DEFER)
		if !exists {
			allFragmentsHasDefer = false
			break
		}
	}

	// TODO: need more checks
	// we don't have to do it if:
	// if we have an intersection between parent defer ids and field defer ids

	// if we under deferred path
	// field should also have defer id from parent

	if allFragmentsHasDefer {
		addInternalTypeNamePlaceholder(f.operation, ref)
	}
}
