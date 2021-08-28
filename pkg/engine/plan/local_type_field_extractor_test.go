package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
)

func TestLocalTypeFieldExtractor_GetAllNodes(t *testing.T) {
	run := func(t *testing.T, SDL string, expectedRoot, expectedChild []TypeField) {
		document := unsafeparser.ParseGraphqlDocumentString(SDL)
		extractor := NewLocalTypeFieldExtractor(&document)
		gotRoot, gotChild := extractor.GetAllNodes()

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
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "Mutation", FieldNames: []string{"addUser", "deleteUser"}},
				{TypeName: "Subscription", FieldNames: []string{"userChanges"}},
			},
			[]TypeField{
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
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"me", "review", "user"}},
			},
			[]TypeField{
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
			[]TypeField{
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]TypeField{
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
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]TypeField{
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
			[]TypeField{
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]TypeField{
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
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"reviews"}},
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]TypeField{
				{TypeName: "Review", FieldNames: []string{"id", "comment", "author"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("union types", func(t *testing.T) {
		run(t, `
			extend type Query {
				search(name: String!): SearchResult
			}

			union SearchResult = Human | Droid | Starship
	
			interface Character {
				name: String!
				friends: [Character]
			}
			
			type Human implements Character {
				name: String!
				height: String!
				friends: [Character]
			}
			
			type Droid implements Character {
				name: String!
				primaryFunction: String!
				friends: [Character]
			}
			
			type Starship {
				name: String!
				length: Float!
			}
		`,
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
			},
			[]TypeField{
				{TypeName: "Human", FieldNames: []string{"name", "height", "friends"}},
				{TypeName: "Droid", FieldNames: []string{"name", "primaryFunction", "friends"}},
				{TypeName: "Character", FieldNames: []string{"name", "friends"}},
				{TypeName: "Starship", FieldNames: []string{"name", "length"}},
			})
	})
	t.Run("interface types", func(t *testing.T) {
		run(t, `
			extend type Query {
				search(name: String!): Character
			}
	
			interface Character {
				name: String!
				friends: [Character]
			}
			
			type Human implements Character {
				name: String!
				height: String!
				friends: [Character]
			}
			
			type Droid implements Character {
				name: String!
				primaryFunction: String!
				friends: [Character]
			}
		`,
			[]TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
			},
			[]TypeField{
				{TypeName: "Human", FieldNames: []string{"name", "height", "friends"}},
				{TypeName: "Droid", FieldNames: []string{"name", "primaryFunction", "friends"}},
				{TypeName: "Character", FieldNames: []string{"name", "friends"}},
			})
	})

}
