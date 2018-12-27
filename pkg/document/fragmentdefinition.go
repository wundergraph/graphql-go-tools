package document

import "bytes"

// FragmentDefinition as specified in
// http://facebook.github.io/graphql/draft/#FragmentDefinition
type FragmentDefinition struct {
	FragmentName  ByteSlice // but not on
	TypeCondition NamedType
	Directives    Directives
	SelectionSet  SelectionSet
}

// FragmentDefinitions is the plural of FragmentDefinition
type FragmentDefinitions []FragmentDefinition

// GetByName returns the fragment definition with the given name if contained
func (f FragmentDefinitions) GetByName(name []byte) (FragmentDefinition, bool) {
	for _, fragment := range f {
		if bytes.Equal(fragment.FragmentName, name) {
			return fragment, true
		}
	}

	return FragmentDefinition{}, false
}
