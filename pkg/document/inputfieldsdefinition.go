package document

// InputValueDefinitions as specified in:
// http://facebook.github.io/graphql/draft/#InputFieldsDefinition
type InputValueDefinitions []InputValueDefinition

// GetByName returns a InputValueDefinition by $name or nil if not found
func (i InputValueDefinitions) GetByName(name string) *InputValueDefinition {
	for _, definition := range i {
		if definition.Name == name {
			return &definition
		}
	}

	return nil
}
