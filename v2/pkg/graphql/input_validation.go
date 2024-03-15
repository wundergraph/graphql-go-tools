package graphql

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/variablesvalidation"
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

func inputValidationResultFromErr(err error) (InputValidationResult, error) {
	result := InputValidationResult{
		Valid:  false,
		Errors: nil,
	}

	if err == nil {
		result.Valid = true
		return result, nil
	}

	result.Errors = RequestErrorsFromError(err)
	return result, nil
}

func (r *Request) ValidateInput(schema *Schema) (InputValidationResult, error) {
	validator := variablesvalidation.NewVariablesValidator()

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return inputValidationResultFromReport(report)
	}

	err := validator.Validate(&r.document, &schema.document, r.Variables)
	return inputValidationResultFromErr(err)
}
