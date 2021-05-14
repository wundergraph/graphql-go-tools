package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendUnionTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendUnionTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterUnionTypeExtensionVisitor(&visitor)
}

type extendUnionTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendUnionTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendUnionTypeDefinitionVisitor) EnterUnionTypeExtension(ref int) {
	name := e.operation.UnionTypeExtensionNameBytes(ref)
	nodes, exists := e.operation.Index.NodesByNameBytes(name)
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindUnionTypeDefinition {
			continue
		}
		e.operation.ExtendUnionTypeDefinitionByUnionTypeExtension(nodes[i].Ref, ref)
		return
	}

	e.operation.ImportUnionTypeDefinitionWithDirectives(
		name.String(),
		e.operation.UnionTypeExtensionDescriptionString(ref),
		e.operation.UnionTypeExtensions[ref].UnionMemberTypes.Refs,
		e.operation.UnionTypeExtensions[ref].Directives.Refs,
	)
}
