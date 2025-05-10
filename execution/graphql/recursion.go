package graphql

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphqlerrors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/middleware/recursion_guard"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const DefaultMaxDepth = 3

// RecursionCalculator is symmetrical to ComplexityCalculator.
type RecursionCalculator interface {
	Calculate(operation, definition *ast.Document) (RecursionResult, error)
}

type defaultRecursionCalculator struct{ maxDepth int }

func NewRecursionCalculator(maxDepth int) RecursionCalculator {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	return &defaultRecursionCalculator{maxDepth: maxDepth}
}

func (d *defaultRecursionCalculator) Calculate(
	operation, definition *ast.Document,
) (RecursionResult, error) {

	report := operationreport.Report{}

	err := recursion_guard.ValidateRecursion(d.maxDepth, operation, definition, &report)

	if err != nil {
		return RecursionResult{}, err
	}

	return recursionResult(report)
}

type RecursionResult struct {
	Errors graphqlerrors.Errors
}

func recursionResult(report operationreport.Report) (RecursionResult, error) {
	if !report.HasErrors() {
		return RecursionResult{Errors: nil}, nil
	}

	errs := graphqlerrors.RequestErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return RecursionResult{Errors: errs}, err
}
