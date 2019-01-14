package document

// Directive as specified in:
// http://facebook.github.io/graphql/draft/#Directive
type Directive struct {
	Name      ByteSlice
	Arguments []int
}

func (d Directive) NodeValueType() ValueType {
	panic("implement me")
}

func (d Directive) NodeValueReference() int {
	panic("implement me")
}

func (d Directive) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (d Directive) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (d Directive) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (d Directive) NodeValue() int {
	panic("implement me")
}

func (d Directive) NodeDefaultValue() int {
	panic("implement me")
}

func (d Directive) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (d Directive) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (d Directive) NodeAlias() string {
	panic("implement me")
}

func (d Directive) NodeOperationType() OperationType {
	panic("implement me")
}

func (d Directive) NodeType() int {
	panic("implement me")
}

func (d Directive) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (d Directive) NodeFields() []int {
	panic("implement me")
}

func (d Directive) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (d Directive) NodeInlineFragments() []int {
	panic("implement me")
}

func (d Directive) NodeName() string {
	return string(d.Name)
}

func (d Directive) NodeDescription() string {
	panic("implement me")
}

func (d Directive) NodeArguments() []int {
	return d.Arguments
}

func (d Directive) NodeDirectives() []int {
	panic("implement me")
}

func (d Directive) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

// Directives as specified in
// http://facebook.github.io/graphql/draft/#Directives
type Directives []Directive
