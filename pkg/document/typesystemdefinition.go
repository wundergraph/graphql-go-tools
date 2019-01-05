package document

// TypeSystemDefinition as specified in:
// http://facebook.github.io/graphql/draft/#TypeSystemDefinition
type TypeSystemDefinition struct {
	SchemaDefinition           SchemaDefinition
	ScalarTypeDefinitions      []int
	ObjectTypeDefinitions      []int
	InterfaceTypeDefinitions   []int
	UnionTypeDefinitions       []int
	EnumTypeDefinitions        []int
	InputObjectTypeDefinitions []int
	DirectiveDefinitions       []int
}
