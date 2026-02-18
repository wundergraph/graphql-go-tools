//go:build integration

package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func startQdrant(t *testing.T) (host string, port int) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "qdrant/qdrant:v1.12.5",
		ExposedPorts: []string{"6333/tcp"},
		WaitingFor:   wait.ForHTTP("/healthz").WithPort("6333/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start qdrant container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate qdrant container: %v", err)
		}
	})

	mappedHost, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "6333")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return mappedHost, mappedPort.Int()
}

func newTestIndex(t *testing.T, host string, port int, name string, fields []searchindex.FieldConfig) searchindex.Index {
	t.Helper()
	factory := NewFactory()
	schema := searchindex.IndexConfig{
		Name:   name,
		Fields: fields,
	}
	cfg, _ := json.Marshal(Config{Host: host, Port: port})
	idx, err := factory.CreateIndex(context.Background(), name, schema, cfg)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestFullLifecycle(t *testing.T) {
	host, port := startQdrant(t)
	ctx := context.Background()

	fields := []searchindex.FieldConfig{
		{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
		{Name: "description", Type: searchindex.FieldTypeText},
		{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
		{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
		{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		{Name: "embedding", Type: searchindex.FieldTypeVector, Dimensions: 4},
	}

	idx := newTestIndex(t, host, port, "test_products", fields)

	// Index documents
	docs := []searchindex.EntityDocument{
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
			Vectors:  map[string][]float32{"embedding": {0.1, 0.2, 0.3, 0.4}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			Fields:   map[string]any{"name": "Basketball Shoes", "description": "High-top basketball sneakers", "category": "Footwear", "price": 129.99, "inStock": true},
			Vectors:  map[string][]float32{"embedding": {0.15, 0.25, 0.35, 0.45}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
			Fields:   map[string]any{"name": "Leather Belt", "description": "Genuine leather dress belt", "category": "Accessories", "price": 35.00, "inStock": false},
			Vectors:  map[string][]float32{"embedding": {0.9, 0.8, 0.7, 0.6}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "4"}},
			Fields:   map[string]any{"name": "Wool Socks", "description": "Warm wool socks for winter", "category": "Footwear", "price": 12.99, "inStock": true},
			Vectors:  map[string][]float32{"embedding": {0.2, 0.3, 0.4, 0.5}},
		},
	}

	if err := idx.IndexDocuments(ctx, docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	// Wait briefly for indexing to complete.
	time.Sleep(1 * time.Second)

	t.Run("vector search", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector:      []float32{0.1, 0.2, 0.3, 0.4},
			VectorField: "embedding",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 4 {
			t.Errorf("expected 4 hits, got %d", len(result.Hits))
		}
		// The closest vector should be Running Shoes (exact match)
		if len(result.Hits) > 0 {
			t.Logf("top hit: %v (score: %f)", result.Hits[0].Representation["name"], result.Hits[0].Score)
		}
	})

	t.Run("vector search with type filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector:   []float32{0.1, 0.2, 0.3, 0.4},
			TypeName: "Product",
			Limit:    10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 4 {
			t.Errorf("expected 4 hits, got %d", len(result.Hits))
		}
		for _, hit := range result.Hits {
			if hit.Identity.TypeName != "Product" {
				t.Errorf("expected TypeName=Product, got %q", hit.Identity.TypeName)
			}
		}
	})

	t.Run("vector search with term filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 3 {
			t.Errorf("expected 3 hits for category=Footwear, got %d", len(result.Hits))
		}
	})

	t.Run("scroll search with no vector", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 4 {
			t.Errorf("expected 4 hits, got %d", len(result.Hits))
		}
	})

	t.Run("term filter on keyword", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Accessories"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 1 {
			t.Errorf("expected 1 hit for category=Accessories, got %d", len(result.Hits))
		}
		if len(result.Hits) > 0 {
			if result.Hits[0].Representation["name"] != "Leather Belt" {
				t.Errorf("expected Leather Belt, got %v", result.Hits[0].Representation["name"])
			}
		}
	})

	t.Run("boolean filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: false},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 1 {
			t.Errorf("expected 1 hit for inStock=false, got %d", len(result.Hits))
		}
	})

	t.Run("numeric range filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Range: &searchindex.RangeFilter{
					Field: "price",
					GTE:   30.0,
					LTE:   100.0,
				},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 2 {
			t.Errorf("expected 2 hits for price 30-100, got %d", len(result.Hits))
			for _, h := range result.Hits {
				t.Logf("  hit: %v price=%v", h.Representation["name"], h.Representation["price"])
			}
		}
	})

	t.Run("AND filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				And: []*searchindex.Filter{
					{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
					{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
				},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 3 {
			t.Errorf("expected 3 hits for Footwear AND inStock, got %d", len(result.Hits))
		}
	})

	t.Run("OR filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Or: []*searchindex.Filter{
					{Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"}},
					{Term: &searchindex.TermFilter{Field: "category", Value: "Accessories"}},
				},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 4 {
			t.Errorf("expected 4 hits for Footwear OR Accessories, got %d", len(result.Hits))
		}
	})

	t.Run("NOT filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Not: &searchindex.Filter{
					Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
				},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 1 {
			t.Errorf("expected 1 hit for NOT Footwear, got %d", len(result.Hits))
		}
	})

	t.Run("terms filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Filter: &searchindex.Filter{
				Terms: &searchindex.TermsFilter{
					Field:  "category",
					Values: []any{"Footwear", "Accessories"},
				},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 4 {
			t.Errorf("expected 4 hits for terms filter, got %d", len(result.Hits))
		}
	})

	t.Run("search hit identity", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Limit:  1,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) == 0 {
			t.Fatal("expected at least 1 hit")
		}
		hit := result.Hits[0]
		if hit.Identity.TypeName != "Product" {
			t.Errorf("TypeName = %q, want %q", hit.Identity.TypeName, "Product")
		}
		if hit.Representation["__typename"] != "Product" {
			t.Errorf("__typename = %v, want %q", hit.Representation["__typename"], "Product")
		}
	})

	t.Run("prefix filter on category", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Prefix: &searchindex.PrefixFilter{Field: "category", Value: "Foot"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 3 {
			t.Errorf("expected 3 hits for category prefix 'Foot', got %d", len(result.Hits))
			for _, h := range result.Hits {
				t.Logf("  hit: %v category=%v", h.Representation["name"], h.Representation["category"])
			}
		}
		for _, hit := range result.Hits {
			cat, _ := hit.Representation["category"].(string)
			if cat != "Footwear" {
				t.Errorf("expected category=Footwear, got %q", cat)
			}
		}
	})

	t.Run("index single document", func(t *testing.T) {
		singleDoc := searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "5"}},
			Fields:   map[string]any{"name": "Canvas Sneakers", "description": "Casual canvas shoes", "category": "Footwear", "price": 49.99, "inStock": true},
			Vectors:  map[string][]float32{"embedding": {0.12, 0.22, 0.32, 0.42}},
		}
		if err := idx.IndexDocument(ctx, singleDoc); err != nil {
			t.Fatalf("IndexDocument: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.12, 0.22, 0.32, 0.42},
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 5 {
			t.Errorf("expected 5 hits after indexing single doc, got %d", len(result.Hits))
		}
		found := false
		for _, hit := range result.Hits {
			if hit.Representation["name"] == "Canvas Sneakers" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find Canvas Sneakers in search results")
		}
	})

	t.Run("upsert overwrites existing document", func(t *testing.T) {
		// Re-index id=5 with different data but same identity.
		updatedDoc := searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "5"}},
			Fields:   map[string]any{"name": "Updated Sneakers", "description": "Updated description", "category": "Footwear", "price": 59.99, "inStock": false},
			Vectors:  map[string][]float32{"embedding": {0.12, 0.22, 0.32, 0.42}},
		}
		if err := idx.IndexDocuments(ctx, []searchindex.EntityDocument{updatedDoc}); err != nil {
			t.Fatalf("IndexDocuments (upsert): %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		// Total count should still be 5 (no duplicate created).
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 5 {
			t.Errorf("expected 5 hits after upsert (no duplicate), got %d", len(result.Hits))
		}

		// Verify the document was actually updated.
		result, err = idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.12, 0.22, 0.32, 0.42},
			Limit:  1,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) == 0 {
			t.Fatal("expected at least 1 hit")
		}
		top := result.Hits[0]
		if top.Representation["name"] != "Updated Sneakers" {
			t.Errorf("expected name='Updated Sneakers' after upsert, got %v", top.Representation["name"])
		}
		if top.Representation["price"] != 59.99 {
			t.Errorf("expected price=59.99 after upsert, got %v", top.Representation["price"])
		}
	})

	t.Run("pagination with offset", func(t *testing.T) {
		// Fetch all results to know the full set.
		allResult, err := idx.Search(ctx, searchindex.SearchRequest{
			Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search (all): %v", err)
		}
		totalCount := len(allResult.Hits)
		if totalCount < 3 {
			t.Fatalf("expected at least 3 documents for pagination test, got %d", totalCount)
		}

		// Fetch page 1: limit=2, offset=0.
		page1, err := idx.Search(ctx, searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  2,
			Offset: 0,
		})
		if err != nil {
			t.Fatalf("Search (page1): %v", err)
		}
		if len(page1.Hits) != 2 {
			t.Errorf("expected 2 hits on page1, got %d", len(page1.Hits))
		}

		// Fetch page 2: limit=2, offset=2.
		page2, err := idx.Search(ctx, searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  2,
			Offset: 2,
		})
		if err != nil {
			t.Fatalf("Search (page2): %v", err)
		}
		if len(page2.Hits) != 2 {
			t.Errorf("expected 2 hits on page2, got %d", len(page2.Hits))
		}

		// Verify no overlap between page1 and page2.
		if len(page1.Hits) >= 2 && len(page2.Hits) >= 1 {
			page1Names := map[string]bool{}
			for _, h := range page1.Hits {
				name, _ := h.Representation["name"].(string)
				page1Names[name] = true
			}
			for _, h := range page2.Hits {
				name, _ := h.Representation["name"].(string)
				if page1Names[name] {
					t.Errorf("page2 hit %q also appeared in page1 (overlap)", name)
				}
			}
		}

		// Offset beyond total should return no results.
		empty, err := idx.Search(ctx, searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  10,
			Offset: 100,
		})
		if err != nil {
			t.Fatalf("Search (offset beyond total): %v", err)
		}
		if len(empty.Hits) != 0 {
			t.Errorf("expected 0 hits for offset beyond total, got %d", len(empty.Hits))
		}
	})

	// Clean up the extra document before the delete tests proceed.
	if err := idx.DeleteDocument(ctx, searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "5"},
	}); err != nil {
		t.Fatalf("cleanup delete id=5: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	t.Run("delete single document", func(t *testing.T) {
		err := idx.DeleteDocument(ctx, searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "1"},
		})
		if err != nil {
			t.Fatalf("DeleteDocument: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search after delete: %v", err)
		}
		if len(result.Hits) != 3 {
			t.Errorf("expected 3 hits after deleting one doc, got %d", len(result.Hits))
		}
	})

	t.Run("delete multiple documents", func(t *testing.T) {
		err := idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{
			{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
		})
		if err != nil {
			t.Fatalf("DeleteDocuments: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search after batch delete: %v", err)
		}
		if len(result.Hits) != 1 {
			t.Errorf("expected 1 hit after batch delete, got %d", len(result.Hits))
		}
	})
}

func TestDocumentIDHashDeterministic(t *testing.T) {
	id := searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	}

	h1 := documentIDHash(id)
	h2 := documentIDHash(id)
	if h1 != h2 {
		t.Errorf("documentIDHash not deterministic: %d != %d", h1, h2)
	}

	// Same fields in different insertion order should produce the same hash.
	id2 := searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"sku": "ABC", "id": "123"},
	}
	h3 := documentIDHash(id2)
	if h1 != h3 {
		t.Errorf("documentIDHash not stable across key order: %d != %d", h1, h3)
	}
}

func TestDocumentIDString(t *testing.T) {
	id := searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	}
	s := documentIDString(id)
	expected := "Product:id=123,sku=ABC"
	if s != expected {
		t.Errorf("documentIDString = %q, want %q", s, expected)
	}
}

func TestNoVectorFields(t *testing.T) {
	host, port := startQdrant(t)
	ctx := context.Background()

	fields := []searchindex.FieldConfig{
		{Name: "name", Type: searchindex.FieldTypeText, Filterable: true},
		{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true},
	}

	idx := newTestIndex(t, host, port, fmt.Sprintf("test_no_vector_%d", time.Now().UnixNano()), fields)

	// Should be able to index documents without vectors.
	err := idx.IndexDocument(ctx, searchindex.EntityDocument{
		Identity: searchindex.DocumentIdentity{TypeName: "Article", KeyFields: map[string]any{"id": "1"}},
		Fields:   map[string]any{"name": "Test Article", "category": "News"},
	})
	if err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Scroll search should work without vectors.
	result, err := idx.Search(ctx, searchindex.SearchRequest{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(result.Hits))
	}
}
