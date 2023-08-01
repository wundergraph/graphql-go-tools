package asttransform

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const typenameFieldName = "__typename"

type TypeNameVisitor struct {
	*astvisitor.Walker
	definition *ast.Document
}

func NewTypeNameVisitor() *TypeNameVisitor {
	walker := astvisitor.NewWalker(48)

	visitor := &TypeNameVisitor{
		Walker: &walker,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterLeaveObjectTypeDefinitionVisitor(visitor)
	walker.RegisterLeaveInterfaceTypeDefinitionVisitor(visitor)
	walker.RegisterLeaveUnionTypeDefinitionVisitor(visitor)

	return visitor
}

func (v *TypeNameVisitor) ExtendSchema(definition *ast.Document) error {
	report := &operationreport.Report{}

	v.Walk(definition, definition, report)

	if report.HasErrors() {
		return report
	}
	return nil
}

func (v *TypeNameVisitor) EnterDocument(definition, _ *ast.Document) {
	v.definition = definition
}

func (v *TypeNameVisitor) LeaveInterfaceTypeDefinition(ref int) {
	if v.definition.InterfaceTypeDefinitions[ref].HasFieldDefinitions &&
		v.definition.FieldDefinitionsContainField(v.definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs, literal.TYPENAME) {
		return
	}

	v.definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs = append(v.definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs, v.addTypeNameField())
	v.definition.InterfaceTypeDefinitions[ref].HasFieldDefinitions = true
}

func (v *TypeNameVisitor) LeaveObjectTypeDefinition(ref int) {
	objectTypeDefName := v.definition.ObjectTypeDefinitionNameBytes(ref)
	if bytes.Equal(objectTypeDefName, v.definition.Index.SubscriptionTypeName) ||
		bytes.Equal(objectTypeDefName, ast.DefaultSubscriptionTypeName) {
		return
	}

	if v.definition.ObjectTypeDefinitions[ref].HasFieldDefinitions &&
		v.definition.ObjectTypeDefinitionHasField(ref, literal.TYPENAME) {
		return
	}

	v.definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs = append(v.definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs, v.addTypeNameField())
	v.definition.ObjectTypeDefinitions[ref].HasFieldDefinitions = true
}

func (v *TypeNameVisitor) LeaveUnionTypeDefinition(ref int) {
	if v.definition.UnionTypeDefinitions[ref].HasFieldDefinitions &&
		v.definition.UnionTypeDefinitionHasField(ref, literal.TYPENAME) {
		return // this makes the operation idempotent
	}
	v.definition.UnionTypeDefinitions[ref].FieldsDefinition.Refs = []int{v.addTypeNameField()}
	v.definition.UnionTypeDefinitions[ref].HasFieldDefinitions = true
}

func (v *TypeNameVisitor) addTypeNameField() (ref int) {
	typeRef := v.definition.AddNonNullNamedType(literal.STRING)
	return v.definition.ImportFieldDefinition(typenameFieldName, "", typeRef, nil, nil)
}
