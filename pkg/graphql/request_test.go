package graphql

import (
	"bytes"
	"strings"
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
				Depth:      1,
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
				Depth:      1,
			},
			{
				TypeName:   "Query",
				FieldName:  "hero",
				Alias:      "jediHero",
				NodeCount:  1,
				Complexity: 1,
				Depth:      1,
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

func TestRequest_IsIntrospectionQuery(t *testing.T) {
	t.Run("named introspection query", func(t *testing.T) {
		var request Request
		err := UnmarshalRequest(strings.NewReader(namedIntrospectionQuery), &request)
		assert.NoError(t, err)

		isIntrospectionQuery, err := request.IsIntrospectionQuery()
		assert.NoError(t, err)
		assert.True(t, isIntrospectionQuery)
	})

	t.Run("silent introspection query", func(t *testing.T) {
		var request Request
		err := UnmarshalRequest(strings.NewReader(silentIntrospectionQuery), &request)
		assert.NoError(t, err)

		isIntrospectionQuery, err := request.IsIntrospectionQuery()
		assert.NoError(t, err)
		assert.True(t, isIntrospectionQuery)
	})

	t.Run("not introspection query", func(t *testing.T) {
		var request Request
		err := UnmarshalRequest(strings.NewReader(nonIntrospectionQuery), &request)
		assert.NoError(t, err)

		isIntrospectionQuery, err := request.IsIntrospectionQuery()
		assert.NoError(t, err)
		assert.False(t, isIntrospectionQuery)
	})

	t.Run("mutation query", func(t *testing.T) {
		var request Request
		err := UnmarshalRequest(strings.NewReader(mutationQuery), &request)
		assert.NoError(t, err)

		isIntrospectionQuery, err := request.IsIntrospectionQuery()
		assert.NoError(t, err)
		assert.False(t, isIntrospectionQuery)
	})
}

func TestRequest_OperationType(t *testing.T) {
	request := Request{
		OperationName: "",
		Variables:     nil,
		Query:         "query HelloQuery { hello: String } mutation HelloMutation { hello: String } subscription HelloSubscription { hello: String }",
	}

	t.Run("should return operation type 'Query'", func(t *testing.T) {
		request.OperationName = "HelloQuery"
		opType, err := request.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeQuery, opType)
	})

	t.Run("should return operation type 'Mutation'", func(t *testing.T) {
		request.OperationName = "HelloMutation"
		opType, err := request.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeMutation, opType)
	})

	t.Run("should return operation type 'Subscription'", func(t *testing.T) {
		request.OperationName = "HelloSubscription"
		opType, err := request.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeSubscription, opType)
	})

	t.Run("should return operation type 'Unknown' on error", func(t *testing.T) {
		emptyRequest := Request{
			Query: "Broken Query",
		}
		opType, err := emptyRequest.OperationType()
		assert.Error(t, err)
		assert.Equal(t, OperationTypeUnknown, opType)
	})

	t.Run("should return operation type 'Unknown' when empty and parsable", func(t *testing.T) {
		emptyRequest := Request{}
		opType, err := emptyRequest.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeUnknown, opType)
	})
}

const namedIntrospectionQuery = `{"operationName":"IntrospectionQuery","variables":{},"query":"query IntrospectionQuery {\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const silentIntrospectionQuery = `{"operationName":null,"variables":{},"query":"{\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const nonIntrospectionQuery = `{"operationName":"Foo","query":"query Foo {bar}"}`
const mutationQuery = `{"operationName":null,"query":"mutation Foo {bar}"}`
