package graphql

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/fields"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type RestrictedFieldsValidator interface {
	Validate(operation, definition *ast.Document, restrictedTypes []fields.Type) (RestrictedFieldsResult, error)
}

type fieldsValidator struct {
}

func (d fieldsValidator) Validate(operation, definition *ast.Document, restrictedTypes []fields.Type) (RestrictedFieldsResult, error) {
	report := operationreport.Report{}
	requestTypes := make(fields.RequestTypes)
	fields.NewGenerator().Generate(operation, definition, &report, requestTypes)

	for i := 0; i < len(restrictedTypes); i++ {
		if typeFields, isTypeRestricted := requestTypes[restrictedTypes[i].Name]; isTypeRestricted {
			for j := 0; j < len(restrictedTypes[i].Fields); j++ {
				if _, isFieldRestricted := typeFields[restrictedTypes[i].Fields[j]]; isFieldRestricted {
					return restrictedFieldsResult(false, report)
				}
			}
		}
	}

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
