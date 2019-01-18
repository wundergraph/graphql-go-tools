//go:generate go-enum -f=$GOFILE
package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

/*
// TypeKind marks Types to identify them
ENUM(
UNDEFINED
NON_NULL
NAMED
LIST
)
*/
type TypeKind int

// Type as specified in:
// http://facebook.github.io/graphql/draft/#Type
type Type struct {
	Kind   TypeKind
	Name   ByteSliceReference
	OfType int
	Position position.Position
}

func (t Type) NodePosition() position.Position {
	return t.Position
}

func (t Type) NodeName() ByteSliceReference {
	return t.Name
}

func (t Type) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (t Type) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (t Type) NodeArguments() []int {
	panic("implement me")
}

func (t Type) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (t Type) NodeDirectives() []int {
	panic("implement me")
}

func (t Type) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (t Type) NodeFields() []int {
	panic("implement me")
}

func (t Type) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (t Type) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (t Type) NodeInlineFragments() []int {
	panic("implement me")
}

func (t Type) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeType() int {
	panic("implement me")
}

func (t Type) NodeOperationType() OperationType {
	panic("implement me")
}

func (t Type) NodeValue() int {
	panic("implement me")
}

func (t Type) NodeDefaultValue() int {
	panic("implement me")
}

func (t Type) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (t Type) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (t Type) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (t Type) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (t Type) NodeValueType() ValueType {
	panic("implement me")
}

func (t Type) NodeValueReference() int {
	panic("implement me")
}

// Types is the plural of Type
type Types []Type
