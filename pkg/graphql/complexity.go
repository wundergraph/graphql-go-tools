package graphql

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/middleware/operation_complexity"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

var DefaultComplexityCalculator = defaultComplexityCalculator{}

type ComplexityCalculator interface {
	Calculate(operation, definition *ast.Document) (nodeCount, complexity int, err error)
}

type defaultComplexityCalculator struct {
}

func (d defaultComplexityCalculator) Calculate(operation, definition *ast.Document) (nodeCount, complexity int, err error) {
	report := operationreport.Report{}
	nodeCount, complexity = operation_complexity.CalculateOperationComplexity(operation, definition, &report)

	if report.HasErrors() {
		return 0, 0, report
	}

	return nodeCount, complexity, nil
}
