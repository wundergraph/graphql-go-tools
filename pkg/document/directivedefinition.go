package document

import "bytes"

// DirectiveDefinition as specified in
// http://facebook.github.io/graphql/draft/#DirectiveDefinition
type DirectiveDefinition struct {
	Description         ByteSlice
	Name                ByteSlice
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
func (d DirectiveDefinitions) GetByName(name []byte) *DirectiveDefinition {
	for _, directive := range d {
		if bytes.Equal(directive.Name, name) {
			return &directive
		}
	}

	return nil
}
