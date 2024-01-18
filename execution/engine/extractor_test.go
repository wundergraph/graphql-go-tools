package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestExtractor_ExtractFieldsFromRequest(t *testing.T) {
	schema, err := graphql.NewSchemaFromString(testDefinition)
	require.NoError(t, err)

	request := graphql.Request{
		OperationName: "PostsUserQuery",
		Variables:     nil,
		Query:         testOperation,
	}

	fields := make(graphql.RequestTypes)
	report := operationreport.Report{}
	graphql.NewExtractor().ExtractFieldsFromRequest(&request, schema, &report, fields)

	expectedFields := graphql.RequestTypes{
		"Foo":   {"fooField": {}},
		"Post":  {"description": {}, "id": {}, "user": {}},
		"Query": {"foo": {}, "posts": {}},
		"User":  {"id": {}, "name": {}},
	}

	assert.False(t, report.HasErrors())
	assert.Equal(t, expectedFields, fields)
}

const testOperation = `query PostsUserQuery {
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
query ArgsQuery {
	foo(bar: "barValue", baz: true){
		fooField
	}
}
query VariableQuery($bar: String, $baz: Boolean) {
	foo(bar: $bar, baz: $baz){
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
