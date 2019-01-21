package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// Value as specified in http://facebook.github.io/graphql/draft/#Value
type Value struct {
	ValueType ValueType
	Reference int
	Position  position.Position
}

func (v Value) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (v Value) NodePosition() position.Position {
	return v.Position
}

func (v Value) NodeValueType() ValueType {
	return v.ValueType
}

func (v Value) NodeValueReference() int {
	return v.Reference
}

func (v Value) NodeName() ByteSliceReference {
	panic("implement me")
}

func (v Value) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (v Value) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (v Value) NodeArguments() []int {
	panic("implement me")
}

func (v Value) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (v Value) NodeDirectives() []int {
	panic("implement me")
}

func (v Value) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (v Value) NodeFields() []int {
	panic("implement me")
}

func (v Value) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (v Value) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (v Value) NodeInlineFragments() []int {
	panic("implement me")
}

func (v Value) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeType() int {
	panic("implement me")
}

func (v Value) NodeOperationType() OperationType {
	panic("implement me")
}

func (v Value) NodeValue() int {
	panic("implement me")
}

func (v Value) NodeDefaultValue() int {
	panic("implement me")
}

func (v Value) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (v Value) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (v Value) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (v Value) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

type VariableValue ByteSlice
type IntValue int32
type FloatValue float32
type StringValue ByteSlice
type BooleanValue bool
type EnumValue ByteSlice
type ListValue []int
type ObjectValue []int
