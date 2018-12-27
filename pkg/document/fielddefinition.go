package document

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// FieldDefinition as specified in:
// http://facebook.github.io/graphql/draft/#FieldDefinition
type FieldDefinition struct {
	Description         ByteSlice
	Name                ByteSlice
	ArgumentsDefinition ArgumentsDefinition
	Type                Type
	Directives          Directives
}

// NameAsTitle trims all prefixed __ and formats the name with strings.Title
func (f FieldDefinition) NameAsTitle() []byte {
	return bytes.Title(bytes.TrimPrefix(f.Name, []byte("__")))
}

// NameAsGoTypeName returns the field definition name as a go type name
func (f FieldDefinition) NameAsGoTypeName() []byte {

	name := f.NameAsTitle()
	name = append(bytes.ToLower(name[:1]), name[1:]...)

	if bytes.Equal(name, literal.TYPE) {
		name = literal.GRAPHQLTYPE
	}

	return name
}
