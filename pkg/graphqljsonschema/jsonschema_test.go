package graphqljsonschema

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/stretchr/testify/assert"
)

func runTest(schema, operation, expectedJsonSchema string, valid []string, invalid []string) func(t *testing.T) {
	return func(t *testing.T) {
		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		operationDoc := unsafeparser.ParseGraphqlDocumentString(operation)

		variableDefinition := operationDoc.OperationDefinitions[0].VariableDefinitions.Refs[0]
		varType := operationDoc.VariableDefinitions[variableDefinition].Type

		jsonSchemaDefinition := FromTypeRef(&operationDoc, &definition, varType)
		actualSchema, err := json.Marshal(jsonSchemaDefinition)
		assert.NoError(t, err)
		assert.Equal(t, expectedJsonSchema, string(actualSchema))

		validator, err := NewValidatorFromString(string(actualSchema))
		assert.NoError(t, err)

		for _, input := range valid {
			assert.True(t, validator.Validate(context.Background(), []byte(input)))
		}

		for _, input := range invalid {
			assert.False(t, validator.Validate(context.Background(), []byte(input)))
		}
	}
}

func TestJsonSchema(t *testing.T) {
	t.Run("object", runTest(
		`scalar String input Test { str: String }`,
		`query ($input: Test){}`,
		`{"type":"object","properties":{"str":{"type":"string"}},"additionalProperties":false}`,
		[]string{
			`{"str":"validString"}`,
		},
		[]string{
			`{"str":true}`,
		},
	))
	t.Run("string", runTest(
		`scalar String input Test { str: String }`,
		`query ($input: String){}`,
		`{"type":"string"}`,
		[]string{
			`"validString"`,
		},
		[]string{
			`null`,
			`false`,
			`true`,
			`nope`,
		},
	))
	t.Run("nested object", runTest(
		`scalar String scalar Boolean input Test { str: String! nested: Nested } input Nested { boo: Boolean }`,
		`query ($input: Test){}`,
		`{"type":"object","properties":{"nested":{"type":"object","properties":{"boo":{"type":"boolean"}},"additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}`,
		[]string{
			`{"str":"validString"}`,
			`{"str":"validString","nested":{"boo":true}}`,
		},
		[]string{
			`{"str":true}`,
			`{"nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":123}}`,
		},
	))
}
