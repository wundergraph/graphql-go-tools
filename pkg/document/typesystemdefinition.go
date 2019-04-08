package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// TypeSystemDefinition as specified in:
// http://facebook.github.io/graphql/draft/#TypeSystemDefinition
type TypeSystemDefinition struct {
	SchemaDefinition SchemaDefinition
	Position         position.Position
}

func (t TypeSystemDefinition) NodeSelectionSet() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeInputValueDefinitions() InputValueDefinitions {
	panic("implement me")
}

func (t TypeSystemDefinition) NodePosition() position.Position {
	return t.Position
}

func (t TypeSystemDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeValueReference() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeUnionMemberTypes() []int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeName() ByteSliceReference {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeArgumentSet() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeDirectiveSet() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeEnumValuesDefinition() EnumValueDefinitions {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeFields() []int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeFieldsDefinition() FieldDefinitions {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeType() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeValue() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (t TypeSystemDefinition) NodeImplementsInterfaces() ByteSliceReferences {
	panic("implement me")
}

type TypeSystemDefinitions []TypeSystemDefinition
