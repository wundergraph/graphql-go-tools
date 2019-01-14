package document

import "bytes"

// InputObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputObjectTypeDefinition
type InputObjectTypeDefinition struct {
	Description           ByteSlice
	Name                  ByteSlice
	InputFieldsDefinition []int
	Directives            []int
}

func (i InputObjectTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeName() string {
	return string(i.Name)
}

func (i InputObjectTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDescription() string {
	return string(i.Description)
}

func (i InputObjectTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDirectives() []int {
	return i.Directives
}

func (i InputObjectTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeFields() []int {
	return i.InputFieldsDefinition
}

func (i InputObjectTypeDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeType() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

// InputObjectTypeDefinitions is the plural of InputObjectTypeDefinition
type InputObjectTypeDefinitions []InputObjectTypeDefinition

// HasDefinition returns true if an InputObjectTypeDefinition with $name is contained
func (i InputObjectTypeDefinitions) HasDefinition(name ByteSlice) bool {

	for _, definition := range i {
		if bytes.Equal(definition.Name, name) {
			return true
		}
	}

	return false
}

// GetByName returns a InputObjectTypeDefinition by $name or nil if not found
func (i InputObjectTypeDefinitions) GetByName(name ByteSlice) *InputObjectTypeDefinition {
	for _, definition := range i {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
