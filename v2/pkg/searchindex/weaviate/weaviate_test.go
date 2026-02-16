//go:build integration

package weaviate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func startWeaviate(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "semitechnologies/weaviate:1.27.0",
			ExposedPorts: []string{"8080/tcp"},
			Env: map[string]string{
				"AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED": "true",
				"PERSISTENCE_DATA_PATH":                   "/var/lib/weaviate",
				"DEFAULT_VECTORIZER_MODULE":                "none",
				"CLUSTER_HOSTNAME":                         "node1",
			},
			WaitingFor: wait.ForHTTP("/v1/.well-known/ready").WithPort("8080/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	return fmt.Sprintf("%s:%s", host, port.Port())
}

func newTestIndex(t *testing.T, host string) searchindex.Index {
	t.Helper()
	factory := NewFactory()
	schema := searchindex.IndexConfig{
		Name: "test_products",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		},
	}

	configJSON := []byte(fmt.Sprintf(`{"host":%q,"scheme":"http"}`, host))
	idx, err := factory.CreateIndex(context.Background(), "test_products", schema, configJSON)
	require.NoError(t, err)
	t.Cleanup(func() { idx.Close() })
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

	// Wait briefly for indexing to complete.
	time.Sleep(1 * time.Second)
}

func TestWeaviateFullLifecycle(t *testing.T) {
	host := startWeaviate(t)
	idx := newTestIndex(t, host)
	populateTestData(t, idx)

	t.Run("text search", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "shoes",
			Limit:     10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Hits), 2, "expected at least 2 hits for 'shoes'")
	})

	t.Run("text search with field restriction", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery:  "shoes",
			TextFields: []searchindex.TextFieldWeight{{Name: "name"}},
			Limit:      10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Hits), 2, "expected at least 2 hits for 'shoes' in name")
	})

	t.Run("term filter on keyword field", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 3, len(result.Hits), "expected 3 hits for category=Footwear")
	})

	t.Run("boolean filter", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: false},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Hits), "expected 1 hit for inStock=false")
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
		assert.Equal(t, 2, len(result.Hits), "expected 2 hits for price 30-100")
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
		assert.Equal(t, 3, len(result.Hits), "expected 3 hits for Footwear AND inStock")
	})

	t.Run("search hit identity", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "running shoes",
			Limit:     1,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result.Hits), 1)
		hit := result.Hits[0]
		assert.Equal(t, "Product", hit.Identity.TypeName)
		assert.Equal(t, "Product", hit.Representation["__typename"])
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
		assert.Equal(t, 4, len(result.Hits), "expected 4 hits for category=Footwear OR category=Accessories")
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
		assert.Equal(t, 1, len(result.Hits), "expected 1 hit for NOT category=Footwear")
	})

	t.Run("sorting by price ascending", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit: 10,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result.Hits), 1)
		assert.Equal(t, "Wool Socks", result.Hits[0].Representation["name"], "cheapest product should be Wool Socks")
	})

	t.Run("pagination", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  2,
			Offset: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Hits), "expected 2 hits with limit=2 offset=2")
	})

	t.Run("TypeName filter", func(t *testing.T) {
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TypeName: "Product",
			Limit:    10,
		})
		require.NoError(t, err)
		assert.Equal(t, 4, len(result.Hits), "expected 4 hits for TypeName=Product")
	})

	t.Run("single IndexDocument", func(t *testing.T) {
		doc := searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "5"}},
			Fields:   map[string]any{"name": "Hiking Boots", "description": "Durable boots for trails", "category": "Footwear", "price": 149.99, "inStock": true},
		}
		err := idx.IndexDocument(context.Background(), doc)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "hiking boots",
			Limit:     10,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result.Hits), 1, "expected at least 1 hit for 'hiking boots'")

		found := false
		for _, hit := range result.Hits {
			if hit.Representation["name"] == "Hiking Boots" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find 'Hiking Boots' in search results")
	})

	t.Run("upsert overwrites existing document", func(t *testing.T) {
		// Re-index product 1 with a new name.
		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for trail running", "category": "Footwear", "price": 99.99, "inStock": true},
		})
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Total count should still be 5 (4 base + 1 from single IndexDocument test).
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 5, len(result.Hits), "expected 5 documents (upsert should not duplicate)")

		// Search for the updated name.
		result, err = idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "trail running",
			Limit:     1,
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.Hits)
		assert.Contains(t, result.Hits[0].Representation["name"], "Trail")
	})

	t.Run("delete single document", func(t *testing.T) {
		err := idx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "4"},
		})
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 3, len(result.Hits), "expected 3 Footwear hits after deleting Wool Socks")
	})

	t.Run("delete multiple documents", func(t *testing.T) {
		err := idx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
			{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
		})
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Hits), "expected 2 documents after batch delete")
	})
}

func TestDeterministicUUID(t *testing.T) {
	id1 := deterministicUUID("Product:id=1")
	id2 := deterministicUUID("Product:id=1")
	id3 := deterministicUUID("Product:id=2")

	assert.Equal(t, id1, id2, "same input should produce same UUID")
	assert.NotEqual(t, id1, id3, "different inputs should produce different UUIDs")
	assert.Len(t, id1, 36, "UUID should be 36 characters")
}

func TestDocumentIDString(t *testing.T) {
	id := documentIDString(searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	})
	expected := "Product:id=123,sku=ABC"
	assert.Equal(t, expected, id)
}

func TestToClassName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"products", "Products"},
		{"my_index", "My_index"},
		{"test-data", "Test_data"},
		{"Products", "Products"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, toClassName(tt.input))
		})
	}
}
