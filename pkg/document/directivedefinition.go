package document

// DirectiveDefinition as specified in
// http://facebook.github.io/graphql/draft/#DirectiveDefinition
type DirectiveDefinition struct {
	Description         string
	Name                string
	ArgumentsDefinition ArgumentsDefinition
	DirectiveLocations  DirectiveLocations
}

// ContainsLocation returns if the $location is contained
func (d DirectiveDefinition) ContainsLocation(location DirectiveLocation) bool {
	for _, dirLoc := range d.DirectiveLocations {
		if dirLoc == location {
			return true
		}
	}

	return false
}

// DirectiveDefinitions is the plural of DirectiveDefinition
type DirectiveDefinitions []DirectiveDefinition

// GetByName returns the DirectiveDefinition via $name
func (d DirectiveDefinitions) GetByName(name string) *DirectiveDefinition {
	for _, directive := range d {
		if directive.Name == name {
			return &directive
		}
	}

	return nil
}
