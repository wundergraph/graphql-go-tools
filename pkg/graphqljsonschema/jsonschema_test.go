package graphqljsonschema

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/stretchr/testify/assert"
)

func runTest(schema, inputType, inputField string, inputJSON, expectedJsonSchema string, expectedValid bool) func(t *testing.T) {
	return func(t *testing.T) {
		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		node, ok := definition.Index.FirstNodeByNameStr(inputType)
		assert.True(t, ok)
		assert.Equal(t, ast.NodeKindInputObjectTypeDefinition, node.Kind)
		inputValueDefinitionRef := definition.InputObjectTypeDefinitionInputValueDefinitionByName(node.Ref, []byte(inputField))
		inputValueDefinition := definition.InputValueDefinitions[inputValueDefinitionRef]
		jsonSchemaDefinition := FromTypeRef(&definition, inputValueDefinition.Type)
		actualSchema, err := json.Marshal(jsonSchemaDefinition)
		assert.NoError(t, err)
		var expected interface{}
		err = json.Unmarshal([]byte(expectedJsonSchema), &expected)
		assert.NoError(t, err)
		expectedClean, err := json.Marshal(expected)
		assert.NoError(t, err)
		assert.Equal(t, string(expectedClean), string(actualSchema))

		validator, err := NewValidatorFromString(string(actualSchema))
		assert.NoError(t, err)

		actualValid := validator.Validate(context.Background(), []byte(inputJSON))
		assert.Equal(t, expectedValid, actualValid)
	}
}

func TestJsonSchema(t *testing.T) {
	t.Run("string", runTest(
		`scalar String input Test { str: String }`,
		"Test",
		"str",
		`"validString"`,
		`{"type":"string"}`,
		true,
	))
}
