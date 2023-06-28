package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func fragmentSpreadInline(walker *astvisitor.Walker) {
	visitor := fragmentSpreadInlineVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterFragmentSpreadVisitor(&visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
}

type fragmentSpreadInlineVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (f *fragmentSpreadInlineVisitor) EnterFragmentDefinition(ref int) {
	f.SkipNode()
}

func (f *fragmentSpreadInlineVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fragmentSpreadInlineVisitor) EnterFragmentSpread(ref int) {
	parentTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)

	fragmentDefinitionRef, exists := f.operation.FragmentDefinitionRef(f.operation.FragmentSpreadNameBytes(ref))
	if !exists {
		fragmentName := f.operation.FragmentSpreadNameBytes(ref)
		f.StopWithExternalErr(operationreport.ErrFragmentUndefined(fragmentName))
		return
	}

	fragmentTypeName := f.operation.FragmentDefinitionTypeName(fragmentDefinitionRef)
	fragmentNode, exists := f.definition.NodeByName(fragmentTypeName)
	if !exists {
		f.StopWithExternalErr(operationreport.ErrTypeUndefined(fragmentTypeName))
		return
	}

	fragmentTypeEqualsParentType := bytes.Equal(parentTypeName, fragmentTypeName)
	var enclosingTypeImplementsFragmentType bool
	var enclosingTypeIsMemberOfFragmentUnion bool
	var fragmentTypeImplementsEnclosingType bool
	var fragmentTypeIsMemberOfEnclosingUnionType bool
	var fragmentUnionIntersectsEnclosingInterface bool
	var fragmentInterfaceIntersectsEnclosingUnion bool

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && f.EnclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition {
		enclosingTypeImplementsFragmentType = f.definition.NodeImplementsInterface(f.EnclosingTypeDefinition, fragmentNode)
	}

	if fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		enclosingTypeIsMemberOfFragmentUnion = f.definition.NodeIsUnionMember(f.EnclosingTypeDefinition, fragmentNode)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentTypeImplementsEnclosingType = f.definition.NodeImplementsInterface(fragmentNode, f.EnclosingTypeDefinition)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition && fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentUnionIntersectsEnclosingInterface = f.definition.UnionNodeIntersectsInterfaceNode(fragmentNode, f.EnclosingTypeDefinition)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition && fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentInterfaceIntersectsEnclosingUnion = f.definition.UnionNodeIntersectsInterfaceNode(f.EnclosingTypeDefinition, fragmentNode)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentTypeIsMemberOfEnclosingUnionType = f.definition.NodeIsUnionMember(fragmentNode, f.EnclosingTypeDefinition)
	}

	selectionSet := f.Ancestors[len(f.Ancestors)-1].Ref
	replaceWith := f.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := f.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	fragmentSpreadHasDirectives := f.operation.FragmentSpreadHasDirectives(ref)
	directiveList := f.operation.FragmentSpreads[ref].Directives

	switch {
	case !fragmentSpreadHasDirectives && (fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType):
		f.operation.ReplaceFragmentSpread(selectionSet, ref, replaceWith)
	case fragmentSpreadHasDirectives && (fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType):
		f.operation.ReplaceFragmentSpreadWithInlineFragment(selectionSet, ref, replaceWith, typeCondition, directiveList)
	case fragmentTypeImplementsEnclosingType || fragmentTypeIsMemberOfEnclosingUnionType || enclosingTypeIsMemberOfFragmentUnion || fragmentUnionIntersectsEnclosingInterface || fragmentInterfaceIntersectsEnclosingUnion:
		f.operation.ReplaceFragmentSpreadWithInlineFragment(selectionSet, ref, replaceWith, typeCondition, directiveList)
	}
}
