package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// removeSelfAliasing registers a visitor to remove redundant aliasing within GraphQL documents.
func removeSelfAliasing(walker *astvisitor.Walker) {
	visitor := removeSelfAliasingVisitor{}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterFieldVisitor(&visitor)
}

type removeSelfAliasingVisitor struct {
	operation *ast.Document
}

func (r *removeSelfAliasingVisitor) EnterDocument(operation, definition *ast.Document) {
	r.operation = operation
}

func (r *removeSelfAliasingVisitor) EnterField(ref int) {
	if !r.operation.Fields[ref].Alias.IsDefined {
		return
	}
	if !bytes.Equal(r.operation.FieldNameBytes(ref), r.operation.FieldAliasBytes(ref)) {
		return
	}
	r.operation.RemoveFieldAlias(ref)
}
