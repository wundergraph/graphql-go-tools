//go:build integration

package pgvector

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// startPgvectorContainer starts a PostgreSQL container with the pgvector extension
// and returns a connected *sql.DB along with a cleanup function.
func startPgvectorContainer(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start pgvector container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Wait for the database to be ready.
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("database not ready: %v", err)
	}

	return db
}

func newTestIndex(t *testing.T, db *sql.DB) searchindex.Index {
	t.Helper()
	factory := NewFactory(db)
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
	idx, err := factory.CreateIndex(context.Background(), "test_products", schema, nil)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func newTestIndexWithVectors(t *testing.T, db *sql.DB) searchindex.Index {
	t.Helper()
	factory := NewFactory(db)
	schema := searchindex.IndexConfig{
		Name: "test_vec",
		Fields: []searchindex.FieldConfig{
			{Name: "name", Type: searchindex.FieldTypeText, Filterable: true, Sortable: true},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "embedding", Type: searchindex.FieldTypeVector, Dimensions: 3},
		},
	}
	idx, err := factory.CreateIndex(context.Background(), "test_vectors", schema, nil)
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
}

func populateVectorData(t *testing.T, idx searchindex.Index) {
	t.Helper()
	docs := []searchindex.EntityDocument{
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Running Shoes", "category": "Footwear", "price": 89.99},
			Vectors:  map[string][]float32{"embedding": {0.1, 0.2, 0.3}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
			Fields:   map[string]any{"name": "Basketball Shoes", "category": "Footwear", "price": 129.99},
			Vectors:  map[string][]float32{"embedding": {0.15, 0.25, 0.35}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
			Fields:   map[string]any{"name": "Leather Belt", "category": "Accessories", "price": 35.00},
			Vectors:  map[string][]float32{"embedding": {0.9, 0.8, 0.7}},
		},
		{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "4"}},
			Fields:   map[string]any{"name": "Wool Socks", "category": "Footwear", "price": 12.99},
			Vectors:  map[string][]float32{"embedding": {0.12, 0.22, 0.32}},
		},
	}
	if err := idx.IndexDocuments(context.Background(), docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}
}

func TestFullLifecycle(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndex(t, db)
	populateTestData(t, idx)

	ctx := context.Background()

	t.Run("text search", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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

	t.Run("term filter on keyword field", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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
		if result.TotalCount != 2 {
			t.Errorf("expected 2 hits for price 30-100, got %d", result.TotalCount)
		}
	})

	t.Run("prefix filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for Footwear AND inStock, got %d", result.TotalCount)
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
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits for Footwear OR Accessories, got %d", result.TotalCount)
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
		if result.TotalCount != 1 {
			t.Errorf("expected 1 hit for NOT Footwear, got %d", result.TotalCount)
		}
	})

	t.Run("sorting by price ascending", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Sort:  []searchindex.SortField{{Field: "price", Ascending: true}},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 4 {
			t.Fatalf("expected 4 hits, got %d", result.TotalCount)
		}
		if len(result.Hits) < 4 {
			t.Fatalf("expected 4 hits in results, got %d", len(result.Hits))
		}
		// First hit should be cheapest (Wool Socks at 12.99).
		if result.Hits[0].Representation["name"] != "Wool Socks" {
			t.Errorf("expected first hit to be Wool Socks (cheapest), got %v", result.Hits[0].Representation["name"])
		}
	})

	t.Run("pagination", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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
		// TotalCount should still reflect the full count.
		if result.TotalCount != 4 {
			t.Errorf("expected TotalCount=4, got %d", result.TotalCount)
		}
	})

	t.Run("facets", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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

	t.Run("type name filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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

	t.Run("search hit identity", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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

	t.Run("text search with field restriction", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			TextQuery:  "shoes",
			TextFields: []searchindex.TextFieldWeight{{Name: "name"}},
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 2 {
			t.Errorf("expected at least 2 hits for 'shoes' with field restriction, got %d", result.TotalCount)
		}
	})

	t.Run("terms filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
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
			t.Errorf("expected 4 hits for category IN (Footwear, Accessories), got %d", result.TotalCount)
		}
	})

	t.Run("upsert overwrites existing document", func(t *testing.T) {
		// Update Running Shoes price.
		doc := searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
			Fields:   map[string]any{"name": "Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 79.99, "inStock": true},
		}
		if err := idx.IndexDocument(ctx, doc); err != nil {
			t.Fatalf("IndexDocument (upsert): %v", err)
		}

		// Total should still be 4.
		result, err := idx.Search(ctx, searchindex.SearchRequest{Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits after upsert, got %d", result.TotalCount)
		}
	})
}

func TestDeleteDocument(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndex(t, db)
	populateTestData(t, idx)

	ctx := context.Background()

	err := idx.DeleteDocument(ctx, searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "1"},
	})
	if err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	result, err := idx.Search(ctx, searchindex.SearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 3 {
		t.Errorf("expected 3 documents after delete, got %d", result.TotalCount)
	}
}

func TestDeleteDocuments(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndex(t, db)
	populateTestData(t, idx)

	ctx := context.Background()

	err := idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{
		{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
		{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
	})
	if err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	result, err := idx.Search(ctx, searchindex.SearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 2 {
		t.Errorf("expected 2 documents after batch delete, got %d", result.TotalCount)
	}
}

func TestVectorSearch(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndexWithVectors(t, db)
	populateVectorData(t, idx)

	ctx := context.Background()

	t.Run("nearest neighbor search", func(t *testing.T) {
		// Query vector close to Running Shoes [0.1, 0.2, 0.3].
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector:      []float32{0.1, 0.2, 0.3},
			VectorField: "embedding",
			Limit:       4,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits, got %d", result.TotalCount)
		}
		if len(result.Hits) < 1 {
			t.Fatal("expected at least 1 hit")
		}
		// The closest vector should be Running Shoes (exact match).
		if result.Hits[0].Identity.TypeName != "Product" {
			t.Errorf("expected Product, got %s", result.Hits[0].Identity.TypeName)
		}
		if result.Hits[0].Distance < 0 {
			t.Errorf("expected non-negative distance, got %f", result.Hits[0].Distance)
		}
	})

	t.Run("vector search with filter", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			Vector:      []float32{0.1, 0.2, 0.3},
			VectorField: "embedding",
			Filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "Footwear"},
			},
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for Footwear, got %d", result.TotalCount)
		}
	})
}

func TestHybridSearch(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndexWithVectors(t, db)
	populateVectorData(t, idx)

	ctx := context.Background()

	t.Run("text and vector combined", func(t *testing.T) {
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			TextQuery:   "shoes",
			Vector:      []float32{0.1, 0.2, 0.3},
			VectorField: "embedding",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 1 {
			t.Errorf("expected at least 1 hit for hybrid search, got %d", result.TotalCount)
		}
	})
}

func TestDocumentID(t *testing.T) {
	id := documentID(searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	})
	expected := "Product:id=123,sku=ABC"
	if id != expected {
		t.Errorf("documentID = %q, want %q", id, expected)
	}
}

func TestDocumentIDNoKeys(t *testing.T) {
	id := documentID(searchindex.DocumentIdentity{
		TypeName: "Singleton",
	})
	if id != "Singleton" {
		t.Errorf("documentID = %q, want %q", id, "Singleton")
	}
}

func TestFormatVector(t *testing.T) {
	got := formatVector([]float32{0.1, 0.2, 0.3})
	expected := "[0.1,0.2,0.3]"
	if got != expected {
		t.Errorf("formatVector = %q, want %q", got, expected)
	}
}

func TestIndexSingleDocument(t *testing.T) {
	db := startPgvectorContainer(t)
	idx := newTestIndex(t, db)
	ctx := context.Background()

	doc := searchindex.EntityDocument{
		Identity: searchindex.DocumentIdentity{
			TypeName:  "Product",
			KeyFields: map[string]any{"id": "42"},
		},
		Fields: map[string]any{
			"name":        "Hiking Boots",
			"description": "Durable boots for mountain trails",
			"category":    "Footwear",
			"price":       149.99,
			"inStock":     true,
		},
	}

	if err := idx.IndexDocument(ctx, doc); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	result, err := idx.Search(ctx, searchindex.SearchRequest{
		TextQuery: "hiking",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 hit for 'hiking', got %d", result.TotalCount)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit in results, got %d", len(result.Hits))
	}
	hit := result.Hits[0]
	if hit.Identity.TypeName != "Product" {
		t.Errorf("TypeName = %q, want %q", hit.Identity.TypeName, "Product")
	}
	if hit.Representation["name"] != "Hiking Boots" {
		t.Errorf("name = %v, want %q", hit.Representation["name"], "Hiking Boots")
	}
	if hit.Representation["category"] != "Footwear" {
		t.Errorf("category = %v, want %q", hit.Representation["category"], "Footwear")
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "withspaces"},
		{"with-dashes", "withdashes"},
		{"with_underscores", "with_underscores"},
		{"MiXeD123", "MiXeD123"},
		{"drop;table", "droptable"},
	}
	for _, tt := range tests {
		got := sanitizeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
