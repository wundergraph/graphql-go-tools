package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InputFieldsDefinition as specified in:
// https://facebook.github.io/graphql/draft/#InputFieldsDefinition
type InputFieldsDefinition struct {
	Position              position.Position
	InputValueDefinitions []int
}

func (i InputFieldsDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeName() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeAlias() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeDirectiveSet() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeFields() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeType() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeValue() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeInputValueDefinitions() []int {
	return i.InputValueDefinitions
}

func (i InputFieldsDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputFieldsDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputFieldsDefinition) NodePosition() position.Position {
	return i.Position
}

type InputFieldsDefinitions []InputFieldsDefinition
