package document

import "bytes"

// UnionTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#UnionTypeDefinition
type UnionTypeDefinition struct {
	Description      ByteSlice
	Name             ByteSlice
	UnionMemberTypes UnionMemberTypes
	Directives       []int
}

func (u UnionTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	return u.UnionMemberTypes
}

func (u UnionTypeDefinition) NodeName() string {
	return string(u.Name)
}

func (u UnionTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDescription() string {
	return string(u.Description)
}

func (u UnionTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDirectives() []int {
	return u.Directives
}

func (u UnionTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeType() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (u UnionTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

// GroupingFuncName returns a name to name a function after. Example:
// "Direction" => "IsDirection"
func (u UnionTypeDefinition) GroupingFuncName() ByteSlice {
	return append([]byte("Is"), u.Name...)
}

// HasMemberType returns true if a member with the given name is contained
func (u UnionTypeDefinition) HasMemberType(name ByteSlice) bool {
	for _, unionMemberType := range u.UnionMemberTypes {
		if bytes.Equal(unionMemberType, name) {
			return true
		}
	}

	return false
}

// UnionMemberTypes as specified in:
// http://facebook.github.io/graphql/draft/#UnionMemberTypes
type UnionMemberTypes []ByteSlice

// UnionTypeDefinitions is the plural of UnionTypeDefinition
type UnionTypeDefinitions []UnionTypeDefinition

// GetByName returns the UnionTypeDefinition by $name if it is contained
func (u UnionTypeDefinitions) GetByName(name ByteSlice) *UnionTypeDefinition {
	for _, definition := range u {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
