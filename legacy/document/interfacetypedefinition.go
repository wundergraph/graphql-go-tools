package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/position"

// InterfaceTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InterfaceTypeDefinition
type InterfaceTypeDefinition struct {
	Description      ByteSliceReference
	Name             ByteSliceReference
	FieldsDefinition FieldDefinitions
	DirectiveSet     int
	Position         position.Position
	IsExtend         bool
}

func (i InterfaceTypeDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodePosition() position.Position {
	return i.Position
}

func (i InterfaceTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFieldsDefinition() FieldDefinitions {
	return i.FieldsDefinition
}

func (i InterfaceTypeDefinition) NodeName() ByteSliceReference {
	return i.Name
}

func (i InterfaceTypeDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDescription() ByteSliceReference {
	return i.Description
}

func (i InterfaceTypeDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeDirectiveSet() int {
	return i.DirectiveSet
}

func (i InterfaceTypeDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeType() int {
	panic("implement me")
}

func (i InterfaceTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

// InterfaceTypeDefinitions is the plural of InterfaceTypeDefinition
type InterfaceTypeDefinitions []InterfaceTypeDefinition
