package introspection_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
