package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestSchemaBuilder_BuildFederationSchema(t *testing.T) {
	actual, err := BuildFederationSchema(baseSchema, serviceSDL)
	assert.NoError(t, err)
	goldie.Assert(t, "federated_schema", []byte(actual))
}

func TestSchemaBuilder_BuildFederationSchemaWithSchemaExtension(t *testing.T) {
	run := func(t *testing.T, sdl string) {
		t.Helper()

		actual, err := BuildFederationSchema(sdl, sdl)
		require.NoError(t, err)

		doc, report := astparser.ParseGraphqlDocumentString(actual)
		require.False(t, report.HasErrors(), report.Error())

		require.Len(t, doc.SchemaExtensions, 0)
		schemaDefinitionRef := doc.SchemaDefinitionRef()
		require.NotEqual(t, -1, schemaDefinitionRef)
		require.Len(t, doc.SchemaDefinitions[schemaDefinitionRef].Directives.Refs, 1)
		assert.Equal(t, "a", doc.DirectiveNameString(doc.SchemaDefinitions[schemaDefinitionRef].Directives.Refs[0]))
		assert.Equal(t, "Query", doc.Index.QueryTypeName.String())
	}

	t.Run("without root operation types", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA

			extend schema @a

			type Entity @key(fields: "id") {
				id: ID!
				name: String!
			}`)
	})

	t.Run("with root operation types", func(t *testing.T) {
		run(t, `
			directive @a on SCHEMA

			extend schema @a {
				query: Query
			}

			type Query {
				entity(id: ID!): Entity
			}

			type Entity @key(fields: "id") {
				id: ID!
				name: String!
			}`)
	})
}

const serviceSDL = `extend type Query {topProducts(first: Int = 5): [Product]}type Product @key(fields: "upc") {upc: String!name: String! price: Int!} extend type Query {me: User} type User @key(fields: "id"){ id: ID! username: String!} type Review { body: String! author: User! @provides(fields: "username") product: Product! } extend type User @key(fields: "id") { id: ID! @external reviews: [Review] } extend type Product @key(fields: "upc") { upc: String! @external reviews: [Review] }`

const baseSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Query {
  me: User
  topProducts(first: Int = 5): [Product]
}

type Product {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review]
}

type Review {
  body: String!
  author: User!
  product: Product!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`
