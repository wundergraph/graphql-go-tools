package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/fastastvisitor"
)

func removeSelfAliasing(walker *fastastvisitor.Walker) {
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
	if !bytes.Equal(r.operation.FieldName(ref), r.operation.FieldAlias(ref)) {
		return
	}
	r.operation.RemoveFieldAlias(ref)
}
