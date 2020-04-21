package fields

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Generator struct {
	Data    *Data
	walker  *astvisitor.Walker
	visitor *requestVisitor
}

func NewGenerator() *Generator {
	walker := astvisitor.NewWalker(48)
	visitor := requestVisitor{
		Walker: &walker,
	}

	// walker.RegisterAllNodesVisitor(&visitor)

	return &Generator{
		walker:  &walker,
		visitor: &visitor,
	}
}

func (g *Generator) Generate(definition *ast.Document, report *operationreport.Report, data *Data) {
	g.visitor.data = data
	g.visitor.definition = definition
	g.walker.Walk(definition, nil, report)
}

type requestVisitor struct {
	*astvisitor.Walker
	definition *ast.Document
	data       *Data
}
