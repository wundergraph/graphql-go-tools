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

	t.Run("should return valid result for introspection query after normalization", func(t *testing.T) {
		schema := starwarsSchema(t)

		rawRequest := starwars.LoadQuery(t, starwars.FileIntrospectionQuery, nil)

		var request Request
		err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
		require.NoError(t, err)

		normalizationResult, err := request.Normalize(schema)
		require.NoError(t, err)
		require.True(t, normalizationResult.Successful)
		require.True(t, request.IsNormalized())

		result, err := request.ValidateForSchema(schema)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Nil(t, result.Errors)
	})

	t.Run("should return valid result when validation is successful", func(t *testing.T) {
		schema := starwarsSchema(t)

		rawRequest := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)

		var request Request
		err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
		require.NoError(t, err)

		result, err := request.ValidateForSchema(schema)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Nil(t, result.Errors)
	})
}

func TestRequest_Normalize(t *testing.T) {
	t.Run("should return error when schema is nil", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		result, err := request.Normalize(nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilSchema, err)
		assert.False(t, result.Successful)
		assert.False(t, request.isNormalized)
	})

	t.Run("should successfully normalize the request", func(t *testing.T) {
		schema := starwarsSchema(t)

		rawRequest := starwars.LoadQuery(t, starwars.FileFragmentsQuery, nil)

		var request Request
		err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
		require.NoError(t, err)

		documentBeforeNormalization := request.document

		result, err := request.Normalize(schema)
		assert.NoError(t, err)
		assert.NotEqual(t, documentBeforeNormalization, request.document)
		assert.True(t, result.Successful)
		assert.True(t, request.isNormalized)
	})
}

func TestRequest_Print(t *testing.T) {
	query := "query Hello { hello }"
	request := Request{
		OperationName: "Hello",
		Variables:     nil,
		Query:         query,
	}

	bytesBuf := new(bytes.Buffer)
	n, err := request.Print(bytesBuf)

	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, query, bytesBuf.String())
}

func TestRequest_CalculateComplexity(t *testing.T) {
	t.Run("should return error when schema is nil", func(t *testing.T) {
		request := Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         `query Hello { hello }`,
		}

		result, err := request.CalculateComplexity(DefaultComplexityCalculator, nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilSchema, err)
		assert.Equal(t, 0, result.NodeCount, "unexpected node count")
		assert.Equal(t, 0, result.Complexity, "unexpected complexity")
		assert.Equal(t, 0, result.Depth, "unexpected depth")
	})

	t.Run("should successfully calculate the complexity of request", func(t *testing.T) {
		schema := starwarsSchema(t)

		rawRequest := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)

		var request Request
		err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
		require.NoError(t, err)

		result, err := request.CalculateComplexity(DefaultComplexityCalculator, schema)
		assert.NoError(t, err)
		assert.Equal(t, 1, result.NodeCount, "unexpected node count")
		assert.Equal(t, 1, result.Complexity, "unexpected complexity")
		assert.Equal(t, 2, result.Depth, "unexpected depth")
	})
}

func starwarsSchema(t *testing.T) *Schema {
	starwars.SetRelativePathToStarWarsPackage("../starwars")
	schemaBytes := starwars.Schema(t)

	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	return schema
}
