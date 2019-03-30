package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// FragmentDefinition as specified in
// http://facebook.github.io/graphql/draft/#FragmentDefinition
type FragmentDefinition struct {
	FragmentName  ByteSliceReference
	TypeCondition int
	DirectiveSet  int
	SelectionSet  int
	Position      position.Position
}

func (f FragmentDefinition) NodeSelectionSet() int {
	return f.SelectionSet
}

func (f FragmentDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (f FragmentDefinition) NodePosition() position.Position {
	return f.Position
}

func (f FragmentDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (f FragmentDefinition) NodeValueReference() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (f FragmentDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeValue() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (f FragmentDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (f FragmentDefinition) NodeType() int {
	return f.TypeCondition
}

func (f FragmentDefinition) NodeVariableDefinitions() []int {
	return nil
}

func (f FragmentDefinition) NodeFields() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeName() ByteSliceReference {
	return f.FragmentName
}

func (f FragmentDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (f FragmentDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeDirectiveSet() int {
	return f.DirectiveSet
}

func (f FragmentDefinition) NodeEnumValuesDefinition() []int {
	return nil
}

// FragmentDefinitions is the plural of FragmentDefinition
type FragmentDefinitions []FragmentDefinition
