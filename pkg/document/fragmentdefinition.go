package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// FragmentDefinition as specified in
// http://facebook.github.io/graphql/draft/#FragmentDefinition
type FragmentDefinition struct {
	FragmentName  ByteSliceReference // but not on
	TypeCondition int
	Directives    []int
	SelectionSet  SelectionSet
	Position      position.Position
}

func (f FragmentDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeInputValueDefinitions() []int {
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

func (f FragmentDefinition) NodeUnionMemberTypes() []ByteSliceReference {
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

func (f FragmentDefinition) NodeImplementsInterfaces() []ByteSliceReference {
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
	return f.SelectionSet.Fields
}

func (f FragmentDefinition) NodeFragmentSpreads() []int {
	return f.SelectionSet.FragmentSpreads
}

func (f FragmentDefinition) NodeInlineFragments() []int {
	return f.SelectionSet.InlineFragments
}

func (f FragmentDefinition) NodeName() ByteSliceReference {
	return f.FragmentName
}

func (f FragmentDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (f FragmentDefinition) NodeArguments() []int {
	return nil
}

func (f FragmentDefinition) NodeDirectives() []int {
	return f.Directives
}

func (f FragmentDefinition) NodeEnumValuesDefinition() []int {
	return nil
}

// FragmentDefinitions is the plural of FragmentDefinition
type FragmentDefinitions []FragmentDefinition
