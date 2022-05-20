package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
)

func TestDocument_RemoveObjectTypeDefinition(t *testing.T) {
	schema := "type Query {queryName: String} type Mutation {mutationName: String} type Country {code: String} interface Model {id: String}"

	prepareDoc := func() *ast.Document {
		doc := unsafeparser.ParseGraphqlDocumentString(schema)
		return &doc
	}

	t.Run("doc remains same when", func(t *testing.T) {
		t.Run("try to remove not existing type", func(t *testing.T) {
			doc := prepareDoc()
			doc.RemoveObjectTypeDefinition([]byte("NotExisting"))
			docStr, _ := astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})

		t.Run("try to remove interface type", func(t *testing.T) {
			doc := prepareDoc()
			doc.RemoveObjectTypeDefinition([]byte("Model"))
			docStr, _ := astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})
	})

	t.Run("remove query type", func(t *testing.T) {
		doc := prepareDoc()
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "type Mutation {mutationName: String} type Country {code: String} interface Model {id: String}", docStr)
	})

	t.Run("remove query and mutations types", func(t *testing.T) {
		doc := prepareDoc()
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		doc.RemoveObjectTypeDefinition([]byte("Mutation"))

		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "type Country {code: String} interface Model {id: String}", docStr)
	})

	t.Run("remove all types", func(t *testing.T) {
		doc := prepareDoc()
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		doc.RemoveObjectTypeDefinition([]byte("Mutation"))
		doc.RemoveObjectTypeDefinition([]byte("Country"))

		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "interface Model {id: String}", docStr)
	})

}
