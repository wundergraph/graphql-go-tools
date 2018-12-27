package document

import "bytes"

// InputObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputObjectTypeDefinition
type InputObjectTypeDefinition struct {
	Description           ByteSlice
	Name                  ByteSlice
	InputFieldsDefinition InputFieldsDefinition
	Directives            Directives
}

// InputObjectTypeDefinitions is the plural of InputObjectTypeDefinition
type InputObjectTypeDefinitions []InputObjectTypeDefinition

// HasDefinition returns true if an InputObjectTypeDefinition with $name is contained
func (i InputObjectTypeDefinitions) HasDefinition(name []byte) bool {

	for _, definition := range i {
		if bytes.Equal(definition.Name, name) {
			return true
		}
	}

	return false
}

// GetByName returns a InputObjectTypeDefinition by $name or nil if not found
func (i InputObjectTypeDefinitions) GetByName(name []byte) *InputObjectTypeDefinition {
	for _, definition := range i {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
