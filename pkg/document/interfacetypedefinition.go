package document

// InterfaceTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#InterfaceTypeDefinition
type InterfaceTypeDefinition struct {
	Description      string
	Name             string
	FieldsDefinition FieldsDefinition
	Directives       Directives
}

// InterfaceTypeDefinitions is the plural of InterfaceTypeDefinition
type InterfaceTypeDefinitions []InterfaceTypeDefinition

// GetByName returns the interface type definition by name if contained
func (i InterfaceTypeDefinitions) GetByName(name string) *InterfaceTypeDefinition {
	for _, iFace := range i {
		if iFace.Name == name {
			return &iFace
		}
	}

	return nil
}
