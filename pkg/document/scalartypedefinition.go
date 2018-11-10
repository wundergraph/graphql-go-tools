package document

// ScalarTypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#sec-Scalars
type ScalarTypeDefinition struct {
	Description string
	Name        string
	Directives  Directives
}

// ScalarTypeDefinitions is the plural of ScalarTypeDefinition
type ScalarTypeDefinitions []ScalarTypeDefinition
