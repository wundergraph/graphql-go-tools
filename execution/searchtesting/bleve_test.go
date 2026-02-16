package searchtesting

import (
	"context"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/bleve"
)

const bleveInlineConfigSDL = `
extend schema @index(name: "products", backend: "bleve", config: "{}")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts", resultsMetaInformation: false) {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`

const bleveConfigSDL = `
extend schema @index(name: "products", backend: "bleve", config: "{}")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`

const bleveCursorConfigSDL = `
extend schema @index(name: "products", backend: "bleve", config: "{}", cursorBasedPagination: true)

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`

func TestBleve(t *testing.T) {
	t.Parallel()
	RunAllScenarios(t, BackendSetup{
		Name:      "bleve",
		ConfigSDL: bleveConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
		ExpectedResponses: map[string]string{
			"supergraph_sdl":                  expectedSupergraphSDL,
			"basic_search_with_entity_join":   `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}}],"totalCount":2}}}`,
			"filter_keyword_with_entity_join": `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","rating":4.7}},{"node":{"id":"1","name":"Running Shoes","rating":4.5}},{"node":{"id":"2","name":"Basketball Shoes","rating":4.2}}]}}}`,
			"filter_boolean":                  `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"filter_numeric_range":            `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"1","manufacturer":"Nike"}}],"totalCount":2}}}`,
			"filter_AND":                      `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":3}}}`,
			"filter_OR":                       `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":2}}}`,
			"filter_NOT":                      `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"sort_with_entity_join":           `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","price":12.99,"manufacturer":"Smartwool"}},{"node":{"id":"3","name":"Leather Belt","price":35,"manufacturer":"Gucci"}},{"node":{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}}]}}}`,
			"pagination_with_entity_join":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","reviews":[{"text":"Nice belt","stars":3}]}},{"node":{"id":"1","reviews":[{"text":"Great shoes","stars":5}]}}],"totalCount":4}}}`,
			"score_and_totalCount":            `{"data":{"searchProducts":{"hits":[{"score":0.7768564486857903,"node":{"id":"4","manufacturer":"Smartwool"}},{"score":0.7768564486857903,"node":{"id":"3","manufacturer":"Gucci"}},{"score":0.7768564486857903,"node":{"id":"1","manufacturer":"Nike"}},{"score":0.7768564486857903,"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":4}}}`,
			"facets_with_entity_join":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"facets":[{"field":"category","values":[{"value":"Footwear","count":3},{"value":"Accessories","count":1}]}]}}}`,
		},
	})
}

func TestBleveInline(t *testing.T) {
	t.Parallel()
	RunInlineScenarios(t, BackendSetup{
		Name:      "bleve_inline",
		ConfigSDL: bleveInlineConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     false, // inline style has no facets
		},
		ExpectedResponses: map[string]string{
			"supergraph_sdl":              expectedInlineSupergraphSDL,
			"basic_search_inline":         `{"data":{"searchProducts":[{"id":"4","name":"Wool Socks","price":12.99,"manufacturer":"Smartwool"},{"id":"3","name":"Leather Belt","price":35,"manufacturer":"Gucci"},{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"},{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}]}}`,
			"filter_keyword_inline":       `{"data":{"searchProducts":[{"id":"4","name":"Wool Socks"},{"id":"1","name":"Running Shoes"},{"id":"2","name":"Basketball Shoes"}]}}`,
			"filter_boolean_inline":       `{"data":{"searchProducts":[{"id":"3","manufacturer":"Gucci"}]}}`,
			"filter_numeric_range_inline": `{"data":{"searchProducts":[{"id":"3","manufacturer":"Gucci"},{"id":"1","manufacturer":"Nike"}]}}`,
			"filter_AND_inline":           `{"data":{"searchProducts":[{"id":"4","manufacturer":"Smartwool"},{"id":"1","manufacturer":"Nike"},{"id":"2","manufacturer":"Adidas"}]}}`,
			"filter_OR_inline":            `{"data":{"searchProducts":[{"id":"3","manufacturer":"Gucci"},{"id":"2","manufacturer":"Adidas"}]}}`,
			"filter_NOT_inline":           `{"data":{"searchProducts":[{"id":"3","manufacturer":"Gucci"}]}}`,
			"sort_inline":                 `{"data":{"searchProducts":[{"id":"4","name":"Wool Socks","price":12.99,"manufacturer":"Smartwool"},{"id":"3","name":"Leather Belt","price":35,"manufacturer":"Gucci"},{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"},{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}]}}`,
			"pagination_inline":           `{"data":{"searchProducts":[{"id":"3","name":"Leather Belt"},{"id":"1","name":"Running Shoes"}]}}`,
		},
	})
}

func TestBleveHybrid(t *testing.T) {
	t.Parallel()
	// Bleve doesn't support vectors, but the hybrid pipeline sets both TextQuery
	// and Vector on SearchRequest. Bleve silently ignores the vector and performs
	// text-only search. This test validates the pipeline doesn't break.
	RunHybridScenarios(t, VectorBackendSetup{
		BackendSetup: BackendSetup{
			Name:      "bleve_hybrid",
			ConfigSDL: vectorConfigSDL("bleve", "{}"),
			CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
				t.Helper()
				factory := bleve.NewFactory()
				idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
				if err != nil {
					t.Fatalf("CreateIndex: %v", err)
				}
				t.Cleanup(func() { idx.Close() })
				return idx
			},
			Caps: BackendCaps{
				HasTextSearch: true,
				HasFacets:     true,
			},
		},
		Embedder: &MockEmbedder{},
	})
}

func TestBleveHighlights(t *testing.T) {
	t.Parallel()
	RunHighlightScenarios(t, BackendSetup{
		Name:      "bleve_highlights",
		ConfigSDL: bleveConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
	})
}

func TestBleveAdditionalFilters(t *testing.T) {
	t.Parallel()
	RunAdditionalFilterScenarios(t, BackendSetup{
		Name:      "bleve_additional_filters",
		ConfigSDL: bleveConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
		ExpectedResponses: map[string]string{
			"filter_string_ne":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"filter_string_in":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":4}}}`,
			"filter_string_startsWith": `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":3}}}`,
		},
	})
}

func TestBleveBoosting(t *testing.T) {
	t.Parallel()
	RunBoostingScenarios(t, BackendSetup{
		Name:      "bleve_boosting",
		ConfigSDL: boostConfigSDL("bleve", "{}"),
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
	})
}

func TestBleveFuzzy(t *testing.T) {
	t.Parallel()
	RunFuzzyScenarios(t, BackendSetup{
		Name:      "bleve_fuzzy",
		ConfigSDL: bleveConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
	})
}

func TestBleveSuggest(t *testing.T) {
	t.Parallel()
	RunSuggestScenarios(t, BackendSetup{
		Name:      "bleve_suggest",
		ConfigSDL: suggestConfigSDL("bleve", "{}"),
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
	})
}

func TestBleveDate(t *testing.T) {
	t.Parallel()
	RunDateScenarios(t, BackendSetup{
		Name:      "bleve_date",
		ConfigSDL: dateConfigSDL("bleve", "{}"),
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
		ExpectedResponses: map[string]string{
			"date_eq_filter":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}}],"totalCount":1}}}`,
			"date_range_gte_lte":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":3}}}`,
			"date_gt_lt":            `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":2}}}`,
			"date_after_before":      `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"datetime_eq_filter":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}}],"totalCount":1}}}`,
			"datetime_range_gte_lte": `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":3}}}`,
			"datetime_after_before":  `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}],"totalCount":1}}}`,
			"date_sort_asc":          `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}]}}}`,
			"date_sort_desc":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}}]}}}`,
			"datetime_sort_asc":      `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}]}}}`,
			"date_combined_filter":   `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}],"totalCount":2}}}`,
		},
	})
}

func TestBleveCursor(t *testing.T) {
	t.Parallel()
	RunCursorScenarios(t, BackendSetup{
		Name:      "bleve_cursor",
		ConfigSDL: bleveCursorConfigSDL,
		CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
			t.Helper()
			factory := bleve.NewFactory()
			idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
			if err != nil {
				t.Fatalf("CreateIndex: %v", err)
			}
			t.Cleanup(func() { idx.Close() })
			return idx
		},
		Caps: BackendCaps{
			HasTextSearch: true,
			HasFacets:     true,
		},
	})
}
