package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

func TestUnmarshalRequest(t *testing.T) {
	t.Run("should return error when request is empty", func(t *testing.T) {
		requestBytes := []byte("")
		requestBuffer := bytes.NewBuffer(requestBytes)

		var request Request
		err := UnmarshalRequest(requestBuffer, &request)

		assert.Error(t, err)
		assert.Equal(t, ErrEmptyRequest, err)
	})

	t.Run("should successfully unmarshal request", func(t *testing.T) {
		requestBytes := []byte(`{"operation_name": "Hello", "variables": "", "query": "query Hello { hello }"}`)
		requestBuffer := bytes.NewBuffer(requestBytes)

		var request Request
		err := UnmarshalRequest(requestBuffer, &request)

		assert.NoError(t, err)
		assert.Equal(t, "Hello", request.OperationName)
		assert.Equal(t, "query Hello { hello }", request.Query)
	})
}

func TestRequest_ValidateForSchema(t *testing.T) {
	t.Run("should return error when schema is nil", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		result, err := request.ValidateForSchema(nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilSchema, err)
		assert.Equal(t, ValidationResult{Valid: false, Errors: nil}, result)
	})

	t.Run("should return gql errors when validation fails", func(t *testing.T) {
		request := Request{
			OperationName: "Goodbye",
			Variables:     nil,
			Query:         `query Goodbye { goodbye }`,
		}

		schema, err := NewSchemaFromString("schema { query: Query } type Query { hello: String }")
		require.NoError(t, err)

		result, err := request.ValidateForSchema(schema)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Greater(t, result.Errors.Count(), 0)
	})

	t.Run("should fail validation when schema definition is missing and query contains Query being not present in schema", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		schema, err := NewSchemaFromString("type Mutation { hello: String }")
		require.NoError(t, err)

		result, err := request.ValidateForSchema(schema)
		assert.Error(t, err)
		assert.False(t, result.Valid)
		assert.NotNil(t, result.Errors)
		assert.Greater(t, 0, result.Errors.Count())
	})

	t.Run("should successfully validate even when schema definition is missing", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		schema, err := NewSchemaFromString("type Query { hello: String }")
		require.NoError(t, err)

		result, err := request.ValidateForSchema(schema)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Nil(t, result.Errors)
	})

	t.Run("should return valid result when validation is successful", func(t *testing.T) {
		starwars.SetRelativePathToStarWarsPackage("../starwars")
		schemaBytes := starwars.Schema(t)

		schema, err := NewSchemaFromString(string(schemaBytes))
		require.NoError(t, err)

		rawRequest := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)

		var request Request
		err = UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
		require.NoError(t, err)

		result, err := request.ValidateForSchema(schema)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Nil(t, result.Errors)
	})
}
