package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func removeSelfAliasing(walker *astvisitor.Walker) {
	visitor := removeSelfAliasingVisitor{}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterFieldVisitor(&visitor)
}

type removeSelfAliasingVisitor struct {
	operation *ast.Document
}

func (r *removeSelfAliasingVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	r.operation = operation
	return astvisitor.Instruction{}
}

func (r *removeSelfAliasingVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {
	if !r.operation.Fields[ref].Alias.IsDefined {
		return astvisitor.Instruction{}
	}
	if !bytes.Equal(r.operation.FieldName(ref), r.operation.FieldAlias(ref)) {
		return astvisitor.Instruction{}
	}
	r.operation.RemoveFieldAlias(ref)
	return astvisitor.Instruction{}
}
