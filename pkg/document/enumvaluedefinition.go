package document

// EnumValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumValueDefinition
type EnumValueDefinition struct {
	Description ByteSliceReference
	EnumValue   ByteSliceReference
	Directives  []int
}

func (e EnumValueDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (e EnumValueDefinition) NodeValueReference() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (e EnumValueDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (e EnumValueDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (e EnumValueDefinition) NodeValue() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (e EnumValueDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (e EnumValueDefinition) NodeType() int {
	panic("implement me")
}

func (e EnumValueDefinition) NodeVariableDefinitions() []int {
	return nil
}

func (e EnumValueDefinition) NodeFields() []int {
	return nil
}

func (e EnumValueDefinition) NodeFragmentSpreads() []int {
	return nil
}

func (e EnumValueDefinition) NodeInlineFragments() []int {
	return nil
}

func (e EnumValueDefinition) NodeName() ByteSliceReference {
	return e.EnumValue
}

func (e EnumValueDefinition) NodeDescription() ByteSliceReference {
	return e.Description
}

func (e EnumValueDefinition) NodeArguments() []int {
	return nil
}

func (e EnumValueDefinition) NodeDirectives() []int {
	return e.Directives
}

func (e EnumValueDefinition) NodeEnumValuesDefinition() []int {
	return nil
}
