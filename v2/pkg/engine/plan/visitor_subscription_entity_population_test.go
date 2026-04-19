package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestVisitorEntityKeyFieldNames(t *testing.T) {
	t.Run("extracts only top level key fields", func(t *testing.T) {
		keys := []FederationFieldConfiguration{
			{
				TypeName:     "User",
				SelectionSet: "id info {a b}",
			},
			{
				TypeName:     "User",
				SelectionSet: "profile {displayName}",
			},
		}

		for i := range keys {
			err := keys[i].parseSelectionSet()
			require.NoError(t, err)
		}

		fieldNames := (&Visitor{}).entityKeyFieldNames(keys)

		assert.Equal(t, map[string]struct{}{
			"id":      {},
			"info":    {},
			"profile": {},
		}, fieldNames)
	})

	t.Run("skips invalid and empty parsed keys", func(t *testing.T) {
		unnamedFieldDoc := ast.NewDocument()
		selectionSetRef := unnamedFieldDoc.AddSelectionSet().Ref
		fieldRef := unnamedFieldDoc.AddField(ast.Field{}).Ref
		unnamedFieldDoc.AddSelection(selectionSetRef, ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  fieldRef,
		})
		unnamedFieldDoc.FragmentDefinitions = append(unnamedFieldDoc.FragmentDefinitions, ast.FragmentDefinition{
			SelectionSet: selectionSetRef,
		})

		fieldNames := (&Visitor{}).entityKeyFieldNames([]FederationFieldConfiguration{
			{
				TypeName:     "User",
				SelectionSet: "{",
			},
			{
				TypeName:          "User",
				SelectionSet:      "id",
				parsedSelectionSet: &ast.Document{},
			},
			{
				TypeName:          "User",
				SelectionSet:      "id",
				parsedSelectionSet: unnamedFieldDoc,
			},
			{
				TypeName:     "User",
				SelectionSet: "name",
			},
		})

		assert.Equal(t, map[string]struct{}{
			"name": {},
		}, fieldNames)
	})
}
