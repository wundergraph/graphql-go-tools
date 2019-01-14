package document

import "bytes"

// InterfaceTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InterfaceTypeDefinition
type InterfaceTypeDefinition struct {
	Description      ByteSlice
	Name             ByteSlice
	FieldsDefinition []int
	Directives       []int
}

func (i InterfaceTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFieldsDefinition() []int {
	return i.FieldsDefinition
}

func (i InterfaceTypeDefinition) NodeName() string {
	return string(i.Name)
}

func (i InterfaceTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDescription() string {
	return string(i.Description)
}

func (i InterfaceTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDirectives() []int {
	return i.Directives
}

func (i InterfaceTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeType() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

// InterfaceTypeDefinitions is the plural of InterfaceTypeDefinition
type InterfaceTypeDefinitions []InterfaceTypeDefinition

// GetByName returns the interface type definition by name if contained
func (i InterfaceTypeDefinitions) GetByName(name ByteSlice) *InterfaceTypeDefinition {
	for _, iFace := range i {
		if bytes.Equal(iFace.Name, name) {
			return &iFace
		}
	}

	return nil
}
