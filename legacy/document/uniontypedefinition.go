package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// UnionTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#UnionTypeDefinition
type UnionTypeDefinition struct {
	Description      ByteSliceReference
	Name             ByteSliceReference
	UnionMemberTypes UnionMemberTypes
	DirectiveSet     int
	Position         position.Position
	IsExtend         bool
}

func (u UnionTypeDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (u UnionTypeDefinition) NodePosition() position.Position {
	return u.Position
}

func (u UnionTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeUnionMemberTypes() []int {
	return u.UnionMemberTypes
}

func (u UnionTypeDefinition) NodeName() ByteSliceReference {
	return u.Name
}

func (u UnionTypeDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDescription() ByteSliceReference {
	return u.Description
}

func (u UnionTypeDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDirectiveSet() int {
	return u.DirectiveSet
}

func (u UnionTypeDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeType() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

// UnionMemberTypes as specified in:
// http://facebook.github.io/graphql/draft/#UnionMemberTypes
type UnionMemberTypes []int

// UnionTypeDefinitions is the plural of UnionTypeDefinition
type UnionTypeDefinitions []UnionTypeDefinition
