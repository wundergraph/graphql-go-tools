package document

import (
	"bytes"
)

// EnumValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumValueDefinition
type EnumValueDefinition struct {
	Description ByteSlice
	EnumValue   ByteSlice
	Directives  []int
}

func (e EnumValueDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (e EnumValueDefinition) NodeValueReference() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (e EnumValueDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (e EnumValueDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (e EnumValueDefinition) NodeValue() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeAlias() string {
	panic("implement me")
}

func (e EnumValueDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (e EnumValueDefinition) NodeType() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeVariableDefinitions() []int {
	return nil
}

func (e EnumValueDefinition) NodeFields() []int {
	return nil
}

func (e EnumValueDefinition) NodeFragmentSpreads() []int {
	return nil
}

func (e EnumValueDefinition) NodeInlineFragments() []int {
	return nil
}

func (e EnumValueDefinition) NodeName() string {
	return string(e.EnumValue)
}

func (e EnumValueDefinition) NodeDescription() string {
	return string(e.Description)
}

func (e EnumValueDefinition) NodeArguments() []int {
	return nil
}

func (e EnumValueDefinition) NodeDirectives() []int {
	return e.Directives
}

func (e EnumValueDefinition) NodeEnumValuesDefinition() []int {
	return nil
}

// ProperCaseVal returns the EnumValueDefinition's EnumValue
// as proper case string. example:
// NORTH => North
func (e EnumValueDefinition) ProperCaseVal() ByteSlice {
	return bytes.Title(bytes.ToLower(e.EnumValue))
}
