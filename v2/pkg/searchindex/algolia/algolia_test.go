//go:build integration

package algolia

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func skipIfNoAlgolia(t *testing.T) (string, string) {
	t.Helper()
	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skip("ALGOLIA_APP_ID and ALGOLIA_API_KEY environment variables are required for integration tests")
	}
	return appID, apiKey
}

func TestAlgoliaIntegration(t *testing.T) {
	appID, apiKey := skipIfNoAlgolia(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	indexName := fmt.Sprintf("test_integration_%d", time.Now().UnixNano())

	schema := searchindex.IndexConfig{
		Name: indexName,
		Fields: []searchindex.FieldConfig{
			{Name: "title", Type: searchindex.FieldTypeText, Filterable: true, Sortable: false},
			{Name: "description", Type: searchindex.FieldTypeText, Filterable: false, Sortable: false},
			{Name: "category", Type: searchindex.FieldTypeKeyword, Filterable: true, Sortable: false},
			{Name: "price", Type: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
			{Name: "inStock", Type: searchindex.FieldTypeBool, Filterable: true, Sortable: false},
		},
	}

	configJSON := fmt.Sprintf(`{"app_id": %q, "api_key": %q}`, appID, apiKey)

	factory := &Factory{}

	t.Run("CreateIndex", func(t *testing.T) {
		idx, err := factory.CreateIndex(ctx, indexName, schema, []byte(configJSON))
		if err != nil {
			t.Fatalf("CreateIndex failed: %v", err)
		}
		defer idx.Close()

		t.Run("IndexDocuments", func(t *testing.T) {
			docs := []searchindex.EntityDocument{
				{
					Identity: searchindex.DocumentIdentity{
						TypeName:  "Product",
						KeyFields: map[string]any{"id": "1"},
					},
					Fields: map[string]any{
						"title":       "Wireless Keyboard",
						"description": "A compact wireless keyboard with Bluetooth connectivity",
						"category":    "electronics",
						"price":       49.99,
						"inStock":     true,
					},
				},
				{
					Identity: searchindex.DocumentIdentity{
						TypeName:  "Product",
						KeyFields: map[string]any{"id": "2"},
					},
					Fields: map[string]any{
						"title":       "USB Mouse",
						"description": "Ergonomic USB mouse with adjustable DPI",
						"category":    "electronics",
						"price":       29.99,
						"inStock":     true,
					},
				},
				{
					Identity: searchindex.DocumentIdentity{
						TypeName:  "Product",
						KeyFields: map[string]any{"id": "3"},
					},
					Fields: map[string]any{
						"title":       "Desk Lamp",
						"description": "LED desk lamp with adjustable brightness",
						"category":    "office",
						"price":       35.00,
						"inStock":     false,
					},
				},
			}

			err := idx.IndexDocuments(ctx, docs)
			if err != nil {
				t.Fatalf("IndexDocuments failed: %v", err)
			}

			// Give Algolia a moment to process
			time.Sleep(2 * time.Second)

			t.Run("SearchTextQuery", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "keyboard",
					Limit:     10,
				})
				if err != nil {
					t.Fatalf("Search failed: %v", err)
				}
				if result.TotalCount == 0 {
					t.Fatal("Expected at least one result for 'keyboard' search")
				}
				found := false
				for _, hit := range result.Hits {
					if hit.Identity.TypeName == "Product" {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("Expected to find a Product hit")
				}
				t.Logf("Search returned %d hits (total: %d)", len(result.Hits), result.TotalCount)
			})

			t.Run("SearchWithFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						Term: &searchindex.TermFilter{
							Field: "category",
							Value: "electronics",
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with filter failed: %v", err)
				}
				if result.TotalCount < 2 {
					t.Fatalf("Expected at least 2 results for category=electronics, got %d", result.TotalCount)
				}
				t.Logf("Filtered search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithRangeFilter", func(t *testing.T) {
				gte := any(30.0)
				lte := any(50.0)
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
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
					t.Fatalf("Search with range filter failed: %v", err)
				}
				if result.TotalCount == 0 {
					t.Fatal("Expected at least one result for price range 30-50")
				}
				t.Logf("Range filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithBoolFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						Term: &searchindex.TermFilter{
							Field: "inStock",
							Value: false,
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with bool filter failed: %v", err)
				}
				if result.TotalCount < 1 {
					t.Fatalf("Expected at least 1 result for inStock=false, got %d", result.TotalCount)
				}
				t.Logf("Bool filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithANDFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						And: []*searchindex.Filter{
							{Term: &searchindex.TermFilter{Field: "category", Value: "electronics"}},
							{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with AND filter failed: %v", err)
				}
				if result.TotalCount < 2 {
					t.Fatalf("Expected at least 2 results for category=electronics AND inStock=true, got %d", result.TotalCount)
				}
				t.Logf("AND filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithORFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						Or: []*searchindex.Filter{
							{Term: &searchindex.TermFilter{Field: "category", Value: "electronics"}},
							{Term: &searchindex.TermFilter{Field: "category", Value: "office"}},
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with OR filter failed: %v", err)
				}
				if result.TotalCount < 3 {
					t.Fatalf("Expected at least 3 results for category=electronics OR category=office, got %d", result.TotalCount)
				}
				t.Logf("OR filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithNOTFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						Not: &searchindex.Filter{
							Term: &searchindex.TermFilter{Field: "category", Value: "electronics"},
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with NOT filter failed: %v", err)
				}
				for _, hit := range result.Hits {
					if cat, ok := hit.Representation["category"].(string); ok && cat == "electronics" {
						t.Fatal("Expected no electronics hits when using NOT category=electronics filter")
					}
				}
				t.Logf("NOT filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchWithTermsFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					Filter: &searchindex.Filter{
						Terms: &searchindex.TermsFilter{
							Field:  "category",
							Values: []any{"electronics", "office"},
						},
					},
					Limit: 10,
				})
				if err != nil {
					t.Fatalf("Search with terms filter failed: %v", err)
				}
				if result.TotalCount < 3 {
					t.Fatalf("Expected at least 3 results for category IN [electronics, office], got %d", result.TotalCount)
				}
				for _, hit := range result.Hits {
					cat, ok := hit.Representation["category"].(string)
					if !ok {
						t.Fatal("Expected category field in hit representation")
					}
					if cat != "electronics" && cat != "office" {
						t.Fatalf("Expected category to be electronics or office, got %q", cat)
					}
				}
				t.Logf("Terms filter search returned %d hits", result.TotalCount)
			})

			t.Run("SearchHitIdentity", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "keyboard",
					Limit:     10,
				})
				if err != nil {
					t.Fatalf("Search failed: %v", err)
				}
				if len(result.Hits) == 0 {
					t.Fatal("Expected at least one hit")
				}
				hit := result.Hits[0]
				if hit.Identity.TypeName != "Product" {
					t.Fatalf("Expected Identity.TypeName to be 'Product', got %q", hit.Identity.TypeName)
				}
				typename, ok := hit.Representation["__typename"]
				if !ok {
					t.Fatal("Expected __typename in hit Representation")
				}
				if typename != "Product" {
					t.Fatalf("Expected Representation[__typename] to be 'Product', got %v", typename)
				}
				t.Logf("Hit identity: TypeName=%s, KeyFields=%v, __typename=%v", hit.Identity.TypeName, hit.Identity.KeyFields, typename)
			})

			t.Run("TypeNameFilter", func(t *testing.T) {
				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "",
					TypeName:  "Product",
					Limit:     10,
				})
				if err != nil {
					t.Fatalf("Search with TypeName filter failed: %v", err)
				}
				if result.TotalCount == 0 {
					t.Fatal("Expected at least one result for TypeName=Product")
				}
				for _, hit := range result.Hits {
					if hit.Identity.TypeName != "Product" {
						t.Fatalf("Expected all hits to have TypeName 'Product', got %q", hit.Identity.TypeName)
					}
				}
				t.Logf("TypeName filter search returned %d hits", result.TotalCount)
			})

			t.Run("IndexSingleDocument", func(t *testing.T) {
				doc := searchindex.EntityDocument{
					Identity: searchindex.DocumentIdentity{
						TypeName:  "Product",
						KeyFields: map[string]any{"id": "4"},
					},
					Fields: map[string]any{
						"title":       "Monitor Stand",
						"description": "Adjustable monitor stand with cable management",
						"category":    "office",
						"price":       59.99,
						"inStock":     true,
					},
				}
				err := idx.IndexDocument(ctx, doc)
				if err != nil {
					t.Fatalf("IndexDocument failed: %v", err)
				}

				time.Sleep(2 * time.Second)

				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "monitor stand",
					Limit:     10,
				})
				if err != nil {
					t.Fatalf("Search after IndexDocument failed: %v", err)
				}
				if result.TotalCount == 0 {
					t.Fatal("Expected to find the newly indexed document")
				}
			})

			t.Run("DeleteDocument", func(t *testing.T) {
				err := idx.DeleteDocument(ctx, searchindex.DocumentIdentity{
					TypeName:  "Product",
					KeyFields: map[string]any{"id": "4"},
				})
				if err != nil {
					t.Fatalf("DeleteDocument failed: %v", err)
				}

				time.Sleep(2 * time.Second)

				result, err := idx.Search(ctx, searchindex.SearchRequest{
					TextQuery: "monitor stand",
					Limit:     10,
				})
				if err != nil {
					t.Fatalf("Search after delete failed: %v", err)
				}
				for _, hit := range result.Hits {
					if kf, ok := hit.Identity.KeyFields["id"]; ok && kf == "4" {
						t.Fatal("Document should have been deleted but was found")
					}
				}
			})

			t.Run("DeleteDocuments", func(t *testing.T) {
				ids := []searchindex.DocumentIdentity{
					{TypeName: "Product", KeyFields: map[string]any{"id": "1"}},
					{TypeName: "Product", KeyFields: map[string]any{"id": "2"}},
					{TypeName: "Product", KeyFields: map[string]any{"id": "3"}},
				}
				err := idx.DeleteDocuments(ctx, ids)
				if err != nil {
					t.Fatalf("DeleteDocuments failed: %v", err)
				}
			})
		})
	})

	t.Run("FactoryValidation", func(t *testing.T) {
		_, err := factory.CreateIndex(ctx, "test", schema, []byte(`{}`))
		if err == nil {
			t.Fatal("Expected error for missing app_id and api_key")
		}

		_, err = factory.CreateIndex(ctx, "test", schema, []byte(`not json`))
		if err == nil {
			t.Fatal("Expected error for invalid JSON config")
		}
	})
}

func TestDocumentObjectID(t *testing.T) {
	tests := []struct {
		name     string
		identity searchindex.DocumentIdentity
		expected string
	}{
		{
			name: "single key field",
			identity: searchindex.DocumentIdentity{
				TypeName:  "Product",
				KeyFields: map[string]any{"id": "123"},
			},
			expected: "Product:id=123",
		},
		{
			name: "multiple key fields sorted",
			identity: searchindex.DocumentIdentity{
				TypeName:  "Order",
				KeyFields: map[string]any{"userId": "u1", "orderId": "o1"},
			},
			expected: "Order:orderId=o1,userId=u1",
		},
		{
			name: "no key fields",
			identity: searchindex.DocumentIdentity{
				TypeName: "Singleton",
			},
			expected: "Singleton",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := documentObjectID(tt.identity)
			if got != tt.expected {
				t.Errorf("documentObjectID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildFilterString(t *testing.T) {
	tests := []struct {
		name     string
		filter   *searchindex.Filter
		expected string
	}{
		{
			name: "term filter string",
			filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "category", Value: "electronics"},
			},
			expected: "category:electronics",
		},
		{
			name: "term filter bool true",
			filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: true},
			},
			expected: "inStock:true",
		},
		{
			name: "term filter bool false",
			filter: &searchindex.Filter{
				Term: &searchindex.TermFilter{Field: "inStock", Value: false},
			},
			expected: "inStock:false",
		},
		{
			name: "range filter",
			filter: &searchindex.Filter{
				Range: &searchindex.RangeFilter{
					Field: "price",
					GTE:   10.0,
					LTE:   100.0,
				},
			},
			expected: "price >= 10 AND price <= 100",
		},
		{
			name: "AND filter",
			filter: &searchindex.Filter{
				And: []*searchindex.Filter{
					{Term: &searchindex.TermFilter{Field: "category", Value: "electronics"}},
					{Term: &searchindex.TermFilter{Field: "inStock", Value: true}},
				},
			},
			expected: "(category:electronics AND inStock:true)",
		},
		{
			name: "OR filter",
			filter: &searchindex.Filter{
				Or: []*searchindex.Filter{
					{Term: &searchindex.TermFilter{Field: "category", Value: "electronics"}},
					{Term: &searchindex.TermFilter{Field: "category", Value: "office"}},
				},
			},
			expected: "(category:electronics OR category:office)",
		},
		{
			name: "NOT filter",
			filter: &searchindex.Filter{
				Not: &searchindex.Filter{
					Term: &searchindex.TermFilter{Field: "category", Value: "electronics"},
				},
			},
			expected: "NOT category:electronics",
		},
		{
			name: "terms filter",
			filter: &searchindex.Filter{
				Terms: &searchindex.TermsFilter{
					Field:  "category",
					Values: []any{"electronics", "office"},
				},
			},
			expected: "(category:electronics OR category:office)",
		},
		{
			name:     "nil filter",
			filter:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFilterString(tt.filter)
			if got != tt.expected {
				t.Errorf("buildFilterString() = %q, want %q", got, tt.expected)
			}
		})
	}
}
