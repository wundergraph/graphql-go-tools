package searche2e

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// BackendCaps describes what capabilities a backend supports.
type BackendCaps struct {
	HasTextSearch        bool
	HasFacets            bool
	HasPrefix            bool
	HasExists            bool
	HasVectorSearch      bool
	HasCursorPagination  bool
}

// BackendHooks provides hooks for backend-specific behavior.
type BackendHooks struct {
	WaitForIndex func(t *testing.T) // called after populating, before querying
}

// SearchResponse represents the parsed JSON response from Source.Load().
type SearchResponse struct {
	Hits       []SearchHit   `json:"hits"`
	TotalCount int           `json:"totalCount"`
	Facets     []SearchFacet `json:"facets"`
}

// SearchHighlight represents a highlighted field in a search hit.
type SearchHighlight struct {
	Field     string   `json:"field"`
	Fragments []string `json:"fragments"`
}

// SearchHit represents a single hit in the response.
type SearchHit struct {
	Score       float64           `json:"score"`
	Distance    float64           `json:"distance"`
	GeoDistance *float64          `json:"geoDistance"`
	Highlights  []SearchHighlight `json:"highlights"`
	Node        map[string]any    `json:"node"`
}

// SearchFacet represents a facet in the response.
type SearchFacet struct {
	Field  string           `json:"field"`
	Values []SearchFacetVal `json:"values"`
}

// SearchFacetVal represents a single facet value.
type SearchFacetVal struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// ConnectionResponse represents a cursor-based pagination response.
type ConnectionResponse struct {
	Edges      []ConnectionEdge `json:"edges"`
	PageInfo   PageInfo         `json:"pageInfo"`
	TotalCount int              `json:"totalCount"`
}

// ConnectionEdge represents a single edge in a connection response.
type ConnectionEdge struct {
	Cursor     string           `json:"cursor"`
	Node       map[string]any   `json:"node"`
	Score      float64          `json:"score"`
	Highlights []SearchHighlight `json:"highlights"`
}

// PageInfo represents page info in a connection response.
type PageInfo struct {
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor"`
	EndCursor       *string `json:"endCursor"`
}

// TestProducts returns the standard 4 test products.
func TestProducts() []searchindex.EntityDocument {
	return []searchindex.EntityDocument{
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			Fields:   map[string]any{"name": "Basketball Shoes", "description": "High-top basketball sneakers", "category": "Footwear", "price": 129.99, "inStock": true},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
			Fields:   map[string]any{"name": "Leather Belt", "description": "Genuine leather dress belt", "category": "Accessories", "price": 35.00, "inStock": false},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "4"}},
			Fields:   map[string]any{"name": "Wool Socks", "description": "Warm wool socks for winter", "category": "Footwear", "price": 12.99, "inStock": true},
		},
	}
}

// GeoTestProducts returns the standard 4 test products with geo locations.
func GeoTestProducts() []searchindex.EntityDocument {
	return []searchindex.EntityDocument{
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true, "location": map[string]any{"lat": 40.7128, "lon": -74.0060}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			Fields:   map[string]any{"name": "Basketball Shoes", "description": "High-top basketball sneakers", "category": "Footwear", "price": 129.99, "inStock": true, "location": map[string]any{"lat": 40.7580, "lon": -73.9855}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
			Fields:   map[string]any{"name": "Leather Belt", "description": "Genuine leather dress belt", "category": "Accessories", "price": 35.00, "inStock": false, "location": map[string]any{"lat": 34.0522, "lon": -118.2437}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "4"}},
			Fields:   map[string]any{"name": "Wool Socks", "description": "Warm wool socks for winter", "category": "Footwear", "price": 12.99, "inStock": true, "location": map[string]any{"lat": 51.5074, "lon": -0.1278}},
		},
	}
}

// GeoProductIndexSchema returns the index schema with a geo location field.
func GeoProductIndexSchema() searchindex.IndexConfig {
	return searchindex.IndexConfig{
		Name: "products",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
			{Name: "location", Type: searchindex.FieldTypeGeo, Filterable: true, Sortable: true},
		},
	}
}

// GeoProductDatasourceConfig returns the search datasource configuration with geo field.
func GeoProductDatasourceConfig() search_datasource.Configuration {
	return search_datasource.Configuration{
		IndexName:      "products",
		SearchField:    "searchProducts",
		EntityTypeName: "Product",
		KeyFields:      []string{"id"},
		Fields: []search_datasource.IndexedFieldConfig{
			{FieldName: "name", GraphQLType: "String", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{FieldName: "description", GraphQLType: "String", IndexType: searchindex.FieldTypeText},
			{FieldName: "category", GraphQLType: "String", IndexType: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{FieldName: "price", GraphQLType: "Float", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{FieldName: "inStock", GraphQLType: "Boolean", IndexType: searchindex.FieldTypeBool, Filterable: true},
			{FieldName: "location", GraphQLType: "GeoPoint", IndexType: searchindex.FieldTypeGeo, Filterable: true, Sortable: true},
		},
		HasTextSearch:          true,
		ResultsMetaInformation: true,
	}
}

// ProductIndexSchema returns the standard index schema for product tests.
func ProductIndexSchema() searchindex.IndexConfig {
	return searchindex.IndexConfig{
		Name: "products",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		},
	}
}

// ProductDatasourceConfig returns the search datasource configuration for products.
func ProductDatasourceConfig() search_datasource.Configuration {
	return search_datasource.Configuration{
		IndexName:      "products",
		SearchField:    "searchProducts",
		EntityTypeName: "Product",
		KeyFields:      []string{"id"},
		Fields: []search_datasource.IndexedFieldConfig{
			{FieldName: "name", GraphQLType: "String", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{FieldName: "description", GraphQLType: "String", IndexType: searchindex.FieldTypeText},
			{FieldName: "category", GraphQLType: "String", IndexType: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{FieldName: "price", GraphQLType: "Float", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{FieldName: "inStock", GraphQLType: "Boolean", IndexType: searchindex.FieldTypeBool, Filterable: true},
		},
		HasTextSearch:          true,
		ResultsMetaInformation: true,
	}
}

// CreateSource creates a Source for the given index and config.
func CreateSource(t *testing.T, idx searchindex.Index, config search_datasource.Configuration) *search_datasource.Source {
	t.Helper()
	factory := search_datasource.NewFactory(context.Background(), nil, nil)
	factory.RegisterIndex(config.IndexName, idx)
	source, err := factory.CreateSourceForConfig(config)
	if err != nil {
		t.Fatalf("CreateSourceForConfig: %v", err)
	}
	if source == nil {
		t.Fatal("CreateSourceForConfig returned nil")
	}
	return source
}

// searchInputBuilder is the struct used to build Source.Load() JSON input.
type searchInputBuilder struct {
	SearchField string          `json:"search_field"`
	Query       string          `json:"query,omitempty"`
	Filter      json.RawMessage `json:"filter,omitempty"`
	Sort        json.RawMessage `json:"sort,omitempty"`
	GeoSort     json.RawMessage `json:"geoSort,omitempty"`
	Limit       *int            `json:"limit,omitempty"`
	Offset      *int            `json:"offset,omitempty"`
	Facets      []string        `json:"facets,omitempty"`
	Fuzziness   *string         `json:"fuzziness,omitempty"`
	First       *int            `json:"first,omitempty"`
	After       *string         `json:"after,omitempty"`
	Last        *int            `json:"last,omitempty"`
	Before      *string         `json:"before,omitempty"`
}

// InputOption configures a search input.
type InputOption func(*searchInputBuilder)

// BuildSearchInput builds JSON input for Source.Load().
func BuildSearchInput(opts ...InputOption) []byte {
	b := &searchInputBuilder{
		SearchField: "searchProducts",
	}
	for _, opt := range opts {
		opt(b)
	}
	data, _ := json.Marshal(b)
	return data
}

// WithQuery sets the text query.
func WithQuery(q string) InputOption {
	return func(b *searchInputBuilder) {
		b.Query = q
	}
}

// WithFilter sets the filter. f is marshaled to JSON.
func WithFilter(f any) InputOption {
	return func(b *searchInputBuilder) {
		data, _ := json.Marshal(f)
		b.Filter = data
	}
}

// WithSort sets the sort order.
func WithSort(sorts []map[string]string) InputOption {
	return func(b *searchInputBuilder) {
		data, _ := json.Marshal(sorts)
		b.Sort = data
	}
}

// WithGeoSort sets the geo distance sort.
func WithGeoSort(field string, lat, lon float64, direction, unit string) InputOption {
	return func(b *searchInputBuilder) {
		data, _ := json.Marshal(map[string]any{
			"field":     field,
			"center":    map[string]any{"lat": lat, "lon": lon},
			"direction": direction,
			"unit":      unit,
		})
		b.GeoSort = data
	}
}

// WithFuzziness sets the fuzziness level ("EXACT", "LOW", "HIGH").
func WithFuzziness(level string) InputOption {
	return func(b *searchInputBuilder) {
		b.Fuzziness = &level
	}
}

// WithLimit sets the result limit.
func WithLimit(n int) InputOption {
	return func(b *searchInputBuilder) {
		b.Limit = &n
	}
}

// WithOffset sets the result offset.
func WithOffset(n int) InputOption {
	return func(b *searchInputBuilder) {
		b.Offset = &n
	}
}

// WithFacets sets the facet fields.
func WithFacets(fields []string) InputOption {
	return func(b *searchInputBuilder) {
		b.Facets = fields
	}
}

// WithFirst sets the first (cursor pagination) limit.
func WithFirst(n int) InputOption {
	return func(b *searchInputBuilder) {
		b.First = &n
	}
}

// WithAfter sets the after cursor.
func WithAfter(cursor string) InputOption {
	return func(b *searchInputBuilder) {
		b.After = &cursor
	}
}

// WithLast sets the last (backward cursor pagination) limit.
func WithLast(n int) InputOption {
	return func(b *searchInputBuilder) {
		b.Last = &n
	}
}

// WithBefore sets the before cursor.
func WithBefore(cursor string) InputOption {
	return func(b *searchInputBuilder) {
		b.Before = &cursor
	}
}

// LoadAndParseConnection calls Source.Load and parses a cursor-based connection response.
func LoadAndParseConnection(t *testing.T, source *search_datasource.Source, input []byte) ConnectionResponse {
	t.Helper()
	data, err := source.Load(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("Source.Load: %v", err)
	}

	var wrapped map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapped); err != nil {
		t.Fatalf("unmarshal wrapped response: %v (raw: %s)", err, string(data))
	}
	dataMap, ok := wrapped["data"]
	if !ok {
		t.Fatalf("missing 'data' key in response (raw: %s)", string(data))
	}
	var inner json.RawMessage
	for _, v := range dataMap {
		inner = v
		break
	}
	if inner == nil {
		t.Fatalf("empty 'data' map in response (raw: %s)", string(data))
	}

	var resp ConnectionResponse
	if err := json.Unmarshal(inner, &resp); err != nil {
		t.Fatalf("unmarshal connection response: %v (raw: %s)", err, string(inner))
	}
	return resp
}

// CursorProductDatasourceConfig returns a search datasource configuration with cursor pagination enabled.
func CursorProductDatasourceConfig() search_datasource.Configuration {
	cfg := ProductDatasourceConfig()
	cfg.CursorBasedPagination = true
	cfg.CursorBidirectional = true
	return cfg
}

// LoadAndParse calls Source.Load and parses the response.
// The source returns {"data": {"<searchField>": {...}}} so we extract the inner result.
func LoadAndParse(t *testing.T, source *search_datasource.Source, input []byte) SearchResponse {
	t.Helper()
	data, err := source.Load(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("Source.Load: %v", err)
	}

	// Extract the search result from the wrapped response.
	var wrapped map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapped); err != nil {
		t.Fatalf("unmarshal wrapped response: %v (raw: %s)", err, string(data))
	}
	dataMap, ok := wrapped["data"]
	if !ok {
		t.Fatalf("missing 'data' key in response (raw: %s)", string(data))
	}
	// Find the first (and only) key inside "data" — the search field result.
	var inner json.RawMessage
	for _, v := range dataMap {
		inner = v
		break
	}
	if inner == nil {
		t.Fatalf("empty 'data' map in response (raw: %s)", string(data))
	}

	var resp SearchResponse
	if err := json.Unmarshal(inner, &resp); err != nil {
		t.Fatalf("unmarshal search result: %v (raw: %s)", err, string(inner))
	}
	return resp
}

// hitCount returns the effective hit count, preferring TotalCount but falling
// back to len(Hits) for backends that don't set TotalCount on scroll queries.
func hitCount(resp SearchResponse) int {
	if resp.TotalCount > 0 {
		return resp.TotalCount
	}
	return len(resp.Hits)
}

// RunBackendTests runs all e2e test scenarios against the given index.
// Read-only tests use t.Parallel() on the shared index.
// Mutation tests each get a fresh index via indexFactory.
func RunBackendTests(t *testing.T, idx searchindex.Index, caps BackendCaps, hooks BackendHooks, indexFactory func(t *testing.T) searchindex.Index) {
	// Populate shared index with test data.
	if err := idx.IndexDocuments(context.Background(), TestProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if hooks.WaitForIndex != nil {
		hooks.WaitForIndex(t)
	}

	config := ProductDatasourceConfig()
	source := CreateSource(t, idx, config)

	// Read-only tests (parallel, shared index).
	t.Run("read", func(t *testing.T) {
		if caps.HasTextSearch {
			t.Run("text_search", func(t *testing.T) {
				t.Parallel()
				resp := LoadAndParse(t, source, BuildSearchInput(WithQuery("shoes"), WithLimit(10)))
				if hitCount(resp) < 2 {
					t.Errorf("expected ≥2 hits for 'shoes', got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
				}
			})
		}

		t.Run("filter_term_keyword", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{"category": map[string]string{"eq": "Footwear"}}),
				WithLimit(10),
			))
			if hitCount(resp) != 3 {
				t.Errorf("expected 3 hits for category=Footwear, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("filter_boolean", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{"inStock": false}),
				WithLimit(10),
			))
			if hitCount(resp) != 1 {
				t.Errorf("expected 1 hit for inStock=false, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("filter_numeric_range", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{"price": map[string]any{"gte": 30, "lte": 100}}),
				WithLimit(10),
			))
			if hitCount(resp) != 2 {
				t.Errorf("expected 2 hits for price 30-100, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("filter_AND", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{
					"AND": []map[string]any{
						{"category": map[string]string{"eq": "Footwear"}},
						{"inStock": true},
					},
				}),
				WithLimit(10),
			))
			if hitCount(resp) != 3 {
				t.Errorf("expected 3 hits for Footwear AND inStock, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("filter_OR", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{
					"OR": []map[string]any{
						{"category": map[string]string{"eq": "Accessories"}},
						{"price": map[string]any{"gte": 100}},
					},
				}),
				WithLimit(10),
			))
			if hitCount(resp) != 2 {
				t.Errorf("expected 2 hits for Accessories OR price>=100, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("filter_NOT", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{
					"NOT": map[string]any{
						"category": map[string]string{"eq": "Footwear"},
					},
				}),
				WithLimit(10),
			))
			if hitCount(resp) != 1 {
				t.Errorf("expected 1 hit for NOT Footwear, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("sorting", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithSort([]map[string]string{{"field": "price", "direction": "ASC"}}),
				WithLimit(10),
			))
			if len(resp.Hits) < 4 {
				t.Fatalf("expected ≥4 hits, got %d", len(resp.Hits))
			}
			name, _ := resp.Hits[0].Node["name"].(string)
			if name != "Wool Socks" {
				t.Errorf("expected first hit to be Wool Socks (cheapest), got %q", name)
			}
		})

		t.Run("pagination", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithSort([]map[string]string{{"field": "price", "direction": "ASC"}}),
				WithLimit(2),
				WithOffset(2),
			))
			if len(resp.Hits) != 2 {
				t.Errorf("expected 2 hits with limit=2 offset=2, got %d", len(resp.Hits))
			}
		})

		if caps.HasFacets {
			t.Run("facets", func(t *testing.T) {
				t.Parallel()
				resp := LoadAndParse(t, source, BuildSearchInput(
					WithFacets([]string{"category"}),
					WithLimit(10),
				))
				if len(resp.Facets) == 0 {
					t.Fatal("expected at least 1 facet")
				}
				found := false
				for _, f := range resp.Facets {
					if f.Field == "category" {
						found = true
						if len(f.Values) < 2 {
							t.Errorf("expected ≥2 facet values for category, got %d", len(f.Values))
						}
					}
				}
				if !found {
					t.Error("expected category facet in response")
				}
			})
		}

		t.Run("identity_roundtrip", func(t *testing.T) {
			t.Parallel()
			var input []byte
			if caps.HasTextSearch {
				input = BuildSearchInput(WithQuery("running shoes"), WithLimit(1))
			} else {
				input = BuildSearchInput(WithLimit(1))
			}
			resp := LoadAndParse(t, source, input)
			if len(resp.Hits) == 0 {
				t.Fatal("expected at least 1 hit")
			}
			hit := resp.Hits[0]
			typename, _ := hit.Node["__typename"].(string)
			if typename != "Product" {
				t.Errorf("__typename = %q, want Product", typename)
			}
			if _, ok := hit.Node["id"]; !ok {
				t.Error("expected 'id' in hit node")
			}
		})

		t.Run("terms_IN", func(t *testing.T) {
			t.Parallel()
			resp := LoadAndParse(t, source, BuildSearchInput(
				WithFilter(map[string]any{
					"category": map[string]any{"in": []string{"Footwear", "Accessories"}},
				}),
				WithLimit(10),
			))
			if hitCount(resp) != 4 {
				t.Errorf("expected 4 hits for category IN [Footwear, Accessories], got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		if caps.HasPrefix {
			t.Run("prefix_filter", func(t *testing.T) {
				t.Parallel()
				resp := LoadAndParse(t, source, BuildSearchInput(
					WithFilter(map[string]any{
						"category": map[string]string{"startsWith": "Foot"},
					}),
					WithLimit(10),
				))
				if hitCount(resp) != 3 {
					t.Errorf("expected 3 hits for category startsWith 'Foot', got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
				}
			})
		}
	})

	// Mutation tests (sequential, fresh indices).
	t.Run("mutations", func(t *testing.T) {
		t.Run("index_single_document", func(t *testing.T) {
			freshIdx := indexFactory(t)
			doc := searchindex.EntityDocument{
				Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "99"}},
				Fields:   map[string]any{"name": "Sandals", "description": "Comfortable summer sandals", "category": "Footwear", "price": 49.99, "inStock": true},
			}
			if err := freshIdx.IndexDocument(context.Background(), doc); err != nil {
				t.Fatalf("IndexDocument: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			mutSource := CreateSource(t, freshIdx, config)
			resp := LoadAndParse(t, mutSource, BuildSearchInput(WithLimit(10)))
			if hitCount(resp) != 1 {
				t.Errorf("expected 1 hit after indexing single doc, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("upsert", func(t *testing.T) {
			freshIdx := indexFactory(t)
			if err := freshIdx.IndexDocuments(context.Background(), TestProducts()); err != nil {
				t.Fatalf("populate: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			// Re-index id=1 with updated name.
			updated := searchindex.EntityDocument{
				Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
				Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
			}
			if err := freshIdx.IndexDocument(context.Background(), updated); err != nil {
				t.Fatalf("IndexDocument (upsert): %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			mutSource := CreateSource(t, freshIdx, config)

			// Total count should still be 4.
			allResp := LoadAndParse(t, mutSource, BuildSearchInput(WithLimit(10)))
			if hitCount(allResp) != 4 {
				t.Errorf("expected 4 hits after upsert, got totalCount=%d len(hits)=%d", allResp.TotalCount, len(allResp.Hits))
			}

			// Verify updated name is findable.
			if caps.HasTextSearch {
				trailResp := LoadAndParse(t, mutSource, BuildSearchInput(WithQuery("trail"), WithLimit(10)))
				if hitCount(trailResp) < 1 {
					t.Errorf("expected ≥1 hit for 'trail' after upsert, got totalCount=%d", trailResp.TotalCount)
				}
				if len(trailResp.Hits) > 0 {
					name, _ := trailResp.Hits[0].Node["name"].(string)
					if name != "Trail Running Shoes" {
						t.Errorf("expected name %q, got %q", "Trail Running Shoes", name)
					}
				}
			}
		})

		t.Run("delete_single", func(t *testing.T) {
			freshIdx := indexFactory(t)
			if err := freshIdx.IndexDocuments(context.Background(), TestProducts()); err != nil {
				t.Fatalf("populate: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			if err := freshIdx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
				TypeName:  "Product",
				KeyFields: map[string]any{"id": "1"},
			}); err != nil {
				t.Fatalf("DeleteDocument: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			mutSource := CreateSource(t, freshIdx, config)
			resp := LoadAndParse(t, mutSource, BuildSearchInput(WithLimit(10)))
			if hitCount(resp) != 3 {
				t.Errorf("expected 3 hits after delete, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})

		t.Run("delete_batch", func(t *testing.T) {
			freshIdx := indexFactory(t)
			if err := freshIdx.IndexDocuments(context.Background(), TestProducts()); err != nil {
				t.Fatalf("populate: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			if err := freshIdx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
				{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
				{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			}); err != nil {
				t.Fatalf("DeleteDocuments: %v", err)
			}
			if hooks.WaitForIndex != nil {
				hooks.WaitForIndex(t)
			}

			mutSource := CreateSource(t, freshIdx, config)
			resp := LoadAndParse(t, mutSource, BuildSearchInput(WithLimit(10)))
			if hitCount(resp) != 2 {
				t.Errorf("expected 2 hits after batch delete, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
			}
		})
	})
}

// RunCursorTests runs cursor-based pagination tests against the given index.
// Products sorted by price ASC: Wool Socks ($12.99), Leather Belt ($35), Running Shoes ($89.99), Basketball Shoes ($129.99).
func RunCursorTests(t *testing.T, idx searchindex.Index, caps BackendCaps, hooks BackendHooks) {
	if err := idx.IndexDocuments(context.Background(), TestProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if hooks.WaitForIndex != nil {
		hooks.WaitForIndex(t)
	}

	config := CursorProductDatasourceConfig()
	source := CreateSource(t, idx, config)

	sortByPrice := WithSort([]map[string]string{{"field": "price", "direction": "ASC"}})

	t.Run("forward_page1", func(t *testing.T) {
		resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithFirst(1)))
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge, got %d", len(resp.Edges))
		}
		name, _ := resp.Edges[0].Node["name"].(string)
		if name != "Wool Socks" {
			t.Errorf("expected Wool Socks, got %q", name)
		}
		if !resp.PageInfo.HasNextPage {
			t.Error("expected hasNextPage=true")
		}
		if resp.PageInfo.HasPreviousPage {
			t.Error("expected hasPreviousPage=false")
		}
		if resp.PageInfo.EndCursor == nil || *resp.PageInfo.EndCursor == "" {
			t.Error("expected non-empty endCursor")
		}
	})

	t.Run("forward_page2", func(t *testing.T) {
		// Get page 1 to obtain cursor.
		page1 := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithFirst(1)))
		if len(page1.Edges) == 0 || page1.PageInfo.EndCursor == nil {
			t.Fatal("page1 has no edges or endCursor")
		}

		// Get page 2 using after cursor.
		resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithFirst(1), WithAfter(*page1.PageInfo.EndCursor)))
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge, got %d", len(resp.Edges))
		}
		name, _ := resp.Edges[0].Node["name"].(string)
		if name != "Leather Belt" {
			t.Errorf("expected Leather Belt, got %q", name)
		}
		if !resp.PageInfo.HasNextPage {
			t.Error("expected hasNextPage=true (2 more items)")
		}
		if !resp.PageInfo.HasPreviousPage {
			t.Error("expected hasPreviousPage=true (after cursor is set)")
		}
	})

	t.Run("forward_last_page", func(t *testing.T) {
		// Page through to the last page to verify hasNextPage=false.
		// Get all 4 in one page to simplify: first=4 should return all, no over-fetch extra.
		resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithFirst(4)))
		if len(resp.Edges) != 4 {
			t.Fatalf("expected 4 edges, got %d", len(resp.Edges))
		}
		if resp.PageInfo.HasNextPage {
			t.Error("expected hasNextPage=false for full result set")
		}
	})

	if caps.HasCursorPagination {
		t.Run("backward_page1", func(t *testing.T) {
			resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithLast(1)))
			if len(resp.Edges) != 1 {
				t.Fatalf("expected 1 edge, got %d", len(resp.Edges))
			}
			name, _ := resp.Edges[0].Node["name"].(string)
			if name != "Basketball Shoes" {
				t.Errorf("expected Basketball Shoes (most expensive), got %q", name)
			}
			if !resp.PageInfo.HasPreviousPage {
				t.Error("expected hasPreviousPage=true")
			}
			if resp.PageInfo.HasNextPage {
				t.Error("expected hasNextPage=false (no before cursor)")
			}
		})

		t.Run("backward_page2", func(t *testing.T) {
			// Get last page to obtain cursor.
			page1 := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithLast(1)))
			if len(page1.Edges) == 0 || page1.PageInfo.StartCursor == nil {
				t.Fatal("page1 has no edges or startCursor")
			}

			// Get previous page.
			resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithLast(1), WithBefore(*page1.PageInfo.StartCursor)))
			if len(resp.Edges) != 1 {
				t.Fatalf("expected 1 edge, got %d", len(resp.Edges))
			}
			name, _ := resp.Edges[0].Node["name"].(string)
			if name != "Running Shoes" {
				t.Errorf("expected Running Shoes ($89.99), got %q", name)
			}
			if !resp.PageInfo.HasPreviousPage {
				t.Error("expected hasPreviousPage=true (2 more items)")
			}
			if !resp.PageInfo.HasNextPage {
				t.Error("expected hasNextPage=true (before cursor is set)")
			}
		})
	}

	t.Run("cursor_identity", func(t *testing.T) {
		// Verify edges have __typename and key fields for entity resolution.
		resp := LoadAndParseConnection(t, source, BuildSearchInput(sortByPrice, WithFirst(10)))
		for i, edge := range resp.Edges {
			if edge.Cursor == "" {
				t.Errorf("edge[%d]: empty cursor", i)
			}
			typename, _ := edge.Node["__typename"].(string)
			if typename != "Product" {
				t.Errorf("edge[%d]: __typename=%q, want Product", i, typename)
			}
			if _, ok := edge.Node["id"]; !ok {
				t.Errorf("edge[%d]: missing key field 'id'", i)
			}
		}
	})
}

// RunGeoTests runs geo-spatial search tests against the given index.
// Products are at:
//
//	#1 Running Shoes:     New York (40.7128, -74.0060)
//	#2 Basketball Shoes:  Midtown Manhattan (40.7580, -73.9855) — ~5km from #1
//	#3 Leather Belt:      Los Angeles (34.0522, -118.2437) — ~3,940km from #1
//	#4 Wool Socks:        London (51.5074, -0.1278) — ~5,570km from #1
func RunGeoTests(t *testing.T, idx searchindex.Index, hooks BackendHooks) {
	if err := idx.IndexDocuments(context.Background(), GeoTestProducts()); err != nil {
		t.Fatalf("populate geo test data: %v", err)
	}
	if hooks.WaitForIndex != nil {
		hooks.WaitForIndex(t)
	}

	config := GeoProductDatasourceConfig()
	source := CreateSource(t, idx, config)

	t.Run("geo_distance_filter", func(t *testing.T) {
		t.Parallel()
		// Search within 10km of New York — should find Running Shoes (#1) and Basketball Shoes (#2).
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithFilter(map[string]any{
				"location_distance": map[string]any{
					"center":   map[string]any{"lat": 40.7128, "lon": -74.0060},
					"distance": "10km",
				},
			}),
			WithLimit(10),
		))
		if hitCount(resp) != 2 {
			t.Errorf("expected 2 hits within 10km of NYC, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
		}
	})

	t.Run("geo_distance_filter_wide", func(t *testing.T) {
		t.Parallel()
		// Search within 5000km of New York — should find Running Shoes, Basketball Shoes, and Leather Belt (LA).
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithFilter(map[string]any{
				"location_distance": map[string]any{
					"center":   map[string]any{"lat": 40.7128, "lon": -74.0060},
					"distance": "5000km",
				},
			}),
			WithLimit(10),
		))
		if hitCount(resp) != 3 {
			t.Errorf("expected 3 hits within 5000km of NYC, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
		}
	})

	t.Run("geo_bounding_box_filter", func(t *testing.T) {
		t.Parallel()
		// Bounding box around New York area — should find Running Shoes and Basketball Shoes.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithFilter(map[string]any{
				"location_boundingBox": map[string]any{
					"topLeft":     map[string]any{"lat": 41.0, "lon": -74.5},
					"bottomRight": map[string]any{"lat": 40.5, "lon": -73.5},
				},
			}),
			WithLimit(10),
		))
		if hitCount(resp) != 2 {
			t.Errorf("expected 2 hits in NYC bounding box, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
		}
	})

	t.Run("geo_distance_sort", func(t *testing.T) {
		t.Parallel()
		// Sort all products by distance from New York ASC.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithGeoSort("location", 40.7128, -74.0060, "ASC", "km"),
			WithLimit(10),
		))
		if len(resp.Hits) < 4 {
			t.Fatalf("expected >= 4 hits, got %d", len(resp.Hits))
		}
		// Nearest to NYC should be Running Shoes (in NYC) or Basketball Shoes (Midtown).
		name, _ := resp.Hits[0].Node["name"].(string)
		if name != "Running Shoes" {
			t.Errorf("expected first hit to be Running Shoes (nearest to NYC), got %q", name)
		}
		// Farthest should be Wool Socks (London).
		lastName, _ := resp.Hits[3].Node["name"].(string)
		if lastName != "Wool Socks" {
			t.Errorf("expected last hit to be Wool Socks (London, farthest), got %q", lastName)
		}
		// All hits should have geoDistance populated.
		for i, hit := range resp.Hits {
			if hit.GeoDistance == nil {
				t.Errorf("hit[%d]: expected geoDistance to be populated", i)
			}
		}
	})

	t.Run("geo_filter_combined_with_keyword", func(t *testing.T) {
		t.Parallel()
		// Combine geo filter with keyword filter: Footwear within 100km of NYC.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithFilter(map[string]any{
				"location_distance": map[string]any{
					"center":   map[string]any{"lat": 40.7128, "lon": -74.0060},
					"distance": "100km",
				},
				"category": map[string]string{"eq": "Footwear"},
			}),
			WithLimit(10),
		))
		if hitCount(resp) != 2 {
			t.Errorf("expected 2 Footwear hits near NYC, got totalCount=%d len(hits)=%d", resp.TotalCount, len(resp.Hits))
		}
	})
}

// RunFuzzyTests runs fuzzy matching / typo tolerance tests against the given index.
// Uses the standard 4 products (Running Shoes, Basketball Shoes, Leather Belt, Wool Socks).
func RunFuzzyTests(t *testing.T, idx searchindex.Index, hooks BackendHooks) {
	if err := idx.IndexDocuments(context.Background(), TestProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if hooks.WaitForIndex != nil {
		hooks.WaitForIndex(t)
	}

	config := ProductDatasourceConfig()
	source := CreateSource(t, idx, config)

	t.Run("fuzzy_low_finds_typo", func(t *testing.T) {
		t.Parallel()
		// "runing" is 1 edit away from "running" — fuzziness LOW should find it.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithQuery("runing"),
			WithFuzziness("LOW"),
			WithLimit(10),
		))
		if hitCount(resp) < 1 {
			t.Errorf("expected >=1 hit for 'runing' with fuzziness LOW, got totalCount=%d", resp.TotalCount)
		}
	})

	t.Run("fuzzy_exact_misses_typo", func(t *testing.T) {
		t.Parallel()
		// "runing" with fuzziness EXACT should find nothing.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithQuery("runing"),
			WithFuzziness("EXACT"),
			WithLimit(10),
		))
		if hitCount(resp) != 0 {
			t.Errorf("expected 0 hits for 'runing' with fuzziness EXACT, got totalCount=%d", resp.TotalCount)
		}
	})

	t.Run("fuzzy_high_finds_typo", func(t *testing.T) {
		t.Parallel()
		// "runnin" is 1 edit away — fuzziness HIGH (2 edits) should find it.
		resp := LoadAndParse(t, source, BuildSearchInput(
			WithQuery("runnin"),
			WithFuzziness("HIGH"),
			WithLimit(10),
		))
		if hitCount(resp) < 1 {
			t.Errorf("expected >=1 hit for 'runnin' with fuzziness HIGH, got totalCount=%d", resp.TotalCount)
		}
	})
}
