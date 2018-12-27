package document

import "bytes"

// ObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ObjectTypeDefinition
type ObjectTypeDefinition struct {
	Description          ByteSlice
	Name                 ByteSlice
	FieldsDefinition     FieldsDefinition
	ImplementsInterfaces ImplementsInterfaces
	Directives           Directives
}

// ObjectTypeDefinitions is the plural of ObjectTypeDefinition
type ObjectTypeDefinitions []ObjectTypeDefinition

// HasType returns if a type with $name is contained
func (o ObjectTypeDefinitions) HasType(name []byte) bool {
	for _, objectType := range o {
		if bytes.Equal(objectType.Name, name) {
			return true
		}
	}

	return false
}

// ObjectTypeDefinitionByName returns ObjectTypeDefinition,true if it is contained
func (o *ObjectTypeDefinitions) ObjectTypeDefinitionByName(name []byte) *ObjectTypeDefinition {
	for _, objectType := range *o {
		if bytes.Equal(objectType.Name, name) {
			return &objectType
		}
	}

	return nil
}
