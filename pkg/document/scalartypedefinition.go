package document

// ScalarTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#sec-Scalars
type ScalarTypeDefinition struct {
	Description ByteSlice
	Name        ByteSlice
	Directives  []int
}

func (s ScalarTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeName() string {
	return string(s.Name)
}

func (s ScalarTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeDescription() string {
	return string(s.Description)
}

func (s ScalarTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeDirectives() []int {
	return s.Directives
}

func (s ScalarTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeType() int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (s ScalarTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

// ScalarTypeDefinitions is the plural of ScalarTypeDefinition
type ScalarTypeDefinitions []ScalarTypeDefinition
