package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendObjectTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendObjectTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterObjectTypeDefinitionVisitor(&visitor)
	return
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.operation = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeDefinition(ref int) {
	return
}

func (e *extendObjectTypeDefinitionVisitor) LeaveObjectTypeDefinition(ref int) {
	return
}
