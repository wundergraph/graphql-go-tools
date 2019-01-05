package document

// OperationDefinition as specified in:
// http://facebook.github.io/graphql/draft/#OperationDefinition
type OperationDefinition struct {
	OperationType       OperationType
	Name                string
	VariableDefinitions []int
	Directives          []int
	SelectionSet        SelectionSet
}

//OperationDefinitions is the plural of OperationDefinition
type OperationDefinitions []OperationDefinition
