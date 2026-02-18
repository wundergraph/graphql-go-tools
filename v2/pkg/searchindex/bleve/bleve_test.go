package bleve

import (
	"context"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func newTestIndex(t *testing.T) searchindex.Index {
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
	idx, err := factory.CreateIndex(context.Background(), "test", schema, nil)
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

func TestIndexAndSearch(t *testing.T) {
	idx := newTestIndex(t)
	populateTestData(t, idx)

	t.Run("text search", func(t *testing.T) {
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
		gte := 30.0
		lte := 100.0
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			Filter: &searchindex.Filter{
				Range: &searchindex.RangeFilter{
					Field: "price",
					GTE:   gte,
					LTE:   lte,
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
		// Footwear (3) minus out-of-stock socks... wait, wool socks are in stock.
		// Running Shoes (in stock), Basketball Shoes (in stock), Wool Socks (in stock) = 3
		if result.TotalCount != 3 {
			t.Errorf("expected 3 hits for Footwear AND inStock, got %d", result.TotalCount)
		}
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 1 {
			t.Errorf("expected 1 hit for NOT Footwear, got %d", result.TotalCount)
		}
	})

	t.Run("sorting", func(t *testing.T) {
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
		// First hit should be cheapest (Wool Socks at 12.99)
		if result.Hits[0].Representation["name"] != "Wool Socks" {
			t.Errorf("expected first hit to be Wool Socks (cheapest), got %v", result.Hits[0].Representation["name"])
		}
	})

	t.Run("pagination", func(t *testing.T) {
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
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 4 {
			t.Errorf("expected 4 hits for category=Footwear OR category=Accessories, got %d", result.TotalCount)
		}
	})

	t.Run("Terms (IN) filter", func(t *testing.T) {
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

	t.Run("TypeName filter", func(t *testing.T) {
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
}

func TestFuzzySearch(t *testing.T) {
	idx := newTestIndex(t)
	populateTestData(t, idx)

	t.Run("fuzziness LOW finds typo", func(t *testing.T) {
		fuzz := searchindex.FuzzinessLow
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "runing",
			Fuzziness: &fuzz,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount < 1 {
			t.Errorf("expected >=1 hit for 'runing' with fuzziness LOW, got %d", result.TotalCount)
		}
	})

	t.Run("fuzziness EXACT misses typo", func(t *testing.T) {
		fuzz := searchindex.FuzzinessExact
		result, err := idx.Search(context.Background(), searchindex.SearchRequest{
			TextQuery: "runing",
			Fuzziness: &fuzz,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if result.TotalCount != 0 {
			t.Errorf("expected 0 hits for 'runing' with fuzziness EXACT, got %d", result.TotalCount)
		}
	})
}

func TestDeleteDocument(t *testing.T) {
	idx := newTestIndex(t)
	populateTestData(t, idx)

	// Delete Running Shoes
	err := idx.DeleteDocument(context.Background(), searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "1"},
	})
	if err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// Should now have 3 documents
	result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 3 {
		t.Errorf("expected 3 documents after delete, got %d", result.TotalCount)
	}
}

func TestDeleteDocuments(t *testing.T) {
	idx := newTestIndex(t)
	populateTestData(t, idx)

	// Delete two documents
	err := idx.DeleteDocuments(context.Background(), []searchindex.DocumentIdentity{
		{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
		{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
	})
	if err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	result, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 2 {
		t.Errorf("expected 2 documents after batch delete, got %d", result.TotalCount)
	}
}

func TestIndexSingleDocument(t *testing.T) {
	idx := newTestIndex(t)

	doc := searchindex.EntityDocument{
		Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "99"}},
		Fields:   map[string]any{"name": "Sandals", "description": "Comfortable summer sandals", "category": "Footwear", "price": 49.99, "inStock": true},
	}
	if err := idx.IndexDocument(context.Background(), doc); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	result, err := idx.Search(context.Background(), searchindex.SearchRequest{
		TextQuery: "sandals",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalCount != 1 {
		t.Errorf("expected 1 hit for 'sandals', got %d", result.TotalCount)
	}
	if len(result.Hits) == 0 {
		t.Fatal("expected at least 1 hit")
	}
	if result.Hits[0].Identity.TypeName != "Product" {
		t.Errorf("TypeName = %q, want %q", result.Hits[0].Identity.TypeName, "Product")
	}
}

func TestUpsertDocument(t *testing.T) {
	idx := newTestIndex(t)
	populateTestData(t, idx)

	// Re-index product id "1" with an updated name
	updated := searchindex.EntityDocument{
		Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
		Fields:   map[string]any{"name": "Trail Running Shoes", "description": "Great for jogging and marathons", "category": "Footwear", "price": 89.99, "inStock": true},
	}
	if err := idx.IndexDocument(context.Background(), updated); err != nil {
		t.Fatalf("IndexDocument (upsert): %v", err)
	}

	// Total count should still be 4 (upsert, not insert)
	allResult, err := idx.Search(context.Background(), searchindex.SearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if allResult.TotalCount != 4 {
		t.Errorf("expected 4 total documents after upsert, got %d", allResult.TotalCount)
	}

	// Search for "trail" should return the updated document
	trailResult, err := idx.Search(context.Background(), searchindex.SearchRequest{
		TextQuery: "trail",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if trailResult.TotalCount != 1 {
		t.Errorf("expected 1 hit for 'trail', got %d", trailResult.TotalCount)
	}
	if len(trailResult.Hits) == 0 {
		t.Fatal("expected at least 1 hit for 'trail'")
	}
	if trailResult.Hits[0].Representation["name"] != "Trail Running Shoes" {
		t.Errorf("expected name %q, got %v", "Trail Running Shoes", trailResult.Hits[0].Representation["name"])
	}
}

func TestDocumentID(t *testing.T) {
	id := documentID(searchindex.DocumentIdentity{
		TypeName:  "Product",
		KeyFields: map[string]any{"id": "123", "sku": "ABC"},
	})
	// Keys should be sorted alphabetically
	expected := "Product:id=123,sku=ABC"
	if id != expected {
		t.Errorf("documentID = %q, want %q", id, expected)
	}
}
