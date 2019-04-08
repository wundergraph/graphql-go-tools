package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// FragmentSpread as specified in:
// http://facebook.github.io/graphql/draft/#FragmentSpread
type FragmentSpread struct {
	FragmentName ByteSliceReference
	DirectiveSet int
	Position     position.Position
}

func (f FragmentSpread) NodeSelectionSet() int {
	panic("implement me")
}

func (f FragmentSpread) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (f FragmentSpread) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (f FragmentSpread) NodePosition() position.Position {
	return f.Position
}

func (f FragmentSpread) NodeValueType() ValueType {
	panic("implement me")
}

func (f FragmentSpread) NodeValueReference() int {
	panic("implement me")
}

func (f FragmentSpread) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (f FragmentSpread) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (f FragmentSpread) NodeValue() int {
	panic("implement me")
}

func (f FragmentSpread) NodeDefaultValue() int {
	panic("implement me")
}

func (f FragmentSpread) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (f FragmentSpread) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (f FragmentSpread) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (f FragmentSpread) NodeOperationType() OperationType {
	panic("implement me")
}

func (f FragmentSpread) NodeName() ByteSliceReference {
	return f.FragmentName
}

func (f FragmentSpread) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (f FragmentSpread) NodeArgumentSet() int {
	panic("implement me")
}

func (f FragmentSpread) NodeDirectiveSet() int {
	return f.DirectiveSet
}

func (f FragmentSpread) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (f FragmentSpread) NodeFields() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeInlineFragments() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeType() int {
	panic("implement me")
}

// FragmentSpreads is the plural of FragmentSpread
type FragmentSpreads []FragmentSpread
