package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

// Field as specified in:
// http://facebook.github.io/graphql/draft/#Field
type Field struct {
	Alias        ByteSliceReference
	Name         ByteSliceReference
	Arguments    []int
	Directives   []int
	SelectionSet SelectionSet
	Position     position.Position
}

func (f Field) NodePosition() position.Position {
	return f.Position
}

func (f Field) NodeValueType() ValueType {
	panic("implement me")
}

func (f Field) NodeValueReference() int {
	panic("implement me")
}

func (f Field) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (f Field) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (f Field) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (f Field) NodeValue() int {
	panic("implement me")
}

func (f Field) NodeDefaultValue() int {
	panic("implement me")
}

func (f Field) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (f Field) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (f Field) NodeAlias() ByteSliceReference {
	return f.Alias
}

func (f Field) NodeOperationType() OperationType {
	panic("implement me")
}

func (f Field) NodeName() ByteSliceReference {
	return f.Name
}

func (f Field) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (f Field) NodeArguments() []int {
	return f.Arguments
}

func (f Field) NodeDirectives() []int {
	return f.Directives
}

func (f Field) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (f Field) NodeFields() []int {
	return f.SelectionSet.Fields
}

func (f Field) NodeFragmentSpreads() []int {
	return f.SelectionSet.FragmentSpreads
}

func (f Field) NodeInlineFragments() []int {
	return f.SelectionSet.InlineFragments
}

func (f Field) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (f Field) NodeType() int {
	panic("implement me")
}

// Fields is the plural of Field
type Fields []Field
