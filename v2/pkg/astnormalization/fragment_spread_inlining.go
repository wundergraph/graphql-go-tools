package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func fragmentSpreadInline(walker *astvisitor.Walker) {
	visitor := fragmentSpreadInlineVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
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

func (f *fragmentSpreadInlineVisitor) EnterSelectionSet(ref int) {
	for _, selection := range f.operation.SelectionSets[ref].SelectionRefs {
		if f.operation.Selections[selection].Kind != ast.SelectionKindFragmentSpread {
			continue
		}
		if f.replaceFragmentSpread(ref, f.operation.Selections[selection].Ref) {
			f.RevisitNode()
			return
		}
	}
}

func (f *fragmentSpreadInlineVisitor) replaceFragmentSpread(selectionSetRef int, ref int) (replaced bool) {
	parentTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)

	spreadName := f.operation.FragmentSpreadNameBytes(ref)
	fragmentDefinitionRef, exists := f.operation.FragmentDefinitionRef(spreadName)
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
	var fragmentInterfaceIntersectsEnclosingInterface bool

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && f.EnclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition {
		enclosingTypeImplementsFragmentType =
			f.definition.NodeImplementsInterface(f.EnclosingTypeDefinition, fragmentTypeName) &&
				f.definition.NodeImplementsInterfaceFields(f.EnclosingTypeDefinition, fragmentNode)
	}

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentInterfaceIntersectsEnclosingInterface = f.definition.InterfacesIntersect(fragmentNode.Ref, f.EnclosingTypeDefinition.Ref)
	}

	if fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		enclosingTypeIsMemberOfFragmentUnion = f.definition.NodeIsUnionMember(f.EnclosingTypeDefinition, fragmentNode)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentTypeImplementsEnclosingType =
			f.definition.NodeImplementsInterface(fragmentNode, parentTypeName) &&
				f.definition.NodeImplementsInterfaceFields(fragmentNode, f.EnclosingTypeDefinition)
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

	replaceWith := f.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := f.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	directiveList := f.operation.FragmentSpreads[ref].Directives

	switch {
	case fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType:
		// NOTE: we always replace fragment with inline fragment
		// it is dangerous to fully inline fragment without checking for the compatibility
		// of possible nested fragments.
		// Such checks are performed in the inlineSelectionsFromInlineFragmentsVisitor
		// on the next stage of the normalization.

		fallthrough
	case fragmentTypeImplementsEnclosingType ||
		fragmentTypeIsMemberOfEnclosingUnionType ||
		enclosingTypeIsMemberOfFragmentUnion ||
		fragmentUnionIntersectsEnclosingInterface ||
		fragmentInterfaceIntersectsEnclosingUnion ||
		fragmentInterfaceIntersectsEnclosingInterface:

		f.operation.ReplaceFragmentSpreadWithInlineFragment(selectionSetRef, ref, replaceWith, typeCondition, directiveList)

		return true
	default:
		// all other case are invalid and should be reported by validation
		return false
	}
}
