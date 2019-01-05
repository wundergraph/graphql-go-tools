package document

// Field as specified in:
// http://facebook.github.io/graphql/draft/#Field
type Field struct {
	Alias        string
	Name         string
	Arguments    []int
	Directives   []int
	SelectionSet SelectionSet
}

// Fields is the plural of Field
type Fields []Field
