package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InputObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputObjectTypeDefinition
type InputObjectTypeDefinition struct {
	Description           ByteSliceReference
	Name                  ByteSliceReference
	InputValueDefinitions []int
	Directives            []int
	Position              position.Position
}

func (i InputObjectTypeDefinition) NodeInputValueDefinitions() []int {
	return i.InputValueDefinitions
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

func (i InputObjectTypeDefinition) NodeUnionMemberTypes() []ByteSliceReference {
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

func (i InputObjectTypeDefinition) NodeImplementsInterfaces() []ByteSliceReference {
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

func (i InputObjectTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InputObjectTypeDefinition) NodeDirectives() []int {
	return i.Directives
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
