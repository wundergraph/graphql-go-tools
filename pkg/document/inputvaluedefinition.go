package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InputValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputValueDefinition
type InputValueDefinition struct {
	Description  ByteSliceReference
	Name         int
	Type         int
	DefaultValue int
	DirectiveSet int
	Position     position.Position
}

func (i InputValueDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodePosition() position.Position {
	return i.Position
}

func (i InputValueDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputValueDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InputValueDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeValue() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDefaultValue() int {
	return i.DefaultValue
}

func (i InputValueDefinition) NodeName() int {
	return i.Name
}

func (i InputValueDefinition) NodeAlias() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDescription() ByteSliceReference {
	return i.Description
}

func (i InputValueDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDirectiveSet() int {
	return i.DirectiveSet
}

func (i InputValueDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFields() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeType() int {
	return i.Type
}

func (i InputValueDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

type InputValueDefinitions []InputValueDefinition
