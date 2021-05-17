package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendInterfaceTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendInterfaceTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(&visitor)
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterInterfaceTypeExtension(ref int) {
	name := e.operation.InterfaceTypeExtensionNameBytes(ref)
	nodes, exists := e.operation.Index.NodesByNameBytes(name)
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		e.operation.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(nodes[i].Ref, ref)
		return
	}

	e.operation.ImportInterfaceTypeDefinitionWithDirectives(
		name.String(),
		e.operation.InterfaceTypeExtensionDescriptionString(ref),
		e.operation.InterfaceTypeExtensions[ref].FieldsDefinition.Refs,
		e.operation.InterfaceTypeExtensions[ref].Directives.Refs,
	)
}
