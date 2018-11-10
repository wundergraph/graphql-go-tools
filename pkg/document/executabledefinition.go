package document

// ExecutableDefinition as specified in:
// http://facebook.github.io/graphql/draft/#ExecutableDefinition
type ExecutableDefinition struct {
	OperationDefinitions OperationDefinitions
	FragmentDefinitions  FragmentDefinitions
}
