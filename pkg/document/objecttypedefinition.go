package document

import "bytes"

// ObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ObjectTypeDefinition
type ObjectTypeDefinition struct {
	Description          ByteSlice
	Name                 ByteSlice
	FieldsDefinition     []int
	ImplementsInterfaces ImplementsInterfaces
	Directives           []int
}

func (o ObjectTypeDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeValueReference() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeUnionMemberTypes() []ByteSlice {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeImplementsInterfaces() []ByteSlice {
	return o.ImplementsInterfaces
}

func (o ObjectTypeDefinition) NodeName() string {
	return string(o.Name)
}

func (o ObjectTypeDefinition) NodeAlias() string {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDescription() string {
	return string(o.Description)
}

func (o ObjectTypeDefinition) NodeArguments() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeArgumentsDefinition() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDirectives() []int {
	return o.Directives
}

func (o ObjectTypeDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeFields() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeFieldsDefinition() []int {
	return o.FieldsDefinition
}

func (o ObjectTypeDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeType() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeValue() int {
	panic("implement me")
}

func (o ObjectTypeDefinition) NodeDefaultValue() int {
	panic("implement me")
}

// ObjectTypeDefinitions is the plural of ObjectTypeDefinition
type ObjectTypeDefinitions []ObjectTypeDefinition

// HasType returns if a type with $name is contained
func (o ObjectTypeDefinitions) HasType(name ByteSlice) bool {
	for _, objectType := range o {
		if bytes.Equal(objectType.Name, name) {
			return true
		}
	}

	return false
}

// ObjectTypeDefinitionByName returns ObjectTypeDefinition,true if it is contained
func (o *ObjectTypeDefinitions) ObjectTypeDefinitionByName(name ByteSlice) *ObjectTypeDefinition {
	for _, objectType := range *o {
		if bytes.Equal(objectType.Name, name) {
			return &objectType
		}
	}

	return nil
}
