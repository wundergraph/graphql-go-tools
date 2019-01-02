package document

// ListType as specified in:
// https://facebook.github.io/graphql/draft/#ListType
type ListType struct {
	Type    Type
	NonNull bool
}

// TypeName returns the unwrapped (in case of list type) type name
func (l ListType) TypeName() string {
	for l.Type.GetTypeKind() == ListTypeKind {
		l = l.Type.(ListType)
	}
	return l.Type.(NamedType).Name
}

// IsBaseType returns if the unwrapped (in case of list type) type name is a base type
func (l ListType) IsBaseType() bool {
	for l.Type.GetTypeKind() == ListTypeKind {
		l = l.Type.(ListType)
	}
	return l.Type.(NamedType).IsBaseType()
}

// GetTypeKind returns the ListTypeKind
func (l ListType) GetTypeKind() TypeKind {
	return ListTypeKind
}

// AsGoType returns the GraphQL List Type Name as valid go type
func (l ListType) AsGoType() string {
	return "[]" + l.Type.AsGoType()
}

// ListTypeKind marks a Type as ListType
var ListTypeKind TypeKind = "ListType"
