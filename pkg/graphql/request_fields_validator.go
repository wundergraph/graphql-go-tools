package graphql

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/fields"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type RequestFieldsValidator interface {
	Validate(operation, definition *ast.Document, restrictions []fields.Type) (RequestFieldsValidationResult, error)
}

type fieldsValidator struct {
}

func (d fieldsValidator) Validate(operation, definition *ast.Document, restrictions []fields.Type) (RequestFieldsValidationResult, error) {
	report := operationreport.Report{}
	if len(restrictions) == 0 {
		return fieldsValidationResult(true, report)
	}

	requestedTypes := make(fields.RequestTypes)
	fields.NewGenerator().Generate(operation, definition, &report, requestedTypes)

	for _, restrictedType := range restrictions {
		requestedFields, hasRestrictedType := requestedTypes[restrictedType.Name]
		if !hasRestrictedType {
			continue
		}
		for _, field := range restrictedType.Fields {
			if _, hasRestrictedField := requestedFields[field]; hasRestrictedField {
				return fieldsValidationResult(false, report)
			}
		}
	}

	return fieldsValidationResult(true, report)
}

type RequestFieldsValidationResult struct {
	Valid  bool
	Errors Errors
}

func fieldsValidationResult(valid bool, report operationreport.Report) (RequestFieldsValidationResult, error) {
	result := RequestFieldsValidationResult{
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
