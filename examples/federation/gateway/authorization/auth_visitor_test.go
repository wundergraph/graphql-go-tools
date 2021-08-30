package authorization

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthVisitor(t *testing.T) {
	run := func(t *testing.T, definition, operation string, expectedRoles []string) {
		definitionDocument, report := astparser.ParseGraphqlDocumentString(definition)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		operationDocument, report := astparser.ParseGraphqlDocumentString(operation)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		requiredRoles, err := GetRoles(&operationDocument, &definitionDocument)
		require.NoError(t, err)

		assert.Equal(t, expectedRoles, requiredRoles)
	}

	t.Run("authorization directive with Query", func(t *testing.T) {
		run(
			t,
			testSchema,
			`
				query GetMe {
					me {
						name
					}
				}
			`,
			[]string{"USER"},
		)
	})

	t.Run("nested authorization directives", func(t *testing.T) {
		run(
			t,
			testSchema,
			`
				query GetMe {
					me {
						reviews {
							product {
								upc
							}
						}
					}
				}
			`,
			[]string{"USER", "ROLE_2", "ROLE_1"},
		)
	})

	t.Run("authorization directive for alias", func(t *testing.T) {
		run(
			t,
			testSchema,
			`
				query GetMe {
					me {
						someAnotherName: reviews {
							id
						}
					}
				}
			`,
			[]string{"USER", "ROLE_2"},
		)
	})
}

const testSchema = `
directive @hasRole(role: Role!) on FIELD_DEFINITION | OBJECT

enum Role {
    ADMIN
    USER
	ROLE_1
	ROLE_2
}

scalar ID
scalar String
scalar Boolean

schema {
	query: Query
	mutation: Mutation
	subscription: Subscription
}

type Query {
	me: User @hasRole(role: USER)
	topProducts(first: Int = 5): [Product]
}
		
type Mutation {
	setPrice(upc: String!, price: Int!): Product @hasRole(role: ADMIN)
} 

type Subscription {
	updatedPrice: Product! @hasRole(role: ROLE_1)
}
		
type User {
	id: ID!
	name: String
	username: String
	reviews: [Review] @hasRole(role: ROLE_2)
}

type Product {
	upc: String!
	name: String
	price: Int
	weight: Int
	reviews: [Review]
}

type Review {
	id: ID!
	body: String
	author: User
	product: Product @hasRole(role: ROLE_1)
}
`
