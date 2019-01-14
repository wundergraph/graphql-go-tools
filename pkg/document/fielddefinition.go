package document

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// FieldDefinition as specified in:
// http://facebook.github.io/graphql/draft/#FieldDefinition
type FieldDefinition struct {
	Description         ByteSlice
	Name                ByteSlice
	ArgumentsDefinition []int
	Type                int
	Directives          []int
}

func (f FieldDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (f FieldDefinition) NodeValueReference() int {
	panic("implement me")
}

func (f FieldDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (f FieldDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (f FieldDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (f FieldDefinition) NodeValue() int {
	panic("implement me")
}

func (f FieldDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (f FieldDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeArgumentsDefinition() []int {
	return f.ArgumentsDefinition
}

func (f FieldDefinition) NodeName() string {
	return string(f.Name)
}

func (f FieldDefinition) NodeAlias() string {
	panic("implement me")
}

func (f FieldDefinition) NodeDescription() string {
	return string(f.Description)
}

func (f FieldDefinition) NodeArguments() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeDirectives() []int {
	return f.Directives
}

func (f FieldDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeFields() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (f FieldDefinition) NodeType() int {
	return f.Type
}

func (f FieldDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

// NameAsTitle trims all prefixed __ and formats the name with strings.Title
func (f FieldDefinition) NameAsTitle() ByteSlice {
	return bytes.Title(bytes.TrimPrefix(f.Name, []byte("__")))
}

// NameAsGoTypeName returns the field definition name as a go type name
func (f FieldDefinition) NameAsGoTypeName() ByteSlice {

	name := f.NameAsTitle()
	name = append(bytes.ToLower(name[:1]), name[1:]...)

	if bytes.Equal(name, literal.TYPE) {
		name = literal.GRAPHQLTYPE
	}

	return name
}
