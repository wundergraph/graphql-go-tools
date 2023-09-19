package graphql

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
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

func TestRequest_parseQueryOnce(t *testing.T) {
	request := func() *Request {
		return &Request{
			OperationName: "Hello",
			Variables:     nil,
			Query:         "query Hello { hello }",
		}
	}

	t.Run("valid query", func(t *testing.T) {
		req := request()
		report := req.parseQueryOnce()
		assert.False(t, report.HasErrors())
		assert.True(t, req.isParsed)
	})

	t.Run("should not parse again", func(t *testing.T) {
		req := request()
		report := req.parseQueryOnce()
		assert.False(t, report.HasErrors())
		assert.True(t, req.isParsed)

		req.Query = "{"
		report = req.parseQueryOnce()
		assert.False(t, report.HasErrors())
	})

	t.Run("should not set is parsed for invalid query", func(t *testing.T) {
		req := request()
		req.Query = "{"
		report := req.parseQueryOnce()
		assert.True(t, report.HasErrors())
		assert.False(t, req.isParsed)
	})
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

func TestRequest_IsIntrospectionQuery(t *testing.T) {
	run := func(queryPayload string, expectedIsIntrospection bool) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var request Request
			err := UnmarshalRequest(strings.NewReader(queryPayload), &request)
			assert.NoError(t, err)

			actualIsIntrospection, err := request.IsIntrospectionQuery()
			assert.NoError(t, err)
			assert.Equal(t, expectedIsIntrospection, actualIsIntrospection)
		}
	}

	t.Run("schema introspection query", func(t *testing.T) {
		t.Run("with operation name IntrospectionQuery", run(namedIntrospectionQuery, true))
		t.Run("without operation name IntrospectionQuery but as single query", run(singleNamedIntrospectionQueryWithoutOperationName, true))
		t.Run("with empty operation name", run(silentIntrospectionQuery, true))
		t.Run("with operation name but as silent query", run(silentIntrospectionQueryWithOperationName, true))
		t.Run("with multiple queries in payload", run(schemaIntrospectionQueryWithMultipleQueries, true))
		t.Run("with inline fragment", run(inlineFragmentedIntrospectionQueryType, true))
		t.Run("with inline fragment on type query", run(inlineFragmentedIntrospectionQueryWithFragmentOnQuery, true))
		t.Run("with fragment", run(fragmentedIntrospectionQuery, true))
	})

	t.Run("type introspection query", func(t *testing.T) {
		t.Run("as single introspection", run(typeIntrospectionQuery, true))
		t.Run("with multiple queries in payload", run(typeIntrospectionQueryWithMultipleQueries, true))
	})

	t.Run("not introspection query", func(t *testing.T) {
		t.Run("query with operation name IntrospectionQuery", run(nonIntrospectionQueryWithIntrospectionQueryName, false))
		t.Run("Foo query", run(nonIntrospectionQuery, false))
		t.Run("Foo mutation", run(mutationQuery, false))
		t.Run("fake schema introspection with alias", run(nonSchemaIntrospectionQueryWithAliases, false))
		t.Run("fake type introspection with alias", run(nonTypeIntrospectionQueryWithAliases, false))
		t.Run("schema introspection query with additional non-introspection fields", run(nonSchemaIntrospectionQueryWithAdditionalFields, false))
		t.Run("type introspection query with additional non-introspection fields", run(nonTypeIntrospectionQueryWithAdditionalFields, false))
		t.Run("schema introspection with multiple queries in payload", run(nonSchemaIntrospectionQueryWithMultipleQueries, false))
		t.Run("type introspection with multiple queries in payload", run(nonTypeIntrospectionQueryWithMultipleQueries, false))
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

	t.Run("should return operation type 'Query' if no name and a single operation is provided", func(t *testing.T) {
		singleOperationQueryRequest := Request{
			OperationName: "",
			Variables:     nil,
			Query:         "{ hello: String }",
		}

		opType, err := singleOperationQueryRequest.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeQuery, opType)
	})

	t.Run("should return operation type 'Mutation' if mutation is the only operation", func(t *testing.T) {
		singleOperationMutationRequest := Request{
			OperationName: "",
			Variables:     nil,
			Query:         "mutation HelloMutation { hello: String }",
		}

		opType, err := singleOperationMutationRequest.OperationType()
		assert.NoError(t, err)
		assert.Equal(t, OperationTypeMutation, opType)
	})
}

const namedIntrospectionQuery = `{"operationName":"IntrospectionQuery","variables":{},"query":"query IntrospectionQuery {\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const singleNamedIntrospectionQueryWithoutOperationName = `{"operationName":"","variables":{},"query":"query IntrospectionQuery {\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const silentIntrospectionQuery = `{"operationName":null,"variables":{},"query":"{\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const silentIntrospectionQueryWithOperationName = `{"operationName":"IntrospectionQuery","variables":{},"query":"{\n  __schema {\n    queryType {\n      name\n    }\n    mutationType {\n      name\n    }\n    subscriptionType {\n      name\n    }\n    types {\n      ...FullType\n    }\n    directives {\n      name\n      description\n      locations\n      args {\n        ...InputValue\n      }\n    }\n  }\n}\n\nfragment FullType on __Type {\n  kind\n  name\n  description\n  fields(includeDeprecated: true) {\n    name\n    description\n    args {\n      ...InputValue\n    }\n    type {\n      ...TypeRef\n    }\n    isDeprecated\n    deprecationReason\n  }\n  inputFields {\n    ...InputValue\n  }\n  interfaces {\n    ...TypeRef\n  }\n  enumValues(includeDeprecated: true) {\n    name\n    description\n    isDeprecated\n    deprecationReason\n  }\n  possibleTypes {\n    ...TypeRef\n  }\n}\n\nfragment InputValue on __InputValue {\n  name\n  description\n  type {\n    ...TypeRef\n  }\n  defaultValue\n}\n\nfragment TypeRef on __Type {\n  kind\n  name\n  ofType {\n    kind\n    name\n    ofType {\n      kind\n      name\n      ofType {\n        kind\n        name\n        ofType {\n          kind\n          name\n          ofType {\n            kind\n            name\n            ofType {\n              kind\n              name\n              ofType {\n                kind\n                name\n              }\n            }\n          }\n        }\n      }\n    }\n  }\n}\n"}`
const schemaIntrospectionQueryWithMultipleQueries = `{"operationName":"IntrospectionQuery","query":"query Hello { world } query IntrospectionQuery { __schema { types { name } } }"}`
const inlineFragmentedIntrospectionQueryType = `{"operationName":"IntrospectionQuery","variables":{},"query":"query IntrospectionQuery { ... IntrospectionFragment } fragment IntrospectionFragment on Query { __schema { queryType { name } mutationType { name } subscriptionType { name } types { ...FullType } directives { name description args { ...InputValue } onOperation onFragment onField } } } fragment FullType on __Type { kind name description fields(includeDeprecated: true) { name description args { ...InputValue } type { ...TypeRef } isDeprecated deprecationReason } inputFields { ...InputValue } interfaces { ...TypeRef } enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason } possibleTypes { ...TypeRef } } fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue } fragment TypeRef on __Type { kind name ofType { kind name ofType { kind name ofType { kind name } } } }"}`
const inlineFragmentedIntrospectionQueryWithFragmentOnQuery = `{"operationName":"IntrospectionQuery","variables":{},"query":"query IntrospectionQuery { ... on Query { __schema { queryType { name } mutationType { name } subscriptionType { name } types { ...FullType } directives { name description args { ...InputValue } onOperation onFragment onField } } } } fragment FullType on __Type { kind name description fields(includeDeprecated: true) { name description args { ...InputValue } type { ...TypeRef } isDeprecated deprecationReason } inputFields { ...InputValue } interfaces { ...TypeRef } enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason } possibleTypes { ...TypeRef } } fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue } fragment TypeRef on __Type { kind name ofType { kind name ofType { kind name ofType { kind name } } } }"}`
const fragmentedIntrospectionQuery = `{"operationName":"IntrospectionQuery","variables":{},"query":"query IntrospectionQuery { ... IntrospectionFragment } fragment IntrospectionFragment on Query { __schema { queryType { name } mutationType { name } subscriptionType { name } types { ...FullType } directives { name description args { ...InputValue } onOperation onFragment onField } } } fragment FullType on __Type { kind name description fields(includeDeprecated: true) { name description args { ...InputValue } type { ...TypeRef } isDeprecated deprecationReason } inputFields { ...InputValue } interfaces { ...TypeRef } enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason } possibleTypes { ...TypeRef } } fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue } fragment TypeRef on __Type { kind name ofType { kind name ofType { kind name ofType { kind name } } } }"}`
const typeIntrospectionQueryWithMultipleQueries = `{"operationName":"IntrospectionQuery","query":"query Hello { world } query IntrospectionQuery { __type(name: \"Droid\") { name } }"}`
const typeIntrospectionQuery = `{"operationName":null,"variables":{},"query":"{__type(name:\"Foo\"){kind}}"}`
const nonIntrospectionQuery = `{"operationName":"Foo","query":"query Foo {bar}"}`
const nonIntrospectionQueryWithIntrospectionQueryName = `{"operationName":"IntrospectionQuery","query":"query IntrospectionQuery {bar}"}`
const nonSchemaIntrospectionQueryWithAliases = `{"operationName":"IntrospectionQuery","query":"query IntrospectionQuery { __schema: user { name types: account { balance } } }"}`
const nonTypeIntrospectionQueryWithAliases = `{"operationName":"IntrospectionQuery","query":"query IntrospectionQuery { __type: user { name } }"}`
const nonSchemaIntrospectionQueryWithAdditionalFields = `{"operationName":"IntrospectionQuery","query":"query IntrospectionQuery { __schema { types { name } } user { name account { balance } } }"}`
const nonTypeIntrospectionQueryWithAdditionalFields = `{"operationName":"IntrospectionQuery","query":"query IntrospectionQuery { __type(name: \"Droid\") { name } user { name account { balance } } }"}`
const nonSchemaIntrospectionQueryWithMultipleQueries = `{"operationName":"Hello","query":"query Hello { world } query IntrospectionQuery { __schema { types { name } } }"}`
const nonTypeIntrospectionQueryWithMultipleQueries = `{"operationName":"Hello","query":"query Hello { world } query IntrospectionQuery { __type(name: \"Droid\") { name } }"}`

const mutationQuery = `{"operationName":null,"query":"mutation Foo {bar}"}`

const testSubscriptionDefinition = `
type Subscription {
	lastRegisteredUser: User
	liveUserCount: Int!
}

type User {
	id: ID!
	username: String!
	email: String!
}
`

const testSubscriptionLastRegisteredUserOperation = `
subscription LastRegisteredUser {
	lastRegisteredUser {
		id
		username
		email
	}
}
`

const testSubscriptionLiveUserCountOperation = `
subscription LiveUserCount {
	liveUserCount
}
`
