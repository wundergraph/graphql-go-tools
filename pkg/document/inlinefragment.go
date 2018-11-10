package document

// InlineFragment as specified in:
// http://facebook.github.io/graphql/draft/#InlineFragment
type InlineFragment struct {
	TypeCondition NamedType
	Directives    Directives
	SelectionSet  SelectionSet
}

// OfKind Desribes of which kind this Selection is
func (i InlineFragment) OfKind() SelectionKind {
	return SelectionKindInlineFragment
}

var _ Selection = InlineFragment{}
