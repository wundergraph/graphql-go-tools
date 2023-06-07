package graphql

import (
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/pkg/variablevalidator"
)

type InputValidationResult struct {
	Valid  bool
	Errors Errors
}

func inputValidationResultFromReport(report operationreport.Report) (InputValidationResult, error) {
	result := InputValidationResult{
		Valid:  false,
		Errors: nil,
	}

	if !report.HasErrors() {
		result.Valid = true
		return result, nil
	}

	result.Errors = RequestErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}

func (r *Request) ValidateInput(schema *Schema) (InputValidationResult, error) {
	validator := variablevalidator.NewVariableValidator()

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return inputValidationResultFromReport(report)
	}
	validator.Validate(&r.document, &schema.document, []byte(r.OperationName), r.Variables, &report)

	return inputValidationResultFromReport(report)
}
