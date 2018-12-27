package document

import "bytes"

// InterfaceTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InterfaceTypeDefinition
type InterfaceTypeDefinition struct {
	Description      ByteSlice
	Name             ByteSlice
	FieldsDefinition FieldsDefinition
	Directives       Directives
}

// InterfaceTypeDefinitions is the plural of InterfaceTypeDefinition
type InterfaceTypeDefinitions []InterfaceTypeDefinition

// GetByName returns the interface type definition by name if contained
func (i InterfaceTypeDefinitions) GetByName(name []byte) *InterfaceTypeDefinition {
	for _, iFace := range i {
		if bytes.Equal(iFace.Name, name) {
			return &iFace
		}
	}

	return nil
}
