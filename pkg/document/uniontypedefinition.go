package document

// UnionTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#UnionTypeDefinition
type UnionTypeDefinition struct {
	Description      ByteSliceReference
	Name             ByteSliceReference
	UnionMemberTypes UnionMemberTypes
	Directives       []int
}

func (u UnionTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeUnionMemberTypes() []ByteSliceReference {
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

func (u UnionTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDirectives() []int {
	return u.Directives
}

func (u UnionTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFieldsDefinition() []int {
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

func (u UnionTypeDefinition) NodeImplementsInterfaces() []ByteSliceReference {
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
type UnionMemberTypes []ByteSliceReference

// UnionTypeDefinitions is the plural of UnionTypeDefinition
type UnionTypeDefinitions []UnionTypeDefinition
