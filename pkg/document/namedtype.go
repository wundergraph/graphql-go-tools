package document

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"strings"
)

// NamedType as specified in:
// https://facebook.github.io/graphql/draft/#NamedType
type NamedType struct {
	Name    string
	NonNull bool
}

// TypeName returns the name of the type and makes NamedType implement the Type interface
func (n NamedType) TypeName() string {
	return n.Name
}

// IsBaseType returns if the type is a base scalar (ID,String,Float,Boolean,Int) or a custom type
func (n NamedType) IsBaseType() bool {

	switch n.Name {
	case literal.ID, literal.STRING, literal.FLOAT, literal.BOOLEAN, literal.INT:
		return true
	default:
		return false
	}
}

// GetTypeKind returns the NamedTypeKind
func (n NamedType) GetTypeKind() TypeKind {
	return NamedTypeKind
}

// AsGoType returns the GraphQL Named Type Name as valid go type
func (n NamedType) AsGoType() string {

	switch n.Name {
	case literal.INT:
		return literal.GOINT32
	case literal.FLOAT:
		return literal.GOFLOAT32
	case literal.STRING:
		return literal.GOSTRING
	case literal.BOOLEAN:
		return literal.GOBOOL
	case literal.NULL:
		return literal.GONIL
	default:
		return strings.Title(strings.TrimPrefix(n.Name, "__"))
	}

}

// NamedTypeKind marks a Type as NamedType
var NamedTypeKind TypeKind = "NamedType"
