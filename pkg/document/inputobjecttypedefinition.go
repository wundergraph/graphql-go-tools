package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InputObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputObjectTypeDefinition
type InputObjectTypeDefinition struct {
	Description           ByteSliceReference
	Name                  ByteSliceReference
	InputFieldsDefinition int
	DirectiveSet          int
	Position              position.Position
}

func (i InputObjectTypeDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeInputFieldsDefinition() int {
	return i.InputFieldsDefinition
}

func (i InputObjectTypeDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodePosition() position.Position {
	return i.Position
}

func (i InputObjectTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeUnionMemberTypes() []int {
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

func (i InputObjectTypeDefinition) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeName() ByteSliceReference {
	return i.Name
}

func (i InputObjectTypeDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDescription() ByteSliceReference {
	return i.Description
}

func (i InputObjectTypeDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDirectiveSet() int {
	return i.DirectiveSet
}

func (i InputObjectTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeFields() []int {
	panic("implement me")
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
