package graphqljsonschema

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
)

func runTest(schema, operation, expectedJsonSchema string, valid []string, invalid []string, overrides map[string]JsonSchema) func(t *testing.T) {
	return func(t *testing.T) {
		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		operationDoc := unsafeparser.ParseGraphqlDocumentString(operation)

		variableDefinition := operationDoc.OperationDefinitions[0].VariableDefinitions.Refs[0]
		varType := operationDoc.VariableDefinitions[variableDefinition].Type

		if overrides == nil {
			overrides = map[string]JsonSchema{}
		}

		jsonSchemaDefinition := FromTypeRefWithOverrides(&operationDoc, &definition, varType, overrides)
		actualSchema, err := json.Marshal(jsonSchemaDefinition)
		fmt.Println(string(actualSchema))
		assert.NoError(t, err)
		assert.Equal(t, expectedJsonSchema, string(actualSchema))

		validator, err := NewValidatorFromString(string(actualSchema))
		assert.NoError(t, err)

		for _, input := range valid {
			assert.NoError(t, validator.Validate(context.Background(), []byte(input)))
		}

		for _, input := range invalid {
			assert.Error(t, validator.Validate(context.Background(), []byte(input)))
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
		nil,
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
		nil,
	))
	t.Run("id", runTest(
		`scalar ID input Test { str: String }`,
		`query ($input: ID){}`,
		`{"type":["string","integer"]}`,
		[]string{
			`"validString"`,
		},
		[]string{
			`null`,
			`false`,
			`true`,
			`nope`,
		},
		nil,
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
		nil,
	))
	t.Run("nested object with override", runTest(
		`scalar String scalar Boolean input Test { str: String! override: Override } input Override { boo: Boolean }`,
		`query ($input: Test){}`,
		`{"type":"object","properties":{"override":{"type":"string"},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}`,
		[]string{
			`{"str":"validString"}`,
			`{"str":"validString","override":"{\"boo\":true}"}`,
		},
		[]string{
			`{"str":true}`,
			`{"override":{"boo":true}}`,
			`{"str":"validString","override":{"boo":123}}`,
		},
		map[string]JsonSchema{
			"Override": NewString(),
		},
	))
	t.Run("recursive object", runTest(
		`scalar String scalar Boolean input Test { str: String! nested: Nested } input Nested { boo: Boolean recursive: Test }`,
		`query ($input: Test){}`,
		`{"type":"object","properties":{"nested":{"type":"object","properties":{"boo":{"type":"boolean"},"recursive":{"type":"object","properties":{"nested":{"type":"object","properties":{"boo":{"type":"boolean"},"recursive":{"type":"object","properties":{"nested":{"type":"object","additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}},"additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}},"additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}`,
		[]string{
			`{"str":"validString"}`,
			`{"str":"validString","nested":{"boo":true}}`,
		},
		[]string{
			`{"str":true}`,
			`{"nested":{"boo":true}}`,
			`{"str":"validString","nested":{"boo":123}}`,
		},
		nil,
	))
	t.Run("recursive object with multiple branches", runTest(
		`scalar String scalar Boolean input Root { test: Test another: Another } input Test { str: String! nested: Nested } input Nested { boo: Boolean recursive: Test another: Another } input Another { boo: Boolean }`,
		`query ($input: Root){}`,
		`{"type":"object","properties":{"another":{"type":"object","properties":{"boo":{"type":"boolean"}},"additionalProperties":false},"test":{"type":"object","properties":{"nested":{"type":"object","properties":{"another":{"type":"object","properties":{"boo":{"type":"boolean"}},"additionalProperties":false},"boo":{"type":"boolean"},"recursive":{"type":"object","properties":{"nested":{"type":"object","properties":{"another":{"type":"object","additionalProperties":false},"boo":{"type":"boolean"},"recursive":{"type":"object","additionalProperties":false}},"additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}},"additionalProperties":false},"str":{"type":"string"}},"required":["str"],"additionalProperties":false}},"additionalProperties":false}`,
		[]string{
			`{"test":{"str":"validString"}}`,
			`{"test":{"str":"validString","nested":{"boo":true}}}`,
		},
		[]string{
			`{"test":{"str":true}}`,
			`{"test":{"nested":{"boo":true}}}`,
			`{"test":{"str":"validString","nested":{"boo":123}}}`,
		},
		nil,
	))
	t.Run("complex recursive schema", runTest(
		complexRecursiveSchema,
		`query ($input: db_messagesWhereInput){}`,
		`{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","additionalProperties":false},"isNot":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"message":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"payload":{"type":"object","properties":{"equals":{"type":"string"},"not":{"type":"string"}},"additionalProperties":false},"user_id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"users":{"type":"object","properties":{"is":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"none":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"some":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"isNot":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"NOT":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","additionalProperties":false},"none":{"type":"object","additionalProperties":false},"some":{"type":"object","additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false},"OR":{"type":"array","item":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"email":{"type":"object","additionalProperties":false},"id":{"type":"object","additionalProperties":false},"lastlogin":{"type":"object","additionalProperties":false},"messages":{"type":"object","additionalProperties":false},"name":{"type":"object","additionalProperties":false},"pet":{"type":"object","additionalProperties":false},"updatedat":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"email":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"id":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","properties":{"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"notIn":{"type":"array","item":null}},"additionalProperties":false},"lastlogin":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"messages":{"type":"object","properties":{"every":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"none":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false},"some":{"type":"object","properties":{"AND":{"type":"object","additionalProperties":false},"NOT":{"type":"object","additionalProperties":false},"OR":{"type":"array","item":{"type":"object","additionalProperties":false}},"id":{"type":"object","additionalProperties":false},"message":{"type":"object","additionalProperties":false},"payload":{"type":"object","additionalProperties":false},"user_id":{"type":"object","additionalProperties":false},"users":{"type":"object","additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false},"name":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"pet":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"mode":{"type":"string"},"not":{"type":"object","properties":{"contains":null,"endsWith":null,"equals":null,"gt":null,"gte":null,"in":{"type":"array","item":null},"lt":null,"lte":null,"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"notIn":{"type":"array","item":null},"startsWith":null},"additionalProperties":false},"updatedat":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","properties":{"equals":{},"gt":{},"gte":{},"in":{"type":"array","item":{}},"lt":{},"lte":{},"not":{"type":"object","additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false},"notIn":{"type":"array","item":{}}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}},"additionalProperties":false}`,
		[]string{},
		[]string{},
		nil,
	))
}

const complexRecursiveSchema = `

input db_NestedIntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntFilter
}

input db_IntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntFilter
}

enum db_QueryMode {
  default
  insensitive
}

input db_NestedStringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: db_NestedStringFilter
}

input db_StringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: db_QueryMode
  not: db_NestedStringFilter
}

enum db_JsonNullValueFilter {
  DbNull
  JsonNull
  AnyNull
}

input db_JsonFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
}

input db_NestedDateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeFilter
}

input db_DateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeFilter
}

input db_MessagesListRelationFilter {
  every: db_messagesWhereInput
  some: db_messagesWhereInput
  none: db_messagesWhereInput
}

input db_usersWhereInput {
  AND: db_usersWhereInput
  OR: [db_usersWhereInput]
  NOT: db_usersWhereInput
  id: db_IntFilter
  email: db_StringFilter
  name: db_StringFilter
  updatedat: db_DateTimeFilter
  lastlogin: db_DateTimeFilter
  pet: db_StringFilter
  messages: db_MessagesListRelationFilter
}

input db_UsersRelationFilter {
  is: db_usersWhereInput
  isNot: db_usersWhereInput
}

input db_messagesWhereInput {
  AND: db_messagesWhereInput
  OR: [db_messagesWhereInput]
  NOT: db_messagesWhereInput
  id: db_IntFilter
  user_id: db_IntFilter
  message: db_StringFilter
  payload: db_JsonFilter
  users: db_UsersRelationFilter
}

enum db_SortOrder {
  asc
  desc
}

input db_messagesOrderByRelationAggregateInput {
  _count: db_SortOrder
}

input db_usersOrderByWithRelationInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
  messages: db_messagesOrderByRelationAggregateInput
}

input db_messagesOrderByWithRelationInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
  users: db_usersOrderByWithRelationInput
}

input db_messagesWhereUniqueInput {
  id: Int
}

enum db_MessagesScalarFieldEnum {
  id
  user_id
  message
  payload
}

type db_UsersCountOutputType {
  messages: Int!
  _join: Query!
}

type db_users {
  id: Int!
  email: String!
  name: String!
  updatedat: DateTime!
  lastlogin: DateTime!
  pet: String!
  messages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): [db_messages]
  _count: db_UsersCountOutputType
  _join: Query!
}

type db_messages {
  id: Int!
  user_id: Int!
  message: String!
  payload: db_Widgets!
  users: db_users!
  _join: Query!
}

type db_MessagesCountAggregateOutputType {
  id: Int!
  user_id: Int!
  message: Int!
  payload: Int!
  _all: Int!
  _join: Query!
}

type db_MessagesAvgAggregateOutputType {
  id: Float
  user_id: Float
  _join: Query!
}

type db_MessagesSumAggregateOutputType {
  id: Int
  user_id: Int
  _join: Query!
}

type db_MessagesMinAggregateOutputType {
  id: Int
  user_id: Int
  message: String
  _join: Query!
}

type db_MessagesMaxAggregateOutputType {
  id: Int
  user_id: Int
  message: String
  _join: Query!
}

type db_AggregateMessages {
  _count: db_MessagesCountAggregateOutputType
  _avg: db_MessagesAvgAggregateOutputType
  _sum: db_MessagesSumAggregateOutputType
  _min: db_MessagesMinAggregateOutputType
  _max: db_MessagesMaxAggregateOutputType
  _join: Query!
}

input db_messagesCountOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
}

input db_messagesAvgOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
}

input db_messagesMaxOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
}

input db_messagesMinOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
}

input db_messagesSumOrderByAggregateInput {
  id: db_SortOrder
  user_id: db_SortOrder
}

input db_messagesOrderByWithAggregationInput {
  id: db_SortOrder
  user_id: db_SortOrder
  message: db_SortOrder
  payload: db_SortOrder
  _count: db_messagesCountOrderByAggregateInput
  _avg: db_messagesAvgOrderByAggregateInput
  _max: db_messagesMaxOrderByAggregateInput
  _min: db_messagesMinOrderByAggregateInput
  _sum: db_messagesSumOrderByAggregateInput
}

input db_NestedFloatFilter {
  equals: Float
  in: [Float]
  notIn: [Float]
  lt: Float
  lte: Float
  gt: Float
  gte: Float
  not: db_NestedFloatFilter
}

input db_NestedIntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntWithAggregatesFilter
  _count: db_NestedIntFilter
  _avg: db_NestedFloatFilter
  _sum: db_NestedIntFilter
  _min: db_NestedIntFilter
  _max: db_NestedIntFilter
}

input db_IntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: db_NestedIntWithAggregatesFilter
  _count: db_NestedIntFilter
  _avg: db_NestedFloatFilter
  _sum: db_NestedIntFilter
  _min: db_NestedIntFilter
  _max: db_NestedIntFilter
}

input db_NestedStringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: db_NestedStringWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedStringFilter
  _max: db_NestedStringFilter
}

input db_StringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: db_QueryMode
  not: db_NestedStringWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedStringFilter
  _max: db_NestedStringFilter
}

input db_NestedJsonFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
}

input db_JsonWithAggregatesFilter {
  equals: db_JsonNullValueFilter
  not: db_JsonNullValueFilter
  _count: db_NestedIntFilter
  _min: db_NestedJsonFilter
  _max: db_NestedJsonFilter
}

input db_messagesScalarWhereWithAggregatesInput {
  AND: db_messagesScalarWhereWithAggregatesInput
  OR: [db_messagesScalarWhereWithAggregatesInput]
  NOT: db_messagesScalarWhereWithAggregatesInput
  id: db_IntWithAggregatesFilter
  user_id: db_IntWithAggregatesFilter
  message: db_StringWithAggregatesFilter
  payload: db_JsonWithAggregatesFilter
}

type db_MessagesGroupByOutputType {
  id: Int!
  user_id: Int!
  message: String!
  payload: JSON!
  _count: db_MessagesCountAggregateOutputType
  _avg: db_MessagesAvgAggregateOutputType
  _sum: db_MessagesSumAggregateOutputType
  _min: db_MessagesMinAggregateOutputType
  _max: db_MessagesMaxAggregateOutputType
  _join: Query!
}

input db_usersWhereUniqueInput {
  id: Int
  email: String
}

enum db_UsersScalarFieldEnum {
  id
  email
  name
  updatedat
  lastlogin
  pet
}

type db_UsersCountAggregateOutputType {
  id: Int!
  email: Int!
  name: Int!
  updatedat: Int!
  lastlogin: Int!
  pet: Int!
  _all: Int!
  _join: Query!
}

type db_UsersAvgAggregateOutputType {
  id: Float
  _join: Query!
}

type db_UsersSumAggregateOutputType {
  id: Int
  _join: Query!
}

type db_UsersMinAggregateOutputType {
  id: Int
  email: String
  name: String
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  _join: Query!
}

type db_UsersMaxAggregateOutputType {
  id: Int
  email: String
  name: String
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  _join: Query!
}

type db_AggregateUsers {
  _count: db_UsersCountAggregateOutputType
  _avg: db_UsersAvgAggregateOutputType
  _sum: db_UsersSumAggregateOutputType
  _min: db_UsersMinAggregateOutputType
  _max: db_UsersMaxAggregateOutputType
  _join: Query!
}

input db_usersCountOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersAvgOrderByAggregateInput {
  id: db_SortOrder
}

input db_usersMaxOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersMinOrderByAggregateInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
}

input db_usersSumOrderByAggregateInput {
  id: db_SortOrder
}

input db_usersOrderByWithAggregationInput {
  id: db_SortOrder
  email: db_SortOrder
  name: db_SortOrder
  updatedat: db_SortOrder
  lastlogin: db_SortOrder
  pet: db_SortOrder
  _count: db_usersCountOrderByAggregateInput
  _avg: db_usersAvgOrderByAggregateInput
  _max: db_usersMaxOrderByAggregateInput
  _min: db_usersMinOrderByAggregateInput
  _sum: db_usersSumOrderByAggregateInput
}

input db_NestedDateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedDateTimeFilter
  _max: db_NestedDateTimeFilter
}

input db_DateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: db_NestedDateTimeWithAggregatesFilter
  _count: db_NestedIntFilter
  _min: db_NestedDateTimeFilter
  _max: db_NestedDateTimeFilter
}

input db_usersScalarWhereWithAggregatesInput {
  AND: db_usersScalarWhereWithAggregatesInput
  OR: [db_usersScalarWhereWithAggregatesInput]
  NOT: db_usersScalarWhereWithAggregatesInput
  id: db_IntWithAggregatesFilter
  email: db_StringWithAggregatesFilter
  name: db_StringWithAggregatesFilter
  updatedat: db_DateTimeWithAggregatesFilter
  lastlogin: db_DateTimeWithAggregatesFilter
  pet: db_StringWithAggregatesFilter
}

type db_UsersGroupByOutputType {
  id: Int!
  email: String!
  name: String!
  updatedat: DateTime!
  lastlogin: DateTime!
  pet: String!
  _count: db_UsersCountAggregateOutputType
  _avg: db_UsersAvgAggregateOutputType
  _sum: db_UsersSumAggregateOutputType
  _min: db_UsersMinAggregateOutputType
  _max: db_UsersMaxAggregateOutputType
  _join: Query!
}

type Query {
  db_findFirstmessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): db_messages
  db_findManymessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int, distinct: [db_MessagesScalarFieldEnum]): [db_messages]!
  db_aggregatemessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithRelationInput], cursor: db_messagesWhereUniqueInput, take: Int, skip: Int): db_AggregateMessages!
  db_groupBymessages(where: db_messagesWhereInput, orderBy: [db_messagesOrderByWithAggregationInput], by: [db_MessagesScalarFieldEnum]!, having: db_messagesScalarWhereWithAggregatesInput, take: Int, skip: Int): [db_MessagesGroupByOutputType]!
  db_findUniquemessages(where: db_messagesWhereUniqueInput!): db_messages
  db_findFirstusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int, distinct: [db_UsersScalarFieldEnum]): db_users
  db_findManyusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int, distinct: [db_UsersScalarFieldEnum]): [db_users]!
  db_aggregateusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithRelationInput], cursor: db_usersWhereUniqueInput, take: Int, skip: Int): db_AggregateUsers!
  db_groupByusers(where: db_usersWhereInput, orderBy: [db_usersOrderByWithAggregationInput], by: [db_UsersScalarFieldEnum]!, having: db_usersScalarWhereWithAggregatesInput, take: Int, skip: Int): [db_UsersGroupByOutputType]!
  db_findUniqueusers(where: db_usersWhereUniqueInput!): db_users
}

input db_usersCreateWithoutMessagesInput {
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
}

input db_usersCreateOrConnectWithoutMessagesInput {
  where: db_usersWhereUniqueInput!
  create: db_usersCreateWithoutMessagesInput!
}

input db_usersCreateNestedOneWithoutMessagesInput {
  create: db_usersCreateWithoutMessagesInput
  connectOrCreate: db_usersCreateOrConnectWithoutMessagesInput
  connect: db_usersWhereUniqueInput
}

input db_messagesCreateInput {
  message: String!
  payload: db_WidgetsInput
  users: db_usersCreateNestedOneWithoutMessagesInput!
}

input db_StringFieldUpdateOperationsInput {
  set: String
}

input db_DateTimeFieldUpdateOperationsInput {
  set: DateTime
}

input db_usersUpdateWithoutMessagesInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
}

input db_usersUpsertWithoutMessagesInput {
  update: db_usersUpdateWithoutMessagesInput!
  create: db_usersCreateWithoutMessagesInput!
}

input db_usersUpdateOneRequiredWithoutMessagesInput {
  create: db_usersCreateWithoutMessagesInput
  connectOrCreate: db_usersCreateOrConnectWithoutMessagesInput
  upsert: db_usersUpsertWithoutMessagesInput
  connect: db_usersWhereUniqueInput
  update: db_usersUpdateWithoutMessagesInput
}

input db_messagesUpdateInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
  users: db_usersUpdateOneRequiredWithoutMessagesInput
}

input db_messagesCreateManyInput {
  id: Int
  user_id: Int!
  message: String!
  payload: db_WidgetsInput
}

type db_AffectedRowsOutput {
  count: Int!
  _join: Query!
}

input db_messagesUpdateManyMutationInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
}

input db_messagesCreateWithoutUsersInput {
  message: String!
  payload: db_WidgetsInput
}

input db_messagesCreateOrConnectWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  create: db_messagesCreateWithoutUsersInput!
}

input db_messagesCreateManyUsersInput {
  id: Int
  message: String!
  payload: db_WidgetsInput
}

input db_messagesCreateManyUsersInputEnvelope {
  data: [db_messagesCreateManyUsersInput]!
  skipDuplicates: Boolean
}

input db_messagesCreateNestedManyWithoutUsersInput {
  create: db_messagesCreateWithoutUsersInput
  connectOrCreate: db_messagesCreateOrConnectWithoutUsersInput
  createMany: db_messagesCreateManyUsersInputEnvelope
  connect: db_messagesWhereUniqueInput
}

input db_usersCreateInput {
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
  messages: db_messagesCreateNestedManyWithoutUsersInput
}

input db_messagesUpdateWithoutUsersInput {
  message: db_StringFieldUpdateOperationsInput
  payload: db_WidgetsInput
}

input db_messagesUpsertWithWhereUniqueWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  update: db_messagesUpdateWithoutUsersInput!
  create: db_messagesCreateWithoutUsersInput!
}

input db_messagesUpdateWithWhereUniqueWithoutUsersInput {
  where: db_messagesWhereUniqueInput!
  data: db_messagesUpdateWithoutUsersInput!
}

input db_messagesScalarWhereInput {
  AND: db_messagesScalarWhereInput
  OR: [db_messagesScalarWhereInput]
  NOT: db_messagesScalarWhereInput
  id: db_IntFilter
  user_id: db_IntFilter
  message: db_StringFilter
  payload: db_JsonFilter
}

input db_messagesUpdateManyWithWhereWithoutUsersInput {
  where: db_messagesScalarWhereInput!
  data: db_messagesUpdateManyMutationInput!
}

input db_messagesUpdateManyWithoutUsersInput {
  create: db_messagesCreateWithoutUsersInput
  connectOrCreate: db_messagesCreateOrConnectWithoutUsersInput
  upsert: db_messagesUpsertWithWhereUniqueWithoutUsersInput
  createMany: db_messagesCreateManyUsersInputEnvelope
  connect: db_messagesWhereUniqueInput
  set: db_messagesWhereUniqueInput
  disconnect: db_messagesWhereUniqueInput
  delete: db_messagesWhereUniqueInput
  update: db_messagesUpdateWithWhereUniqueWithoutUsersInput
  updateMany: db_messagesUpdateManyWithWhereWithoutUsersInput
  deleteMany: db_messagesScalarWhereInput
}

input db_usersUpdateInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
  messages: db_messagesUpdateManyWithoutUsersInput
}

input db_usersCreateManyInput {
  id: Int
  email: String!
  name: String!
  updatedat: DateTime
  lastlogin: DateTime
  pet: String
}

input db_usersUpdateManyMutationInput {
  email: db_StringFieldUpdateOperationsInput
  name: db_StringFieldUpdateOperationsInput
  updatedat: db_DateTimeFieldUpdateOperationsInput
  lastlogin: db_DateTimeFieldUpdateOperationsInput
  pet: db_StringFieldUpdateOperationsInput
}

type Mutation {
  db_createOnemessages(data: db_messagesCreateInput!): db_messages
  db_upsertOnemessages(where: db_messagesWhereUniqueInput!, create: db_messagesCreateInput!, update: db_messagesUpdateInput!): db_messages
  db_createManymessages(data: [db_messagesCreateManyInput]!, skipDuplicates: Boolean): db_AffectedRowsOutput
  db_deleteOnemessages(where: db_messagesWhereUniqueInput!): db_messages
  db_updateOnemessages(data: db_messagesUpdateInput!, where: db_messagesWhereUniqueInput!): db_messages
  db_updateManymessages(data: db_messagesUpdateManyMutationInput!, where: db_messagesWhereInput): db_AffectedRowsOutput
  db_deleteManymessages(where: db_messagesWhereInput): db_AffectedRowsOutput
  db_createOneusers(data: db_usersCreateInput!): db_users
  db_upsertOneusers(where: db_usersWhereUniqueInput!, create: db_usersCreateInput!, update: db_usersUpdateInput!): db_users
  db_createManyusers(data: [db_usersCreateManyInput]!, skipDuplicates: Boolean): db_AffectedRowsOutput
  db_deleteOneusers(where: db_usersWhereUniqueInput!): db_users
  db_updateOneusers(data: db_usersUpdateInput!, where: db_usersWhereUniqueInput!): db_users
  db_updateManyusers(data: db_usersUpdateManyMutationInput!, where: db_usersWhereInput): db_AffectedRowsOutput
  db_deleteManyusers(where: db_usersWhereInput): db_AffectedRowsOutput
}

scalar DateTime

scalar JSON

scalar UUID

type db_Widget {
  id: ID!
  type: String!
  name: String
  options: JSON
  x: Int!
  y: Int!
  width: Int!
  height: Int!
  _join: Query!
}

type db_Widgets {
  items: [db_Widget]!
  _join: Query!
}

input db_WidgetInput {
  id: ID!
  type: String!
  name: String
  options: JSON
  x: Int!
  y: Int!
  width: Int!
  height: Int!
}

input db_WidgetsInput {
  items: [db_WidgetInput]!
}
`
