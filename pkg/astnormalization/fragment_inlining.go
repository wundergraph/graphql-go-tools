package astnormalization

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func InlineFragments(operation, definition *ast.Document) error {
	inline := FragmentInline{}
	return inline.Do(operation, definition)
}

type FragmentInline struct {
	walker              astvisitor.Walker
	visitor             fragmentInlineVisitor
	fragmentSpreadDepth FragmentSpreadDepth
}

func (a *FragmentInline) Do(operation, definition *ast.Document) error {
	a.visitor.operation = operation
	a.visitor.definition = definition
	a.visitor.err = nil
	a.visitor.transformer.Reset()
	a.visitor.depths = a.visitor.depths[:0]

	err := a.fragmentSpreadDepth.Get(operation, definition, &a.visitor.depths)
	if err != nil {
		return err
	}

	err = a.walker.Visit(operation, definition, &a.visitor)
	if err != nil {
		return err
	}

	a.visitor.transformer.ApplyTransformations(operation)

	return nil
}

type fragmentInlineVisitor struct {
	operation, definition *ast.Document
	err                   error
	transformer           asttransform.Transformer
	depths                Depths
}

func (m *fragmentInlineVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	return
}

func (m *fragmentInlineVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterField(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

	parentTypeName := m.definition.NodeTypeName(info.EnclosingTypeDefinition)

	fragmentDefinitionRef, exists := m.operation.FragmentDefinitionRef(m.operation.FragmentSpreadName(ref))
	if !exists {
		m.err = fmt.Errorf("FragmentDefinition not found for FragmentSpread: %s", m.operation.FragmentSpreadNameString(ref))
		return
	}

	fragmentTypeName := m.operation.FragmentDefinitionTypeName(fragmentDefinitionRef)
	fragmentNode, exists := m.definition.NodeByName(fragmentTypeName)
	if !exists {
		m.err = fmt.Errorf("node not indexed with name: %s", string(fragmentTypeName))
	}

	_parentTypeName := string(parentTypeName)
	_fragmentTypeName := string(fragmentTypeName)

	_, _ = _parentTypeName, _fragmentTypeName

	fragmentTypeEqualsParentType := bytes.Equal(parentTypeName, fragmentTypeName)
	var enclosingTypeImplementsFragmentType bool
	var fragmentTypeImplementsEnclosingType bool
	var fragmentTypeIsMemberOfEnclosingUnionType bool

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition {
		enclosingTypeImplementsFragmentType = m.definition.NodeImplementsInterface(info.EnclosingTypeDefinition, fragmentNode)
	}

	if info.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentTypeImplementsEnclosingType = m.definition.NodeImplementsInterface(fragmentNode, info.EnclosingTypeDefinition)
	}

	if info.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentTypeIsMemberOfEnclosingUnionType = m.definition.NodeIsUnionMember(fragmentNode, info.EnclosingTypeDefinition)
	}

	nestedDepth, ok := m.depths.ByRef(ref)
	if !ok {
		m.err = fmt.Errorf("nested depth missing on depths for FragmentSpread: %s", m.operation.FragmentSpreadNameString(ref))
		return
	}

	precedence := asttransform.Precedence{
		Depth: nestedDepth,
		Order: 0,
	}

	replaceWith := m.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := m.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	switch {
	case fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType:
		m.transformer.ReplaceFragmentSpread(precedence, info.SelectionSet, ref, replaceWith)
	case fragmentTypeImplementsEnclosingType || fragmentTypeIsMemberOfEnclosingUnionType:
		m.transformer.ReplaceFragmentSpreadWithInlineFragment(precedence, info.SelectionSet, ref, replaceWith, typeCondition)
	}

}

func (m *fragmentInlineVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (m *fragmentInlineVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
