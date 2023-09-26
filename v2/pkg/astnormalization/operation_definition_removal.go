package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func removeOperationDefinitions(walker *astvisitor.Walker) *removeOperationDefinitionsVisitor {
	visitor := &removeOperationDefinitionsVisitor{
		Walker: walker,
	}
	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	return visitor
}

type removeOperationDefinitionsVisitor struct {
	*astvisitor.Walker
	operation          *ast.Document
	operationName      []byte
	operationsToRemove map[int]struct{}
}

func (r *removeOperationDefinitionsVisitor) EnterDocument(operation, definition *ast.Document) {
	r.operationsToRemove = make(map[int]struct{})
	r.operation = operation
}

func (r *removeOperationDefinitionsVisitor) EnterOperationDefinition(ref int) {
	if !bytes.Equal(r.operation.OperationDefinitionNameBytes(ref), r.operationName) {
		r.operationsToRemove[ref] = struct{}{}
	}
}

func (r *removeOperationDefinitionsVisitor) LeaveDocument(operation, definition *ast.Document) {
	for i := range operation.RootNodes {
		if operation.RootNodes[i].Kind == ast.NodeKindOperationDefinition {
			if _, ok := r.operationsToRemove[operation.RootNodes[i].Ref]; ok {
				operation.RootNodes[i].Kind = ast.NodeKindUnknown
			}
		}
	}
}
