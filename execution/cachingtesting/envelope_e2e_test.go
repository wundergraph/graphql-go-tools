package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestEnvelopeEndToEnd drives the stored value's envelope through the REAL
// ExecutionEngine: what lands in the store and how the key's subgraph segment
// keeps two subgraphs' entries for one entity apart.
func TestEnvelopeEndToEnd(t *testing.T) {
	t.Run("the stored value carries data and cache control", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock warehouse { id location } } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5,"warehouse":{"id":"w1","location":"Berlin"}}}}}`, body)

		// The write moment is real-clock; everything else is pinned literally.
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:4f796e3bbd360fce"},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   "v1:1:4f796e3bbd360fce",
						Value: `{"data":{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						// Tags live on the store Item; the envelope above carries
						// none of them.
						Tags: []string{
							"subgraph:1",
							"type:1:Product",
							"entity:1:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(store.Ops()))
	})

	t.Run("one entity cached by two subgraphs produces two keyspace-separated entries", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: time.Minute},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory, "reviews": reviews})

		body := Execute(t, executionEngine, `{ products(first: 1) { upc stock reviews { body } } }`, controller)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5,"reviews":[{"body":"Solid"}]}]}}`, body)

		// The SAME representation {"__typename":"Product","upc":"1"} is cached
		// once per subgraph: the subgraph segment leads both the visible key and
		// the hashed preimage, so the entries can never collide or be reused
		// across subgraphs (datasource 1 is inventory, 2 is reviews). The two
		// entity fetches are siblings and run in parallel, so the entries are
		// read back by key instead of from the order-dependent op log.
		inventoryValue, ok := store.Value("v1:1:4f796e3bbd360fce")
		require.True(t, ok)
		assert.Equal(t,
			`{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
			NormalizeEnvelopeClock(string(inventoryValue)))
		reviewsValue, ok := store.Value("v1:2:48d227258abb2a74")
		require.True(t, ok)
		assert.Equal(t,
			`{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
			NormalizeEnvelopeClock(string(reviewsValue)))

		// Request 2 serves both entity fetches from their own subgraph's entry;
		// the uncached products root field goes to the network again.
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5,"reviews":[{"body":"Solid"}]}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock reviews { body } } }`, controller))
		assert.Equal(t, int64(2), products.Requests())
		assert.Equal(t, int64(1), inventory.Requests())
		assert.Equal(t, int64(1), reviews.Requests())
	})
}

// TestEnvelopeLegacyBytesEndToEnd: an entry that is not a decodable envelope
// (a pre-envelope value, corruption) is a plain miss — never an error — and the
// fetch's own write replaces it.
func TestEnvelopeLegacyBytesEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	caching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"inventory": {DefaultTTL: ptr(time.Minute)},
		},
	}
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

	// A bare, unenveloped entity value under an otherwise valid key.
	store.Seed("v1:1:4f796e3bbd360fce", []byte(`{"__typename":"Product","stock":9}`), time.Minute)

	body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
	// The undecodable entry never reaches the response.
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:4f796e3bbd360fce"},
			Hits: []bool{true},
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					Key:   "v1:1:4f796e3bbd360fce",
					Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
	}, NormalizeStoreOpsClock(store.Ops()))

	// The replacement entry is a normal envelope and serves the next request.
	store.ResetOps()
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`,
		Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:4f796e3bbd360fce"},
			Hits: []bool{true},
		},
	}, store.Ops())
}
