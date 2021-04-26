package ast_test

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/assert"
)

func TestDocument_RemoveObjectTypeDefinition(t *testing.T) {
	doc := ast.NewDocument()
	typeRef := doc.AddNamedType([]byte("String"))

	queryFieldRef := doc.ImportFieldDefinition("queryName", "", typeRef, nil, nil)
	doc.ImportObjectTypeDefinition("Query", "", []int{queryFieldRef}, nil)

	mutationFieldRef := doc.ImportFieldDefinition("mutationName", "", typeRef, nil, nil)
	doc.ImportObjectTypeDefinition("Mutation", "", []int{mutationFieldRef}, nil)

	codeFieldRef := doc.ImportFieldDefinition("code", "", typeRef, nil, nil)
	doc.ImportObjectTypeDefinition("Country", "", []int{codeFieldRef}, nil)

	idFieldRef := doc.ImportFieldDefinition("id", "", typeRef, nil, nil)
	doc.ImportInterfaceTypeDefinition("Model", "", []int{idFieldRef})

	schema := "type Query {queryName: String} type Mutation {mutationName: String} type Country {code: String} interface Model {id: String}"
	docStr, _ := astprinter.PrintString(doc, nil)
	assert.Equal(t, schema, docStr)

	t.Run("doc remains same when", func(t *testing.T) {
		t.Run("try to remove not existing type", func(t *testing.T) {
			doc.RemoveObjectTypeDefinition([]byte("NotExisting"))
			docStr, _ = astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})

		t.Run("try to remove interface type", func(t *testing.T) {
			doc.RemoveObjectTypeDefinition([]byte("Model"))
			docStr, _ = astprinter.PrintString(doc, nil)
			assert.Equal(t, schema, docStr)
		})
	})

	t.Run("remove query type", func(t *testing.T) {
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		docStr, _ = astprinter.PrintString(doc, nil)
		assert.Equal(t, "type Mutation {mutationName: String} type Country {code: String} interface Model {id: String}", docStr)
	})

	t.Run("remove query and mutations types", func(t *testing.T) {
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		doc.RemoveObjectTypeDefinition([]byte("Mutation"))

		docStr, _ = astprinter.PrintString(doc, nil)
		assert.Equal(t, "type Country {code: String} interface Model {id: String}", docStr)
	})

	t.Run("remove all types", func(t *testing.T) {
		doc.RemoveObjectTypeDefinition([]byte("Query"))
		doc.RemoveObjectTypeDefinition([]byte("Mutation"))
		doc.RemoveObjectTypeDefinition([]byte("Country"))

		docStr, _ = astprinter.PrintString(doc, nil)
		assert.Equal(t, "interface Model {id: String}", docStr)
	})

}
