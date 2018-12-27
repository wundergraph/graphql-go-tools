package document

import "bytes"

// InputFieldsDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputFieldsDefinition
type InputFieldsDefinition []InputValueDefinition

// GetByName returns a InputValueDefinition by $name or nil if not found
func (i InputFieldsDefinition) GetByName(name []byte) *InputValueDefinition {
	for _, definition := range i {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
