package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// OperationDefinition as specified in:
// http://facebook.github.io/graphql/draft/#OperationDefinition
type OperationDefinition struct {
	OperationType       OperationType
	Name                ByteSliceReference
	VariableDefinitions []int
	DirectiveSet        int
	SelectionSet        int
	Position            position.Position
}

func (o OperationDefinition) NodeSelectionSet() int {
	return o.SelectionSet
}

func (o OperationDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (o OperationDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
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

func (o OperationDefinition) NodeUnionMemberTypes() []int {
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

func (o OperationDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (o OperationDefinition) NodeValue() int {
	panic("implement me")
}

func (o OperationDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (o OperationDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (o OperationDefinition) NodeArgumentsDefinition() int {
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
	panic("implement me")
}

func (o OperationDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (o OperationDefinition) NodeName() ByteSliceReference {
	return o.Name
}

func (o OperationDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (o OperationDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (o OperationDefinition) NodeDirectiveSet() int {
	return o.DirectiveSet
}

func (o OperationDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

//OperationDefinitions is the plural of OperationDefinition
type OperationDefinitions []OperationDefinition
