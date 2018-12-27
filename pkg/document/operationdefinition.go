package document

// OperationDefinition as specified in:
// http://facebook.github.io/graphql/draft/#OperationDefinition
type OperationDefinition struct {
	OperationType       OperationType
	Name                ByteSlice
	VariableDefinitions VariableDefinitions
	Directives          Directives
	SelectionSet        SelectionSet
}

//OperationDefinitions is the plural of OperationDefinition
type OperationDefinitions []OperationDefinition
