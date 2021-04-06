package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

func runRequiredFieldExtractor(t *testing.T, SDL string, expected plan.FieldConfigurations) {
	document := unsafeparser.ParseGraphqlDocumentString(SDL)
	extractor := &federationSDLRequiredFieldExtractor{document: &document}
	got := extractor.getAllFieldRequires()
	assert.Equal(t, expected, got)
}

func runRootNodeExtractor(t *testing.T, SDL string, expected []plan.TypeField) {
	document := unsafeparser.ParseGraphqlDocumentString(SDL)
	extractor := NewRootNodeExtractor(&document)
	got := extractor.GetAllRootNodes()
	assert.Equal(t, expected, got)
}

func TestRootNodeExtractor_GetAllRootNodes(t *testing.T) {
	t.Run("non Entity object", func(t *testing.T) {
		runRootNodeExtractor(t, `
		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
		}
		`, nil)
	})
	t.Run("non Entity object extension", func(t *testing.T) {
		runRootNodeExtractor(t, `
		type Review {
			body: String!
		}

		extend type Review {
			title: String!
		}
		`, nil)
	})
	t.Run("Entity object", func(t *testing.T) {
		runRootNodeExtractor(t, `
		type Review @key(fields: "id"){
			id: Int!
			body: String!
			title: String
		}
		`, []plan.TypeField{{TypeName: "Review", FieldNames: []string{"id", "body", "title"}}})
	})
	t.Run("Entity object extension", func(t *testing.T) {
		runRootNodeExtractor(t, `
		extend type Review @key(fields: "id"){
			id: Int! @external
			body: String! @external
			title: String @requires(fields: "id body")
			author: String!
		}
		`, []plan.TypeField{{TypeName: "Review", FieldNames: []string{"title", "author"}}})
	})
	t.Run("Root operation types", func(t *testing.T) {
		runRootNodeExtractor(t, `
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
		`, []plan.TypeField{
			{ TypeName: "Query", FieldNames: []string{"me"}},
			{ TypeName: "Mutation", FieldNames: []string{"addUser", "deleteUser"}},
			{ TypeName: "Subscription", FieldNames: []string{"userChanges"}},
		})
	})
}

func TestRequiredFieldExtractor_GetAllFieldRequires(t *testing.T) {
	t.Run("non Entity object", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
		}
		`, nil)
	})
	t.Run("non Entity object extension", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		type Review {
			body: String!
		}

		extend type Review {
			title: String!
		}
		`, nil)
	})
	t.Run("Entity with simple primary key", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		type Review @key(fields: "id"){
			id: Int!
			body: String!
			title: String
		}
		`, plan.FieldConfigurations{
			{TypeName: "Review", FieldName: "body", RequiresFields: []string{"id"}},
			{TypeName: "Review", FieldName: "title", RequiresFields: []string{"id"}},
		})
	})
	t.Run("Entity with composed primary key", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		type Review @key(fields: "id author"){
			id: Int!
			body: String!
			title: String
			author: String!
		}
		`, plan.FieldConfigurations{
			{TypeName: "Review", FieldName: "body", RequiresFields: []string{"id", "author"}},
			{TypeName: "Review", FieldName: "title", RequiresFields: []string{"id", "author"}},
		})
	})
	t.Run("Entity object extension without non-primary external fields", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		extend type Review @key(fields: "id"){
			id: Int! @external
			author: String!
		}
		`, plan.FieldConfigurations{
			{TypeName: "Review", FieldName: "author", RequiresFields: []string{"id"}},
		})
	})
	t.Run("Entity object extension with \"requires\" directive", func(t *testing.T) {
		runRequiredFieldExtractor(t, `
		extend type Review @key(fields: "id"){
			id: Int! @external
			title: String! @external
			author: String! @external
			slug: String @requires(fields: "title author")
		}
		`, plan.FieldConfigurations{
			{TypeName: "Review", FieldName: "slug", RequiresFields: []string{"id", "title", "author"}},
		})
	})
}
