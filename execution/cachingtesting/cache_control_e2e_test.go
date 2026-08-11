package cachingtesting

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestCacheControlTTLEndToEnd drives the two-tier TTL ladder through the REAL
// ExecutionEngine over REAL HTTP upstreams, so the subgraph's Cache-Control
// header travels the whole loader path into the written entry.
func TestCacheControlTTLEndToEnd(t *testing.T) {
	t.Run("header max-age beats the configured DefaultTTL", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "max-age=60")
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// Both the envelope's cc.ttl and the store item's TTL carry 60s — the
		// runtime truth — instead of the configured 5m.
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
	})

	t.Run("a subgraph that sends no Cache-Control leaves the configured DefaultTTL in charge", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

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
		}, NormalizeStoreOpsClock(store.Ops()))
	})

	t.Run("MaxTTL clamps the header value and shows up in cc.ttl", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{MaxTTL: 30 * time.Second},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "max-age=3600")
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// The clamp is a deliberate deviation from origin authority: 30s, not the
		// hour the subgraph asked for.
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
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":30,"created":-1,"scope":"public"}}`,
						TTL:   30 * time.Second,
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

	t.Run("a root-field coordinate without its own TTL inherits the subgraph DefaultTTL", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(2 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
			},
		}
		first1 := Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`)
		products := Rules(first1)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products})

		body := ExecuteWithVariables(t, executionEngine, `query($first: Int!) { products(first: $first) { upc name } }`, `{"first":1}`, controller)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`, body)

		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		key := ops[0].Keys[0]
		// The coordinate entry alone declares participation; its lifetime came
		// from the subgraph DefaultTTL rung of the ladder.
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key,
						Value: `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]},"cc":{"ttl":120,"created":-1,"scope":"public"}}`,
						TTL:   2 * time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:Query",
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))
	})
}

// TestCacheControlStorabilityEndToEnd covers the directives that decide whether
// a subgraph result is kept at all, end to end.
func TestCacheControlStorabilityEndToEnd(t *testing.T) {
	t.Run("no-store keeps the result out of BOTH layers", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		// The initial inventory fetch selects {stock, warehouse} and answers
		// no-store; the deferred fetch of the SAME representation selects {stock}
		// alone and would ride L1 if anything had been written there. Its canned
		// stock is tampered to 999, so an L1 serve is visible in the response.
		products := Rules(
			Rule(`product(upc: $a)`, `{"data":{"product":{"__typename":"Product","upc":"1"},"products":[{"__typename":"Product","upc":"1"}]}}`),
		)
		initialFetch := &SubgraphRule{
			Match:    `warehouse`,
			Response: `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`,
			Headers:  http.Header{"Cache-Control": []string{"no-store"}},
		}
		deferredFetch := Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`)
		inventory := Rules(initialFetch, deferredFetch)
		executionEngine := NewEngine(t, inventoryL1Caching(time.Minute), Subgraphs{"products": products, "inventory": inventory})

		frames := ExecuteDefer(t, executionEngine,
			`{ product(upc: "1") { stock warehouse { id location } } products(first: 1) { upc ... @defer { stock } } }`,
			cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, []string{
			`{"data":{"product":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
			// stock 999, the tampered value: nothing was in L1 to serve, so the
			// deferred group really fetched.
			`{"incremental":[{"data":{"stock":999},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
		}, frames)
		assert.Equal(t, int64(1), initialFetch.Count.Load())
		assert.Equal(t, int64(1), deferredFetch.Count.Load())

		// Two lookups (one per fetch, neither served) and NOT ONE write: the
		// deferred fetch answered without a Cache-Control and would have written,
		// but it never reached the flush because its own result is a fresh miss
		// whose write is the second op.
		ops := store.Ops()
		require.Len(t, ops, 3)
		require.Len(t, ops[0].Keys, 1)
		key := ops[0].Keys[0]
		assert.Equal(t, []cachetesting.StoreOp{
			// The initial fetch's lookup.
			{
				Kind: "GetMany",
				Keys: []string{key},
				Hits: []bool{false},
			},
			// The deferred fetch's lookup under the SAME key: the no-store result
			// left the entry unwritten, so it misses too.
			{
				Kind: "GetMany",
				Keys: []string{key},
				Hits: []bool{false},
			},
			// The request-end flush carries the DEFERRED fetch's value only — the
			// no-store result contributed nothing.
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key,
						Value: `{"data":{"__typename":"Product","stock":999},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
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
	})

	t.Run("no-cache behaves as no-store: nothing is written", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "no-cache, max-age=600")
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// The lookup happened; the max-age riding alongside no-cache never
		// produced a write.
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:4f796e3bbd360fce"},
				Hits: []bool{false},
			},
		}, store.Ops())
	})

	t.Run("a runtime-private result writes no store entry and is reported once", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		observer := &cachetesting.RecordingObserver{}
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "private, max-age=600")
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, cachetesting.NewRealishCache(store, observer))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:4f796e3bbd360fce"},
				Hits: []bool{false},
			},
		}, store.Ops())
		// One hint per fetch result, naming the datasource that declared private
		// (datasource 1 is inventory, as its key prefix shows) and the remedy.
		assert.Equal(t, []cache.UncacheablePrivate{
			{Subgraph: "1", Reason: cache.UncacheablePrivateResponseHeader},
		}, observer.UncacheablePrivate())

		// The next request finds nothing and fetches again.
		assert.Equal(t, body, Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, cachetesting.NewRealishCache(store, observer)))
		assert.Equal(t, int64(2), inventory.Requests())
	})
}
