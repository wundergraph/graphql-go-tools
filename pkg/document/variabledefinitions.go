package document

// VariableDefinition as specified in:
// http://facebook.github.io/graphql/draft/#VariableDefinition
type VariableDefinition struct {
	Variable     ByteSlice
	Type         int
	DefaultValue int
}

func (v VariableDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (v VariableDefinition) NodeValueReference() int {
	panic("implement me")
}

func (v VariableDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (v VariableDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (v VariableDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (v VariableDefinition) NodeValue() int {
	panic("implement me")
}

func (v VariableDefinition) NodeDefaultValue() int {
	return v.DefaultValue
}

func (v VariableDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeAlias() string {
	panic("implement me")
}

func (v VariableDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (v VariableDefinition) NodeType() int {
	return v.Type
}

func (v VariableDefinition) NodeName() string {
	return string(v.Variable)
}

func (v VariableDefinition) NodeDescription() string {
	panic("implement me")
}

func (v VariableDefinition) NodeArguments() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeDirectives() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeFields() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (v VariableDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

// VariableDefinitions as specified in:
// http://facebook.github.io/graphql/draft/#VariableDefinitions
type VariableDefinitions []VariableDefinition
