package document

// TypeDefinition as specified in:
// http://facebook.github.io/graphql/draft/#TypeDefinition
type TypeDefinition struct {
	Description      ByteSliceReference
	Name             ByteSliceReference
	FieldsDefinition FieldDefinitions
}
