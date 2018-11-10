package document

// FragmentDefinition as specified in
// http://facebook.github.io/graphql/draft/#FragmentDefinition
type FragmentDefinition struct {
	FragmentName  string // but not on
	TypeCondition NamedType
	Directives    Directives
	SelectionSet  SelectionSet
}

// FragmentDefinitions is the plural of FragmentDefinition
type FragmentDefinitions []FragmentDefinition

// GetByName returns the fragment definition with the given name if contained
func (f FragmentDefinitions) GetByName(name string) (FragmentDefinition, bool) {
	for _, fragment := range f {
		if fragment.FragmentName == name {
			return fragment, true
		}
	}

	return FragmentDefinition{}, false
}
