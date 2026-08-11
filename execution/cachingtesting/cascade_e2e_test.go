package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestCachingCascadeEndToEnd drives the configuration cascade through the REAL
// ExecutionEngine over a chain that reaches TWO subgraphs' entity fetches in a
// fixed order (users root -> products entity -> inventory entity), so the whole
// store op log pins as a literal.
func TestCachingCascadeEndToEnd(t *testing.T) {
	t.Run("the global DefaultTTL alone caches every subgraph", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: time.Minute},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`

		assert.Equal(t, expected, Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))

		ops := store.Ops()
		require.Len(t, ops, 3)
		require.Len(t, ops[0].Keys, 1)
		require.Len(t, ops[1].Keys, 1)
		userKey, productKey := ops[0].Keys[0], ops[1].Keys[0]
		assert.Equal(t, []cachetesting.StoreOp{
			// The products entity fetch (User) looks up first — it produces the
			// representation the inventory fetch keys on.
			{
				Kind: "GetMany",
				Keys: []string{userKey},
				Hits: []bool{false},
			},
			// The inventory entity fetch (Product) looks up second.
			{
				Kind: "GetMany",
				Keys: []string{productKey},
				Hits: []bool{false},
			},
			// One request-end flush carrying both misses' values, each at the
			// global TTL: neither subgraph has an entry in the cascade.
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   userKey,
						Value: `{"data":{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:User",
							"entity:0:User:19ce389448be3351", // id "u1"
						},
					},
					{
						Key:   productKey,
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
		}, NormalizeStoreOpsClock(ops))

		// A second request serves both entity fetches from L2: only the uncached
		// users root field goes to the network again.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{userKey},
				Hits: []bool{true},
			},
			{
				Kind: "GetMany",
				Keys: []string{productKey},
				Hits: []bool{true},
			},
		}, store.Ops())
		assert.Equal(t, int64(2), users.Requests())
		assert.Equal(t, int64(1), products.Requests())
		assert.Equal(t, int64(1), inventory.Requests())
	})

	t.Run("a per-subgraph TTL overrides the global one", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: time.Minute},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		assert.Equal(t,
			`{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`,
			Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))

		ops := store.Ops()
		require.Len(t, ops, 3)
		require.Len(t, ops[0].Keys, 1)
		require.Len(t, ops[1].Keys, 1)
		userKey, productKey := ops[0].Keys[0], ops[1].Keys[0]
		assert.Equal(t, []cachetesting.StoreOp{
			// The products entity fetch (User), inheriting the global TTL.
			{
				Kind: "GetMany",
				Keys: []string{userKey},
				Hits: []bool{false},
			},
			// The inventory entity fetch (Product), on its own TTL.
			{
				Kind: "GetMany",
				Keys: []string{productKey},
				Hits: []bool{false},
			},
			// The flush shows both TTLs side by side: inherited 1m for products,
			// overridden 5m for inventory.
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   userKey,
						Value: `{"data":{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:User",
							"entity:0:User:19ce389448be3351", // id "u1"
						},
					},
					{
						Key:   productKey,
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":300,"created":-1,"scope":"public"}}`,
						TTL:   5 * time.Minute,
						Tags: []string{
							"subgraph:1",
							"type:1:Product",
							"entity:1:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))
	})

	t.Run("Enabled false vetoes one subgraph while the other stays cached", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: time.Minute},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {Enabled: ptr(false)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`

		assert.Equal(t, expected, Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))

		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		userKey := ops[0].Keys[0]
		assert.Equal(t, []cachetesting.StoreOp{
			// Only the products entity fetch reaches the store; the vetoed
			// inventory fetch has no cache config at all.
			{
				Kind: "GetMany",
				Keys: []string{userKey},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   userKey,
						Value: `{"data":{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:User",
							"entity:0:User:19ce389448be3351", // id "u1"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))

		// A second request: the products fetch is served from L2, the vetoed
		// inventory subgraph is fetched again.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller))
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{userKey},
				Hits: []bool{true},
			},
		}, store.Ops())
		assert.Equal(t, int64(1), products.Requests())
		assert.Equal(t, int64(2), inventory.Requests())
	})
}
