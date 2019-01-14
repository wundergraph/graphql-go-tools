package document

import (
	"bytes"
)

// EnumTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumTypeDefinition
type EnumTypeDefinition struct {
	Description          ByteSlice
	Name                 ByteSlice
	EnumValuesDefinition []int
	Directives           []int
}

func (e EnumTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeType() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeVariableDefinitions() []int {
	return nil
}

func (e EnumTypeDefinition) NodeFields() []int {
	return nil
}

func (e EnumTypeDefinition) NodeFragmentSpreads() []int {
	return nil
}

func (e EnumTypeDefinition) NodeInlineFragments() []int {
	return nil
}

func (e EnumTypeDefinition) NodeEnumValuesDefinition() []int {
	return e.EnumValuesDefinition
}

func (e EnumTypeDefinition) NodeName() string {
	return string(e.Name)
}

func (e EnumTypeDefinition) NodeDescription() string {
	return string(e.Description)
}

func (e EnumTypeDefinition) NodeArguments() []int {
	return nil
}

func (e EnumTypeDefinition) NodeDirectives() []int {
	return e.Directives
}

// TitleCaseName returns the EnumTypeDefinition's Name
// as title case string. example:
// episode => Episode
func (e EnumTypeDefinition) TitleCaseName() ByteSlice {
	return bytes.Title(e.Name)
}

// EnumTypeDefinitions is the plural of EnumTypeDefinition
type EnumTypeDefinitions []EnumTypeDefinition

// HasDefinition returns true if a EnumTypeDefinition with $name is contained
func (e EnumTypeDefinitions) HasDefinition(name ByteSlice) bool {
	for _, definition := range e {
		if bytes.Equal(definition.Name, name) {
			return true
		}
	}

	return false
}
