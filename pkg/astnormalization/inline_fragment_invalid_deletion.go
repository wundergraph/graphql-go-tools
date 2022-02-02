package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func deleteInvalidInlineFragments(walker *astvisitor.Walker) {
	visitor := deleteInvalidInlineFragmentsVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type deleteInvalidInlineFragmentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (m *deleteInvalidInlineFragmentsVisitor) EnterDocument(operation, definition *ast.Document) {
	m.operation = operation
	m.definition = definition
}

func (d *deleteInvalidInlineFragmentsVisitor) EnterSelectionSet(ref int) {
	selections := d.operation.SelectionSets[ref].SelectionRefs

	if len(selections) == 0 {
		return
	}

	for index := len(selections) - 1; index >= 0; index -= 1 {
		if d.operation.Selections[selections[index]].Kind != ast.SelectionKindInlineFragment {
			continue
		}

		inlineFragment := d.operation.Selections[selections[index]].Ref

		if len(d.operation.InlineFragmentSelections(inlineFragment)) == 0 {
			d.operation.RemoveFromSelectionSet(ref, index)
			continue
		}

		typeName := d.operation.InlineFragmentTypeConditionName(inlineFragment)

		node, exists := d.definition.Index.FirstNonExtensionNodeByNameBytes(typeName)
		if !exists {
			d.operation.RemoveFromSelectionSet(ref, index)
			continue
		}

		if !d.definition.NodeFragmentIsAllowedOnNode(node, d.EnclosingTypeDefinition) {
			d.operation.RemoveFromSelectionSet(ref, index)
		}
	}
}
