package graphql

import (
	"fmt"

	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

const asteriskCharacter = "*"

type RequestFieldsValidator interface {
	Validate(request *Request, schema *Schema, restrictions []Type) (RequestFieldsValidationResult, error)
}

type FieldRestrictionValidator interface {
	ValidateByFieldList(request *Request, schema *Schema, restrictionList FieldRestrictionList) (RequestFieldsValidationResult, error)
}

type FieldRestrictionListKind int

const (
	AllowList FieldRestrictionListKind = iota
	BlockList
)

type FieldRestrictionList struct {
	Kind  FieldRestrictionListKind
	Types []Type
}

type DefaultFieldsValidator struct {
}

// Validate validates a request by checking if `restrictions` contains blocked fields.
//
// Deprecated: This function can only handle blocked fields. Use `ValidateByFieldList` if you
// want to check for blocked or allowed fields instead.
func (d DefaultFieldsValidator) Validate(request *Request, schema *Schema, restrictions []Type) (RequestFieldsValidationResult, error) {
	restrictionList := FieldRestrictionList{
		Kind:  BlockList,
		Types: restrictions,
	}

	return d.ValidateByFieldList(request, schema, restrictionList)
}

// ValidateByFieldList will validate a request by using a list of allowed or blocked fields.
func (d DefaultFieldsValidator) ValidateByFieldList(request *Request, schema *Schema, restrictionList FieldRestrictionList) (RequestFieldsValidationResult, error) {
	report := operationreport.Report{}
	if len(restrictionList.Types) == 0 {
		return fieldsValidationResult(report, true, "", "")
	}

	requestedTypes := make(RequestTypes)
	NewExtractor().ExtractFieldsFromRequest(request, schema, &report, requestedTypes)

	if restrictionList.Kind == BlockList {
		return d.checkForBlockedFields(restrictionList, requestedTypes, report)
	}

	return d.checkForAllowedFields(restrictionList, requestedTypes, report)
}

func (d DefaultFieldsValidator) checkForBlockedFields(restrictionList FieldRestrictionList, requestTypes RequestTypes, report operationreport.Report) (RequestFieldsValidationResult, error) {
	restrictedFieldsLookupMap := make(map[string]map[string]bool)
	for _, restrictedType := range restrictionList.Types {
		restrictedFieldsLookupMap[restrictedType.Name] = make(map[string]bool)
		for _, restrictedField := range restrictedType.Fields {
			restrictedFieldsLookupMap[restrictedType.Name][restrictedField] = true
		}
	}

	for requestType, requestFields := range requestTypes {
		for requestField := range requestFields {
			if _, ok := restrictedFieldsLookupMap[requestType][asteriskCharacter]; ok {
				return fieldsValidationResultForAsterisk(report, false, requestType)
			}

			isRestrictedType := restrictedFieldsLookupMap[requestType][requestField]
			if isRestrictedType {
				return fieldsValidationResult(report, false, requestType, requestField)
			}
		}
	}

	return fieldsValidationResult(report, true, "", "")
}

func (d DefaultFieldsValidator) checkForAllowedFields(restrictionList FieldRestrictionList, requestTypes RequestTypes, report operationreport.Report) (RequestFieldsValidationResult, error) {
	// Group allowed fields and types for easy access.
	allowedFieldsLookupMap := make(map[string]map[string]bool)
	for _, allowedType := range restrictionList.Types {
		allowedFieldsLookupMap[allowedType.Name] = make(map[string]bool)
		for _, allowedField := range allowedType.Fields {
			allowedFieldsLookupMap[allowedType.Name][allowedField] = true
		}
	}

	// Try to find a disallowed field.
	for requestType, requestFields := range requestTypes {
		if _, ok := allowedFieldsLookupMap[requestType][asteriskCharacter]; ok {
			// Every field is allowed to access for this type.
			continue
		}

		for requestField := range requestFields {
			isAllowedField := allowedFieldsLookupMap[requestType][requestField]
			if !isAllowedField {
				// The requested field is not allowed to access.
				return fieldsValidationResult(report, false, requestType, requestField)
			}
		}
	}

	return fieldsValidationResult(report, true, "", "")
}

type RequestFieldsValidationResult struct {
	Valid  bool
	Errors Errors
}

func fieldsValidationResultCommon(report operationreport.Report, valid bool, requestErrors RequestErrors) (RequestFieldsValidationResult, error) {
	result := RequestFieldsValidationResult{
		Valid:  valid,
		Errors: nil,
	}

	result.Errors = requestErrors
	if !report.HasErrors() {
		return result, nil
	}

	requestErrors = append(requestErrors, RequestErrorsFromOperationReport(report)...)
	result.Errors = requestErrors

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}

func fieldsValidationResult(report operationreport.Report, valid bool, typeName, fieldName string) (RequestFieldsValidationResult, error) {
	var requestErrors RequestErrors
	if !valid {
		requestErrors = append(requestErrors, RequestError{
			Message: fmt.Sprintf("field: %s is restricted on type: %s", fieldName, typeName),
		})
	}
	return fieldsValidationResultCommon(report, valid, requestErrors)
}

func fieldsValidationResultForAsterisk(report operationreport.Report, valid bool, typeName string) (RequestFieldsValidationResult, error) {
	var requestErrors RequestErrors
	if !valid {
		requestErrors = append(requestErrors, RequestError{
			Message: fmt.Sprintf("all fields of %s type are restricted", typeName),
		})
	}
	return fieldsValidationResultCommon(report, valid, requestErrors)
}
