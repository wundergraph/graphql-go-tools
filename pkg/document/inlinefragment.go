package document

// InlineFragment as specified in:
// http://facebook.github.io/graphql/draft/#InlineFragment
type InlineFragment struct {
	TypeCondition NamedType
	Directives    []int
	SelectionSet  SelectionSet
}

// InlineFragments is the plural of InlineFragment
type InlineFragments []InlineFragment
