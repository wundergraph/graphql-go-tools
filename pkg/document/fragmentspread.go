package document

// FragmentSpread as specified in:
// http://facebook.github.io/graphql/draft/#FragmentSpread
type FragmentSpread struct {
	FragmentName ByteSliceReference
	Directives   []int
}

func (f FragmentSpread) NodeValueType() ValueType {
	panic("implement me")
}

func (f FragmentSpread) NodeValueReference() int {
	panic("implement me")
}

func (f FragmentSpread) NodeUnionMemberTypes() []ByteSliceReference {
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

func (f FragmentSpread) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (f FragmentSpread) NodeValue() int {
	panic("implement me")
}

func (f FragmentSpread) NodeDefaultValue() int {
	panic("implement me")
}

func (f FragmentSpread) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (f FragmentSpread) NodeArgumentsDefinition() []int {
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

func (f FragmentSpread) NodeArguments() []int {
	panic("implement me")
}

func (f FragmentSpread) NodeDirectives() []int {
	return f.Directives
}

func (f FragmentSpread) NodeEnumValuesDefinition() []int {
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
