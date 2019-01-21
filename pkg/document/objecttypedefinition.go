package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// ObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ObjectTypeDefinition
type ObjectTypeDefinition struct {
	Description          ByteSliceReference
	Name                 ByteSliceReference
	FieldsDefinition     []int
	ImplementsInterfaces ImplementsInterfaces
	Directives           []int
	Position             position.Position
}

func (o ObjectTypeDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodePosition() position.Position {
	return o.Position
}

func (o ObjectTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeImplementsInterfaces() []ByteSliceReference {
	return o.ImplementsInterfaces
}

func (o ObjectTypeDefinition) NodeName() ByteSliceReference {
	return o.Name
}

func (o ObjectTypeDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDescription() ByteSliceReference {
	return o.Description
}

func (o ObjectTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDirectives() []int {
	return o.Directives
}

func (o ObjectTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeFieldsDefinition() []int {
	return o.FieldsDefinition
}

func (o ObjectTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeType() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

// ObjectTypeDefinitions is the plural of ObjectTypeDefinition
type ObjectTypeDefinitions []ObjectTypeDefinition
