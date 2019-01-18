package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// OperationDefinition as specified in:
// http://facebook.github.io/graphql/draft/#OperationDefinition
type OperationDefinition struct {
	OperationType       OperationType
	Name                ByteSliceReference
	VariableDefinitions []int
	Directives          []int
	SelectionSet        SelectionSet
	Position position.Position
}

func (o OperationDefinition) NodePosition() position.Position {
	return o.Position
}

func (o OperationDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (o OperationDefinition) NodeValueReference() int {
	panic("implement me")
}

func (o OperationDefinition) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (o OperationDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (o OperationDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (o OperationDefinition) NodeValue() int {
	panic("implement me")
}

func (o OperationDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (o OperationDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (o OperationDefinition) NodeOperationType() OperationType {
	return o.OperationType
}

func (o OperationDefinition) NodeType() int {
	panic("implement me")
}

func (o OperationDefinition) NodeVariableDefinitions() []int {
	return o.VariableDefinitions
}

func (o OperationDefinition) NodeFields() []int {
	return o.SelectionSet.Fields
}

func (o OperationDefinition) NodeFragmentSpreads() []int {
	return o.SelectionSet.FragmentSpreads
}

func (o OperationDefinition) NodeInlineFragments() []int {
	return o.SelectionSet.InlineFragments
}

func (o OperationDefinition) NodeName() ByteSliceReference {
	return o.Name
}

func (o OperationDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (o OperationDefinition) NodeArguments() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeDirectives() []int {
	return o.Directives
}

func (o OperationDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

//OperationDefinitions is the plural of OperationDefinition
type OperationDefinitions []OperationDefinition
