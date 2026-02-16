//go:build integration

package elasticsearch

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

// startElasticsearch spins up an Elasticsearch container and returns the
// base URL (e.g. "http://localhost:49200") plus a cleanup function.
func startElasticsearch(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:8.13.4",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
			"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").
			WithPort("9200/tcp").
			WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start elasticsearch container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9200/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func newTestIndex(t *testing.T, baseURL string) searchindex.Index {
	t.Helper()

	factory := NewFactory()
	schema := searchindex.IndexConfig{
		Name: "test-products",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "description", Type: searchindex.FieldTypeText},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true},
		},
	}

	cfg := Config{
		Addresses: []string{baseURL},
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	// Use a unique index name per test to avoid collisions.
	indexName := fmt.Sprintf("test-products-%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, schema, cfgJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
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
	if err := idx.IndexDocuments(context.Background(), docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	// Elasticsearch is near-real-time; wait for a refresh.
	time.Sleep(2 * time.Second)
}

func TestFullLifecycle(t *testing.T) {
	baseURL := startElasticsearch(t)

	t.Run("create index and batch index", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		// Verify all 4 documents are searchable.
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 documents, got %d", result.TotalCount)
		}
	})

	t.Run("text search", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "shoes",
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 2 {
			t.Errorf("expected at least 2 hits for 'shoes', got %d", result.TotalCount)
		}
	})

	t.Run("text search with field restriction", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery:  "shoes",
			TextFields: []searchindex.TextFieldWeight{{Name: "name"}},
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 2 {
			t.Errorf("expected at least 2 hits for 'shoes' in name, got %d", result.TotalCount)
		}
	})

	t.Run("term filter on keyword field", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for category=Footwear, got %d", result.TotalCount)
		}
	})

	t.Run("boolean filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: false},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 1 {
			t.Errorf("expected 1 hit for inStock=false, got %d", result.TotalCount)
		}
	})

	t.Run("numeric range filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 2 {
			t.Errorf("expected 2 hits for price 30-100, got %d", result.TotalCount)
		}
	})

	t.Run("prefix filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Prefix: &searchindex.PrefixFilter{Field: "category", Value: "Foot"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for category prefix 'Foot', got %d", result.TotalCount)
		}
	})

	t.Run("AND filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for Footwear AND inStock, got %d", result.TotalCount)
		}
	})

	t.Run("OR filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		// Accessories=1 (Leather Belt) + price>=100=1 (Basketball Shoes) = 2
		if result.TotalCount != 2 {
			t.Errorf("expected 2 hits for Accessories OR price>=100, got %d", result.TotalCount)
		}
	})

	t.Run("NOT filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
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
		if result.TotalCount != 1 {
			t.Errorf("expected 1 hit for NOT Footwear, got %d", result.TotalCount)
		}
	})

	t.Run("sorting", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 4 {
			t.Fatalf("expected 4 hits, got %d", result.TotalCount)
		}
		// First hit should be cheapest (Wool Socks at 12.99).
		if name, _ := result.Hits[0].Representation["name"].(string); name != "Wool Socks" {
			t.Errorf("expected first hit to be Wool Socks (cheapest), got %v", result.Hits[0].Representation["name"])
		}
	})

	t.Run("pagination", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Sort:   []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit:  2,
			Offset: 2,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(result.Hits) != 2 {
			t.Errorf("expected 2 hits with offset, got %d", len(result.Hits))
		}
	})

	t.Run("facets", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Facets: []searchindex.FacetRequest{{Field: "category", Size: 10}},
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		facet, ok := result.Facets["category"]
		if !ok {
			t.Fatal("expected category facet")
		}
		if len(facet.Values) < 2 {
			t.Errorf("expected at least 2 facet values, got %d", len(facet.Values))
		}
	})

	t.Run("search hit identity", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "running shoes",
			Limit:     1,
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

	t.Run("delete single document", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		err := idx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "1"},
		})
		if err != nil {
			t.Fatalf("DeleteDocument: %v", err)
		}

		// Wait for refresh.
		time.Sleep(2 * time.Second)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 3 {
			t.Errorf("expected 3 documents after delete, got %d", result.TotalCount)
		}
	})

	t.Run("delete multiple documents", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		err := idx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
			{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
		})
		if err != nil {
			t.Fatalf("DeleteDocuments: %v", err)
		}

		// Wait for refresh.
		time.Sleep(2 * time.Second)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 2 {
			t.Errorf("expected 2 documents after batch delete, got %d", result.TotalCount)
		}
	})

	t.Run("TypeName filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TypeName: "Product",
			Limit:    10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits for TypeName=Product, got %d", result.TotalCount)
		}
	})

	t.Run("terms (IN) filter", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits for category IN [Footwear, Accessories], got %d", result.TotalCount)
		}
	})

	t.Run("IndexDocument single", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)

		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "100"}},
			Fields:   map[string]any{"name": "Flip Flops", "description": "Casual summer footwear", "category": "Footwear", "price": 19.99, "inStock": true},
		})
		if err != nil {
			t.Fatalf("IndexDocument: %v", err)
		}

		// Wait for refresh.
		time.Sleep(2 * time.Second)

		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 1 {
			t.Errorf("expected 1 document after IndexDocument, got %d", result.TotalCount)
		}
	})

	t.Run("upsert document", func(t *testing.T) {
		idx := newTestIndex(t, baseURL)
		populateTestData(t, idx)

		// Re-index product id="1" with an updated name.
		err := idx.IndexDocument(context.Background(), searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
		})
		if err != nil {
			t.Fatalf("IndexDocument (upsert): %v", err)
		}

		// Wait for refresh.
		time.Sleep(2 * time.Second)

		// Total count should still be 4 (upsert, not insert).
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 documents after upsert, got %d", result.TotalCount)
		}

		// Search for the updated name to verify the change took effect.
		result, err = idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "trail",
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("Search for 'trail': %v", err)
		}
		if result.TotalCount < 1 {
			t.Errorf("expected at least 1 hit for 'trail' after upsert, got %d", result.TotalCount)
		}
	})
}

func TestDocumentID(t *testing.T) {
	id := documentID(searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	})
	// Keys should be sorted alphabetically.
	expected := "Product:id=123,sku=ABC"
	if id != expected {
		t.Errorf("documentID = %q, want %q", id, expected)
	}
}
