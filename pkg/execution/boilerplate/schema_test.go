package boilerplate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSchemaBytesWithBoilerplate(t *testing.T) {
	schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
	newSchemaBytes := NewSchemaBytesWithBoilerplate(schemaBytes)

	assert.NotEqual(t, schemaBytes, newSchemaBytes)
	assert.Greater(t, len(newSchemaBytes), len(schemaBytes))
}
