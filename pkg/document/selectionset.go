package document

// SelectionSet as specified in:
// http://facebook.github.io/graphql/draft/#SelectionSet
type SelectionSet struct {
	Fields          []int
	FragmentSpreads []int
	InlineFragments []int
}

// IsEmpty returns true if fields, fragment spreads and inline fragments are 0
func (s SelectionSet) IsEmpty() bool {
	return len(s.Fields) == 0 &&
		len(s.FragmentSpreads) == 0 &&
		len(s.InlineFragments) == 0
}
