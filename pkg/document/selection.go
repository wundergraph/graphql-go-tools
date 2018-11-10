package document

// Selection as specified in:
// http://facebook.github.io/graphql/draft/#Selection
type Selection interface {
	OfKind() SelectionKind
}
