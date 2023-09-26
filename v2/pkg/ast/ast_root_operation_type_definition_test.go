package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func TestDocument_ReplaceRootOperationTypeDefinition(t *testing.T) {
	schema := "schema {query: Query} type Query {queryName: String} type Country {code: String} interface Model {id: String}"

	prepareDoc := func() *ast.Document {
		doc := unsafeparser.ParseGraphqlDocumentString(schema)
		return &doc
	}

	t.Run("should replace query type with existing type", func(t *testing.T) {
		doc := prepareDoc()
		ref, ok := doc.ReplaceRootOperationTypeDefinition("Country", ast.OperationTypeQuery)
		assert.NotEqual(t, -1, ref)
		assert.True(t, ok)

		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "schema {query: Country} type Query {queryName: String} type Country {code: String} interface Model {id: String}", docStr)
	})

	t.Run("should not modify document", func(t *testing.T) {
		t.Run("when replacing query type with not existing type", func(t *testing.T) {
			doc := prepareDoc()
			ref, ok := doc.ReplaceRootOperationTypeDefinition("NotExisting", ast.OperationTypeQuery)
			assert.Equal(t, -1, ref)
			assert.False(t, ok)

			docStr, _ := astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})

		t.Run("when replacing query type with not an object type", func(t *testing.T) {
			doc := prepareDoc()
			ref, ok := doc.ReplaceRootOperationTypeDefinition("Model", ast.OperationTypeQuery)
			assert.Equal(t, -1, ref)
			assert.False(t, ok)

			docStr, _ := astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})

		t.Run("when replacing mutation which was not defined", func(t *testing.T) {
			doc := prepareDoc()
			ref, ok := doc.ReplaceRootOperationTypeDefinition("Country", ast.OperationTypeMutation)
			assert.Equal(t, -1, ref)
			assert.False(t, ok)

			docStr, _ := astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})
	})
}
