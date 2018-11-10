package document

// Kind is the graphql object kind as specified in:
// https://facebook.github.io/graphql/draft/#sec-Type-Kinds
type Kind string

// ... various kind types
const (
	KindObject      Kind = "KIND"
	KindScalar      Kind = "SCALAR"
	KindInterface   Kind = "INTERFACE"
	KindUnion       Kind = "UNION"
	KindEnum        Kind = "ENUM"
	KindInputObject Kind = "INPUT_OBJECT"
	KindList        Kind = "LIST"
	KindNonNull     Kind = "NON_NULL"
)
