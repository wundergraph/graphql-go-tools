package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func mergeInlineFragments(walker *astvisitor.Walker) {
	visitor := mergeInlineFragmentsVisitor{}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type mergeInlineFragmentsVisitor struct {
	operation, definition *ast.Document
}

func (m *mergeInlineFragmentsVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	m.operation = operation
	m.definition = definition
	return astvisitor.Instruction{}
}

func (m *mergeInlineFragmentsVisitor) couldInline(set, inlineFragment int, info astvisitor.Info) bool {
	if m.operation.InlineFragmentHasDirectives(inlineFragment) {
		return false
	}
	if !m.operation.InlineFragmentHasTypeCondition(inlineFragment) {
		return true
	}
	if bytes.Equal(m.operation.InlineFragmentTypeConditionName(inlineFragment), m.definition.NodeTypeName(info.EnclosingTypeDefinition)) {
		return true
	}

	inlineFragmentTypeName := m.operation.InlineFragmentTypeConditionName(inlineFragment)
	enclosingTypeName := m.definition.NodeTypeName(info.EnclosingTypeDefinition)
	if !m.definition.TypeDefinitionContainsImplementsInterface(enclosingTypeName, inlineFragmentTypeName) {
		return false
	}

	return true
}

func (m *mergeInlineFragmentsVisitor) resolveInlineFragment(set, index, inlineFragment int) {
	m.operation.ReplaceSelectionOnSelectionSet(set, index, m.operation.InlineFragments[inlineFragment].SelectionSet)
}

func (m *mergeInlineFragmentsVisitor) EnterSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {

	for index, selection := range m.operation.SelectionSets[ref].SelectionRefs {
		if m.operation.Selections[selection].Kind != ast.SelectionKindInlineFragment {
			continue
		}
		inlineFragment := m.operation.Selections[selection].Ref
		if !m.couldInline(ref, inlineFragment, info) {
			continue
		}
		m.resolveInlineFragment(ref, index, inlineFragment)
		return astvisitor.Instruction{
			Action: astvisitor.RevisitCurrentNode,
		}
	}

	return astvisitor.Instruction{}
}
