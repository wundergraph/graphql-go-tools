package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

func TestNodeExtractor_GetAllNodes(t *testing.T) {
	run := func(t *testing.T, SDL string, expectedRoot, expectedChild []plan.TypeField) {
		document := unsafeparser.ParseGraphqlDocumentString(SDL)
		extractor := newNodeExtractor(&document)
		gotRoot, gotChild := extractor.getAllNodes()

		assert.Equal(t, expectedRoot, gotRoot, "root nodes dont match")
		assert.Equal(t, expectedChild, gotChild, "child nodes dont match")
	}

	t.Run("only root operation", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}
	
			extend type Mutation {
				addUser(id: ID!): User
				deleteUser(id: ID!): User
			}
	
			extend type Subscription {
				userChanges(id: ID!): User!
			}
	
			type User {
				id: ID!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "Mutation", FieldNames: []string{"addUser", "deleteUser"}},
				{TypeName: "Subscription", FieldNames: []string{"userChanges"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id"}},
			})
	})
	t.Run("nested child nodes", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				review(id: ID!): Review
				user(id: ID!): User
			}
	
			type User {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me", "review", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
				{TypeName: "Review", FieldNames: []string{"id", "comment", "rating", "user"}},
			})
	})
	t.Run("Entity definition", func(t *testing.T) {
		run(t, `
			type User @key(fields: "id") {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
			}
		`,
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"id", "comment", "rating"}},
			})
	})
	t.Run("nested Entity definition", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}
	
			type User @key(fields: "id") {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
				{TypeName: "Review", FieldNames: []string{"id", "comment", "rating"}},
			})
	})
	t.Run("extended Entity", func(t *testing.T) {
		run(t, `
			extend type User @key(fields: "id") {
				id: ID! @external
				username: String! @external
				reviews: [Review!]
			}

			type Review {
				comment: String!
				author: User! @provide(fields: "username")
			}
		`,
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"comment", "author"}},
				{TypeName: "User", FieldNames: []string{"id", "username", "reviews"}},
			})
	})
	t.Run("extended Entity with root definitions", func(t *testing.T) {
		run(t, `
			extend type Query {
				reviews(IDs: [ID!]!): [Review!]
			}

			extend type User @key(fields: "id") {
				id: ID! @external
				reviews: [Review!]
			}

			type Review {
				id: String!
				comment: String!
				author: User!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"reviews"}},
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"id", "comment", "author"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})

}
