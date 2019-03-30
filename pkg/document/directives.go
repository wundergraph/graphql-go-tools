package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// Directive as specified in:
// http://facebook.github.io/graphql/draft/#Directive
type Directive struct {
	Name        ByteSliceReference
	ArgumentSet int
	Position    position.Position
}

func (d Directive) NodeSelectionSet() int {
	panic("implement me")
}

func (d Directive) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (d Directive) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (d Directive) NodePosition() position.Position {
	return d.Position
}

func (d Directive) NodeValueType() ValueType {
	panic("implement me")
}

func (d Directive) NodeValueReference() int {
	panic("implement me")
}

func (d Directive) NodeUnionMemberTypes() []int {
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

func (d Directive) NodeImplementsInterfaces() []int {
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

func (d Directive) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (d Directive) NodeAlias() ByteSliceReference {
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

func (d Directive) NodeName() ByteSliceReference {
	return d.Name
}

func (d Directive) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (d Directive) NodeArgumentSet() int {
	return d.ArgumentSet
}

func (d Directive) NodeDirectiveSet() int {
	panic("implement me")
}

func (d Directive) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

// Directives as specified in
// http://facebook.github.io/graphql/draft/#Directives
type Directives []Directive

type DirectiveSet []int
