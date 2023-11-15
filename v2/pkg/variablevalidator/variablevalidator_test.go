package variablevalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

const testDefinition = `
input StringQueryInput{
    eq: String
    contains: String
    nested: StringQueryInput
}

input CustomInput {
    requiredField: String!
    optionalField: String
	query: StringQueryInput
	arrayField: [String!]
}

input QueryInput{
    name: StringQueryInput
    tag: StringQueryInput
}

type Query{
    simpleQuery(code: ID): String
    inputOfInt(code: Int!): String
    queryUsingStringQuery(in: QueryInput!): String
}

type Mutation {
    customInputNonNull(in: CustomInput!): String
}`

const (
	testStringQuery = `
query main($in: QueryInput!){
    queryUsingStringQuery(in: $in)
}`

	testQuery = `
query testQuery($code: ID!){
  simpleQuery(code: $code)
}
`

	testQueryNonNullInput = `
query testQuery($code: ID){
  simpleQuery(code: $code)
}
`

	testQueryInt = `
query testQuery($code: Int!){
  inputOfInt(code: $code)
}
`

	testCustomInputMutation = `
mutation testMutation($in: CustomInput!){
	customInputNonNull(in: $in)
}`

	testCustomMultipleOperation = `
query testQuery($code: ID!){
  simpleQuery(code: $code)
}

mutation testMutation($in: CustomInput!){
	customInputNonNull(in: $in)
}
`
)

func TestVariableValidator(t *testing.T) {
	testCases := []struct {
		name          string
		operation     string
		operationName string
		variables     string
		expectedError string
	}{
		{
			name:      "basic variable query",
			operation: testQuery,
			variables: `{"code":"NG"}`,
		},
		{
			name:      "basic variable query of int",
			operation: testQueryInt,
			variables: `{"code":1}`,
		},
		{
			name:          "missing variable",
			operation:     testQuery,
			variables:     `{"codes":"NG"}`,
			expectedError: `Required variable "$code" was not provided`,
		},
		{
			name:          "no variable passed",
			operation:     testQuery,
			variables:     "",
			expectedError: `Required variable "$code" was not provided`,
		},
		{
			name:          "nested input variable",
			operation:     testCustomInputMutation,
			variables:     `{"in":{"optionalField":"test"}}`,
			expectedError: `Validation for variable "in" failed: missing properties: 'requiredField'`,
		},
		{
			name:          "invalid variable type",
			operation:     testCustomInputMutation,
			variables:     `{"in":{"query":{"eq":2}, "requiredField": "test"}}`,
			expectedError: `Validation for variable "in" failed: field query.eq, expected string or null, but got number`,
		},
		{
			name:      "multiple operation should validate first operation",
			operation: testCustomMultipleOperation,
			variables: `{"code":"NG"}`,
		},
		{
			name:          "multiple operation should validate operation name",
			operation:     testCustomMultipleOperation,
			operationName: "testMutation",
			variables:     `{"in":{"requiredField":"test"}}`,
		},
		{
			name:          "invalid variable json",
			operation:     testQuery,
			variables:     `"\n            {\"code\":{\"code\":{\"in\":[\"PL\",\"UA\"],\"extra\":\"koza\"}}}\n        "`,
			expectedError: `Required variable "$code" was not provided`,
		},
		{
			name:      "invalid variable json non null input",
			operation: testQueryNonNullInput,
			variables: `"\n            {\"code\":{\"code\":{\"in\":[\"PL\",\"UA\"],\"extra\":\"koza\"}}}\n        "`,
		},
		{
			name:          "should use $refs",
			operation:     testStringQuery,
			variables:     `{"in":{"name":{"eq":1}}}`,
			expectedError: `Validation for variable "in" failed: field name.eq, expected string or null, but got number`,
		},
		{
			name:          "array type",
			operation:     testCustomInputMutation,
			variables:     `{"in":{"requiredField":"test","arrayField":{"value":1}}}`,
			expectedError: `Validation for variable "in" failed: field arrayField, expected array or null, but got object`,
		},
		{
			name:          "deeply nested field",
			operation:     testStringQuery,
			variables:     `{"in":{"name":{"nested":{"eq":1}}}}`,
			expectedError: `Validation for variable "in" failed: field name.nested.eq, expected string or null, but got number`,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			definitionDocument := unsafeparser.ParseGraphqlDocumentString(testDefinition)
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

			operationDocument := unsafeparser.ParseGraphqlDocumentString(c.operation)

			report := operationreport.Report{}
			validator := NewVariableValidator()
			validator.Validate(&operationDocument, &definitionDocument, []byte(c.operationName), []byte(c.variables), &report)

			if c.expectedError == "" && report.HasErrors() {
				t.Fatalf("expected no error, instead got %s", report.Error())
			}
			if c.expectedError != "" {
				require.Equal(t, 1, len(report.ExternalErrors))
				assert.Equal(t, c.expectedError, report.ExternalErrors[0].Message)
			}
		})
	}
}
