package ast_test

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/assert"
)

func TestDocument_ReplaceRootOperationTypeDefinition(t *testing.T) {
	prepareDoc := func(t *testing.T) *ast.Document {
		doc := ast.NewDocument()
		typeRef := doc.AddNamedType([]byte("String"))

		queryFieldRef := doc.ImportFieldDefinition("queryName", "", typeRef, nil, nil)
		doc.ImportObjectTypeDefinition("Query", "", []int{queryFieldRef}, nil)
		doc.ImportSchemaDefinition("Query", "", "")

		codeFieldRef := doc.ImportFieldDefinition("code", "", typeRef, nil, nil)
		doc.ImportObjectTypeDefinition("Country", "", []int{codeFieldRef}, nil)

		schema := "schema {query: Query} type Query {queryName: String} type Country {code: String}"
		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, schema, docStr)
		return doc
	}

	t.Run("replace query type with existing type", func(t *testing.T) {
		doc := prepareDoc(t)
		doc.ReplaceRootOperationTypeDefinition("Country", ast.OperationTypeQuery)
		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "schema {query: Country} type Query {queryName: String} type Country {code: String}", docStr)
	})

	t.Run("replace query type with not existing type", func(t *testing.T) {
		doc := prepareDoc(t)
		doc.ReplaceRootOperationTypeDefinition("NotExisting", ast.OperationTypeQuery)
		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "schema {query: NotExisting} type Query {queryName: String} type Country {code: String}", docStr)
	})

	t.Run("replace mutation type when it is not present", func(t *testing.T) {
		doc := prepareDoc(t)
		doc.ReplaceRootOperationTypeDefinition("Country", ast.OperationTypeMutation)
		docStr, _ := astprinter.PrintString(doc, nil)
		assert.Equal(t, "schema {query: Query} type Query {queryName: String} type Country {code: String}", docStr)
	})
}
