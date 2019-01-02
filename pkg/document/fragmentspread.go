package document

// FragmentSpread as specified in:
// http://facebook.github.io/graphql/draft/#FragmentSpread
type FragmentSpread struct {
	FragmentName string
	Directives   Directives
}

// OfKind Desribes of which kind this Selection is
func (i FragmentSpread) OfKind() SelectionKind {
	return SelectionKindFragmentSpread
}

var _ Selection = FragmentSpread{}
