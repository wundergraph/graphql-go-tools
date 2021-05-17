package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendEnumTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendEnumTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterEnumTypeExtensionVisitor(&visitor)
}

type extendEnumTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendEnumTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendEnumTypeDefinitionVisitor) EnterEnumTypeExtension(ref int) {
	name := e.operation.EnumTypeExtensionNameBytes(ref)
	nodes, exists := e.operation.Index.NodesByNameBytes(name)
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindEnumTypeDefinition {
			continue
		}
		e.operation.ExtendEnumTypeDefinitionByEnumTypeExtension(nodes[i].Ref, ref)
		return
	}

	e.operation.ImportEnumTypeDefinitionWithDirectives(
		name.String(),
		e.operation.EnumTypeExtensionDescriptionString(ref),
		e.operation.EnumTypeExtensions[ref].EnumValuesDefinition.Refs,
		e.operation.EnumTypeExtensions[ref].Directives.Refs,
	)
}
