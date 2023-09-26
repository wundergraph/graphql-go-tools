package testsgo

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const (
	NotSupportedSuggestionsSkipMsg = "Suggestions is not supported"

	RuleHasNoMapping = `Validation rule: "%s" has no mapped rule`
)

const (
	ExecutableDefinitionsRule                 = "ExecutableDefinitionsRule"
	FieldsOnCorrectTypeRule                   = "FieldsOnCorrectTypeRule"
	KnownArgumentNamesRule                    = "KnownArgumentNamesRule"
	KnownArgumentNamesOnDirectivesRule        = "KnownArgumentNamesOnDirectivesRule"
	KnownDirectivesRule                       = "KnownDirectivesRule"
	KnownTypeNamesRule                        = "KnownTypeNamesRule"
	KnownTypeNamesOperationRule               = "KnownTypeNamesOperationRule"
	LoneAnonymousOperationRule                = "LoneAnonymousOperationRule"
	NoUndefinedVariablesRule                  = "NoUndefinedVariablesRule"
	NoUnusedVariablesRule                     = "NoUnusedVariablesRule"
	OverlappingFieldsCanBeMergedRule          = "OverlappingFieldsCanBeMergedRule"
	ProvidedRequiredArgumentsRule             = "ProvidedRequiredArgumentsRule"
	ProvidedRequiredArgumentsOnDirectivesRule = "ProvidedRequiredArgumentsOnDirectivesRule"
	SingleFieldSubscriptionsRule              = "SingleFieldSubscriptionsRule"
	UniqueArgumentNamesRule                   = "UniqueArgumentNamesRule"
	UniqueDirectivesPerLocationRule           = "UniqueDirectivesPerLocationRule"
	UniqueEnumValueNamesRule                  = "UniqueEnumValueNamesRule"
	UniqueFieldDefinitionNamesRule            = "UniqueFieldDefinitionNamesRule"
	UniqueOperationNamesRule                  = "UniqueOperationNamesRule"
	UniqueOperationTypesRule                  = "UniqueOperationTypesRule"
	UniqueTypeNamesRule                       = "UniqueTypeNamesRule"
	UniqueVariableNamesRule                   = "UniqueVariableNamesRule"
	ValuesOfCorrectTypeRule                   = "ValuesOfCorrectTypeRule"
	VariablesAreInputTypesRule                = "VariablesAreInputTypesRule"
	VariablesInAllowedPositionRule            = "VariablesInAllowedPositionRule"

	FragmentsOnCompositeTypesRule = "FragmentsOnCompositeTypesRule"
	KnownFragmentNamesRule        = "KnownFragmentNamesRule"
	NoFragmentCyclesRule          = "NoFragmentCyclesRule"
	NoUnusedFragmentsRule         = "NoUnusedFragmentsRule"
	PossibleFragmentSpreadsRule   = "PossibleFragmentSpreadsRule"
	UniqueFragmentNamesRule       = "UniqueFragmentNamesRule"

	UniqueInputFieldNamesRule  = "UniqueInputFieldNamesRule"
	UniqueDirectiveNamesRule   = "UniqueDirectiveNamesRule"
	LoneSchemaDefinitionRule   = "LoneSchemaDefinitionRule"
	ScalarLeafsRule            = "ScalarLeafsRule"
	PossibleTypeExtensionsRule = "PossibleTypeExtensionsRule"
)

var rulesMap = map[string][]astvalidation.Rule{
	ExecutableDefinitionsRule:                 {astvalidation.DocumentContainsExecutableOperation()},
	FieldsOnCorrectTypeRule:                   {astvalidation.FieldSelections()},
	KnownArgumentNamesRule:                    {astvalidation.KnownArguments()},
	KnownArgumentNamesOnDirectivesRule:        {},
	KnownDirectivesRule:                       {astvalidation.DirectivesAreDefined()},
	KnownTypeNamesRule:                        {astvalidation.KnownTypeNames()},
	LoneAnonymousOperationRule:                {astvalidation.LoneAnonymousOperation()},
	NoUndefinedVariablesRule:                  {astvalidation.AllVariableUsesDefined()},
	NoUnusedVariablesRule:                     {astvalidation.AllVariablesUsed()},
	OverlappingFieldsCanBeMergedRule:          {astvalidation.FieldSelectionMerging()},
	ProvidedRequiredArgumentsRule:             {astvalidation.RequiredArguments()},
	ProvidedRequiredArgumentsOnDirectivesRule: {},
	SingleFieldSubscriptionsRule:              {astvalidation.SubscriptionSingleRootField()},
	UniqueArgumentNamesRule:                   {astvalidation.ArgumentUniqueness()},
	UniqueDirectivesPerLocationRule:           {astvalidation.DirectivesAreUniquePerLocation()},
	UniqueEnumValueNamesRule:                  {astvalidation.UniqueEnumValueNames()},
	UniqueFieldDefinitionNamesRule:            {astvalidation.UniqueFieldDefinitionNames()},
	UniqueOperationNamesRule:                  {astvalidation.OperationNameUniqueness()},
	UniqueOperationTypesRule:                  {astvalidation.UniqueOperationTypes()},
	UniqueTypeNamesRule:                       {astvalidation.UniqueTypeNames()},
	UniqueVariableNamesRule:                   {astvalidation.VariableUniqueness()},
	ValuesOfCorrectTypeRule:                   {astvalidation.Values()},
	VariablesAreInputTypesRule:                {astvalidation.VariablesAreInputTypes()},
	KnownTypeNamesOperationRule:               {astvalidation.VariablesAreInputTypes(), astvalidation.Fragments()},
	VariablesInAllowedPositionRule:            {astvalidation.ValidArguments(), astvalidation.Values()},

	// fragments rules
	FragmentsOnCompositeTypesRule: {astvalidation.Fragments()},
	KnownFragmentNamesRule:        {astvalidation.Fragments()},
	NoFragmentCyclesRule:          {astvalidation.Fragments()},
	NoUnusedFragmentsRule:         {astvalidation.Fragments()},
	PossibleFragmentSpreadsRule:   {astvalidation.Fragments()},
	UniqueFragmentNamesRule:       {astvalidation.Fragments()},

	// not mapped rules

	UniqueInputFieldNamesRule:  {astvalidation.Values()},
	UniqueDirectiveNamesRule:   {},
	LoneSchemaDefinitionRule:   {},
	ScalarLeafsRule:            {},
	PossibleTypeExtensionsRule: {},
}

func operationValidatorFor(rule string) (*astvalidation.OperationValidator, bool) {
	rules, ok := rulesMap[rule]
	if !ok {
		return nil, false
	}
	return astvalidation.NewOperationValidator(rules), true
}

func definitionValidatorFor(rule string) (*astvalidation.DefinitionValidator, bool) {
	rules, ok := rulesMap[rule]
	if !ok {
		return nil, false
	}
	return astvalidation.NewDefinitionValidator(rules...), true
}

// Loc - local type representing location of validation error message
type Loc struct {
	line, column uint32
}

// Err - local type representing validation error message
type Err struct {
	message   string
	locations []Loc
}

// MessageCompare - is a function which allows to check that report has an expectedErrMsg
type MessageCompare func(expectedErrMsg string)

// ResultCompare - is a function to compare report errors with expectedErrors
type ResultCompare func(expectedErrors []Err)

// ExpectValidationErrorsWithSchema - is a helper to run operation validation
// returns ResultCompare function
func ExpectValidationErrorsWithSchema(t *testing.T, schema string, rule string, queryStr string) ResultCompare {
	t.Helper()

	op, opReport := astparser.ParseGraphqlDocumentString(queryStr)
	def := prepareSchema(schema)

	if opReport.HasErrors() {
		t.Log("operation report has errors")
		return compareReportErrors(t, opReport)
	}

	var (
		report    = operationreport.Report{}
		validator *astvalidation.OperationValidator
	)

	validator, ok := operationValidatorFor(rule)
	if !ok {
		t.Fatalf(RuleHasNoMapping, rule)
		return nil
	}

	validator.Validate(&op, &def, &report)

	return compareReportErrors(t, report)
}

// ExpectValidationErrors - a wrapper for ExpectValidationErrorsWithSchema which uses default testSchema
// returns ResultCompare function
func ExpectValidationErrors(t *testing.T, rule string, queryStr string) ResultCompare {
	return ExpectValidationErrorsWithSchema(t, testSchema, rule, queryStr)
}

// ExpectSDLValidationErrors - is a helper to run schema definition validation
// returns ResultCompare function
// in reference tests schema is optional but leaves on a first param
func ExpectSDLValidationErrors(t *testing.T, schema string, rule string, sdlStr string) ResultCompare {
	t.Helper()

	def := prepareSchema(sdlStr)

	if schema != "" {
		// merge schema additions
		def.Input.AppendInputBytes([]byte(schema))
		parser := astparser.NewParser()
		mergeReport := operationreport.Report{}
		parser.Parse(&def, &mergeReport)

		if mergeReport.HasErrors() {
			t.Log("merge failed")
			return compareReportErrors(t, mergeReport)
		}
	}

	// validate schema sdl
	var (
		report    = operationreport.Report{}
		validator *astvalidation.DefinitionValidator
	)

	validator, ok := definitionValidatorFor(rule)
	if !ok {
		t.Fatalf(RuleHasNoMapping, rule)
		return nil
	}

	validator.Validate(&def, &report)

	return compareReportErrors(t, report)
}

// BuildSchema - helper used in reference test.
// As we handle validation differently return same schema string
func BuildSchema(sdl string) string {
	return sdl
}

// ExpectValidationErrorMessage - is a helper to run operation validation and check single error message
// returns MessageCompare
func ExpectValidationErrorMessage(t *testing.T, schema string, queryStr string) MessageCompare {
	op, opReport := astparser.ParseGraphqlDocumentString(queryStr)
	def := prepareSchema(schema)

	if opReport.HasErrors() {
		t.Log("operation report has errors")
		return hasReportError(t, opReport)
	}

	report := operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, &report)

	return hasReportError(t, report)
}

// ExtendSchema - helper to extend schema with provided sdl
//
//nolint:unused
func ExtendSchema(schema string, sdlStr string) string {
	if sdlStr != "" {
		schema = schema + "\n" + sdlStr
	}

	definition := prepareSchema(schema)
	parser := astparser.NewParser()
	report := operationreport.Report{}
	parser.Parse(&definition, &report)

	res, _ := astprinter.PrintStringIndent(&definition, nil, "  ")

	return res
}

func prepareSchema(schema string) ast.Document {
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	if report.HasErrors() {
		panic(report.Error())
	}

	_ = asttransform.MergeDefinitionWithBaseSchema(&definition)

	return definition
}

// externalErrors - converts external errors to simple local type Err
// convertor could be adjusted to use exact type
func externalErrors(report operationreport.Report) (out []Err) {
	out = make([]Err, 0)

	for _, externalError := range report.ExternalErrors {
		var locations []Loc

		for _, location := range externalError.Locations {
			locations = append(locations, Loc{
				line:   location.Line,
				column: location.Column,
			})
		}

		out = append(out, Err{
			message:   externalError.Message,
			locations: locations,
		})
	}

	return
}

// compareReportErrors - helper returns ResultCompare function for operationreport.Report
func compareReportErrors(t *testing.T, report operationreport.Report) ResultCompare {
	return func(expectedErrors []Err) {
		actualErrors := externalErrors(report)
		assert.Equal(t, expectedErrors, actualErrors)
	}
}

// hasReportError - helper returns MessageCompare function for operationreport.Report
func hasReportError(t *testing.T, report operationreport.Report) MessageCompare {
	return func(msg string) {
		actualErrors := externalErrors(report)

		var messages []string
		for _, actualError := range actualErrors {
			messages = append(messages, actualError.message)
		}

		assert.Contains(t, messages, msg)
	}
}

// testSchema - represents schema definition used in reference tests
var testSchema string

func init() {
	content, err := ioutil.ReadFile("test_schema.graphql")
	if err != nil {
		panic(err)
	}

	testSchema = string(content)
}
