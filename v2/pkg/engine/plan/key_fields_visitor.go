package plan

import (
	"strings"

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
		walker: &walker,
		input:  input,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(input.key, input.definition, input.report)

	return visitor.keyPaths
}

type isKeyFieldVisitor struct {
	walker *astvisitor.Walker
	input  *keyVisitorInput

	keyPaths []string
}

func (v *isKeyFieldVisitor) EnterField(ref int) {
	if v.input.key.FieldHasSelections(ref) {
		return
	}

	fieldName := v.input.key.FieldNameUnsafeString(ref)
	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.input.typeName)
	currentPath := parentPath + "." + fieldName

	v.keyPaths = append(v.keyPaths, currentPath)
}
