package document

// ObjectField as specified in:
// http://facebook.github.io/graphql/draft/#ObjectField
type ObjectField struct {
	Name  ByteSliceReference
	Value int
}

func (o ObjectField) NodeType() int {
	panic("implement me")
}

func (o ObjectField) NodeName() ByteSliceReference {
	return o.Name
}

func (o ObjectField) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (o ObjectField) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (o ObjectField) NodeArguments() []int {
	panic("implement me")
}

func (o ObjectField) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (o ObjectField) NodeDirectives() []int {
	panic("implement me")
}

func (o ObjectField) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (o ObjectField) NodeFields() []int {
	panic("implement me")
}

func (o ObjectField) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (o ObjectField) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (o ObjectField) NodeInlineFragments() []int {
	panic("implement me")
}

func (o ObjectField) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeOperationType() OperationType {
	panic("implement me")
}

func (o ObjectField) NodeValue() int {
	return o.Value
}

func (o ObjectField) NodeDefaultValue() int {
	panic("implement me")
}

func (o ObjectField) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (o ObjectField) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (o ObjectField) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (o ObjectField) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (o ObjectField) NodeValueType() ValueType {
	panic("implement me")
}

func (o ObjectField) NodeValueReference() int {
	panic("implement me")
}

// ObjectFields is the plural of ObjectField
type ObjectFields []ObjectField
