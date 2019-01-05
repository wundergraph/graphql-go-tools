package document

// InputObjectTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InputObjectTypeDefinition
type InputObjectTypeDefinition struct {
	Description           string
	Name                  string
	InputFieldsDefinition []int
	Directives            []int
}

// InputObjectTypeDefinitions is the plural of InputObjectTypeDefinition
type InputObjectTypeDefinitions []InputObjectTypeDefinition

// HasDefinition returns true if an InputObjectTypeDefinition with $name is contained
func (i InputObjectTypeDefinitions) HasDefinition(name string) bool {

	for _, definition := range i {
		if definition.Name == name {
			return true
		}
	}

	return false
}

// GetByName returns a InputObjectTypeDefinition by $name or nil if not found
func (i InputObjectTypeDefinitions) GetByName(name string) *InputObjectTypeDefinition {
	for _, definition := range i {
		if definition.Name == name {
			return &definition
		}
	}

	return nil
}
