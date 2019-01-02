package document

// VariableDefinition as specified in:
// http://facebook.github.io/graphql/draft/#VariableDefinition
type VariableDefinition struct {
	Variable     string
	Type         Type
	DefaultValue Value
}

// VariableDefinitions as specified in:
// http://facebook.github.io/graphql/draft/#VariableDefinitions
type VariableDefinitions []VariableDefinition
