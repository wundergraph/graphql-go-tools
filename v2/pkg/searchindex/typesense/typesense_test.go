//go:build integration

package typesense

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

const testAPIKey = "test-api-key"

// startTypesense launches a Typesense container and returns the host, port, and
// a cleanup function.
func startTypesense(t *testing.T) (host string, port int) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "typesense/typesense:27.1",
		ExposedPorts: []string{"8108/tcp"},
		Env: map[string]string{
			"TYPESENSE_API_KEY":  testAPIKey,
			"TYPESENSE_DATA_DIR": "/data",
		},
		Tmpfs:      map[string]string{"/data": ""},
		WaitingFor: wait.ForHTTP("/health").WithPort("8108/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	mappedPort, err := container.MappedPort(ctx, "8108")
	require.NoError(t, err)

	hostIP, err := container.Host(ctx)
	require.NoError(t, err)

	return hostIP, mappedPort.Int()
}

func newTestIndex(t *testing.T, host string, port int) searchindex.Index {
	t.Helper()

	factory := NewFactory()
	schema := searchindex.IndexConfig{
		Name: "test_products",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: false},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		},
	}

	cfgJSON, _ := json.Marshal(Config{
		Host:     host,
		Port:     port,
		APIKey:   testAPIKey,
		Protocol: "http",
	})

	// Use a unique collection name per test to avoid conflicts.
	collectionName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	idx, err := factory.CreateIndex(context.Background(), collectionName, schema, cfgJSON)
	require.NoError(t, err)

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
	require.NoError(t, err)
}

func TestTypesenseLifecycle(t *testing.T) {
	host, port := startTypesense(t)

	t.Run("index and text search", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "shoes",
			Limit:     10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalCount, 2, "expected at least 2 hits for 'shoes'")
	})

	t.Run("text search with field restriction", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery:  "shoes",
			TextFields: []searchindex.TextFieldWeight{{Name: "name"}},
			Limit:      10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalCount, 2)
	})

	t.Run("term filter on keyword", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount)
	})

	t.Run("boolean filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: false},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.TotalCount)
	})

	t.Run("numeric range filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

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
		assert.Equal(t, 2, result.TotalCount)
	})

	t.Run("AND filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

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
		assert.Equal(t, 3, result.TotalCount)
	})

	t.Run("OR filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Or: []*searchindex.Filter{
					{Term: &searchindex.TermFilter{Field: "category", Value: "Accessories"}},
					{Range: &searchindex.RangeFilter{Field: "price", GTE: 100.0}},
				},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount) // Belt + Basketball Shoes
	})

	t.Run("sorting", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit: 10,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result.Hits), 4)
		// First hit should be cheapest (Wool Socks at 12.99).
		assert.Equal(t, "Wool Socks", result.Hits[0].Representation["name"])
	})

	t.Run("pagination", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  2,
			Offset: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Hits))
	})

	t.Run("facets", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Facets: []searchindex.FacetRequest{{Field: "category", Size: 10}},
			Limit:  10,
		})
		require.NoError(t, err)
		facet, ok := result.Facets["category"]
		require.True(t, ok, "expected category facet")
		assert.GreaterOrEqual(t, len(facet.Values), 2)
	})

	t.Run("identity roundtrip", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "running shoes",
			Limit:     1,
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.Hits)

		hit := result.Hits[0]
		assert.Equal(t, "Product", hit.Identity.TypeName)
		assert.Equal(t, "Product", hit.Representation["__typename"])
	})

	t.Run("delete single document", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		err := idx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "1"},
		})
		require.NoError(t, err)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount)
	})

	t.Run("delete batch documents", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		err := idx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
			{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
		})
		require.NoError(t, err)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
	})

	t.Run("upsert overwrites existing document", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		// Re-index product 1 with a new name.
		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for trail running", "category": "Footwear", "price": 99.99, "inStock": true},
		})
		require.NoError(t, err)

		// Total count should still be 4.
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 4, result.TotalCount)

		// Search for the updated name.
		result, err = idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "trail running",
			Limit:     1,
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.Hits)
		assert.Contains(t, result.Hits[0].Representation["name"], "Trail")
	})

	t.Run("terms filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

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
		assert.Equal(t, 4, result.TotalCount) // All products
	})

	t.Run("type name filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TypeName: "Product",
			Limit:    10,
		})
		require.NoError(t, err)
		assert.Equal(t, 4, result.TotalCount)
	})

	t.Run("NOT filter", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Not: &searchindex.Filter{
					Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
				},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.TotalCount, "expected 1 hit (only Accessories after excluding Footwear)")
	})

	t.Run("prefix filter returns error", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		_, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Prefix: &searchindex.PrefixFilter{Field: "category", Value: "Foot"},
			},
			Limit: 10,
		})
		require.Error(t, err, "prefix filter should return an error because Typesense does not support it")
		assert.Contains(t, err.Error(), "prefix filter is not supported")
	})

	t.Run("exists filter returns error", func(t *testing.T) {
		idx := newTestIndex(t, host, port)
		populateTestData(t, idx)

		// Typesense does not support exists filter with empty value check.
		_, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Exists: &searchindex.ExistsFilter{Field: "category"},
			},
			Limit: 10,
		})
		require.Error(t, err, "exists filter should return an error in Typesense")
	})

	t.Run("single IndexDocument", func(t *testing.T) {
		idx := newTestIndex(t, host, port)

		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "99"}},
			Fields:   map[string]any{"name": "Hiking Boots", "description": "Sturdy boots for mountain trails", "category": "Footwear", "price": 149.99, "inStock": true},
		})
		require.NoError(t, err)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "hiking boots",
			Limit:     10,
		})
		require.NoError(t, err)
		require.Equal(t, 1, result.TotalCount, "expected exactly 1 hit for the single indexed document")
		assert.Equal(t, "Hiking Boots", result.Hits[0].Representation["name"])
		assert.Equal(t, "Product", result.Hits[0].Identity.TypeName)
	})
}
