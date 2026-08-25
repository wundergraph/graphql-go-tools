package introspection_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestSource_Load(t *testing.T) {
	run := func(schema string, input string, fixtureName string) func(t *testing.T) {
		t.Helper()
		return func(t *testing.T) {
			def, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

			var data introspection.Data
			gen := introspection.NewGenerator()
			gen.Generate(&def, &report, &data)
			require.False(t, report.HasErrors())
			require.NoError(t, data.Schema.BuildJSON())

			source := &Source{introspectionData: &data}
			responseData, err := source.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			actualResponse := &bytes.Buffer{}
			require.NoError(t, json.Indent(actualResponse, responseData, "", "  "))
			// Trim the trailing newline that json.Indent adds
			responseBytes := actualResponse.Bytes()
			if len(responseBytes) > 0 && responseBytes[len(responseBytes)-1] == '\n' {
				responseBytes = responseBytes[:len(responseBytes)-1]
			}
			goldie.Assert(t, fixtureName, responseBytes)
		}
	}

	t.Run("schema introspection", run(testSchema, `{"request_type":1}`, `schema_introspection`))
	t.Run("schema introspection with custom root operation types", run(testSchemaWithCustomRootOperationTypes, `{"request_type":1}`, `schema_introspection_with_custom_root_operation_types`))
	t.Run("type introspection", run(testSchema, `{"request_type":2,"type_name":"Query"}`, `type_introspection`))
	t.Run("type introspection of not existing type", run(testSchema, `{"request_type":2,"type_name":"NotExisting"}`, `not_existing_type`))
}

const testSchema = `
schema {
    query: Query
}

type Query {
    me: Droid @deprecated
    droid(id: ID!): Droid
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI @deprecated
}

type Droid {
    name: String!
}
`

const testSchemaWithCustomRootOperationTypes = `
schema {
    query: CustomQuery
	mutation: CustomMutation
	subscription: CustomSubscription
}

type CustomQuery {
    me: Droid @deprecated
    droid(id: ID!): Droid
}

type CustomMutation {
	destroyDroid(id: ID!): Boolean!
}

type CustomSubscription {
	destroyedDroid: Droid
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI @deprecated
}

type Droid {
    name: String!
}
`

func TestSource_Load_OfTypeFields(t *testing.T) {
	// Regression test for https://github.com/wundergraph/cosmo/issues/991
	// Verifies that ofType nodes carry full type data (fields, description, etc.)
	// and that self-referencing types (User -> friends -> [User!]!) don't infinite loop.
	const schema = `
schema {
    query: Query
    mutation: Mutation
}

type Query {
    user: User
}

type Mutation {
    createUser(name: String!): User!
}

type User {
    id: ID!
    name: String!
    friends: [User!]!
}
`
	run := func(input string, fixtureName string) func(t *testing.T) {
		return func(t *testing.T) {
			def, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

			var data introspection.Data
			gen := introspection.NewGenerator()
			gen.Generate(&def, &report, &data)
			require.False(t, report.HasErrors())
			require.NoError(t, data.Schema.BuildJSON())

			source := &Source{introspectionData: &data}
			responseData, err := source.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			actualResponse := &bytes.Buffer{}
			require.NoError(t, json.Indent(actualResponse, responseData, "", "  "))
			responseBytes := actualResponse.Bytes()
			if len(responseBytes) > 0 && responseBytes[len(responseBytes)-1] == '\n' {
				responseBytes = responseBytes[:len(responseBytes)-1]
			}
			goldie.Assert(t, fixtureName, responseBytes)
		}
	}

	t.Run("ofType has fields for mutation return types", run(`{"request_type":2,"type_name":"Mutation"}`, `oftype_mutation`))
	t.Run("self-referencing type", run(`{"request_type":2,"type_name":"User"}`, `oftype_user`))
	t.Run("schema with enriched ofType", run(`{"request_type":1}`, `oftype_schema`))
}

func TestSource_Load_Comprehensive(t *testing.T) {
	// Comprehensive test exercising all enrichment paths:
	// - Interfaces (FullType.Interfaces []TypeRef)
	// - Interface possibleTypes (FullType.PossibleTypes []TypeRef)
	// - Union possibleTypes
	// - Input objects with nested input type refs
	// - Enum type refs (field returning an enum)
	// - Cross-type cycles (User→Review→User)
	// - Self-referencing cycles (User→friends→User)
	const schema = `
schema {
    query: Query
    mutation: Mutation
}

type Query {
    user: User
    search: [SearchResult]
    node: Node
    status: Status
    configure(input: ConfigInput!): String
}

type Mutation {
    createUser(name: String!): User!
}

type User implements Node {
    id: ID!
    name: String!
    friends: [User!]!
    reviews: [Review]
    status: Status
}

type Review {
    body: String!
    author: User!
}

interface Node {
    id: ID!
}

union SearchResult = User | Review

enum Status {
    ACTIVE
    INACTIVE @deprecated(reason: "Use ACTIVE")
}

input ConfigInput {
    key: String!
    nested: NestedInput
}

input NestedInput {
    value: Int!
}
`
	run := func(input string, fixtureName string) func(t *testing.T) {
		return func(t *testing.T) {
			def, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

			var data introspection.Data
			gen := introspection.NewGenerator()
			gen.Generate(&def, &report, &data)
			require.False(t, report.HasErrors())
			require.NoError(t, data.Schema.BuildJSON())

			source := &Source{introspectionData: &data}
			responseData, err := source.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			actualResponse := &bytes.Buffer{}
			require.NoError(t, json.Indent(actualResponse, responseData, "", "  "))
			responseBytes := actualResponse.Bytes()
			if len(responseBytes) > 0 && responseBytes[len(responseBytes)-1] == '\n' {
				responseBytes = responseBytes[:len(responseBytes)-1]
			}
			goldie.Assert(t, fixtureName, responseBytes)
		}
	}

	t.Run("user type with interfaces and cross-cycle", run(`{"request_type":2,"type_name":"User"}`, `comprehensive_user`))
	t.Run("interface with possibleTypes", run(`{"request_type":2,"type_name":"Node"}`, `comprehensive_node`))
	t.Run("union with possibleTypes", run(`{"request_type":2,"type_name":"SearchResult"}`, `comprehensive_search_result`))
	t.Run("input object with nested input ref", run(`{"request_type":2,"type_name":"ConfigInput"}`, `comprehensive_config_input`))
	t.Run("review type with back-ref cycle", run(`{"request_type":2,"type_name":"Review"}`, `comprehensive_review`))
	t.Run("full schema", run(`{"request_type":1}`, `comprehensive_schema`))
}
