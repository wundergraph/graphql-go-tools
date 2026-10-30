package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func extendSchemaDefinition(walker *astvisitor.Walker) {
	visitor := extendSchemaDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSchemaExtensionVisitor(&visitor)
}

func extendSchemaDefinitionKeepingOrphans(walker *astvisitor.Walker) {
	visitor := extendSchemaDefinitionVisitor{
		Walker:               walker,
		keepExtensionOrphans: true,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSchemaExtensionVisitor(&visitor)
}

type extendSchemaDefinitionVisitor struct {
	*astvisitor.Walker

	operation            *ast.Document
	keepExtensionOrphans bool
}

func (e *extendSchemaDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendSchemaDefinitionVisitor) EnterSchemaExtension(ref int) {
	schemaDefinitionRef := e.operation.SchemaDefinitionRef()
	if schemaDefinitionRef != ast.InvalidRef {
		e.operation.ExtendSchemaDefinitionBySchemaExtension(schemaDefinitionRef, ref)
		return
	}

	if e.keepExtensionOrphans {
		return
	}

	e.operation.ImportAndExtendSchemaDefinitionBySchemaExtension(ref)
}
