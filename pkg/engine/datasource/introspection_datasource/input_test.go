package introspection_datasource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildInput(t *testing.T) {
	run := func(fieldName string, expectedJson string) func(t *testing.T) {
		t.Helper()
		return func(t *testing.T) {
			actualResult := buildInput(fieldName)
			assert.Equal(t, expectedJson, actualResult)
		}
	}

	t.Run("schema introspection", run(schemaFieldName, `{"request_type":1}`))
	t.Run("type introspection", run(typeFieldName, `{"request_type":2,"type_name":"{{ .arguments.name }}"}`))
	t.Run("type fields", run(fieldsFieldName, `{"request_type":3,"on_type_name":"{{ .object.name }}","include_deprecated":{{ .arguments.includeDeprecated }}}`))
	t.Run("type enum values", run(enumValuesFieldName, `{"request_type":4,"on_type_name":"{{ .object.name }}","include_deprecated":{{ .arguments.includeDeprecated }}}`))
}

func TestUnmarshalIntrospectionInput(t *testing.T) {
	run := func(input string, expected introspectionInput) func(t *testing.T) {
		t.Helper()
		return func(t *testing.T) {
			var actual introspectionInput
			require.NoError(t, json.Unmarshal([]byte(input), &actual))
			assert.Equal(t, expected, actual)
		}
	}

	foo := "Foo"

	t.Run("schema introspection", run(`{"request_type":1}`, introspectionInput{RequestType: SchemaRequestType}))
	t.Run("type introspection", run(`{"request_type":2,"type_name":"Foo"}`, introspectionInput{RequestType: TypeRequestType, TypeName: &foo}))
	t.Run("type fields", run(`{"request_type":3,"on_type_name":"Foo","include_deprecated":true}`, introspectionInput{RequestType: TypeFieldsRequestType, OnTypeName: &foo, IncludeDeprecated: true}))
	t.Run("type enum values", run(`{"request_type":4,"on_type_name":"Foo","include_deprecated":false}`, introspectionInput{RequestType: TypeEnumValuesRequestType, OnTypeName: &foo, IncludeDeprecated: false}))
}
