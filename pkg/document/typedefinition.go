package document

// TypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#TypeDefinition
type TypeDefinition struct {
	Description      ByteSlice
	Name             ByteSlice
	FieldsDefinition FieldsDefinition
}
