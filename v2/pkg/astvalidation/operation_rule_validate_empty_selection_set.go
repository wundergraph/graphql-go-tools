package astvalidation

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// ValidateEmptySelectionSets validates if selection sets are not empty
// should be used only when the operation is created on the fly
func ValidateEmptySelectionSets() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := emptySelectionSetVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterSelectionSetVisitor(&visitor)
	}
}

type emptySelectionSetVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (r *emptySelectionSetVisitor) EnterDocument(operation, definition *ast.Document) {
	r.operation = operation
	r.definition = definition
}

func (r *emptySelectionSetVisitor) EnterSelectionSet(ref int) {
	if r.operation.SelectionSetIsEmpty(ref) {
		r.Walker.StopWithInternalErr(fmt.Errorf("astvalidation selection set on path %s is empty", r.Walker.Path.DotDelimitedString()))
	}
}
