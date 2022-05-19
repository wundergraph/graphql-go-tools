package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type validateTypeBody struct {
	*astvisitor.Walker
	operation        *ast.Document
	lastEnumRef      int
	lastInputRef     int
	lastInterfaceRef int
	lastObjectRef    int
}

func validateTypeBodyVisitor(walker *astvisitor.Walker) {
	visitor := validateTypeBody{
		Walker:           walker,
		operation:        nil,
		lastEnumRef:      ast.InvalidRef,
		lastInputRef:     ast.InvalidRef,
		lastInterfaceRef: ast.InvalidRef,
		lastObjectRef:    ast.InvalidRef,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterEnumTypeDefinitionVisitor(&visitor)
	walker.RegisterEnterInputObjectTypeDefinitionVisitor(&visitor)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(&visitor)
	walker.RegisterEnterObjectTypeDefinitionVisitor(&visitor)
}

func (v *validateTypeBody) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
}

func (v validateTypeBody) EnterEnumTypeDefinition(ref int) {
	if v.lastEnumRef >= ref {
		return
	}
	operation := v.operation
	if !operation.EnumTypeDefinitions[ref].HasEnumValuesDefinition {
		v.Walker.StopWithExternalErr(operationreport.ErrTypeBodyMustNotBeEmpty("enum", operation.EnumTypeDefinitionNameString(ref)))
	}
	v.lastEnumRef = ref
}

func (v validateTypeBody) EnterInputObjectTypeDefinition(ref int) {
	if v.lastInputRef >= ref {
		return
	}
	operation := v.operation
	if !operation.InputObjectTypeDefinitions[ref].HasInputFieldsDefinition {
		v.Walker.StopWithExternalErr(operationreport.ErrTypeBodyMustNotBeEmpty("input", operation.InputObjectTypeDefinitionNameString(ref)))
	}
	v.lastInputRef = ref
}

func (v validateTypeBody) EnterInterfaceTypeDefinition(ref int) {
	if v.lastInterfaceRef >= ref {
		return
	}
	operation := v.operation
	if !operation.InterfaceTypeDefinitions[ref].HasFieldDefinitions {
		v.Walker.StopWithExternalErr(operationreport.ErrTypeBodyMustNotBeEmpty("interface", operation.InterfaceTypeDefinitionNameString(ref)))
	}
	v.lastInterfaceRef = ref
}

func (v validateTypeBody) EnterObjectTypeDefinition(ref int) {
	if v.lastObjectRef >= ref {
		return
	}
	operation := v.operation
	if !operation.ObjectTypeDefinitions[ref].HasFieldDefinitions {
		v.Walker.StopWithExternalErr(operationreport.ErrTypeBodyMustNotBeEmpty("object", operation.ObjectTypeDefinitionNameString(ref)))
	}
	v.lastObjectRef = ref
}
