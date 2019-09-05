package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/position"

// ArgumentsDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ArgumentsDefinition
type ArgumentsDefinition struct {
	InputValueDefinitions InputValueDefinitions
	Position              position.Position
}

func (a ArgumentsDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	return a.InputValueDefinitions
}

func (a ArgumentsDefinition) NodeName() ByteSliceReference {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeDirectiveSet() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeFields() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeType() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeValue() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (a ArgumentsDefinition) NodeValueReference() int {
	panic("implement me")
}

func (a ArgumentsDefinition) NodePosition() position.Position {
	return a.Position
}

type ArgumentsDefinitions []ArgumentsDefinition
