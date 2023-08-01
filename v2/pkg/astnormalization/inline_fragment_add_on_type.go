package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// inlineFragmentAddOnType adds on type condition to inline fragments of the same type
// this is needed for the planner to work correctly
// Such typename will not be printed by astprinter
func inlineFragmentAddOnType(walker *astvisitor.Walker) {
	visitor := inlineFragmentAddOnTypeVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterInlineFragmentVisitor(&visitor)
}

type inlineFragmentAddOnTypeVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (f *inlineFragmentAddOnTypeVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *inlineFragmentAddOnTypeVisitor) EnterInlineFragment(ref int) {
	parentTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)

	if f.operation.InlineFragmentHasTypeCondition(ref) {
		return
	}

	// NOTE: we are internally adding on type condition name for the planner needs
	// but printed query remains the same
	f.operation.InlineFragments[ref].TypeCondition.Type = f.operation.AddNamedType(parentTypeName)
	f.operation.InlineFragments[ref].IsOfTheSameType = true
}
