//go:build integration

package meilisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

const testMasterKey = "test-master-key"

// startMeilisearch starts a Meilisearch container and returns the host URL and a cleanup function.
func startMeilisearch(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "getmeili/meilisearch:v1.6",
		ExposedPorts: []string{"7700/tcp"},
		Env: map[string]string{
			"MEILI_MASTER_KEY": testMasterKey,
		},
		WaitingFor: wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Meilisearch container")
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "7700")
	require.NoError(t, err)

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func newTestIndex(t *testing.T, meiliHost string) searchindex.Index {
	t.Helper()
	factory := NewFactory()
	schema := searchindex.IndexConfig{
		Name: "test",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		},
	}

	cfg := Config{
		Host:   meiliHost,
		APIKey: testMasterKey,
	}
	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	// Use a unique index name per test to avoid collisions.
	indexName := fmt.Sprintf("test_%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, schema, cfgJSON)
	require.NoError(t, err, "CreateIndex")
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func populateTestData(t *testing.T, idx searchindex.Index) {
	t.Helper()
	docs := []searchindex.EntityDocument{
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
	err := idx.IndexDocuments(context.Background(), docs)
	require.NoError(t, err, "IndexDocuments")
}

func TestMeilisearchLifecycle(t *testing.T) {
	meiliHost := startMeilisearch(t)

	t.Run("full lifecycle", func(t *testing.T) {
		idx := newTestIndex(t, meiliHost)
		populateTestData(t, idx)

		t.Run("text search", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				TextQuery: "shoes",
				Limit:     10,
			})
			require.NoError(t, err)
			assert.GreaterOrEqual(t, result.TotalCount, 2, "expected at least 2 hits for 'shoes'")
		})

		t.Run("text search with field restriction", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				TextQuery:  "shoes",
				TextFields: []searchindex.TextFieldWeight{{Name: "name"}},
				Limit:      10,
			})
			require.NoError(t, err)
			assert.GreaterOrEqual(t, result.TotalCount, 2, "expected at least 2 hits for 'shoes' in name")
		})

		t.Run("term filter on keyword field", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 3, result.TotalCount, "expected 3 hits for category=Footwear")
		})

		t.Run("boolean filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Term: &searchindex.TermFilter{Field: "inStock", Value: false},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 1, result.TotalCount, "expected 1 hit for inStock=false")
		})

		t.Run("numeric range filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Range: &searchindex.RangeFilter{
						Field: "price",
						GTE:   30.0,
						LTE:   100.0,
					},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 2, result.TotalCount, "expected 2 hits for price 30-100")
		})

		t.Run("AND filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					And: []*searchindex.Filter{
						{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
						{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
					},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 3, result.TotalCount, "expected 3 hits for Footwear AND inStock")
		})

		t.Run("OR filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Or: []*searchindex.Filter{
						{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
						{Term: &searchindex.TermFilter{Field: "category", Value: "Accessories"}},
					},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 4, result.TotalCount, "expected 4 hits for Footwear OR Accessories")
		})

		t.Run("NOT filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Not: &searchindex.Filter{
						Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
					},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 1, result.TotalCount, "expected 1 hit for NOT Footwear")
		})

		t.Run("terms filter (IN)", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Terms: &searchindex.TermsFilter{
						Field:  "category",
						Values: []any{"Footwear", "Accessories"},
					},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 4, result.TotalCount, "expected 4 hits for category IN [Footwear, Accessories]")
		})

		t.Run("sorting ascending", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
				Limit: 10,
			})
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(result.Hits), 4)
			// First hit should be cheapest (Wool Socks at 12.99).
			assert.Equal(t, "Wool Socks", result.Hits[0].Representation["name"])
		})

		t.Run("sorting descending", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Sort:  []searchindex.SortField{{Field: "price", Ascending: false}},
				Limit: 10,
			})
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(result.Hits), 4)
			// First hit should be most expensive (Basketball Shoes at 129.99).
			assert.Equal(t, "Basketball Shoes", result.Hits[0].Representation["name"])
		})

		t.Run("pagination", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
				Limit:  2,
				Offset: 2,
			})
			require.NoError(t, err)
			assert.Equal(t, 2, len(result.Hits), "expected 2 hits with offset")
		})

		t.Run("facets", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Facets: []searchindex.FacetRequest{{Field: "category", Size: 10}},
				Limit:  10,
			})
			require.NoError(t, err)
			facet, ok := result.Facets["category"]
			require.True(t, ok, "expected category facet")
			assert.GreaterOrEqual(t, len(facet.Values), 2, "expected at least 2 facet values")
		})

		t.Run("search hit identity", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				TextQuery: "running shoes",
				Limit:     1,
			})
			require.NoError(t, err)
			require.NotEmpty(t, result.Hits, "expected at least 1 hit")
			hit := result.Hits[0]
			assert.Equal(t, "Product", hit.Identity.TypeName)
			assert.Equal(t, "Product", hit.Representation["__typename"])
		})

		t.Run("type name filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				TypeName: "Product",
				Limit:    10,
			})
			require.NoError(t, err)
			assert.Equal(t, 4, result.TotalCount, "expected 4 products")
		})

		t.Run("prefix filter is unsupported", func(t *testing.T) {
			_, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Prefix: &searchindex.PrefixFilter{Field: "category", Value: "Foot"},
				},
				Limit: 10,
			})
			require.Error(t, err, "prefix filter should return an error in Meilisearch")
			assert.Contains(t, err.Error(), "prefix filter is not supported")
		})

		t.Run("exists filter", func(t *testing.T) {
			result, err := idx.Search(context.Background(), searchindex.SearchRequest{
				Filter: &searchindex.Filter{
					Exists: &searchindex.ExistsFilter{Field: "category"},
				},
				Limit: 10,
			})
			require.NoError(t, err)
			assert.Equal(t, 4, result.TotalCount, "expected 4 hits where category exists")
		})

		t.Run("upsert overwrites", func(t *testing.T) {
			// Re-index product id="1" with an updated name.
			err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
				Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
				Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
			})
			require.NoError(t, err, "upsert IndexDocument")

			// Total count should still be 4 (upsert, not insert).
			allResult, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
			require.NoError(t, err)
			assert.Equal(t, 4, allResult.TotalCount, "expected 4 documents after upsert (not 5)")

			// Searching for "trail" should find the updated document.
			trailResult, err := idx.Search(context.Background(), searchindex.SearchRequest{
				TextQuery: "trail",
				Limit:     10,
			})
			require.NoError(t, err)
			require.NotEmpty(t, trailResult.Hits, "expected at least 1 hit for 'trail'")

			found := false
			for _, hit := range trailResult.Hits {
				if hit.Representation["name"] == "Trail Running Shoes" {
					found = true
					break
				}
			}
			assert.True(t, found, "expected to find 'Trail Running Shoes' in search results")
		})
	})

	t.Run("delete documents", func(t *testing.T) {
		idx := newTestIndex(t, meiliHost)
		populateTestData(t, idx)

		// Delete Running Shoes.
		err := idx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "1"},
		})
		require.NoError(t, err, "DeleteDocument")

		// Should now have 3 documents.
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount, "expected 3 documents after delete")
	})

	t.Run("batch delete documents", func(t *testing.T) {
		idx := newTestIndex(t, meiliHost)
		populateTestData(t, idx)

		// Delete two documents.
		err := idx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
			{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
		})
		require.NoError(t, err, "DeleteDocuments")

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount, "expected 2 documents after batch delete")
	})

	t.Run("index single document", func(t *testing.T) {
		idx := newTestIndex(t, meiliHost)

		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "100"}},
			Fields:   map[string]any{"name": "Single Item", "category": "Test", "price": 9.99, "inStock": true},
		})
		require.NoError(t, err, "IndexDocument")

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "Single Item",
			Limit:     10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalCount, 1, "expected at least 1 hit")
	})
}

func TestDocumentID(t *testing.T) {
	id := documentID(searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	})
	// Keys should be sorted alphabetically, using Meilisearch-safe characters.
	expected := "Product_id-123_sku-ABC"
	assert.Equal(t, expected, id)
}

func TestFilterTranslation(t *testing.T) {
	tests := []struct {
		name     string
		filter   *searchindex.Filter
		expected string
	}{
		{
			name:     "term string",
			filter:   &searchindex.Filter{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
			expected: `category = "Footwear"`,
		},
		{
			name:     "term numeric",
			filter:   &searchindex.Filter{Term: &searchindex.TermFilter{Field: "price", Value: 42.5}},
			expected: `price = 42.5`,
		},
		{
			name:     "term bool",
			filter:   &searchindex.Filter{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
			expected: `inStock = true`,
		},
		{
			name: "terms IN",
			filter: &searchindex.Filter{Terms: &searchindex.TermsFilter{
				Field: "category", Values: []any{"Footwear", "Accessories"},
			}},
			expected: `category IN ["Footwear", "Accessories"]`,
		},
		{
			name: "range GTE and LTE",
			filter: &searchindex.Filter{Range: &searchindex.RangeFilter{
				Field: "price", GTE: 10.0, LTE: 100.0,
			}},
			expected: `price >= 10 AND price <= 100`,
		},
		{
			name:     "exists",
			filter:   &searchindex.Filter{Exists: &searchindex.ExistsFilter{Field: "category"}},
			expected: `category EXISTS`,
		},
		{
			name: "AND",
			filter: &searchindex.Filter{And: []*searchindex.Filter{
				{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
				{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
			}},
			expected: `(category = "Footwear") AND (inStock = true)`,
		},
		{
			name: "OR",
			filter: &searchindex.Filter{Or: []*searchindex.Filter{
				{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
				{Term: &searchindex.TermFilter{Field: "category", Value: "Accessories"}},
			}},
			expected: `(category = "Footwear") OR (category = "Accessories")`,
		},
		{
			name: "NOT",
			filter: &searchindex.Filter{Not: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			}},
			expected: `NOT (category = "Footwear")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translateFilter(tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
