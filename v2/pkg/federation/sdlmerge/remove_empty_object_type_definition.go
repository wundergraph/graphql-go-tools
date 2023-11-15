package sdlmerge

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
)

func newRemoveEmptyObjectTypeDefinition() *removeEmptyObjectTypeDefinition {
	return &removeEmptyObjectTypeDefinition{}
}

type removeEmptyObjectTypeDefinition struct{}

func (r *removeEmptyObjectTypeDefinition) Register(walker *astvisitor.Walker) {
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeEmptyObjectTypeDefinition) LeaveDocument(operation, _ *ast.Document) {
	for ref := range operation.ObjectTypeDefinitions {
		if operation.ObjectTypeDefinitions[ref].HasFieldDefinitions {
			continue
		}

		name := operation.ObjectTypeDefinitionNameString(ref)
		node, ok := operation.Index.FirstNodeByNameStr(name)
		if !ok {
			return
		}

		operation.RemoveRootNode(node)
	}
}
