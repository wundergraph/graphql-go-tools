package responsejsonschema

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func parseDocument(t *testing.T, input string) ast.Document {
	t.Helper()

	return unsafeparser.ParseGraphqlDocumentString(input)
}

func buildSchema(t *testing.T, definitionInput, operationInput string, fieldPath []string, opts ...Option) json.RawMessage {
	t.Helper()

	definition := parseDocument(t, definitionInput)
	operation := parseDocument(t, operationInput)
	schema, err := Build(&operation, &definition, fieldPath, opts...)
	require.NoError(t, err)
	require.True(t, json.Valid(schema), "Build returned invalid JSON Schema: %s", schema)

	return schema
}

func requireSchemaValidation(t *testing.T, schema json.RawMessage, valid, invalid []string) {
	t.Helper()

	compiled, err := jsonschema.CompileString("response.schema.json", string(schema))
	require.NoError(t, err)

	for _, input := range valid {
		var value any
		require.NoError(t, json.Unmarshal([]byte(input), &value), "invalid test fixture: %s", input)
		require.NoError(t, compiled.Validate(value), "expected valid response value: %s", input)
	}

	for _, input := range invalid {
		var value any
		require.NoError(t, json.Unmarshal([]byte(input), &value), "invalid test fixture: %s", input)
		require.Error(t, compiled.Validate(value), "expected invalid response value: %s", input)
	}
}
