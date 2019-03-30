package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InlineFragment as specified in:
// http://facebook.github.io/graphql/draft/#InlineFragment
type InlineFragment struct {
	TypeCondition int
	DirectiveSet  int
	SelectionSet  int
	Position      position.Position
}

func (i InlineFragment) NodeSelectionSet() int {
	return i.SelectionSet
}

func (i InlineFragment) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (i InlineFragment) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (i InlineFragment) NodePosition() position.Position {
	return i.Position
}

func (i InlineFragment) NodeValueType() ValueType {
	panic("implement me")
}

func (i InlineFragment) NodeValueReference() int {
	panic("implement me")
}

func (i InlineFragment) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (i InlineFragment) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InlineFragment) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (i InlineFragment) NodeValue() int {
	panic("implement me")
}

func (i InlineFragment) NodeDefaultValue() int {
	panic("implement me")
}

func (i InlineFragment) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (i InlineFragment) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InlineFragment) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (i InlineFragment) NodeOperationType() OperationType {
	panic("implement me")
}

func (i InlineFragment) NodeName() ByteSliceReference {
	panic("implement me")
}

func (i InlineFragment) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (i InlineFragment) NodeArgumentSet() int {
	panic("implement me")
}

func (i InlineFragment) NodeDirectiveSet() int {
	return i.DirectiveSet
}

func (i InlineFragment) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InlineFragment) NodeFields() []int {
	panic("implement me")
}

func (i InlineFragment) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InlineFragment) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InlineFragment) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InlineFragment) NodeType() int {
	return i.TypeCondition
}

// InlineFragments is the plural of InlineFragment
type InlineFragments []InlineFragment
