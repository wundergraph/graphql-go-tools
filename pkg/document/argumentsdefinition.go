package document

import "bytes"

// ArgumentsDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ArgumentsDefinition
type ArgumentsDefinition []InputValueDefinition

// GetByName returns InputValueDefinition by $name or nil if not found
func (a ArgumentsDefinition) GetByName(name []byte) *InputValueDefinition {

	for _, definition := range a {
		if bytes.Equal(definition.Name, name) {
			return &definition
		}
	}

	return nil
}
