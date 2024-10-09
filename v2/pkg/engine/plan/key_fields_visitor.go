package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type keyVisitorInput struct {
	typeName        string
	parentPath      string
	key, definition *ast.Document
	report          *operationreport.Report
}

func keyFieldPaths(input *keyVisitorInput) []string {
	walker := astvisitor.NewWalker(48)
	visitor := &isKeyFieldVisitor{
		walker:    &walker,
		input:     input,
		operation: input.key,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(input.key, input.definition, input.report)

	return visitor.keyPaths
}

type isKeyFieldVisitor struct {
	walker    *astvisitor.Walker
	operation *ast.Document
	input     *keyVisitorInput

	keyPaths []string
}

func (v *isKeyFieldVisitor) EnterField(ref int) {
	if v.input.key.FieldHasSelections(ref) {
		return
	}
	v.keyPaths = append(v.keyPaths, v.operation.FieldPath(ref, v.walker.Path))
}
