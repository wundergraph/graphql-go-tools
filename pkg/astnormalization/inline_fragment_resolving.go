package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func ResolveInlineFragments(operation, definition *ast.Document) error {
	resolver := &InlineFragmentResolver{}
	return resolver.Do(operation, definition)
}

type InlineFragmentResolver struct {
	walker  astvisitor.Walker
	visitor inlineFragmentResolverVisitor
}

func (i *InlineFragmentResolver) Do(operation, definition *ast.Document) error {
	i.visitor.err = nil
	i.visitor.operation = operation
	i.visitor.definition = definition

	err := i.walker.Visit(operation, definition, &i.visitor)
	if err == nil {
		err = i.visitor.err
	}
	return err
}

type inlineFragmentResolverVisitor struct {
	err                   error
	operation, definition *ast.Document
}

func (i *inlineFragmentResolverVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) couldInline(set, inlineFragment int, info astvisitor.Info) bool {
	if i.operation.InlineFragmentHasDirectives(inlineFragment) {
		return false
	}
	if !i.operation.InlineFragmentHasTypeCondition(inlineFragment) {
		return true
	}
	if bytes.Equal(i.operation.InlineFragmentTypeConditionName(inlineFragment), i.definition.NodeTypeName(info.EnclosingTypeDefinition)) {
		return true
	}

	inlineFragmentTypeName := i.operation.InlineFragmentTypeConditionName(inlineFragment)
	enclosingTypeName := i.definition.NodeTypeName(info.EnclosingTypeDefinition)
	if !i.definition.TypeDefinitionContainsImplementsInterface(enclosingTypeName, inlineFragmentTypeName) {
		return false
	}

	return true
}

func (i *inlineFragmentResolverVisitor) resolveInlineFragment(set, index, inlineFragment int) {
	i.operation.ReplaceSelectionOnSelectionSet(set, index, i.operation.InlineFragments[inlineFragment].SelectionSet)
}

func (i *inlineFragmentResolverVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {

	for index, selection := range i.operation.SelectionSets[ref].SelectionRefs {
		if i.operation.Selections[selection].Kind != ast.SelectionKindInlineFragment {
			continue
		}
		inlineFragment := i.operation.Selections[selection].Ref
		if !i.couldInline(ref, inlineFragment, info) {
			continue
		}
		i.resolveInlineFragment(ref, index, inlineFragment)
		return astvisitor.Instruction{
			Action: astvisitor.RevisitCurrentNode,
		}
	}

	return
}

func (i *inlineFragmentResolverVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) EnterField(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {
}

func (i *inlineFragmentResolverVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (i *inlineFragmentResolverVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
