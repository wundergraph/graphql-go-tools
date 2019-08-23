package astnormalization

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func fragmentSpreadInline(walker *astvisitor.Walker) {
	visitor := fragmentSpreadInlineVisitor{}
	walker.RegisterDocumentVisitor(&visitor)
	walker.RegisterEnterFragmentSpreadVisitor(&visitor)
}

type fragmentSpreadInlineVisitor struct {
	operation, definition *ast.Document
	transformer           asttransform.Transformer
	fragmentSpreadDepth   FragmentSpreadDepth
	depths                Depths
}

func (f *fragmentSpreadInlineVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	f.transformer.Reset()
	f.depths = f.depths[:0]
	f.operation = operation
	f.definition = definition

	err := f.fragmentSpreadDepth.Get(operation, definition, &f.depths)
	if err != nil {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: err.Error(),
		}
	}

	return astvisitor.Instruction{}
}

func (f *fragmentSpreadInlineVisitor) LeaveDocument(operation, definition *ast.Document) astvisitor.Instruction {
	f.transformer.ApplyTransformations(operation)
	return astvisitor.Instruction{}
}

func (f *fragmentSpreadInlineVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) astvisitor.Instruction {

	parentTypeName := f.definition.NodeTypeName(info.EnclosingTypeDefinition)

	fragmentDefinitionRef, exists := f.operation.FragmentDefinitionRef(f.operation.FragmentSpreadName(ref))
	if !exists {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("FragmentDefinition not found for FragmentSpread: %s", f.operation.FragmentSpreadNameString(ref)),
		}
	}

	fragmentTypeName := f.operation.FragmentDefinitionTypeName(fragmentDefinitionRef)
	fragmentNode, exists := f.definition.NodeByName(fragmentTypeName)
	if !exists {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("node not indexed with name: %s", string(fragmentTypeName)),
		}
	}

	fragmentTypeEqualsParentType := bytes.Equal(parentTypeName, fragmentTypeName)
	var enclosingTypeImplementsFragmentType bool
	var enclosingTypeIsMemberOfFragmentUnion bool
	var fragmentTypeImplementsEnclosingType bool
	var fragmentTypeIsMemberOfEnclosingUnionType bool
	var fragmentUnionIntersectsEnclosingInterface bool

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && info.EnclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition {
		enclosingTypeImplementsFragmentType = f.definition.NodeImplementsInterface(info.EnclosingTypeDefinition, fragmentNode)
	}

	if fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		enclosingTypeIsMemberOfFragmentUnion = f.definition.NodeIsUnionMember(info.EnclosingTypeDefinition, fragmentNode)
	}

	if info.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentTypeImplementsEnclosingType = f.definition.NodeImplementsInterface(fragmentNode, info.EnclosingTypeDefinition)
	}

	if info.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition && fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentUnionIntersectsEnclosingInterface = f.definition.UnionNodeIntersectsInterfaceNode(fragmentNode, info.EnclosingTypeDefinition)
	}

	if info.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentTypeIsMemberOfEnclosingUnionType = f.definition.NodeIsUnionMember(fragmentNode, info.EnclosingTypeDefinition)
	}

	nestedDepth, ok := f.depths.ByRef(ref)
	if !ok {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("nested depth missing on depths for FragmentSpread: %s", f.operation.FragmentSpreadNameString(ref)),
		}
	}

	precedence := asttransform.Precedence{
		Depth: nestedDepth,
		Order: 0,
	}

	replaceWith := f.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := f.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	switch {
	case fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType:
		f.transformer.ReplaceFragmentSpread(precedence, info.SelectionSet, ref, replaceWith)
	case fragmentTypeImplementsEnclosingType || fragmentTypeIsMemberOfEnclosingUnionType || enclosingTypeIsMemberOfFragmentUnion || fragmentUnionIntersectsEnclosingInterface:
		f.transformer.ReplaceFragmentSpreadWithInlineFragment(precedence, info.SelectionSet, ref, replaceWith, typeCondition)
	}

	return astvisitor.Instruction{}
}
