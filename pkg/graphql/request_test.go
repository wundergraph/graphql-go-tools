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
		requestBytes := []byte(`{"operationName": "Hello", "variables": "", "query": "query Hello { hello }"}`)
		requestBuffer := bytes.NewBuffer(requestBytes)

		var request Request
		err := UnmarshalRequest(requestBuffer, &request)

		assert.NoError(t, err)
		assert.Equal(t, "Hello", request.OperationName)
		assert.Equal(t, "query Hello { hello }", request.Query)
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
		request := Request{}
		result, err := request.CalculateComplexity(DefaultComplexityCalculator, nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilSchema, err)
		assert.Equal(t, 0, result.NodeCount, "unexpected node count")
		assert.Equal(t, 0, result.Complexity, "unexpected complexity")
		assert.Equal(t, 0, result.Depth, "unexpected depth")
		assert.Nil(t, result.PerRootField, "per root field results is not nil")
	})

	t.Run("should successfully calculate the complexity of request", func(t *testing.T) {
		schema := starwarsSchema(t)

		request := requestForQuery(t, starwars.FileSimpleHeroQuery)
		result, err := request.CalculateComplexity(DefaultComplexityCalculator, schema)
		assert.NoError(t, err)
		assert.Equal(t, 1, result.NodeCount, "unexpected node count")
		assert.Equal(t, 1, result.Complexity, "unexpected complexity")
		assert.Equal(t, 2, result.Depth, "unexpected depth")
		assert.Equal(t, []FieldComplexityResult{
			{
				TypeName:   "Query",
				FieldName:  "hero",
				Alias:      "",
				NodeCount:  1,
				Complexity: 1,
				Depth:      2,
			},
		}, result.PerRootField, "unexpected per root field results")
	})

	t.Run("should successfully calculate the complexity of request with multiple query fields", func(t *testing.T) {
		schema := starwarsSchema(t)

		request := requestForQuery(t, starwars.FileHeroWithAliasesQuery)
		result, err := request.CalculateComplexity(DefaultComplexityCalculator, schema)
		assert.NoError(t, err)
		assert.Equal(t, 2, result.NodeCount, "unexpected node count")
		assert.Equal(t, 2, result.Complexity, "unexpected complexity")
		assert.Equal(t, 2, result.Depth, "unexpected depth")
		assert.Equal(t, []FieldComplexityResult{
			{
				TypeName:   "Query",
				FieldName:  "hero",
				Alias:      "empireHero",
				NodeCount:  1,
				Complexity: 1,
				Depth:      2,
			},
			{
				TypeName:   "Query",
				FieldName:  "hero",
				Alias:      "jediHero",
				NodeCount:  1,
				Complexity: 1,
				Depth:      2,
			}}, result.PerRootField, "unexpected per root field results")
	})
}

func starwarsSchema(t *testing.T) *Schema {
	starwars.SetRelativePathToStarWarsPackage("../starwars")
	schemaBytes := starwars.Schema(t)

	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	return schema
}

func requestForQuery(t *testing.T, fileName string) Request {
	rawRequest := starwars.LoadQuery(t, fileName, nil)

	var request Request
	err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
	require.NoError(t, err)

	return request
}
