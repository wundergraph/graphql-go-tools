package document

// ObjectField as specified in:
// http://facebook.github.io/graphql/draft/#ObjectField
type ObjectField struct {
	Name  ByteSlice
	Value Value
}

// ObjectFields is the plural of ObjectField
type ObjectFields []ObjectField
