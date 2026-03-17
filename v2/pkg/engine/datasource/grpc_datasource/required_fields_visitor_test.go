package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func runRequiredFieldsVisitor(t *testing.T, schema string, mapping *GRPCMapping, typeName, requiredFields string) *RPCMessage {
	t.Helper()
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
	report := operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	// Pre-generate the fragment doc so the planCtx.operation matches what the walker will use.
	fragmentDoc, fragReport := plan.RequiredFieldsFragment(typeName, requiredFields, false)
	require.False(t, fragReport != nil && fragReport.HasErrors())

	planCtx := newRPCPlanningContext(fragmentDoc, &definition, mapping)
	message := &RPCMessage{Name: "TestMessage"}
	walker := astvisitor.NewWalker(0)
	visitor := newRequiredFieldsVisitor(&walker, message, planCtx)
	err := visitor.visitWithDefaults(&definition, typeName, requiredFields)
	require.NoError(t, err)
	return message
}

func runRequiredFieldsVisitorWithConfig(t *testing.T, schema string, mapping *GRPCMapping, typeName, requiredFields string, config requiredFieldVisitorConfig) *RPCMessage {
	t.Helper()
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
	report := operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	fragmentDoc, fragReport := plan.RequiredFieldsFragment(typeName, requiredFields, false)
	require.False(t, fragReport != nil && fragReport.HasErrors())

	planCtx := newRPCPlanningContext(fragmentDoc, &definition, mapping)
	message := &RPCMessage{Name: "TestMessage"}
	walker := astvisitor.NewWalker(0)
	visitor := newRequiredFieldsVisitor(&walker, message, planCtx)
	err := visitor.visit(&definition, typeName, requiredFields, config)
	require.NoError(t, err)
	return message
}

func TestRequiredFieldsVisitor_ScalarFields(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! name: String! itemCount: Int! price: Float! active: Boolean! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage": {
				"itemCount": {TargetName: "item_count"},
			},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "itemCount")

	require.Len(t, message.Fields, 1)
	field := message.Fields[0]
	assert.Equal(t, "item_count", field.Name)
	assert.Equal(t, DataTypeInt32, field.ProtoTypeName)
	assert.Equal(t, "itemCount", field.JSONPath)
}

func TestRequiredFieldsVisitor_MultipleScalarFields(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! name: String! itemCount: Int! price: Float! active: Boolean! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage": {
				"itemCount": {TargetName: "item_count"},
			},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "itemCount name")

	require.Len(t, message.Fields, 2)

	assert.Equal(t, "item_count", message.Fields[0].Name)
	assert.Equal(t, DataTypeInt32, message.Fields[0].ProtoTypeName)
	assert.Equal(t, "itemCount", message.Fields[0].JSONPath)

	assert.Equal(t, "name", message.Fields[1].Name)
	assert.Equal(t, DataTypeString, message.Fields[1].ProtoTypeName)
	assert.Equal(t, "name", message.Fields[1].JSONPath)
}

func TestRequiredFieldsVisitor_NestedObject(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! metadata: Metadata! }
		type Metadata { capacity: Int! zone: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":  {"metadata": {TargetName: "metadata"}},
			"Metadata": {"capacity": {TargetName: "capacity"}, "zone": {TargetName: "zone"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "metadata { capacity zone }")

	require.Len(t, message.Fields, 1)
	metadataField := message.Fields[0]
	assert.Equal(t, "metadata", metadataField.Name)
	assert.Equal(t, DataTypeMessage, metadataField.ProtoTypeName)
	require.NotNil(t, metadataField.Message)
	assert.Equal(t, "Metadata", metadataField.Message.Name)

	require.Len(t, metadataField.Message.Fields, 2)
	assert.Equal(t, "capacity", metadataField.Message.Fields[0].Name)
	assert.Equal(t, DataTypeInt32, metadataField.Message.Fields[0].ProtoTypeName)
	assert.Equal(t, "zone", metadataField.Message.Fields[1].Name)
	assert.Equal(t, DataTypeString, metadataField.Message.Fields[1].ProtoTypeName)
}

func TestRequiredFieldsVisitor_DeeplyNestedObject(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! metadata: Metadata! }
		type Metadata { inner: InnerData! }
		type InnerData { value: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":   {"metadata": {TargetName: "metadata"}},
			"Metadata":  {"inner": {TargetName: "inner"}},
			"InnerData": {"value": {TargetName: "value"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "metadata { inner { value } }")

	require.Len(t, message.Fields, 1)
	metadataField := message.Fields[0]
	require.NotNil(t, metadataField.Message)
	assert.Equal(t, "Metadata", metadataField.Message.Name)

	require.Len(t, metadataField.Message.Fields, 1)
	innerField := metadataField.Message.Fields[0]
	assert.Equal(t, "inner", innerField.Name)
	assert.Equal(t, DataTypeMessage, innerField.ProtoTypeName)
	require.NotNil(t, innerField.Message)
	assert.Equal(t, "InnerData", innerField.Message.Name)

	require.Len(t, innerField.Message.Fields, 1)
	assert.Equal(t, "value", innerField.Message.Fields[0].Name)
	assert.Equal(t, DataTypeString, innerField.Message.Fields[0].ProtoTypeName)
}

func TestRequiredFieldsVisitor_EnumField(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! kind: StorageKind! }
		enum StorageKind { COLD HOT }
	`
	mapping := &GRPCMapping{}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "kind")

	require.Len(t, message.Fields, 1)
	field := message.Fields[0]
	assert.Equal(t, "kind", field.Name)
	assert.Equal(t, DataTypeEnum, field.ProtoTypeName)
	assert.Equal(t, "StorageKind", field.EnumName)
}

func TestRequiredFieldsVisitor_NestedObjectWithEnum(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! info: Info! }
		type Info { kind: StorageKind! name: String! }
		enum StorageKind { COLD HOT }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage": {"info": {TargetName: "info"}},
			"Info":    {"kind": {TargetName: "kind"}, "name": {TargetName: "name"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "info { kind name }")

	require.Len(t, message.Fields, 1)
	infoField := message.Fields[0]
	require.NotNil(t, infoField.Message)

	require.Len(t, infoField.Message.Fields, 2)

	kindField := infoField.Message.Fields[0]
	assert.Equal(t, "kind", kindField.Name)
	assert.Equal(t, DataTypeEnum, kindField.ProtoTypeName)
	assert.Equal(t, "StorageKind", kindField.EnumName)

	nameField := infoField.Message.Fields[1]
	assert.Equal(t, "name", nameField.Name)
	assert.Equal(t, DataTypeString, nameField.ProtoTypeName)
}

func TestRequiredFieldsVisitor_ListField(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! tags: [String!]! }
	`
	mapping := &GRPCMapping{}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "tags")

	require.Len(t, message.Fields, 1)
	field := message.Fields[0]
	assert.Equal(t, "tags", field.Name)
	assert.True(t, field.Repeated)
	assert.Equal(t, DataTypeString, field.ProtoTypeName)
}

func TestRequiredFieldsVisitor_InterfaceField(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! pet: Animal! }
		interface Animal { name: String! }
		type Cat implements Animal { name: String! meowVolume: Int! }
		type Dog implements Animal { name: String! barkVolume: Int! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage": {"pet": {TargetName: "pet"}},
			"Cat":     {"name": {TargetName: "name"}, "meowVolume": {TargetName: "meow_volume"}},
			"Dog":     {"name": {TargetName: "name"}, "barkVolume": {TargetName: "bark_volume"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", `pet { ... on Cat { name meowVolume } ... on Dog { name barkVolume } }`)

	require.Len(t, message.Fields, 1)
	petField := message.Fields[0]
	assert.Equal(t, "pet", petField.Name)
	assert.Equal(t, DataTypeMessage, petField.ProtoTypeName)
	require.NotNil(t, petField.Message)

	assert.Equal(t, OneOfTypeInterface, petField.Message.OneOfType)
	assert.ElementsMatch(t, []string{"Cat", "Dog"}, petField.Message.MemberTypes)

	require.NotNil(t, petField.Message.FragmentFields)
	catFields := petField.Message.FragmentFields["Cat"]
	require.Len(t, catFields, 2)
	assert.Equal(t, "name", catFields[0].Name)
	assert.Equal(t, DataTypeString, catFields[0].ProtoTypeName)
	assert.Equal(t, "meow_volume", catFields[1].Name)
	assert.Equal(t, DataTypeInt32, catFields[1].ProtoTypeName)

	dogFields := petField.Message.FragmentFields["Dog"]
	require.Len(t, dogFields, 2)
	assert.Equal(t, "name", dogFields[0].Name)
	assert.Equal(t, DataTypeString, dogFields[0].ProtoTypeName)
	assert.Equal(t, "bark_volume", dogFields[1].Name)
	assert.Equal(t, DataTypeInt32, dogFields[1].ProtoTypeName)
}

func TestRequiredFieldsVisitor_UnionField(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! result: ActionResult! }
		union ActionResult = ActionSuccess | ActionError
		type ActionSuccess { message: String! timestamp: String! }
		type ActionError { message: String! code: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":       {"result": {TargetName: "result"}},
			"ActionSuccess": {"message": {TargetName: "message"}, "timestamp": {TargetName: "timestamp"}},
			"ActionError":   {"message": {TargetName: "message"}, "code": {TargetName: "code"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", `result { ... on ActionSuccess { message timestamp } ... on ActionError { message code } }`)

	require.Len(t, message.Fields, 1)
	resultField := message.Fields[0]
	assert.Equal(t, "result", resultField.Name)
	assert.Equal(t, DataTypeMessage, resultField.ProtoTypeName)
	require.NotNil(t, resultField.Message)

	assert.Equal(t, OneOfTypeUnion, resultField.Message.OneOfType)
	assert.ElementsMatch(t, []string{"ActionSuccess", "ActionError"}, resultField.Message.MemberTypes)

	require.NotNil(t, resultField.Message.FragmentFields)
	successFields := resultField.Message.FragmentFields["ActionSuccess"]
	require.Len(t, successFields, 2)
	assert.Equal(t, "message", successFields[0].Name)
	assert.Equal(t, DataTypeString, successFields[0].ProtoTypeName)
	assert.Equal(t, "timestamp", successFields[1].Name)
	assert.Equal(t, DataTypeString, successFields[1].ProtoTypeName)

	errorFields := resultField.Message.FragmentFields["ActionError"]
	require.Len(t, errorFields, 2)
	assert.Equal(t, "message", errorFields[0].Name)
	assert.Equal(t, DataTypeString, errorFields[0].ProtoTypeName)
	assert.Equal(t, "code", errorFields[1].Name)
	assert.Equal(t, DataTypeString, errorFields[1].ProtoTypeName)
}

func TestRequiredFieldsVisitor_ReferenceNestedMessages(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! metadata: Metadata! }
		type Metadata { capacity: Int! zone: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":  {"metadata": {TargetName: "metadata"}},
			"Metadata": {"capacity": {TargetName: "capacity"}, "zone": {TargetName: "zone"}},
		},
	}

	message := runRequiredFieldsVisitorWithConfig(t, schema, mapping, "Storage", "metadata { capacity zone }", requiredFieldVisitorConfig{
		includeMemberType:       false,
		skipFieldResolvers:      false,
		referenceNestedMessages: true,
	})

	require.Len(t, message.Fields, 1)
	metadataField := message.Fields[0]
	require.NotNil(t, metadataField.Message)
	assert.Equal(t, "TestMessage.Metadata", metadataField.Message.Name)
}

func TestRequiredFieldsVisitor_MemberTypes(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! name: String! }
	`
	mapping := &GRPCMapping{}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "name")

	assert.Equal(t, []string{"Storage"}, message.MemberTypes)
}

func TestRequiredFieldsVisitor_DuplicateFields(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! name: String! }
	`
	mapping := &GRPCMapping{}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage", "name name")

	require.Len(t, message.Fields, 1)
	assert.Equal(t, "name", message.Fields[0].Name)
	assert.Equal(t, DataTypeString, message.Fields[0].ProtoTypeName)
}
