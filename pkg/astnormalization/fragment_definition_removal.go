package astnormalization

import "github.com/jensneuse/graphql-go-tools/pkg/ast"

type FragmentDefinitionRemoval struct {
}

func (f *FragmentDefinitionRemoval) Do(operation *ast.Document) {
	for i := range operation.RootNodes {
		if operation.RootNodes[i].Kind == ast.NodeKindFragmentDefinition {
			operation.RootNodes[i].Kind = ast.NodeKindUnknown
		}
	}
}
