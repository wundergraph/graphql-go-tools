package document

import "strings"

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
	case "ID", "String", "Float", "Boolean", "Int":
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
	case "Int":
		return "int32"
	case "Float":
		return "float32"
	case "String":
		return "string"
	case "Boolean":
		return "bool"
	case "null":
		return "nil"
	default:
		return strings.Title(strings.TrimPrefix(n.Name, "__"))
	}
}

// NamedTypeKind marks a Type as NamedType
const NamedTypeKind TypeKind = "NamedType"
