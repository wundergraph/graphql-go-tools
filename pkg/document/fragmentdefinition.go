package document

import "bytes"

// FragmentDefinition as specified in
// http://facebook.github.io/graphql/draft/#FragmentDefinition
type FragmentDefinition struct {
	FragmentName  ByteSlice // but not on
	TypeCondition int
	Directives    []int
	SelectionSet  SelectionSet
}

func (f FragmentDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (f FragmentDefinition) NodeValueReference() int {
	panic("implement me")
}

func (f FragmentDefinition) NodeUnionMemberTypes() []ByteSlice {
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

func (f FragmentDefinition) NodeImplementsInterfaces() []ByteSlice {
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

func (f FragmentDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (f FragmentDefinition) NodeAlias() string {
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

func (f FragmentDefinition) NodeName() string {
	return string(f.FragmentName)
}

func (f FragmentDefinition) NodeDescription() string {
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

// GetByName returns the fragment definition with the given name if contained
func (f FragmentDefinitions) GetByName(name ByteSlice) (FragmentDefinition, bool) {
	for _, fragment := range f {
		if bytes.Equal(fragment.FragmentName, name) {
			return fragment, true
		}
	}

	return FragmentDefinition{}, false
}
