package searchtesting

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/bleve"
)

// mockSubscriber implements search_datasource.GraphQLSubscriber for tests.
type mockSubscriber struct {
	ch chan []byte
}

func newMockSubscriber() *mockSubscriber {
	return &mockSubscriber{ch: make(chan []byte, 10)}
}

func (m *mockSubscriber) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return m.ch, nil
}

// mockExecutor implements search_datasource.GraphQLExecutor that returns pre-configured data.
type mockExecutor struct {
	response []byte
}

func (m *mockExecutor) Execute(_ context.Context, _ string) ([]byte, error) {
	return m.response, nil
}

const subscriptionConfigSDL = `
extend schema
  @index(name: "products", backend: "bleve", config: "{}")
  @populate(index: "products", entity: "Product", path: "data.products", query: "{ products { id name description category price inStock } }")
  @subscribe(index: "products", entity: "Product", path: "data.productUpdated", deletionPath: "data.productDeleted", subscription: "subscription { productUpdated { id name description category price inStock } productDeleted { id } }")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`

func TestSubscriptionUpdatesIndex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up registries.
	indexRegistry := searchindex.NewIndexFactoryRegistry()
	indexRegistry.Register("bleve", bleve.NewFactory())
	embedderRegistry := searchindex.NewEmbedderRegistry()

	// Parse config.
	parsedConfig := parseConfig(t, subscriptionConfigSDL)

	// Provide initial population data (2 products).
	populationResponse, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"products": []map[string]any{
				{"id": "1", "name": "Running Shoes", "description": "Great for jogging", "category": "Footwear", "price": 89.99, "inStock": true},
				{"id": "2", "name": "Basketball Shoes", "description": "High-top sneakers", "category": "Footwear", "price": 129.99, "inStock": true},
			},
		},
	})

	executor := &mockExecutor{response: populationResponse}
	subscriber := newMockSubscriber()

	// Create Manager and start it.
	factory := search_datasource.NewFactory(ctx, indexRegistry, embedderRegistry)
	manager := search_datasource.NewManager(factory, indexRegistry, embedderRegistry, executor, parsedConfig)
	manager.SetSubscriber(subscriber)

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}
	defer manager.Stop()

	// Verify initial population: 2 documents indexed.
	idx, ok := manager.GetIndex("products")
	if !ok {
		t.Fatal("index 'products' not found after Start")
	}
	assertSearchCount(t, idx, "shoes", 2)

	// --- Test 1: Subscription upsert adds a new document ---
	upsertEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productUpdated": map[string]any{
				"id": "3", "name": "Leather Belt", "description": "Genuine leather", "category": "Accessories", "price": 35.0, "inStock": false,
			},
		},
	})
	subscriber.ch <- upsertEvent

	// Wait for the event to be processed (empty TextQuery = match all in bleve).
	waitForCondition(t, 2*time.Second, func() bool {
		result, err := idx.Search(ctx, searchindex.SearchRequest{Limit: 10})
		return err == nil && result.TotalCount == 3
	})
	assertSearchCount(t, idx, "", 3)

	// Verify the new document is searchable by name.
	result, err := idx.Search(ctx, searchindex.SearchRequest{TextQuery: "leather", Limit: 10})
	if err != nil {
		t.Fatalf("search for 'leather': %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("expected at least 1 hit for 'leather', got 0")
	}
	foundBelt := false
	for _, hit := range result.Hits {
		if hit.Identity.KeyFields["id"] == "3" {
			foundBelt = true
			break
		}
	}
	if !foundBelt {
		t.Fatalf("expected to find product id=3 in results, hits: %+v", result.Hits)
	}

	// --- Test 2: Subscription upsert updates an existing document ---
	updateEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productUpdated": map[string]any{
				"id": "1", "name": "Trail Running Shoes", "description": "Great for trail running", "category": "Footwear", "price": 99.99, "inStock": true,
			},
		},
	})
	subscriber.ch <- updateEvent

	waitForCondition(t, 2*time.Second, func() bool {
		result, err := idx.Search(ctx, searchindex.SearchRequest{TextQuery: "trail", Limit: 10})
		return err == nil && len(result.Hits) > 0
	})

	result, err = idx.Search(ctx, searchindex.SearchRequest{TextQuery: "trail", Limit: 10})
	if err != nil {
		t.Fatalf("search for 'trail': %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("expected at least 1 hit for 'trail' after update, got 0")
	}

	// Total count should still be 3 (upsert, not insert).
	assertSearchCount(t, idx, "", 3)

	// --- Test 3: Subscription deletion removes a document ---
	deleteEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productDeleted": map[string]any{
				"id": "3",
			},
		},
	})
	subscriber.ch <- deleteEvent

	waitForCondition(t, 2*time.Second, func() bool {
		result, err := idx.Search(ctx, searchindex.SearchRequest{Limit: 10})
		return err == nil && result.TotalCount == 2
	})
	assertSearchCount(t, idx, "", 2)

	// Verify the deleted document is no longer searchable.
	result, err = idx.Search(ctx, searchindex.SearchRequest{TextQuery: "leather", Limit: 10})
	if err != nil {
		t.Fatalf("search for 'leather' after delete: %v", err)
	}
	for _, hit := range result.Hits {
		if hit.Identity.KeyFields["id"] == "3" {
			t.Fatal("product id=3 should have been deleted but was found in search results")
		}
	}
}

// parseConfig parses the config SDL into a ParsedConfig.
func parseConfig(t *testing.T, sdl string) *search_datasource.ParsedConfig {
	t.Helper()
	doc, parseReport := astparser.ParseGraphqlDocumentString(sdl)
	if parseReport.HasErrors() {
		t.Fatalf("parse config SDL: %s", parseReport.Error())
	}
	config, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	return config
}

// assertSearchCount verifies the total number of documents matching a query.
func assertSearchCount(t *testing.T, idx searchindex.Index, query string, expected int) {
	t.Helper()
	result, err := idx.Search(context.Background(), searchindex.SearchRequest{TextQuery: query, Limit: 10})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if result.TotalCount != expected {
		t.Fatalf("expected %d results, got %d", expected, result.TotalCount)
	}
}

// waitForCondition polls the condition function until it returns true or the timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
