package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// Argument as specified in
// http://facebook.github.io/graphql/draft/#Argument
type Argument struct {
	Name     ByteSliceReference
	Value    int
	Position position.Position
}

func (a Argument) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (a Argument) NodePosition() position.Position {
	return a.Position
}

func (a Argument) NodeValueType() ValueType {
	panic("implement me")
}

func (a Argument) NodeValueReference() int {
	panic("implement me")
}

func (a Argument) NodeUnionMemberTypes() []ByteSliceReference {
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

func (a Argument) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (a Argument) NodeValue() int {
	return a.Value
}

func (a Argument) NodeDefaultValue() int {
	panic("implement me")
}

func (a Argument) NodeFieldsDefinition() []int {
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

func (a Argument) NodeArguments() []int {
	panic("implement me")
}

func (a Argument) NodeDirectives() []int {
	panic("implement me")
}

func (a Argument) NodeEnumValuesDefinition() []int {
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
