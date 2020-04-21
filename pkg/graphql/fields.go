package graphql

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/fields"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type RestrictedFieldsValidator interface {
	Validate(operation *ast.Document, restrictedFields fields.Types) (RestrictedFieldsResult, error)
}

type fieldsValidator struct {
}

func (d fieldsValidator) Validate(operation *ast.Document, restrictedFields fields.Types) (RestrictedFieldsResult, error) {
	report := operationreport.Report{}

	// call fields visitor

	return restrictedFieldsResult(true, report)
}

type RestrictedFieldsResult struct {
	Valid  bool
	Errors Errors
}

func restrictedFieldsResult(valid bool, report operationreport.Report) (RestrictedFieldsResult, error) {
	result := RestrictedFieldsResult{
		Valid:  valid,
		Errors: nil,
	}

	if !report.HasErrors() {
		return result, nil
	}

	result.Errors = operationValidationErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}
