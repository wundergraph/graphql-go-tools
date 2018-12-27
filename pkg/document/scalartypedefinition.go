package document

// ScalarTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#sec-Scalars
type ScalarTypeDefinition struct {
	Description ByteSlice
	Name        ByteSlice
	Directives  Directives
}

// ScalarTypeDefinitions is the plural of ScalarTypeDefinition
type ScalarTypeDefinitions []ScalarTypeDefinition
