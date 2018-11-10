package document

// TypeKind marks Types to identify them
type TypeKind string

// Type as specified in:
// http://facebook.github.io/graphql/draft/#Type
type Type interface {
	GetTypeKind() TypeKind
	AsGoType() string
	IsBaseType() bool
	TypeName() string
}
