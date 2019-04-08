package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// DirectiveDefinition as specified in
// http://facebook.github.io/graphql/draft/#DirectiveDefinition
type DirectiveDefinition struct {
	Description         ByteSliceReference
	Name                ByteSliceReference
	ArgumentsDefinition int
	DirectiveLocations  []int
	Position            position.Position
}

func (d DirectiveDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (d DirectiveDefinition) NodePosition() position.Position {
	return d.Position
}

func (d DirectiveDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (d DirectiveDefinition) NodeValueReference() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeName() ByteSliceReference {
	return d.Name
}

func (d DirectiveDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (d DirectiveDefinition) NodeDescription() ByteSliceReference {
	return d.Description
}

func (d DirectiveDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeArgumentsDefinition() int {
	return d.ArgumentsDefinition
}

func (d DirectiveDefinition) NodeDirectiveSet() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (d DirectiveDefinition) NodeFields() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (d DirectiveDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeType() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (d DirectiveDefinition) NodeValue() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

func (d DirectiveDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (d DirectiveDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (d DirectiveDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

// DirectiveDefinitions is the plural of DirectiveDefinition
type DirectiveDefinitions []DirectiveDefinition
