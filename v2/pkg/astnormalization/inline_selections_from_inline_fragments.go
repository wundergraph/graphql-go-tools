package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func inlineSelectionsFromInlineFragments(walker *astvisitor.Walker) {
	visitor := inlineSelectionsFromInlineFragmentsVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type inlineSelectionsFromInlineFragmentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (m *inlineSelectionsFromInlineFragmentsVisitor) EnterDocument(operation, definition *ast.Document) {
	m.operation = operation
	m.definition = definition
}

func (m *inlineSelectionsFromInlineFragmentsVisitor) couldInline(inlineFragmentRef int) bool {
	if m.operation.InlineFragmentHasDirectives(inlineFragmentRef) {
		return false
	}
	if !m.operation.InlineFragmentHasTypeCondition(inlineFragmentRef) {
		return true
	}

	inlineFragmentTypeName := m.operation.InlineFragmentTypeConditionName(inlineFragmentRef)
	enclosingTypeName := m.definition.NodeNameBytes(m.EnclosingTypeDefinition)

	// check that enclosing type name and inline fragment type condition name are the same
	if bytes.Equal(inlineFragmentTypeName, enclosingTypeName) {
		return true
	}

	// check that enclosing type implements interface type of the fragment type
	if !m.definition.TypeDefinitionContainsImplementsInterface(enclosingTypeName, inlineFragmentTypeName) {
		return false
	}

	selectionSetRef, exists := m.operation.InlineFragmentSelectionSet(inlineFragmentRef)
	if !exists {
		return false
	}

	fragmentSelectionRefs := m.operation.SelectionSetInlineFragmentSelections(selectionSetRef)
	if len(fragmentSelectionRefs) == 0 {
		return true
	}

	// we could inline the current fragment only if all nested fragment types are of the same type as enclosing type
	// or enclosing type implements nested fragment type

	for _, fragmentSelectionRef := range fragmentSelectionRefs {
		nestedFragmentRef := m.operation.Selections[fragmentSelectionRef].Ref
		nestedInlineFragmentTypeName := m.operation.InlineFragmentTypeConditionName(nestedFragmentRef)

		isCompatibleFragmentType := bytes.Equal(nestedInlineFragmentTypeName, enclosingTypeName) ||
			m.definition.TypeDefinitionContainsImplementsInterface(enclosingTypeName, nestedInlineFragmentTypeName)

		if !isCompatibleFragmentType {
			return false
		}
	}

	return true
}

func (m *inlineSelectionsFromInlineFragmentsVisitor) resolveInlineFragment(selectionSetRef, index, inlineFragment int) {
	m.operation.ReplaceSelectionOnSelectionSet(selectionSetRef, index, m.operation.InlineFragments[inlineFragment].SelectionSet)
}

func (m *inlineSelectionsFromInlineFragmentsVisitor) EnterSelectionSet(ref int) {

	for index, selection := range m.operation.SelectionSets[ref].SelectionRefs {
		if m.operation.Selections[selection].Kind != ast.SelectionKindInlineFragment {
			continue
		}
		inlineFragment := m.operation.Selections[selection].Ref
		if !m.couldInline(inlineFragment) {
			continue
		}
		m.resolveInlineFragment(ref, index, inlineFragment)
		m.RevisitNode()
		return
	}
}
