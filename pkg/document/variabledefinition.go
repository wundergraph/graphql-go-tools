package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// VariableDefinition as specified in:
// http://facebook.github.io/graphql/draft/#VariableDefinition
type VariableDefinition struct {
	Variable     ByteSliceReference
	Type         int
	DefaultValue int
	Position     position.Position
}

func (v VariableDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (v VariableDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (v VariableDefinition) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (v VariableDefinition) NodePosition() position.Position {
	return v.Position
}

func (v VariableDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (v VariableDefinition) NodeValueReference() int {
	panic("implement me")
}

func (v VariableDefinition) NodeUnionMemberTypes() []int {
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

func (v VariableDefinition) NodeImplementsInterfaces() []int {
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

func (v VariableDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (v VariableDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (v VariableDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (v VariableDefinition) NodeType() int {
	return v.Type
}

func (v VariableDefinition) NodeName() ByteSliceReference {
	return v.Variable
}

func (v VariableDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (v VariableDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (v VariableDefinition) NodeDirectiveSet() int {
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
