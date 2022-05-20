package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
)

func TestRequiredFieldExtractor_GetAllFieldRequires(t *testing.T) {
	run := func(t *testing.T, SDL string, expected FieldConfigurations) {
		document := unsafeparser.ParseGraphqlDocumentString(SDL)
		extractor := &RequiredFieldExtractor{document: &document}
		got := extractor.GetAllRequiredFields()
		assert.Equal(t, expected, got)
	}

	t.Run("non Entity object", func(t *testing.T) {
		run(t, `
		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
		}
		`, nil)
	})
	t.Run("non Entity object extension", func(t *testing.T) {
		run(t, `
		type Review {
			body: String!
		}

		extend type Review {
			title: String!
		}
		`, nil)
	})
	t.Run("Entity with simple primary key", func(t *testing.T) {
		run(t, `
		type Review @key(fields: "id"){
			id: Int!
			body: String!
			title: String
		}
		`, FieldConfigurations{
			{TypeName: "Review", FieldName: "body", RequiresFields: []string{"id"}},
			{TypeName: "Review", FieldName: "title", RequiresFields: []string{"id"}},
		})
	})
	t.Run("Entity with composed primary key", func(t *testing.T) {
		run(t, `
		type Review @key(fields: "id author"){
			id: Int!
			body: String!
			title: String
			author: String!
		}
		`, FieldConfigurations{
			{TypeName: "Review", FieldName: "body", RequiresFields: []string{"id", "author"}},
			{TypeName: "Review", FieldName: "title", RequiresFields: []string{"id", "author"}},
		})
	})
	t.Run("Entity object extension without non-primary external fields", func(t *testing.T) {
		run(t, `
		extend type Review @key(fields: "id"){
			id: Int! @external
			author: String!
		}
		`, FieldConfigurations{
			{TypeName: "Review", FieldName: "author", RequiresFields: []string{"id"}},
		})
	})
	t.Run("Entity object extension with \"requires\" directive", func(t *testing.T) {
		run(t, `
		extend type Review @key(fields: "id"){
			id: Int! @external
			title: String! @external
			author: String! @external
			slug: String @requires(fields: "title author")
		}
		`, FieldConfigurations{
			{TypeName: "Review", FieldName: "slug", RequiresFields: []string{"id", "title", "author"}},
		})
	})
}
