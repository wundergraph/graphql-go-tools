package searche2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/bleve"
)

// mockExecutor returns a canned JSON response for the population query.
type mockExecutor struct {
	response []byte
}

func (m *mockExecutor) Execute(_ context.Context, _ string) ([]byte, error) {
	return m.response, nil
}

// mockSubscriber provides a channel-based subscriber for testing.
type mockSubscriber struct {
	ch chan []byte
}

func newMockSubscriber() *mockSubscriber {
	return &mockSubscriber{ch: make(chan []byte, 10)}
}

func (m *mockSubscriber) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return m.ch, nil
}

func (m *mockSubscriber) Send(data []byte) {
	m.ch <- data
}

func (m *mockSubscriber) Close() {
	close(m.ch)
}

func TestSubscriptionUpsert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build the population response with 2 initial products.
	populationResp, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"products": []map[string]any{
				{"id": "1", "name": "Running Shoes", "description": "Great for jogging", "category": "Footwear", "price": 89.99, "inStock": true},
				{"id": "2", "name": "Basketball Shoes", "description": "High-top sneakers", "category": "Footwear", "price": 129.99, "inStock": true},
			},
		},
	})

	// Setup registries and factory.
	indexRegistry := searchindex.NewIndexFactoryRegistry()
	indexRegistry.Register("bleve", bleve.NewFactory())

	factory := search_datasource.NewFactory(ctx, indexRegistry, nil)
	executor := &mockExecutor{response: populationResp}
	subscriber := newMockSubscriber()

	config := &search_datasource.ParsedConfig{
		Indices: []search_datasource.IndexDirective{
			{Name: "products", Backend: "bleve"},
		},
		Entities: []search_datasource.SearchableEntity{
			{
				TypeName:               "Product",
				IndexName:              "products",
				SearchField:            "searchProducts",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []search_datasource.IndexedField{
					{FieldName: "name", GraphQLType: "String", IndexType: searchindex.FieldTypeText, Filterable: true, Sortable: true},
					{FieldName: "description", GraphQLType: "String", IndexType: searchindex.FieldTypeText},
					{FieldName: "category", GraphQLType: "String", IndexType: searchindex.FieldTypeKeyword, Filterable: true},
					{FieldName: "price", GraphQLType: "Float", IndexType: searchindex.FieldTypeNumeric, Filterable: true, Sortable: true},
					{FieldName: "inStock", GraphQLType: "Boolean", IndexType: searchindex.FieldTypeBool, Filterable: true},
				},
			},
		},
		Populations: []search_datasource.PopulateDirective{
			{
				IndexName:      "products",
				EntityTypeName: "Product",
				Path:           "data.products",
				Query:          "{ products { id name description category price inStock } }",
			},
		},
		Subscriptions: []search_datasource.SubscribeDirective{
			{
				IndexName:      "products",
				EntityTypeName: "Product",
				Path:           "data.productUpdated",
				Subscription:   "subscription { productUpdated { id name description category price inStock } }",
			},
		},
	}

	manager := search_datasource.NewManager(factory, indexRegistry, nil, executor, config)
	manager.SetSubscriber(subscriber)

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}
	defer manager.Stop()

	// Verify initial population: search for "shoes" should return 2 hits.
	idx, ok := manager.GetIndex("products")
	if !ok {
		t.Fatal("index 'products' not found")
	}

	searchAndExpect := func(query string, expectedCount int) *searchindex.SearchResult {
		t.Helper()
		result, err := idx.Search(ctx, searchindex.SearchRequest{
			TextQuery: query,
			TypeName:  "Product",
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if len(result.Hits) != expectedCount {
			t.Fatalf("Search(%q): got %d hits, want %d", query, len(result.Hits), expectedCount)
		}
		return result
	}

	searchAndExpect("shoes", 2)

	// Send a subscription event: upsert a new product.
	upsertEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productUpdated": map[string]any{
				"id": "3", "name": "Tennis Shoes", "description": "Court shoes for tennis", "category": "Footwear", "price": 99.99, "inStock": true,
			},
		},
	})
	subscriber.Send(upsertEvent)

	// Give the subscription goroutine time to process.
	time.Sleep(200 * time.Millisecond)

	// Now "shoes" should return 3 hits.
	searchAndExpect("shoes", 3)

	// Send another event: update an existing product's name.
	updateEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productUpdated": map[string]any{
				"id": "1", "name": "Trail Running Boots", "description": "Great for trail running", "category": "Footwear", "price": 89.99, "inStock": true,
			},
		},
	})
	subscriber.Send(updateEvent)
	time.Sleep(200 * time.Millisecond)

	// "boots" should return 1 hit (the updated product).
	result := searchAndExpect("boots", 1)
	if result.Hits[0].Identity.KeyFields["id"] != "1" {
		t.Errorf("expected updated product id=1, got %v", result.Hits[0].Identity.KeyFields["id"])
	}
}

func TestSubscriptionDeletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	populationResp, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"products": []map[string]any{
				{"id": "1", "name": "Running Shoes", "description": "Great for jogging", "category": "Footwear", "price": 89.99, "inStock": true},
				{"id": "2", "name": "Basketball Shoes", "description": "High-top sneakers", "category": "Footwear", "price": 129.99, "inStock": true},
				{"id": "3", "name": "Leather Belt", "description": "Genuine leather belt", "category": "Accessories", "price": 35.00, "inStock": false},
			},
		},
	})

	indexRegistry := searchindex.NewIndexFactoryRegistry()
	indexRegistry.Register("bleve", bleve.NewFactory())

	factory := search_datasource.NewFactory(ctx, indexRegistry, nil)
	executor := &mockExecutor{response: populationResp}
	subscriber := newMockSubscriber()

	config := &search_datasource.ParsedConfig{
		Indices: []search_datasource.IndexDirective{
			{Name: "products", Backend: "bleve"},
		},
		Entities: []search_datasource.SearchableEntity{
			{
				TypeName:               "Product",
				IndexName:              "products",
				SearchField:            "searchProducts",
				KeyFields:              []string{"id"},
				ResultsMetaInformation: true,
				Fields: []search_datasource.IndexedField{
					{FieldName: "name", GraphQLType: "String", IndexType: searchindex.FieldTypeText},
					{FieldName: "description", GraphQLType: "String", IndexType: searchindex.FieldTypeText},
					{FieldName: "category", GraphQLType: "String", IndexType: searchindex.FieldTypeKeyword, Filterable: true},
					{FieldName: "price", GraphQLType: "Float", IndexType: searchindex.FieldTypeNumeric},
					{FieldName: "inStock", GraphQLType: "Boolean", IndexType: searchindex.FieldTypeBool},
				},
			},
		},
		Populations: []search_datasource.PopulateDirective{
			{
				IndexName:      "products",
				EntityTypeName: "Product",
				Path:           "data.products",
				Query:          "{ products { id name description category price inStock } }",
			},
		},
		Subscriptions: []search_datasource.SubscribeDirective{
			{
				IndexName:      "products",
				EntityTypeName: "Product",
				Path:           "data.productUpdated",
				DeletionPath:   "data.productDeleted",
				Subscription:   "subscription { productUpdated { id name description category price inStock } productDeleted { id } }",
			},
		},
	}

	manager := search_datasource.NewManager(factory, indexRegistry, nil, executor, config)
	manager.SetSubscriber(subscriber)

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}
	defer manager.Stop()

	idx, ok := manager.GetIndex("products")
	if !ok {
		t.Fatal("index 'products' not found")
	}

	// Verify initial: "shoes" returns 2.
	result, err := idx.Search(ctx, searchindex.SearchRequest{TextQuery: "shoes", TypeName: "Product", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("initial search: got %d hits, want 2", len(result.Hits))
	}

	// Send a deletion event.
	deleteEvent, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"productDeleted": map[string]any{
				"id": "1",
			},
		},
	})
	subscriber.Send(deleteEvent)
	time.Sleep(200 * time.Millisecond)

	// "shoes" should now return 1 (only basketball shoes).
	result, err = idx.Search(ctx, searchindex.SearchRequest{TextQuery: "shoes", TypeName: "Product", Limit: 10})
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("after deletion: got %d hits, want 1", len(result.Hits))
	}
	if result.Hits[0].Identity.KeyFields["id"] != "2" {
		t.Errorf("expected remaining product id=2, got %v", result.Hits[0].Identity.KeyFields["id"])
	}
}
