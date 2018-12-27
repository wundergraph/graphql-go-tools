package document

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// NamedType as specified in:
// https://facebook.github.io/graphql/draft/#NamedType
type NamedType struct {
	Name    ByteSlice
	NonNull bool
}

// TypeName returns the name of the type and makes NamedType implement the Type interface
func (n NamedType) TypeName() []byte {
	return n.Name
}

// IsBaseType returns if the type is a base scalar (ID,String,Float,Boolean,Int) or a custom type
func (n NamedType) IsBaseType() bool {

	if bytes.Equal(n.Name, literal.ID) ||
		bytes.Equal(n.Name, literal.STRING) ||
		bytes.Equal(n.Name, literal.FLOAT) ||
		bytes.Equal(n.Name, literal.BOOLEAN) ||
		bytes.Equal(n.Name, literal.INT) {
		return true
	}

	return false
}

// GetTypeKind returns the NamedTypeKind
func (n NamedType) GetTypeKind() TypeKind {
	return NamedTypeKind
}

// AsGoType returns the GraphQL Named Type Name as valid go type
func (n NamedType) AsGoType() []byte {

	if bytes.Equal(n.Name, literal.INT) {
		return literal.GOINT32
	} else if bytes.Equal(n.Name, literal.FLOAT) {
		return literal.GOFLOAT32
	} else if bytes.Equal(n.Name, literal.STRING) {
		return literal.GOSTRING
	} else if bytes.Equal(n.Name, literal.BOOLEAN) {
		return literal.GOBOOL
	} else if bytes.Equal(n.Name, literal.NULL) {
		return literal.GONIL
	}

	return bytes.Title(bytes.TrimPrefix(n.Name, []byte("__")))

}

// NamedTypeKind marks a Type as NamedType
var NamedTypeKind TypeKind = []byte("NamedType")
