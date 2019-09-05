package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/position"

// Argument as specified in
// http://facebook.github.io/graphql/draft/#Argument
type Argument struct {
	Name     ByteSliceReference
	Value    int
	Position position.Position
}

func (a Argument) NodeSelectionSet() int {
	panic("implement me")
}

func (a Argument) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (a Argument) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (a Argument) NodePosition() position.Position {
	return a.Position
}

func (a Argument) NodeValueType() ValueType {
	panic("implement me")
}

func (a Argument) NodeValueReference() int {
	return a.Value
}

func (a Argument) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (a Argument) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (a Argument) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (a Argument) NodeValue() int {
	return a.Value
}

func (a Argument) NodeDefaultValue() int {
	panic("implement me")
}

func (a Argument) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (a Argument) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (a Argument) NodeName() ByteSliceReference {
	return a.Name
}

func (a Argument) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (a Argument) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (a Argument) NodeArgumentSet() int {
	panic("implement me")
}

func (a Argument) NodeDirectiveSet() int {
	panic("implement me")
}

func (a Argument) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (a Argument) NodeFields() []int {
	panic("implement me")
}

func (a Argument) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (a Argument) NodeInlineFragments() []int {
	panic("implement me")
}

func (a Argument) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodeType() int {
	panic("implement me")
}

func (a Argument) NodeOperationType() OperationType {
	panic("implement me")
}

// Arguments as specified in
// http://facebook.github.io/graphql/draft/#Arguments
type Arguments []Argument

type ArgumentSet []int
