package astnormalization

import (
	"fmt"
	"testing"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/require"
)

const inputCoercionForListDefinition = `
schema {
	query: Query
}

type Character {
	id: Int
	name: String
}

type Query {
	nestedList(ids: [[Int]]): [Character]
	charactersByIds(ids: [Int]): [Character]

}`

func TestInputCoercion(t *testing.T) {
	runWithReport := func(t *testing.T, normalizeFunc registerNormalizeFunc, definition, operation string) *operationreport.Report {
		definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
		err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
		if err != nil {
			panic(err)
		}

		operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
		report := operationreport.Report{}
		walker := astvisitor.NewWalker(48)

		normalizeFunc(&walker)

		walker.Walk(&operationDocument, &definitionDocument, &report)
		return &report
	}

	t.Run("Incorrect item value", func(t *testing.T) {
		report := runWithReport(t, inputCoercionForList, inputCoercionForListDefinition, `
				query{
					charactersByIds(ids: "foobar") {
    					id
    					name
  					}
				}`)
		require.True(t, report.HasErrors())
		require.Equal(t, fmt.Sprintf("internal: %s", errIncorrectItemValue), report.Error())
	})

	t.Run("Convert to list", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					charactersByIds(ids: 1) {
    					id
    					name
  					}
				}`,
			`
				query{
					charactersByIds(ids: [1]) {
						id
						name
					}
				}`)
	})

	t.Run("List of integers", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					charactersByIds(ids: [1, 2, 3]) {
    					id
    					name
  					}
				}`,
			`
				query{
					charactersByIds(ids: [1, 2, 3]) {
						id
						name
					}
				}`)
	})

	t.Run("Incorrect item value", func(t *testing.T) {
		report := runWithReport(t, inputCoercionForList, inputCoercionForListDefinition, `
				query{
					charactersByIds(ids: [1, "b", true]) {
    					id
    					name
  					}
				}`,
		)
		require.True(t, report.HasErrors())
		require.Equal(t, fmt.Sprintf("internal: %s", errIncorrectItemValue), report.Error())
	})

	t.Run("Nested list", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					nestedList(ids: [[1], [2, 3]]) {
    					id
    					name
  					}
				}`,
			`
				query{
					nestedList(ids: [[1], [2, 3]]) {
						id
						name
					}
				}`)
	})

	t.Run("Nested list, but incorrect item value", func(t *testing.T) {
		report := runWithReport(t, inputCoercionForList, inputCoercionForListDefinition, `
				query{
					nestedList(ids: [1, 2, 3]) {
    					id
    					name
  					}
				}`)
		require.True(t, report.HasErrors())
		require.Equal(t, fmt.Sprintf("internal: %s", errIncorrectItemValue), report.Error())
	})

	t.Run("null value", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					charactersByIds(ids: null) {
    					id
    					name
  					}
				}`,
			`
				query{
					charactersByIds(ids: null) {
						id
						name
					}
				}`)
	})

	t.Run("nested list, but null value", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					nestedList(ids: null) {
    					id
    					name
  					}
				}`,
			`
				query{
					nestedList(ids: null) {
						id
						name
					}
				}`)
	})

	t.Run("nested list, but integer value", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
				query{
					nestedList(ids: 1) {
    					id
    					name
  					}
				}`,
			`
				query{
					nestedList(ids: [[1]]) {
						id
						name
					}
				}`)
	})

	t.Run("nested list, but list value", func(t *testing.T) {
		report := runWithReport(t, inputCoercionForList, inputCoercionForListDefinition, `
				query{
					nestedList(ids: [1]) {
    					id
    					name
  					}
				}`)
		require.True(t, report.HasErrors())
		require.Equal(t, fmt.Sprintf("internal: %s", errIncorrectItemValue), report.Error())
	})
}
