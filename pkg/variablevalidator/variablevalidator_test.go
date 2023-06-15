package variablevalidator

import (
	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const testDefinition = `
input CustomInput {
    requiredField: String!
    optionalField: String
}

type Query{
    simpleQuery(code: ID): String
    inputOfInt(code: Int!): String
}

type Mutation {
    customInputNonNull(in: CustomInput!): String
}`

const (
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

	customInputMutation = `
mutation testMutation($in: CustomInput!){
	customInputNonNull(in: $in)
}`

	customMultipleOperation = `
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
			operation:     customInputMutation,
			variables:     `{"in":{"optionalField":"test"}}`,
			expectedError: `Validation for variable "in" failed: validation failed: /: {"optionalField":"te... "requiredField" value is required`,
		},
		{
			name:      "multiple operation should validate first operation",
			operation: customMultipleOperation,
			variables: `{"code":"NG"}`,
		},
		{
			name:          "multiple operation should validate operation name",
			operation:     customMultipleOperation,
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
