package document

// InputValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputValueDefinition
type InputValueDefinition struct {
	Description  ByteSliceReference
	Name         ByteSliceReference
	Type         int
	DefaultValue int
	Directives   []int
}

func (i InputValueDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputValueDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeUnionMemberTypes() []ByteSliceReference {
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

func (i InputValueDefinition) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (i InputValueDefinition) NodeValue() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDefaultValue() int {
	return i.DefaultValue
}

func (i InputValueDefinition) NodeName() ByteSliceReference {
	return i.Name
}

func (i InputValueDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (i InputValueDefinition) NodeDescription() ByteSliceReference {
	return i.Description
}

func (i InputValueDefinition) NodeArguments() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDirectives() []int {
	return i.Directives
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
