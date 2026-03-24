package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

type FragmentDefinitionRemoval struct {
}

// removeFragmentDefinitions registers a visitor to mark all the unused fragment definitions
// as NodeKindUnknown in the operation.
func removeFragmentDefinitions(walker *astvisitor.Walker) {
	visitor := &removeFragmentDefinitionsVisitor{
		walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterLeaveDocumentVisitor(visitor)
	walker.RegisterEnterFragmentSpreadVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)
}

type removeFragmentDefinitionsVisitor struct {
	operation     *ast.Document
	usedFragments map[string]struct{}
	walker        *astvisitor.Walker
}

func (r *removeFragmentDefinitionsVisitor) EnterDocument(operation, _ *ast.Document) {
	r.operation = operation
	r.usedFragments = make(map[string]struct{})
}

func (r *removeFragmentDefinitionsVisitor) EnterFragmentSpread(ref int) {
	fragmentName := r.operation.FragmentSpreadNameString(ref)
	r.usedFragments[fragmentName] = struct{}{}
}

func (r *removeFragmentDefinitionsVisitor) EnterFragmentDefinition(ref int) {
	r.walker.SkipNode()
}

func (r *removeFragmentDefinitionsVisitor) LeaveDocument(operation, _ *ast.Document) {
	for i := range operation.RootNodes {
		if operation.RootNodes[i].Kind == ast.NodeKindFragmentDefinition {
			if _, exists := r.usedFragments[operation.FragmentDefinitionNameString(operation.RootNodes[i].Ref)]; !exists {
				operation.RootNodes[i].Kind = ast.NodeKindUnknown
			}
		}
	}
}
