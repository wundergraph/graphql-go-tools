package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type VariablesMapper struct {
	walker                  *astvisitor.Walker
	variablesMappingVisitor *variablesMappingVisitor
}

func NewVariablesMapper() *VariablesMapper {
	walker := astvisitor.NewDefaultWalker()
	mapper := remapVariables(&walker)

	return &VariablesMapper{
		walker:                  &walker,
		variablesMappingVisitor: mapper,
	}
}

func (v *VariablesMapper) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) map[string]string {
	v.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}

	return v.variablesMappingVisitor.mapping
}
