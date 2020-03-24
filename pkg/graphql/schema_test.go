package graphql

import (
	"bytes"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
)

func TestNewSchemaFromReader(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from io.Reader", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.Content)
		assert.Equal(t, abstractlogger.NoopLogger, schema.logger)
		assert.NotNil(t, schema.basePlanner)
	})
}

func TestNewSchemaFromString(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from string", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.Content)
		assert.Equal(t, abstractlogger.NoopLogger, schema.logger)
		assert.NotNil(t, schema.basePlanner)
	})
}
