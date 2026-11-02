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
	walker.RegisterLeaveDocumentVisitor(&visitor)
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
	// importedSchemaDefinitionRef is the schema definition created from an orphan schema extension
	importedSchemaDefinitionRef int
}

func (e *extendSchemaDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
	e.importedSchemaDefinitionRef = ast.InvalidRef
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
	e.importedSchemaDefinitionRef = e.operation.SchemaDefinitionRef()
}

// LeaveDocument runs after every schema extension is merged,
// so that the implicit root operation types only fill what no schema extension defines.
func (e *extendSchemaDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if e.importedSchemaDefinitionRef == ast.InvalidRef {
		return
	}

	e.addImplicitRootOperationTypes(e.importedSchemaDefinitionRef)
}

// addImplicitRootOperationTypes adds the default root operation types the schema extensions do not define.
// Orphan schema extensions extend the implicit schema, so e.g. `extend schema { mutation: CustomMutation }`
// still has to resolve `Query` as the query type. implicitSchemaDefinition can't do it,
// as it leaves schema definitions with root operation types untouched.
func (e *extendSchemaDefinitionVisitor) addImplicitRootOperationTypes(schemaDefinitionRef int) {
	implicitTypeNames := []struct {
		operationType ast.OperationType
		typeName      string
	}{
		{ast.OperationTypeQuery, implicitQueryTypeName},
		{ast.OperationTypeMutation, implicitMutationTypeName},
		{ast.OperationTypeSubscription, implicitSubscriptionTypeName},
	}

	for _, implicit := range implicitTypeNames {
		if e.hasRootOperationType(schemaDefinitionRef, implicit.operationType) {
			continue
		}

		typeName := implicitRootOperationTypeName(implicit.typeName, e.operation)
		if typeName == "" {
			continue
		}

		e.operation.SchemaDefinitions[schemaDefinitionRef].AddRootOperationTypeDefinitionRefs(
			e.operation.ImportRootOperationTypeDefinition(typeName, implicit.operationType),
		)
	}
}

func (e *extendSchemaDefinitionVisitor) hasRootOperationType(schemaDefinitionRef int, operationType ast.OperationType) bool {
	for _, ref := range e.operation.SchemaDefinitions[schemaDefinitionRef].RootOperationTypeDefinitions.Refs {
		if e.operation.RootOperationTypeDefinitions[ref].OperationType == operationType {
			return true
		}
	}

	return false
}
