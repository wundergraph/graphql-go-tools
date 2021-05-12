package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendObjectTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendObjectTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterObjectTypeExtensionVisitor(&visitor)
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {

	name := e.operation.ObjectTypeExtensionNameBytes(ref)
	nodes, exists := e.operation.Index.NodesByNameBytes(name)
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		e.operation.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		return
	}

	e.operation.ImportObjectTypeDefinition(
		name.String(),
		e.operation.ObjectTypeExtensionDescriptionNameString(ref),
		e.operation.ObjectTypeExtensions[ref].FieldsDefinition.Refs,
		e.operation.ObjectTypeExtensions[ref].ImplementsInterfaces.Refs,
	)
}
