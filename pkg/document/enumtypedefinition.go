package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// EnumTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumTypeDefinition
type EnumTypeDefinition struct {
	Description          ByteSliceReference
	Name                 ByteSliceReference
	EnumValuesDefinition EnumValueDefinitions
	DirectiveSet         int
	Position             position.Position
	NextRef              int
}

func (e EnumTypeDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (e EnumTypeDefinition) NodePosition() position.Position {
	return e.Position
}

func (e EnumTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeType() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeVariableDefinitions() []int {
	return nil
}

func (e EnumTypeDefinition) NodeFields() []int {
	return nil
}

func (e EnumTypeDefinition) NodeFragmentSpreads() []int {
	return nil
}

func (e EnumTypeDefinition) NodeInlineFragments() []int {
	return nil
}

func (e EnumTypeDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	return e.EnumValuesDefinition
}

func (e EnumTypeDefinition) NodeName() ByteSliceReference {
	return e.Name
}

func (e EnumTypeDefinition) NodeDescription() ByteSliceReference {
	return e.Description
}

func (e EnumTypeDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (e EnumTypeDefinition) NodeDirectiveSet() int {
	return e.DirectiveSet
}
