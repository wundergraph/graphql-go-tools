package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendObjectTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendObjectTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterObjectTypeExtensionVisitor(&visitor)
	return
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {

	extension := e.operation.ObjectTypeExtensions[ref]

	if extension.HasFieldDefinitions {
		for fieldDefinitionRef, _ := range extension.FieldsDefinition.Refs {
			e.operation.ExtendObjectTypeDefinitionByFieldDefinition(extension.ObjectTypeDefinition, fieldDefinitionRef)
		}
	}

	if extension.HasDirectives {
		for directiveRef, _ := range extension.Directives.Refs {
			e.operation.ExtendObjectTypeDefinitionByDirective(extension.ObjectTypeDefinition, directiveRef)
		}
	}

	return
}
