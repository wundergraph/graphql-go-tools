package astnormalization

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/fastastvisitor"
)

func fragmentSpreadInline(walker *fastastvisitor.Walker) {
	visitor := fragmentSpreadInlineVisitor{
		Walker: walker,
	}
	walker.RegisterDocumentVisitor(&visitor)
	walker.RegisterEnterFragmentSpreadVisitor(&visitor)
}

type fragmentSpreadInlineVisitor struct {
	*fastastvisitor.Walker
	operation, definition *ast.Document
	transformer           asttransform.Transformer
	fragmentSpreadDepth   FragmentSpreadDepth
	depths                Depths
}

func (f *fragmentSpreadInlineVisitor) EnterDocument(operation, definition *ast.Document) {
	f.transformer.Reset()
	f.depths = f.depths[:0]
	f.operation = operation
	f.definition = definition

	err := f.fragmentSpreadDepth.Get(operation, definition, &f.depths)
	if err != nil {
		f.StopWithErr(err)
		return
	}
}

func (f *fragmentSpreadInlineVisitor) LeaveDocument(operation, definition *ast.Document) {
	f.transformer.ApplyTransformations(operation)
}

func (f *fragmentSpreadInlineVisitor) EnterFragmentSpread(ref int) {

	parentTypeName := f.definition.NodeTypeName(f.EnclosingTypeDefinition)

	fragmentDefinitionRef, exists := f.operation.FragmentDefinitionRef(f.operation.FragmentSpreadName(ref))
	if !exists {
		f.StopWithErr(fmt.Errorf("FragmentDefinition not found for FragmentSpread: %s", f.operation.FragmentSpreadNameString(ref)))
		return
	}

	fragmentTypeName := f.operation.FragmentDefinitionTypeName(fragmentDefinitionRef)
	fragmentNode, exists := f.definition.NodeByName(fragmentTypeName)
	if !exists {
		f.StopWithErr(fmt.Errorf("node not indexed with name: %s", string(fragmentTypeName)))
		return
	}

	fragmentTypeEqualsParentType := bytes.Equal(parentTypeName, fragmentTypeName)
	var enclosingTypeImplementsFragmentType bool
	var enclosingTypeIsMemberOfFragmentUnion bool
	var fragmentTypeImplementsEnclosingType bool
	var fragmentTypeIsMemberOfEnclosingUnionType bool
	var fragmentUnionIntersectsEnclosingInterface bool

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

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentTypeIsMemberOfEnclosingUnionType = f.definition.NodeIsUnionMember(fragmentNode, f.EnclosingTypeDefinition)
	}

	nestedDepth, ok := f.depths.ByRef(ref)
	if !ok {
		f.StopWithErr(fmt.Errorf("nested depth missing on depths for FragmentSpread: %s", f.operation.FragmentSpreadNameString(ref)))
		return
	}

	precedence := asttransform.Precedence{
		Depth: nestedDepth,
		Order: 0,
	}

	selectionSet := f.Ancestors[len(f.Ancestors)-1].Ref
	replaceWith := f.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := f.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	switch {
	case fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType:
		f.transformer.ReplaceFragmentSpread(precedence, selectionSet, ref, replaceWith)
	case fragmentTypeImplementsEnclosingType || fragmentTypeIsMemberOfEnclosingUnionType || enclosingTypeIsMemberOfFragmentUnion || fragmentUnionIntersectsEnclosingInterface:
		f.transformer.ReplaceFragmentSpreadWithInlineFragment(precedence, selectionSet, ref, replaceWith, typeCondition)
	}
}
