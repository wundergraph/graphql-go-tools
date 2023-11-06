package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

func TestExtractor_ExtractFieldsFromRequest(t *testing.T) {
	schema, err := NewSchemaFromString(testDefinition)
	require.NoError(t, err)

	request := Request{
		OperationName: "PostsUserQuery",
		Variables:     nil,
		Query:         testOperation,
	}

	t.Run("no operation limit", func(t *testing.T) {
		fields := make(RequestTypes)
		report := operationreport.Report{}
		NewExtractor().ExtractFieldsFromRequest(&request, schema, &report, fields)

		expectedFields := RequestTypes{
			"Foo":   {"fooField": {}},
			"Post":  {"description": {}, "id": {}, "user": {}},
			"Query": {"foo": {}, "posts": {}},
			"User":  {"id": {}, "name": {}},
		}

		assert.False(t, report.HasErrors())
		assert.Equal(t, expectedFields, fields)
	})

	t.Run("with operation limit", func(t *testing.T) {
		fields := make(RequestTypes)
		report := operationreport.Report{}

		NewExtractor().ExtractFieldsFromRequestSingleOperation(&request, schema, &report, fields)
		expectedFields := RequestTypes{
			"Post":  {"description": {}, "id": {}, "user": {}},
			"Query": {"posts": {}},
			"User":  {"id": {}, "name": {}},
		}
		assert.Equal(t, expectedFields, fields)
	})

	t.Run("with operation limit no operation", func(t *testing.T) {
		fields := make(RequestTypes)
		report := operationreport.Report{}
		request.OperationName = ""

		NewExtractor().ExtractFieldsFromRequestSingleOperation(&request, schema, &report, fields)
		expectedFields := RequestTypes{
			"Foo":   {"fooField": {}},
			"Query": {"foo": {}},
		}
		assert.Equal(t, expectedFields, fields)
	})

	t.Run("with operation limit no operation custom query", func(t *testing.T) {
		schema, err := NewSchemaFromString(testDefinitionCustomQuery)
		require.NoError(t, err)
		fields := make(RequestTypes)
		report := operationreport.Report{}
		request.OperationName = ""

		NewExtractor().ExtractFieldsFromRequestSingleOperation(&request, schema, &report, fields)
		expectedFields := RequestTypes{
			"Foo":         {"fooField": {}},
			"CustomQuery": {"foo": {}},
		}
		assert.Equal(t, expectedFields, fields)
	})
}

const testOperation = `query ArgsQuery {
  foo(bar: "barValue", baz: true) {
    fooField
  }
}
query PostsUserQuery {
  posts {
    id
    description
    user {
      id
      name
    }
  }
}
fragment FirstFragment on Post {
  id
}

query VariableQuery($bar: String, $baz: Boolean) {
  foo(bar: $bar, baz: $baz) {
    fooField
  }
}
query VariableQuery {
  posts {
    id @include(if: true)
  }
}

`

const testDefinition = `
directive @include(if: Boolean!) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
schema {
	query: Query
}
type Query {
	posts: [Post]
	foo(bar: String!, baz: Boolean!): Foo
}
type User {
	id: ID
	name: String
	posts: [Post]
}
type Post {
	id: ID
	description: String
	user: User
}
type Foo {
	fooField: String
}
scalar ID
scalar String
`

const testDefinitionCustomQuery = `
directive @include(if: Boolean!) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
schema {
	query: CustomQuery
}
type CustomQuery {
	posts: [Post]
	foo(bar: String!, baz: Boolean!): Foo
}
type User {
	id: ID
	name: String
	posts: [Post]
}
type Post {
	id: ID
	description: String
	user: User
}
type Foo {
	fooField: String
}
scalar ID
scalar String`
