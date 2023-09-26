package graphql

import (
	"github.com/wundergraph/graphql-go-tools/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type ValidationResult struct {
	Valid  bool
	Errors ValidationErrors
}

type ValidationErrors Errors

func (r *Request) ValidateForSchema(schema *Schema) (result ValidationResult, err error) {
	if schema == nil {
		return ValidationResult{Valid: false, Errors: nil}, ErrNilSchema
	}

	schemaHash := schema.Hash()

	if r.validForSchema == nil {
		r.validForSchema = map[uint64]ValidationResult{}
	}

	if result, ok := r.validForSchema[schemaHash]; ok {
		return result, nil
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return operationValidationResultFromReport(report)
	}

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&r.document, &schema.document, &report)
	result, err = operationValidationResultFromReport(report)
	if err != nil {
		return result, err
	}
	r.validForSchema[schemaHash] = result
	return result, err
}

// ValidateRestrictedFields validates a request by checking if `restrictedFields` contains blocked fields.
//
// Deprecated: This function can only handle blocked fields. Use `ValidateFieldRestrictions` if you
// want to check for blocked or allowed fields instead.
func (r *Request) ValidateRestrictedFields(schema *Schema, restrictedFields []Type) (RequestFieldsValidationResult, error) {
	if schema == nil {
		return RequestFieldsValidationResult{Valid: false}, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return fieldsValidationResult(report, false, "", "")
	}

	var fieldsValidator RequestFieldsValidator = DefaultFieldsValidator{}
	return fieldsValidator.Validate(r, schema, restrictedFields)
}

// ValidateFieldRestrictions will validate a request by using a list of allowed or blocked fields.
func (r *Request) ValidateFieldRestrictions(schema *Schema, restrictedFieldsList FieldRestrictionList, validator FieldRestrictionValidator) (RequestFieldsValidationResult, error) {
	if schema == nil {
		return RequestFieldsValidationResult{Valid: false}, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return fieldsValidationResult(report, false, "", "")
	}

	return validator.ValidateByFieldList(r, schema, restrictedFieldsList)
}

func operationValidationResultFromReport(report operationreport.Report) (ValidationResult, error) {
	result := ValidationResult{
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
