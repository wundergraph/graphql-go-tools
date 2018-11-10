package document

// TypeSystemDefinition as specified in:
// http://facebook.github.io/graphql/draft/#TypeSystemDefinition
type TypeSystemDefinition struct {
	SchemaDefinition           SchemaDefinition
	ScalarTypeDefinitions      ScalarTypeDefinitions
	ObjectTypeDefinitions      ObjectTypeDefinitions
	InterfaceTypeDefinitions   InterfaceTypeDefinitions
	UnionTypeDefinitions       UnionTypeDefinitions
	EnumTypeDefinitions        EnumTypeDefinitions
	InputObjectTypeDefinitions InputObjectTypeDefinitions
	DirectiveDefinitions       DirectiveDefinitions
}
