package document

// TypeKind marks Types to identify them
type TypeKind []byte

// Type as specified in:
// http://facebook.github.io/graphql/draft/#Type
type Type interface {
	GetTypeKind() TypeKind
	AsGoType() []byte
	IsBaseType() bool
	TypeName() []byte
}
