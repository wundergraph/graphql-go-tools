package document

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"strings"
)

// FieldDefinition as specified in:
// http://facebook.github.io/graphql/draft/#FieldDefinition
type FieldDefinition struct {
	Description         string
	Name                string
	ArgumentsDefinition ArgumentsDefinition
	Type                Type
	Directives          Directives
}

// NameAsTitle trims all prefixed __ and formats the name with strings.Title
func (f FieldDefinition) NameAsTitle() string {
	return strings.Title(strings.TrimPrefix(f.Name, "__"))
}

// NameAsGoTypeName returns the field definition name as a go type name
func (f FieldDefinition) NameAsGoTypeName() string {

	name := f.NameAsTitle()
	name = strings.ToLower(name[:1]) + name[1:]

	if name == literal.TYPE {
		name = literal.GRAPHQLTYPE
	}

	return name
}
