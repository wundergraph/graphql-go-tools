package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

// runTestFillDefaults parses the given SDL (merging the base schema so built-in scalars
// resolve) and runs fillListSizeDefaults against the supplied map.
// The map is mutated in place; the test owns it and asserts directly on its entries.
//
// Pass sdl == "" to exercise the nil-schema no-op path.
func runTestFillDefaults(t *testing.T, sdl string, listSizes map[FieldCoordinate]*FieldListSize) {
	t.Helper()
	var schema *ast.Document
	if sdl != "" {
		doc := unsafeparser.ParseGraphqlDocumentString(sdl)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&doc))
		schema = &doc
	}
	fillListSizeDefaults(listSizes, schema)
}

func TestEnrichListSizeDefaultsFromSchema(t *testing.T) {
	t.Run("extract flat slicing arg with Int default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"limit"}}
		runTestFillDefaults(t, `
			type Query {
				boards(limit: Int = 25): [Board]
			}
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Equal(t, map[string]int{"limit": 25}, ls.SlicingArgumentDefaults)
	})

	t.Run("map is empty after flat slicing arg without default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"limit"}}
		runTestFillDefaults(t, `
			type Query {
				boards(limit: Int): [Board]
			}
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("multiple slicing args, only some defaulted", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"first", "last"}}
		runTestFillDefaults(t, `
			type Query {
				users(first: Int = 20, last: Int): [User]
			}
			type User { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "users"}: ls,
		})
		assert.Equal(t, map[string]int{"first": 20}, ls.SlicingArgumentDefaults)
	})

	t.Run("two fields in the same schema are enriched independently", func(t *testing.T) {
		boards := &FieldListSize{SlicingArguments: []string{"limit"}}
		users := &FieldListSize{SlicingArguments: []string{"first", "last"}}
		runTestFillDefaults(t, `
			type Query {
				boards(limit: Int = 25): [Board]
				users(first: Int = 20, last: Int): [User]
			}
			type Board { id: ID }
			type User { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: boards,
			{TypeName: "Query", FieldName: "users"}:  users,
		})
		assert.Equal(t, map[string]int{"limit": 25}, boards.SlicingArgumentDefaults)
		assert.Equal(t, map[string]int{"first": 20}, users.SlicingArgumentDefaults)
	})

	t.Run("fill dot-path with leaf input-field default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput!): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.first": 10}, ls.SlicingArgumentDefaults)
	})

	t.Run("fill dot-path with outer-arg default supplying leaf", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput = { first: 15 }): [Book]
			}
			input PaginationInput { first: Int }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.first": 15}, ls.SlicingArgumentDefaults)
	})

	t.Run("dot-path with outer-overrides-inner default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput = { first: 15 }): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.first": 15}, ls.SlicingArgumentDefaults)
	})

	t.Run("explicit null in outer default shadows inner Int default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput = { first: null }): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("non-Int default is skipped", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"filter"}}
		runTestFillDefaults(t, `
			type Query {
				items(filter: String = "all"): [Item]
			}
			type Item { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "items"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("unresolved leading segment is silently skipped", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"wrong.missing"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput!): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("unresolved path segment is silently skipped", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.missing"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput!): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("missing field on the schema - skipped without panic", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"limit"}}
		runTestFillDefaults(t, `
			type Query {
				other(x: Int): [Item]
			}
			type Item { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("nil schema is a no-op", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"limit"}}
		runTestFillDefaults(t, "", map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("outer default missing nested field falls through to inner Int default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput = { other: 99 }): [Book]
			}
			input PaginationInput { first: Int = 10, other: Int }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.first": 10}, ls.SlicingArgumentDefaults)
	})

	t.Run("outer with non-Int leaf shadows inner Int default", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: PaginationInput = { first: "many" }): [Book]
			}
			input PaginationInput { first: Int = 10 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("three-segment path resolves through nested input objects", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.page.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: SearchInput!): [Book]
			}
			input SearchInput { page: PageInput }
			input PageInput  { first: Int = 7 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.page.first": 7}, ls.SlicingArgumentDefaults)
	})

	t.Run("three-segment path with outer object provides the full chain", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"input.page.first"}}
		runTestFillDefaults(t, `
			type Query {
				search(input: SearchInput = { page: { first: 42 } }): [Book]
			}
			input SearchInput { page: PageInput }
			input PageInput  { first: Int = 7 }
			type Book { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "search"}: ls,
		})
		assert.Equal(t, map[string]int{"input.page.first": 42}, ls.SlicingArgumentDefaults)
	})

	t.Run("path descends into a non-input-object type is skipped", func(t *testing.T) {
		// `limit` is a scalar Int — a dotted path beneath it has no input-object
		// to descend into and must be silently skipped.
		ls := &FieldListSize{SlicingArguments: []string{"limit.first"}}
		runTestFillDefaults(t, `
			type Query {
				boards(limit: Int = 25): [Board]
			}
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("nil FieldListSize entry is skipped without panic", func(t *testing.T) {
		runTestFillDefaults(t, `
			type Query { boards(limit: Int = 25): [Board] }
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: nil,
		})
	})

	t.Run("empty SlicingArguments leaves defaults nil", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{}}
		runTestFillDefaults(t, `
			type Query { boards(limit: Int = 25): [Board] }
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})

	t.Run("missing parent type is skipped without panic", func(t *testing.T) {
		ls := &FieldListSize{SlicingArguments: []string{"limit"}}
		runTestFillDefaults(t, `
			type Query { boards(limit: Int = 25): [Board] }
			type Board { id: ID }
		`, map[FieldCoordinate]*FieldListSize{
			{TypeName: "DoesNotExist", FieldName: "boards"}: ls,
		})
		assert.Nil(t, ls.SlicingArgumentDefaults)
	})
}
