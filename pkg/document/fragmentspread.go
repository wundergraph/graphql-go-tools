package document

// FragmentSpread as specified in:
// http://facebook.github.io/graphql/draft/#FragmentSpread
type FragmentSpread struct {
	FragmentName string
	Directives   []int
}

// FragmentSpreads is the plural of FragmentSpread
type FragmentSpreads []FragmentSpread
