package document

// Field as specified in:
// http://facebook.github.io/graphql/draft/#Field
type Field struct {
	Alias        string
	Name         string
	Arguments    Arguments
	Directives   Directives
	SelectionSet SelectionSet
}

// OfKind Desribes of which kind this Selection is
func (f Field) OfKind() SelectionKind {
	return SelectionKindField
}

var _ Selection = Field{}

// Fields is the plural of Field
type Fields []Field
