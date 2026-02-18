package searche2e

import (
	"strings"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestGenerateSubgraphSDL_TextOnly(t *testing.T) {
	config := &search_datasource.ParsedConfig{
		Entities: []search_datasource.SearchableEntity{
			{
				TypeName:               "Product",
				IndexName:              "products",
				SearchField:            "searchProducts",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []search_datasource.IndexedField{
					{FieldName: "name", GraphQLType: "String", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					{FieldName: "category", GraphQLType: "String", IndexType: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
					{FieldName: "price", GraphQLType: "Float", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
					{FieldName: "inStock", GraphQLType: "Boolean", IndexType: searchindex.FieldTypeBool, Filterable: true},
				},
			},
		},
	}

	sdl, err := search_datasource.GenerateSubgraphSDL(config)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	// Shared types
	assertContains(t, sdl, "input StringFilter {")
	assertContains(t, sdl, "input FloatFilter {")
	assertContains(t, sdl, "enum SortDirection {")
	assertContains(t, sdl, "type SearchFacet {")
	assertContains(t, sdl, "type SearchFacetValue {")

	// Filter input
	assertContains(t, sdl, "input ProductFilter {")
	assertContains(t, sdl, "name: StringFilter")
	assertContains(t, sdl, "category: StringFilter")
	assertContains(t, sdl, "price: FloatFilter")
	assertContains(t, sdl, "inStock: Boolean")
	assertContains(t, sdl, "AND: [ProductFilter!]")
	assertContains(t, sdl, "OR: [ProductFilter!]")
	assertContains(t, sdl, "NOT: ProductFilter")

	// Sort enum and input
	assertContains(t, sdl, "enum ProductSortField {")
	assertContains(t, sdl, "RELEVANCE")
	assertContains(t, sdl, "NAME")
	assertContains(t, sdl, "CATEGORY")
	assertContains(t, sdl, "PRICE")
	assertContains(t, sdl, "input ProductSort {")

	// Result types
	assertContains(t, sdl, "type SearchProductResult {")
	assertContains(t, sdl, "hits: [SearchProductHit!]!")
	assertContains(t, sdl, "totalCount: Int!")
	assertContains(t, sdl, "facets: [SearchFacet!]")
	assertContains(t, sdl, "type SearchProductHit {")
	assertContains(t, sdl, "score: Float!")
	assertContains(t, sdl, "node: Product!")

	// No distance field for text-only
	assertNotContains(t, sdl, "distance: Float")

	// No SearchProductInput @oneOf for text-only
	assertNotContains(t, sdl, "input SearchProductInput")

	// Query type
	assertContains(t, sdl, "type Query {")
	assertContains(t, sdl, "searchProducts(")
	assertContains(t, sdl, "query: String!")
	assertContains(t, sdl, "filter: ProductFilter")
	assertContains(t, sdl, "sort: [ProductSort!]")
	assertContains(t, sdl, "limit: Int")
	assertContains(t, sdl, "offset: Int")
	assertContains(t, sdl, "facets: [String!]")
	assertContains(t, sdl, "): SearchProductResult!")

	// Entity stub
	assertContains(t, sdl, `type Product @key(fields: "id") {`)
	assertContains(t, sdl, "id: ID! @external")
}

func TestGenerateSubgraphSDL_Vector(t *testing.T) {
	config := &search_datasource.ParsedConfig{
		Entities: []search_datasource.SearchableEntity{
			{
				TypeName:               "Article",
				IndexName:              "articles",
				SearchField:            "searchArticles",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []search_datasource.IndexedField{
					{FieldName: "title", GraphQLType: "String", IndexType: searchindex.FieldTypeText, Filterable: true},
					{FieldName: "embedding", GraphQLType: "[Float!]", IndexType: searchindex.FieldTypeVector, Dimensions: 1536},
				},
			},
		},
	}

	sdl, err := search_datasource.GenerateSubgraphSDL(config)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	// Vector search input (@oneOf)
	assertContains(t, sdl, "input SearchArticleInput @oneOf {")
	assertContains(t, sdl, "query: String")
	assertContains(t, sdl, "vector: [Float!]")

	// Result types with distance
	assertContains(t, sdl, "type SearchArticleHit {")
	assertContains(t, sdl, "score: Float!")
	assertContains(t, sdl, "distance: Float")
	assertContains(t, sdl, "node: Article!")

	// Query uses search: instead of query:
	assertContains(t, sdl, "searchArticles(")
	assertContains(t, sdl, "search: SearchArticleInput!")

	// No facets argument for vector entities
	assertNotContains(t, sdl, "facets: [String!]")

	// Entity stub
	assertContains(t, sdl, `type Article @key(fields: "id") {`)
	assertContains(t, sdl, "id: ID! @external")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected SDL to contain %q\n\nActual SDL:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected SDL NOT to contain %q\n\nActual SDL:\n%s", substr, s)
	}
}
