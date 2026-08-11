package cachingtesting

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// requesterIdentity is the integrator's partition hook: the identity this
// request's private cache entries belong to, the same for every subgraph. An
// empty identity is the anonymous request the hook cannot name.
type requesterIdentity string

func (r requesterIdentity) PrivatePartition(*resolve.Context, string) (string, bool) {
	return string(r), r != ""
}

// forwardedHeaders is the subgraph header source: one header line per subgraph
// request plus the hash the engine identifies it by — the hash the cache uses
// to vary public entries and to partition private ones.
type forwardedHeaders struct {
	tenant string
	hash   uint64
}

func (f forwardedHeaders) HeadersForSubgraph(string) (http.Header, uint64) {
	return http.Header{"X-Tenant": []string{f.tenant}}, f.hash
}

func (f forwardedHeaders) HashAll() uint64 {
	return f.hash
}

// TestPrivatePartitionEndToEnd drives privacy through the REAL ExecutionEngine
// over REAL HTTP upstreams: who gets their own entry, who gets none, and what
// the store ends up holding.
func TestPrivatePartitionEndToEnd(t *testing.T) {
	t.Run("two requesters alternate: two entries, each hit only by its own owner", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {
					DefaultTTL: ptr(time.Minute),
					Scope:      ptr(cacheconfig.CacheScopePrivate),
				},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})
		query := `{ me { favoriteProduct { upc stock } } }`
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`

		// Request 1 (user-a): miss, one write.
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-a"))))
		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		keyA := ops[0].Keys[0]

		// Request 2 (user-b): the SAME entity, a different key — so a miss again.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-b"))))
		ops = store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		keyB := ops[0].Keys[0]
		assert.NotEqual(t, keyA, keyB)
		// Neither is the key a public inventory fetch derives (pinned literally by
		// the public rows), so the partition really moved the entry.
		assert.NotEqual(t, "v1:1:4f796e3bbd360fce", keyA)
		assert.NotEqual(t, "v1:1:4f796e3bbd360fce", keyB)
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{keyB},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   keyB,
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"private"}}`,
						TTL:   time.Minute,
						// The partition lives in the KEY only: user B's tags are
						// byte-identical to user A's, so one purge of the product
						// clears every requester's partition of it.
						Tags: []string{
							"subgraph:1",
							"type:1:Product",
							"entity:1:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))

		// Requests 3 and 4 repeat both identities: each hits its OWN entry, and a
		// served hit owes no write.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-a"))))
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-b"))))
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{keyA},
				Hits: []bool{true},
			},
			{
				Kind: "GetMany",
				Keys: []string{keyB},
				Hits: []bool{true},
			},
		}, store.Ops())
		// Exactly two inventory fetches for four requests: the two misses.
		assert.Equal(t, int64(2), inventory.Requests())
	})

	t.Run("an anonymous requester writes nothing but still reuses within its request", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		observer := &cachetesting.RecordingObserver{}
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {
					DefaultTTL: ptr(time.Minute),
					Scope:      ptr(cacheconfig.CacheScopePrivate),
				},
			},
		}
		products := Rules(
			Rule(`product(upc: $a)`, `{"data":{"product":{"__typename":"Product","upc":"1"},"products":[{"__typename":"Product","upc":"1"}]}}`),
		)
		// The initial fetch selects {stock, warehouse}; the DEFERRED fetch of the
		// same representation selects {stock} alone and would ride L1. Its canned
		// stock is tampered to 999, so any real network use is visible.
		initialFetch := Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`)
		deferredFetch := Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`)
		inventory := Rules(initialFetch, deferredFetch)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		frames := ExecuteDefer(t, executionEngine,
			`{ product(upc: "1") { stock warehouse { id location } } products(first: 1) { upc ... @defer { stock } } }`,
			cachetesting.NewRealishCache(store, observer),
			engine.WithPrivatePartitionProvider(requesterIdentity("")))
		assert.Equal(t, []string{
			`{"data":{"product":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
			// stock 5, not the tampered 999: the deferred group read L1, which
			// privacy never disables.
			`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
		}, frames)
		assert.Equal(t, int64(1), initialFetch.Count.Load())
		assert.Equal(t, int64(0), deferredFetch.Count.Load())
		// Not one store op: no key was derived for either fetch.
		assert.Equal(t, []cachetesting.StoreOp(nil), store.Ops())
		// One hint per fetch that lost its store access, naming the remedy.
		assert.Equal(t, []cache.UncacheablePrivate{
			{Subgraph: "1", Reason: cache.UncacheablePrivateNoIdentity},
			{Subgraph: "1", Reason: cache.UncacheablePrivateNoIdentity},
		}, observer.UncacheablePrivate())
	})

	t.Run("without a hook the forwarded header hash partitions the entries", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {
					DefaultTTL:             ptr(time.Minute),
					Scope:                  ptr(cacheconfig.CacheScopePrivate),
					IncludeSubgraphHeaders: ptr(true),
				},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})
		query := `{ me { favoriteProduct { upc stock } } }`
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`

		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithSubgraphHeadersBuilder(forwardedHeaders{tenant: "acme", hash: 111})))
		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		acmeKey := ops[0].Keys[0]

		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithSubgraphHeadersBuilder(forwardedHeaders{tenant: "globex", hash: 222})))
		ops = store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		globexKey := ops[0].Keys[0]
		assert.NotEqual(t, acmeKey, globexKey)
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{globexKey},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   globexKey,
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"private"}}`,
						TTL:   time.Minute,
						// The header-derived partition rides the key alone; the
						// tags carry no header material either.
						Tags: []string{
							"subgraph:1",
							"type:1:Product",
							"entity:1:Product:d3cc039c7a9789e7", // upc "1"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))

		// The first tenant's repeat request hits its own entry — nothing crossed.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithSubgraphHeadersBuilder(forwardedHeaders{tenant: "acme", hash: 111})))
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{acmeKey},
				Hits: []bool{true},
			},
		}, store.Ops())
	})

	t.Run("config drift: a private entry under a public key is discarded and refetched", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		observer := &cachetesting.RecordingObserver{}
		// The deployment that wrote this entry had inventory PRIVATE; this one has
		// it public, so the very key it reads carries a private-scoped value.
		store.Seed("v1:1:4f796e3bbd360fce",
			[]byte(`{"data":{"__typename":"Product","stock":999},"cc":{"ttl":60,"created":1785852117,"scope":"private"}}`),
			time.Minute)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`,
			cachetesting.NewRealishCache(store, observer))
		// stock 5 from the origin, never the seeded 999.
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)
		assert.Equal(t, int64(1), inventory.Requests())
		assert.Equal(t, map[cache.ScopeMismatch]int{
			{Subgraph: "1", StoredScope: "private"}: 1,
		}, observer.ScopeMismatches())
		// The read HIT and was thrown away; the fresh public value replaced it.
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
	})

	t.Run("a public subgraph keys exactly as it did before privacy existed", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(5 * time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`,
			cachetesting.NewRealishCache(store, nil),
			// A partition provider that WOULD identify this requester changes
			// nothing: only a private declaration partitions a key.
			engine.WithPrivatePartitionProvider(requesterIdentity("user-a")))
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
}
