package cachingtesting

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestStoreBatchingEndToEnd drives the REAL ExecutionEngine and pins the store
// CALL SHAPE end to end: one read per fetch carrying every key it needs, one
// write per request carrying every deferred item, and the degradation contract
// when the store fails.
func TestStoreBatchingEndToEnd(t *testing.T) {
	t.Run("a batch entity fetch reads all keys in one call and writes them in one call", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"reviews": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "reviews": reviews})

		body := Execute(t, executionEngine, `{ products(first: 2) { upc reviews { body } } }`, controller)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`, body)
		assert.Equal(t, int64(1), reviews.Requests())

		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 2)
		key1, key2 := ops[0].Keys[0], ops[0].Keys[1]
		assert.NotEqual(t, key1, key2)
		assert.Equal(t, []cachetesting.StoreOp{
			{
				// ONE read for the whole batch: both entity keys, both missing.
				Kind: "GetMany",
				Keys: []string{key1, key2},
				Hits: []bool{false, false},
			},
			{
				// ONE request-end write carrying both entities.
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key1,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
					{
						Key:   key2,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Wobbly"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))
	})

	t.Run("a fully served request reads once and writes nothing", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"reviews": {DefaultTTL: ptr(time.Minute)},
			},
		}
		query := `{ products(first: 2) { upc reviews { body } } }`
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "reviews": reviews})
		expected := `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`

		// Request 1 primes both entries.
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		primeOps := store.Ops()
		require.Len(t, primeOps, 2)
		require.Len(t, primeOps[0].Keys, 2)
		key1, key2 := primeOps[0].Keys[0], primeOps[0].Keys[1]

		// Request 2's ops assert in isolation.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		// The reviews upstream is NOT hit again.
		assert.Equal(t, int64(1), reviews.Requests())
		assert.Equal(t, []cachetesting.StoreOp{
			{
				// The full-batch hit: one read, both keys hitting. A served hit
				// owes no write, so the request ends with no write call at all.
				Kind: "GetMany",
				Keys: []string{key1, key2},
				Hits: []bool{true, true},
			},
		}, store.Ops())
	})

	t.Run("a failed read falls back to the origin and still writes back", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		observer := &cachetesting.RecordingObserver{}
		controller := cachetesting.NewRealishCache(store, observer)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"reviews": {DefaultTTL: ptr(time.Minute)},
			},
		}
		query := `{ products(first: 2) { upc reviews { body } } }`
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "reviews": reviews})
		expected := `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`

		// Request 1 primes both entries, so only the injected failure can send
		// request 2 back to the origin.
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		primeOps := store.Ops()
		require.Len(t, primeOps, 2)
		require.Len(t, primeOps[0].Keys, 2)
		key1, key2 := primeOps[0].Keys[0], primeOps[0].Keys[1]

		store.ResetOps()
		readErr := errors.New("store read down")
		store.FailGetMany(1, readErr)
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		// The response is correct AND the reviews upstream served it: every key
		// of the failed read is a miss.
		assert.Equal(t, int64(2), reviews.Requests())

		assert.Equal(t, []cachetesting.StoreOp{
			{
				// The failed call, with the keys it carried.
				Kind:   "GetMany",
				Keys:   []string{key1, key2},
				Failed: true,
			},
			{
				// The refetch still writes back: a failed read never blocks writes.
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key1,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
					{
						Key:   key2,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Wobbly"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(store.Ops()))

		// ONE event for the failed call, naming the fetch's datasource. The
		// committed fixture names its datasources by subgraph id, and "2" is
		// the reviews subgraph.
		assert.Equal(t, []cache.StoreError{
			{Op: "GetMany", Subgraph: "2", KeyCount: 2, Err: readErr},
		}, observer.StoreErrors())
	})

	t.Run("a failed write is dropped without touching the response", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		observer := &cachetesting.RecordingObserver{}
		controller := cachetesting.NewRealishCache(store, observer)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"reviews": {DefaultTTL: ptr(time.Minute)},
			},
		}
		query := `{ products(first: 2) { upc reviews { body } } }`
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "reviews": reviews})
		expected := `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`

		writeErr := errors.New("store write down")
		store.FailSetMany(1, writeErr)
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))

		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 2)
		key1, key2 := ops[0].Keys[0], ops[0].Keys[1]
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key1, key2},
				Hits: []bool{false, false},
			},
			{
				// The flush was rejected, so nothing was stored.
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key1,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
					{
						Key:   key2,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Wobbly"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
				Failed: true,
			},
		}, NormalizeStoreOpsClock(ops))
		_, stored := store.Value(key1)
		assert.False(t, stored)

		// The request-end flush spans every fetch, so it names no subgraph.
		assert.Equal(t, []cache.StoreError{
			{Op: "SetMany", Subgraph: "", KeyCount: 2, Err: writeErr},
		}, observer.StoreErrors())

		// The dropped writes leave the entries unprimed: the next request is a
		// plain miss that fetches again and writes again (the injection budget
		// is spent, so this flush lands).
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		assert.Equal(t, int64(2), reviews.Requests())
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key1, key2},
				Hits: []bool{false, false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key1,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
					{
						Key:   key2,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Wobbly"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(store.Ops()))
	})
}
