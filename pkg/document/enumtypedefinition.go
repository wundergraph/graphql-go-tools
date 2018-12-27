package document

import (
	"bytes"
)

// EnumTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#EnumTypeDefinition
type EnumTypeDefinition struct {
	Description          ByteSlice
	Name                 ByteSlice
	EnumValuesDefinition EnumValuesDefinition
	Directives           Directives
}

// TitleCaseName returns the EnumTypeDefinition's Name
// as title case string. example:
// episode => Episode
func (e EnumTypeDefinition) TitleCaseName() []byte {
	return bytes.Title(e.Name)
}

// EnumTypeDefinitions is the plural of EnumTypeDefinition
type EnumTypeDefinitions []EnumTypeDefinition

// HasDefinition returns true if a EnumTypeDefinition with $name is contained
func (e EnumTypeDefinitions) HasDefinition(name []byte) bool {
	for _, definition := range e {
		if bytes.Equal(definition.Name, name) {
			return true
		}
	}

	return false
}
