package document

// Argument as specified in
// http://facebook.github.io/graphql/draft/#Argument
type Argument struct {
	Name  ByteSlice
	Value Value
}

// Arguments as specified in
// http://facebook.github.io/graphql/draft/#Arguments
type Arguments []Argument
