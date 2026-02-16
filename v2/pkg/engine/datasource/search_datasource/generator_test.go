package search_datasource

import (
	"strings"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestGenerateSubgraphSDL(t *testing.T) {
	t.Run("text-only entity", func(t *testing.T) {
		config := &ParsedConfig{
			Entities: []SearchableEntity{
				{
					TypeName:         "Product",
					IndexName:        "products",
					SearchField:      "searchProducts",
					KeyFields:        []string{"id"},
					ResultsMetaInformation: true,
					Fields: []IndexedField{
						{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
						{FieldName: "description", GraphQLType: "String!", IndexType: searchindex.FieldTypeText},
						{FieldName: "category", GraphQLType: "String!", IndexType: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
						{FieldName: "price", GraphQLType: "Float!", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
						{FieldName: "inStock", GraphQLType: "Boolean!", IndexType: searchindex.FieldTypeBool, Filterable: true},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have query field
		assertContains(t, sdl, "searchProducts(")
		assertContains(t, sdl, "query: String!")

		// Should have filter type with filterable fields
		assertContains(t, sdl, "input ProductFilter {")
		assertContains(t, sdl, "name: StringFilter")
		assertContains(t, sdl, "category: StringFilter")
		assertContains(t, sdl, "price: FloatFilter")
		assertContains(t, sdl, "inStock: Boolean")
		assertContains(t, sdl, "AND: [ProductFilter!]")
		assertContains(t, sdl, "OR: [ProductFilter!]")
		assertContains(t, sdl, "NOT: ProductFilter")

		// Should have sort enum
		assertContains(t, sdl, "enum ProductSortField {")
		assertContains(t, sdl, "RELEVANCE")
		assertContains(t, sdl, "NAME")
		assertContains(t, sdl, "CATEGORY")
		assertContains(t, sdl, "PRICE")

		// Should have result types
		assertContains(t, sdl, "type SearchProductResult {")
		assertContains(t, sdl, "type SearchProductHit {")
		assertContains(t, sdl, "node: Product!")

		// Should have entity stub
		assertContains(t, sdl, `type Product @key(fields: "id") {`)
		assertContains(t, sdl, "id: ID!")

		// Should NOT have vector search input
		assertNotContains(t, sdl, "SearchProductInput")

		// Should have facets argument
		assertContains(t, sdl, "facets: [String!]")

		// Should have shared types
		assertContains(t, sdl, "input StringFilter {")
		assertContains(t, sdl, "input FloatFilter {")
	})

	t.Run("vector entity with embedding", func(t *testing.T) {
		config := &ParsedConfig{
			Entities: []SearchableEntity{
				{
					TypeName:         "Article",
					IndexName:        "articles",
					SearchField:      "searchArticles",
					KeyFields:        []string{"id"},
					ResultsMetaInformation: true,
					Fields: []IndexedField{
						{FieldName: "title", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true},
						{FieldName: "body", GraphQLType: "String!", IndexType: searchindex.FieldTypeText},
					},
					EmbeddingFields: []EmbeddingField{
						{FieldName: "_embedding", SourceFields: []string{"title", "body"}, Template: "{{title}}. {{body}}", Model: "text-embedding-3-small"},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have @oneOf search input
		assertContains(t, sdl, "input SearchArticleInput @oneOf {")
		assertContains(t, sdl, "query: String")
		assertContains(t, sdl, "vector: [Float!]")

		// Search field should use search: input
		assertContains(t, sdl, "search: SearchArticleInput!")

		// Should have distance field in hit type
		assertContains(t, sdl, "distance: Float")
	})

	t.Run("inline style text-only entity", func(t *testing.T) {
		config := &ParsedConfig{
			Entities: []SearchableEntity{
				{
					TypeName:         "Product",
					IndexName:        "products",
					SearchField:      "searchProducts",
					KeyFields:        []string{"id"},
					ResultsMetaInformation: false,
					Fields: []IndexedField{
						{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
						{FieldName: "description", GraphQLType: "String!", IndexType: searchindex.FieldTypeText},
						{FieldName: "category", GraphQLType: "String!", IndexType: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
						{FieldName: "price", GraphQLType: "Float!", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
						{FieldName: "inStock", GraphQLType: "Boolean!", IndexType: searchindex.FieldTypeBool, Filterable: true},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should return [Product!]! instead of wrapper
		assertContains(t, sdl, "): [Product!]!")

		// Should NOT have wrapper types
		assertNotContains(t, sdl, "SearchProductResult")
		assertNotContains(t, sdl, "SearchProductHit")
		assertNotContains(t, sdl, "SearchFacet")
		assertNotContains(t, sdl, "SearchFacetValue")

		// Should NOT have facets argument
		assertNotContains(t, sdl, "facets:")

		// Should still have filter/sort/limit/offset types
		assertContains(t, sdl, "input ProductFilter {")
		assertContains(t, sdl, "enum ProductSortField {")
		assertContains(t, sdl, "input ProductSort {")
		assertContains(t, sdl, "limit: Int")
		assertContains(t, sdl, "offset: Int")

		// Should still have entity stub
		assertContains(t, sdl, `type Product @key(fields: "id") {`)
		assertContains(t, sdl, "id: ID!")
	})

	t.Run("pre-computed vector entity", func(t *testing.T) {
		config := &ParsedConfig{
			Entities: []SearchableEntity{
				{
					TypeName:         "Image",
					IndexName:        "images",
					SearchField:      "searchImages",
					KeyFields:        []string{"id"},
					ResultsMetaInformation: true,
					Fields: []IndexedField{
						{FieldName: "caption", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true},
						{FieldName: "embedding", GraphQLType: "[Float!]!", IndexType: searchindex.FieldTypeVector, Dimensions: 512},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertContains(t, sdl, "input SearchImageInput @oneOf {")
		assertContains(t, sdl, "search: SearchImageInput!")
	})

	t.Run("cursor pagination bidirectional", func(t *testing.T) {
		config := &ParsedConfig{
			Indices: []IndexDirective{
				{Name: "products", Backend: "bleve", CursorBasedPagination: true},
			},
			Entities: []SearchableEntity{
				{
					TypeName:               "Product",
					IndexName:              "products",
					SearchField:            "searchProducts",
					KeyFields:              []string{"id"},
					ResultsMetaInformation: true,
					CursorBasedPagination:  true,
					CursorBidirectional:    true,
					Fields: []IndexedField{
						{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
						{FieldName: "price", GraphQLType: "Float!", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have Connection/Edge/PageInfo types
		assertContains(t, sdl, "type SearchProductConnection {")
		assertContains(t, sdl, "edges: [SearchProductEdge!]!")
		assertContains(t, sdl, "pageInfo: SearchPageInfo!")
		assertContains(t, sdl, "totalCount: Int!")

		assertContains(t, sdl, "type SearchProductEdge {")
		assertContains(t, sdl, "cursor: String!")
		assertContains(t, sdl, "node: Product!")
		assertContains(t, sdl, "score: Float!") // meta=true

		assertContains(t, sdl, "type SearchPageInfo {")
		assertContains(t, sdl, "hasNextPage: Boolean!")
		assertContains(t, sdl, "hasPreviousPage: Boolean!")
		assertContains(t, sdl, "startCursor: String")
		assertContains(t, sdl, "endCursor: String")

		// Should have bidirectional cursor args
		assertContains(t, sdl, "first: Int")
		assertContains(t, sdl, "after: String")
		assertContains(t, sdl, "last: Int")
		assertContains(t, sdl, "before: String")

		// Should NOT have offset/limit
		assertNotContains(t, sdl, "limit: Int")
		assertNotContains(t, sdl, "offset: Int")

		// Return type should be Connection
		assertContains(t, sdl, "): SearchProductConnection!")

		// Should NOT have old-style wrapper types
		assertNotContains(t, sdl, "SearchProductResult")
		assertNotContains(t, sdl, "SearchProductHit")
	})

	t.Run("cursor pagination forward-only", func(t *testing.T) {
		config := &ParsedConfig{
			Indices: []IndexDirective{
				{Name: "articles", Backend: "elasticsearch", CursorBasedPagination: true},
			},
			Entities: []SearchableEntity{
				{
					TypeName:               "Article",
					IndexName:              "articles",
					SearchField:            "searchArticles",
					KeyFields:              []string{"id"},
					ResultsMetaInformation: true,
					CursorBasedPagination:  true,
					CursorBidirectional:    false,
					Fields: []IndexedField{
						{FieldName: "title", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have first/after but NOT last/before
		assertContains(t, sdl, "first: Int")
		assertContains(t, sdl, "after: String")
		assertNotContains(t, sdl, "last: Int")
		assertNotContains(t, sdl, "before: String")
	})

	t.Run("cursor pagination meta disabled", func(t *testing.T) {
		config := &ParsedConfig{
			Indices: []IndexDirective{
				{Name: "products", Backend: "bleve", CursorBasedPagination: true},
			},
			Entities: []SearchableEntity{
				{
					TypeName:               "Product",
					IndexName:              "products",
					SearchField:            "searchProducts",
					KeyFields:              []string{"id"},
					ResultsMetaInformation: false,
					CursorBasedPagination:  true,
					CursorBidirectional:    true,
					Fields: []IndexedField{
						{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Edge should NOT have score/distance when meta is disabled
		assertContains(t, sdl, "type SearchProductEdge {")
		assertContains(t, sdl, "cursor: String!")
		assertContains(t, sdl, "node: Product!")
		assertNotContains(t, sdl, "score: Float!")
	})

	t.Run("cursor pagination vector entity", func(t *testing.T) {
		config := &ParsedConfig{
			Indices: []IndexDirective{
				{Name: "images", Backend: "bleve", CursorBasedPagination: true},
			},
			Entities: []SearchableEntity{
				{
					TypeName:               "Image",
					IndexName:              "images",
					SearchField:            "searchImages",
					KeyFields:              []string{"id"},
					ResultsMetaInformation: true,
					CursorBasedPagination:  true,
					CursorBidirectional:    true,
					Fields: []IndexedField{
						{FieldName: "caption", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true},
						{FieldName: "embedding", GraphQLType: "[Float!]!", IndexType: searchindex.FieldTypeVector, Dimensions: 512},
					},
				},
			},
		}

		sdl, err := GenerateSubgraphSDL(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Edge should have distance for vector
		assertContains(t, sdl, "type SearchImageEdge {")
		assertContains(t, sdl, "distance: Float")
	})

	t.Run("unsupported backend error", func(t *testing.T) {
		config := &ParsedConfig{
			Indices: []IndexDirective{
				{Name: "products", Backend: "typesense", CursorBasedPagination: true},
			},
			Entities: []SearchableEntity{
				{
					TypeName:              "Product",
					IndexName:             "products",
					SearchField:           "searchProducts",
					KeyFields:             []string{"id"},
					CursorBasedPagination: true,
					Fields: []IndexedField{
						{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText},
					},
				},
			},
		}

		_, err := GenerateSubgraphSDL(config)
		if err == nil {
			t.Fatal("expected error for unsupported backend with cursor pagination")
		}
		if !strings.Contains(err.Error(), "typesense") {
			t.Errorf("error should mention the backend name, got: %v", err)
		}
	})
}

func TestGenerateSubgraphSDL_GeoEntity(t *testing.T) {
	config := &ParsedConfig{
		Entities: []SearchableEntity{
			{
				TypeName:               "Store",
				IndexName:              "stores",
				SearchField:            "searchStores",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []IndexedField{
					{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					{FieldName: "category", GraphQLType: "String!", IndexType: searchindex.FieldTypeKeyword, Filterable: true},
					{FieldName: "location", GraphQLType: "GeoPoint", IndexType: searchindex.FieldTypeGeo, Filterable: true, Sortable: true},
				},
			},
		},
	}

	sdl, err := GenerateSubgraphSDL(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have shared geo types
	assertContains(t, sdl, "input GeoPointInput {")
	assertContains(t, sdl, "input GeoDistanceFilterInput {")
	assertContains(t, sdl, "input GeoBoundingBoxFilterInput {")
	assertContains(t, sdl, "input GeoDistanceSortInput {")

	// Filter should have geo-specific fields
	assertContains(t, sdl, "input StoreFilter {")
	assertContains(t, sdl, "location_distance: GeoDistanceFilterInput")
	assertContains(t, sdl, "location_boundingBox: GeoBoundingBoxFilterInput")

	// Should NOT have a plain "location:" filter (GEO fields get _distance/_boundingBox instead)
	assertNotContains(t, sdl, "  location: ")

	// Sort enum should NOT include location (GEO fields use geoSort instead)
	assertNotContains(t, sdl, "LOCATION")

	// Search field should have geoSort argument
	assertContains(t, sdl, "geoSort: GeoDistanceSortInput")

	// Hit type should have geoDistance field
	assertContains(t, sdl, "type SearchStoreHit {")
	assertContains(t, sdl, "geoDistance: Float")

	// Should have query field
	assertContains(t, sdl, "searchStores(")
}

func TestGenerateSubgraphSDL_DateEntity(t *testing.T) {
	config := &ParsedConfig{
		Entities: []SearchableEntity{
			{
				TypeName:               "Event",
				IndexName:              "events",
				SearchField:            "searchEvents",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []IndexedField{
					{FieldName: "title", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					{FieldName: "eventDate", GraphQLType: "Date", IndexType: searchindex.FieldTypeDate, Filterable: true, Sortable: true},
					{FieldName: "createdAt", GraphQLType: "DateTime", IndexType: searchindex.FieldTypeDateTime, Filterable: true, Sortable: true},
				},
			},
		},
	}

	sdl, err := GenerateSubgraphSDL(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have Date and DateTime scalars
	assertContains(t, sdl, "scalar Date")
	assertContains(t, sdl, "scalar DateTime")

	// Should have DateFilter input type
	assertContains(t, sdl, "input DateFilter {")
	assertContains(t, sdl, "eq: Date")
	assertContains(t, sdl, "gt: Date")
	assertContains(t, sdl, "gte: Date")
	assertContains(t, sdl, "lt: Date")
	assertContains(t, sdl, "lte: Date")
	assertContains(t, sdl, "after: Date")
	assertContains(t, sdl, "before: Date")

	// Should have DateTimeFilter input type
	assertContains(t, sdl, "input DateTimeFilter {")
	assertContains(t, sdl, "eq: DateTime")
	assertContains(t, sdl, "gt: DateTime")
	assertContains(t, sdl, "after: DateTime")
	assertContains(t, sdl, "before: DateTime")

	// Filter should use DateFilter for Date fields and DateTimeFilter for DateTime fields
	assertContains(t, sdl, "input EventFilter {")
	assertContains(t, sdl, "eventDate: DateFilter")
	assertContains(t, sdl, "createdAt: DateTimeFilter")

	// Sort enum should include date fields
	assertContains(t, sdl, "enum EventSortField {")
	assertContains(t, sdl, "EVENTDATE")
	assertContains(t, sdl, "CREATEDAT")

	// Should NOT have geo types (no geo fields)
	assertNotContains(t, sdl, "GeoPointInput")
	assertNotContains(t, sdl, "GeoDistanceFilterInput")
}

func TestGenerateSubgraphSDL_NoDateScalarsWithoutDateFields(t *testing.T) {
	config := &ParsedConfig{
		Entities: []SearchableEntity{
			{
				TypeName:               "Product",
				IndexName:              "products",
				SearchField:            "searchProducts",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []IndexedField{
					{FieldName: "name", GraphQLType: "String!", IndexType: searchindex.FieldTypeText, Filterable: true},
					{FieldName: "price", GraphQLType: "Float!", IndexType: searchindex.FieldTypeNumeric, Filterable: true},
				},
			},
		},
	}

	sdl, err := GenerateSubgraphSDL(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have date scalars or filters when no date fields exist
	assertNotContains(t, sdl, "scalar Date")
	assertNotContains(t, sdl, "scalar DateTime")
	assertNotContains(t, sdl, "DateFilter")
	assertNotContains(t, sdl, "DateTimeFilter")
}

func assertContains(t *testing.T, sdl, substr string) {
	t.Helper()
	if !strings.Contains(sdl, substr) {
		t.Errorf("SDL should contain %q\n\nSDL:\n%s", substr, sdl)
	}
}

func assertNotContains(t *testing.T, sdl, substr string) {
	t.Helper()
	if strings.Contains(sdl, substr) {
		t.Errorf("SDL should NOT contain %q", substr)
	}
}
