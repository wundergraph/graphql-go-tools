package document

// ArgumentsDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ArgumentsDefinition
type ArgumentsDefinition []InputValueDefinition

// GetByName returns InputValueDefinition by $name or nil if not found
func (a ArgumentsDefinition) GetByName(name string) *InputValueDefinition {

	for _, definition := range a {
		if definition.Name == name {
			return &definition
		}
	}

	return nil
}
