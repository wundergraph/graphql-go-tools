package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// InputValueDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputValueDefinition
type InputValueDefinition struct {
	Description  ByteSliceReference
	Name         ByteSliceReference
	Type         int
	DefaultValue int
	DirectiveSet int
	Position     position.Position
	NextRef      int
}

func (i InputValueDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (i InputValueDefinition) NodePosition() position.Position {
	return i.Position
}

func (i InputValueDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (i InputValueDefinition) NodeValueReference() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (i InputValueDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeImplementsInterfaces() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeValue() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDefaultValue() int {
	return i.DefaultValue
}

func (i InputValueDefinition) NodeName() ByteSliceReference {
	return i.Name
}

func (i InputValueDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (i InputValueDefinition) NodeDescription() ByteSliceReference {
	return i.Description
}

func (i InputValueDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (i InputValueDefinition) NodeDirectiveSet() int {
	return i.DirectiveSet
}

func (i InputValueDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFields() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (i InputValueDefinition) NodeType() int {
	return i.Type
}

func (i InputValueDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

type InputValueDefinitionGetter interface {
	InputValueDefinition(ref int) InputValueDefinition
}

type InputValueDefinitions struct {
	nextRef    int
	currentRef int
	current    InputValueDefinition
}

func NewInputValueDefinitions(nextRef int) InputValueDefinitions {
	return InputValueDefinitions{
		nextRef: nextRef,
	}
}

func (i *InputValueDefinitions) HasNext() bool {
	return i.nextRef != -1
}

func (i *InputValueDefinitions) Next(getter InputValueDefinitionGetter) bool {
	if i.nextRef == -1 {
		return false
	}

	i.currentRef = i.nextRef
	i.current = getter.InputValueDefinition(i.nextRef)
	i.nextRef = i.current.NextRef
	return true
}

func (i *InputValueDefinitions) Value() (InputValueDefinition, int) {
	return i.current, i.currentRef
}
