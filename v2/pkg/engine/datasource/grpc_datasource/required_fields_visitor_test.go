package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func runRequiredFieldsVisitor(t *testing.T, schema string, mapping *GRPCMapping, typeName, requiredFields string) *RPCMessage {
	t.Helper()
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
	report := operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	message := &RPCMessage{Name: "TestMessage"}
	walker := astvisitor.NewWalker(0)
	visitor := newRequiredFieldsVisitor(&walker, message, mapping)
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

	message := &RPCMessage{Name: "TestMessage"}
	walker := astvisitor.NewWalker(0)
	visitor := newRequiredFieldsVisitor(&walker, message, mapping)
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

func TestRequiredFieldsVisitor_InterfaceWithNestedConcreteMessage(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! item: StorageItem! }
		interface StorageItem { id: ID! name: String! }
		type PalletItem implements StorageItem { id: ID! name: String! handler: ItemHandler! }
		type ContainerItem implements StorageItem { id: ID! name: String! handler: ItemHandler! }
		type ItemHandler { id: ID! name: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":       {"item": {TargetName: "item"}},
			"PalletItem":    {"handler": {TargetName: "handler"}},
			"ContainerItem": {"handler": {TargetName: "handler"}},
			"ItemHandler":   {"name": {TargetName: "name"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage",
		`item { ... on PalletItem { handler { name } } ... on ContainerItem { handler { name } } }`)

	require.Len(t, message.Fields, 1)
	itemField := message.Fields[0]
	assert.Equal(t, "item", itemField.Name)
	assert.Equal(t, DataTypeMessage, itemField.ProtoTypeName)
	require.NotNil(t, itemField.Message)
	assert.Equal(t, OneOfTypeInterface, itemField.Message.OneOfType)
	assert.ElementsMatch(t, []string{"PalletItem", "ContainerItem"}, itemField.Message.MemberTypes)

	require.NotNil(t, itemField.Message.FragmentFields)

	palletFields := itemField.Message.FragmentFields["PalletItem"]
	require.Len(t, palletFields, 1)
	assert.Equal(t, "handler", palletFields[0].Name)
	assert.Equal(t, DataTypeMessage, palletFields[0].ProtoTypeName)
	require.NotNil(t, palletFields[0].Message)
	assert.Equal(t, "ItemHandler", palletFields[0].Message.Name)
	require.Len(t, palletFields[0].Message.Fields, 1)
	assert.Equal(t, "name", palletFields[0].Message.Fields[0].Name)
	assert.Equal(t, DataTypeString, palletFields[0].Message.Fields[0].ProtoTypeName)

	containerFields := itemField.Message.FragmentFields["ContainerItem"]
	require.Len(t, containerFields, 1)
	assert.Equal(t, "handler", containerFields[0].Name)
	assert.Equal(t, DataTypeMessage, containerFields[0].ProtoTypeName)
	require.NotNil(t, containerFields[0].Message)
	assert.Equal(t, "ItemHandler", containerFields[0].Message.Name)
	require.Len(t, containerFields[0].Message.Fields, 1)
	assert.Equal(t, "name", containerFields[0].Message.Fields[0].Name)
	assert.Equal(t, DataTypeString, containerFields[0].Message.Fields[0].ProtoTypeName)
}

func TestRequiredFieldsVisitor_InterfaceWithDeepConcreteNesting(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! item: StorageItem! }
		interface StorageItem { id: ID! name: String! }
		type PalletItem implements StorageItem { id: ID! name: String! specs: PalletSpecs! }
		type ContainerItem implements StorageItem { id: ID! name: String! specs: ContainerSpecs! }
		type PalletSpecs { name: String! dimensions: Dimensions! }
		type ContainerSpecs { name: String! dimensions: Dimensions! }
		type Dimensions { length: Float! width: Float! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":        {"item": {TargetName: "item"}},
			"PalletItem":     {"specs": {TargetName: "specs"}},
			"ContainerItem":  {"specs": {TargetName: "specs"}},
			"PalletSpecs":    {"name": {TargetName: "name"}, "dimensions": {TargetName: "dimensions"}},
			"ContainerSpecs": {"name": {TargetName: "name"}, "dimensions": {TargetName: "dimensions"}},
			"Dimensions":     {"length": {TargetName: "length"}, "width": {TargetName: "width"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage",
		`item { ... on PalletItem { specs { name dimensions { length width } } } ... on ContainerItem { specs { name dimensions { length width } } } }`)

	require.Len(t, message.Fields, 1)
	itemField := message.Fields[0]
	assert.Equal(t, OneOfTypeInterface, itemField.Message.OneOfType)

	// Verify PalletItem fragment: specs → dimensions
	palletFields := itemField.Message.FragmentFields["PalletItem"]
	require.Len(t, palletFields, 1)
	specsField := palletFields[0]
	assert.Equal(t, "specs", specsField.Name)
	assert.Equal(t, DataTypeMessage, specsField.ProtoTypeName)
	require.NotNil(t, specsField.Message)
	require.Len(t, specsField.Message.Fields, 2)
	assert.Equal(t, "name", specsField.Message.Fields[0].Name)
	dimsField := specsField.Message.Fields[1]
	assert.Equal(t, "dimensions", dimsField.Name)
	assert.Equal(t, DataTypeMessage, dimsField.ProtoTypeName)
	require.NotNil(t, dimsField.Message)
	require.Len(t, dimsField.Message.Fields, 2)
	assert.Equal(t, "length", dimsField.Message.Fields[0].Name)
	assert.Equal(t, DataTypeDouble, dimsField.Message.Fields[0].ProtoTypeName)
	assert.Equal(t, "width", dimsField.Message.Fields[1].Name)
	assert.Equal(t, DataTypeDouble, dimsField.Message.Fields[1].ProtoTypeName)

	// Verify ContainerItem fragment has same structure
	containerFields := itemField.Message.FragmentFields["ContainerItem"]
	require.Len(t, containerFields, 1)
	cSpecsField := containerFields[0]
	assert.Equal(t, "specs", cSpecsField.Name)
	require.NotNil(t, cSpecsField.Message)
	require.Len(t, cSpecsField.Message.Fields, 2)
	cDimsField := cSpecsField.Message.Fields[1]
	assert.Equal(t, "dimensions", cDimsField.Name)
	require.NotNil(t, cDimsField.Message)
	require.Len(t, cDimsField.Message.Fields, 2)
}

func TestRequiredFieldsVisitor_ConcreteWrappingAbstract(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! setup: SecuritySetup! }
		type SecuritySetup { securityLevel: String! primaryItem: StorageItem! }
		interface StorageItem { id: ID! name: String! }
		type PalletItem implements StorageItem { id: ID! name: String! palletCount: Int! }
		type ContainerItem implements StorageItem { id: ID! name: String! containerSize: String! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":       {"setup": {TargetName: "setup"}},
			"SecuritySetup": {"securityLevel": {TargetName: "security_level"}, "primaryItem": {TargetName: "primary_item"}},
			"PalletItem":    {"name": {TargetName: "name"}, "palletCount": {TargetName: "pallet_count"}},
			"ContainerItem": {"name": {TargetName: "name"}, "containerSize": {TargetName: "container_size"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage",
		`setup { securityLevel primaryItem { ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } } }`)

	require.Len(t, message.Fields, 1)
	setupField := message.Fields[0]
	assert.Equal(t, "setup", setupField.Name)
	assert.Equal(t, DataTypeMessage, setupField.ProtoTypeName)
	require.NotNil(t, setupField.Message)
	assert.Equal(t, "SecuritySetup", setupField.Message.Name)

	require.Len(t, setupField.Message.Fields, 2)
	assert.Equal(t, "security_level", setupField.Message.Fields[0].Name)
	assert.Equal(t, DataTypeString, setupField.Message.Fields[0].ProtoTypeName)

	itemField := setupField.Message.Fields[1]
	assert.Equal(t, "primary_item", itemField.Name)
	assert.Equal(t, DataTypeMessage, itemField.ProtoTypeName)
	require.NotNil(t, itemField.Message)
	assert.Equal(t, OneOfTypeInterface, itemField.Message.OneOfType)
	assert.ElementsMatch(t, []string{"PalletItem", "ContainerItem"}, itemField.Message.MemberTypes)

	palletFields := itemField.Message.FragmentFields["PalletItem"]
	require.Len(t, palletFields, 2)
	assert.Equal(t, "name", palletFields[0].Name)
	assert.Equal(t, "pallet_count", palletFields[1].Name)

	containerFields := itemField.Message.FragmentFields["ContainerItem"]
	require.Len(t, containerFields, 2)
	assert.Equal(t, "name", containerFields[0].Name)
	assert.Equal(t, "container_size", containerFields[1].Name)
}

func TestRequiredFieldsVisitor_NestedAbstractThroughConcrete(t *testing.T) {
	t.Parallel()

	schema := `
		type Storage { id: ID! item: StorageItem! }
		interface StorageItem { id: ID! name: String! }
		type PalletItem implements StorageItem { id: ID! name: String! palletCount: Int! handler: ItemHandler! }
		type ContainerItem implements StorageItem { id: ID! name: String! containerSize: String! handler: ItemHandler! }
		type ItemHandler { id: ID! name: String! assignedItem: StorageItem! }
	`
	mapping := &GRPCMapping{
		Fields: map[string]FieldMap{
			"Storage":       {"item": {TargetName: "item"}},
			"PalletItem":    {"handler": {TargetName: "handler"}, "name": {TargetName: "name"}, "palletCount": {TargetName: "pallet_count"}},
			"ContainerItem": {"handler": {TargetName: "handler"}, "name": {TargetName: "name"}, "containerSize": {TargetName: "container_size"}},
			"ItemHandler":   {"assignedItem": {TargetName: "assigned_item"}, "name": {TargetName: "name"}},
		},
	}

	message := runRequiredFieldsVisitor(t, schema, mapping, "Storage",
		`item { ... on PalletItem { handler { assignedItem { ... on ContainerItem { name containerSize } ... on PalletItem { name palletCount } } } } ... on ContainerItem { handler { name } } }`)

	require.Len(t, message.Fields, 1)
	itemField := message.Fields[0]
	assert.Equal(t, OneOfTypeInterface, itemField.Message.OneOfType)

	// PalletItem fragment: handler → assignedItem (abstract)
	palletFields := itemField.Message.FragmentFields["PalletItem"]
	require.Len(t, palletFields, 1)
	handlerField := palletFields[0]
	assert.Equal(t, "handler", handlerField.Name)
	assert.Equal(t, DataTypeMessage, handlerField.ProtoTypeName)
	require.NotNil(t, handlerField.Message)

	require.Len(t, handlerField.Message.Fields, 1)
	assignedField := handlerField.Message.Fields[0]
	assert.Equal(t, "assigned_item", assignedField.Name)
	assert.Equal(t, DataTypeMessage, assignedField.ProtoTypeName)
	require.NotNil(t, assignedField.Message)
	assert.Equal(t, OneOfTypeInterface, assignedField.Message.OneOfType)
	assert.ElementsMatch(t, []string{"ContainerItem", "PalletItem"}, assignedField.Message.MemberTypes)

	innerContainerFields := assignedField.Message.FragmentFields["ContainerItem"]
	require.Len(t, innerContainerFields, 2)
	assert.Equal(t, "name", innerContainerFields[0].Name)
	assert.Equal(t, "container_size", innerContainerFields[1].Name)

	innerPalletFields := assignedField.Message.FragmentFields["PalletItem"]
	require.Len(t, innerPalletFields, 2)
	assert.Equal(t, "name", innerPalletFields[0].Name)
	assert.Equal(t, "pallet_count", innerPalletFields[1].Name)

	// ContainerItem fragment: handler → name (scalar)
	containerFields := itemField.Message.FragmentFields["ContainerItem"]
	require.Len(t, containerFields, 1)
	cHandlerField := containerFields[0]
	assert.Equal(t, "handler", cHandlerField.Name)
	require.NotNil(t, cHandlerField.Message)
	require.Len(t, cHandlerField.Message.Fields, 1)
	assert.Equal(t, "name", cHandlerField.Message.Fields[0].Name)
	assert.Equal(t, DataTypeString, cHandlerField.Message.Fields[0].ProtoTypeName)
}
