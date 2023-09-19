package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestSchemaBuilder_BuildFederationSchema(t *testing.T) {
	actual, err := BuildFederationSchema(baseSchema, serviceSDL)
	assert.NoError(t, err)
	goldie.Assert(t, "federated_schema", []byte(actual))
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
