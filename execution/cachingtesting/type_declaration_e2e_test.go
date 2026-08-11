package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestTypeDeclarationEndToEnd drives the narrowest static tier — the per-type
// declaration composition carries out of a subgraph's @cache directives —
// through the REAL ExecutionEngine over REAL HTTP upstreams: what it does to a
// written entry's lifetime, how it isolates one type of a subgraph from its
// siblings, and where the response header still overrules it.
func TestTypeDeclarationEndToEnd(t *testing.T) {
	t.Run("a declared type is written under its own lifetime, not the subgraph default", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {
					DefaultTTL: ptr(5 * time.Minute),
					Types: map[string]cacheconfig.TypeCacheConfig{
						"Product": {MaxAge: 90 * time.Second},
					},
				},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// Both the envelope's cc.ttl and the store item's TTL carry the declared
		// 90s instead of the subgraph's 5m.
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
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":90,"created":-1,"scope":"public"}}`,
						TTL:   90 * time.Second,
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

	t.Run("a type declared uncacheable touches the store not once while its sibling stays cached", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL:       ptr(time.Minute),
					NegativeCacheTTL: ptr(5 * time.Second),
					Types: map[string]cacheconfig.TypeCacheConfig{
						"Product": {MaxAge: 0},
					},
				},
			},
		}
		// One sequential chain reaching the products subgraph TWICE under two
		// entity types: users.me -> products(User) -> reviews(Product) ->
		// products(Product).
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		productsUser := Rule(`favoriteProduct`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		productsProduct := Rule(``, `{"data":{"_entities":[{"__typename":"Product","name":"Table"}]}}`)
		products := Rules(productsUser, productsProduct)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"2"}}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "reviews": reviews})
		query := `{ me { favoriteProduct { upc reviews { product { name } } } } }`
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","reviews":[{"product":{"name":"Table"}}]}}}}`

		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))

		// Only the User fetch reaches the store: the Product fetch of the SAME
		// subgraph carries no cache configuration at all, negative sentinel
		// included, so it neither looks up nor writes.
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:0:ae7bd989bf833fe9"},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   "v1:0:ae7bd989bf833fe9",
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
		}, NormalizeStoreOpsClock(store.Ops()))

		// A second request serves the User fetch from L2 and fetches the
		// uncacheable Product again.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller))
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:0:ae7bd989bf833fe9"},
				Hits: []bool{true},
			},
		}, store.Ops())
		assert.Equal(t, int64(1), productsUser.Count.Load())
		assert.Equal(t, int64(2), productsProduct.Count.Load())
	})

	t.Run("a PRIVATE declaration partitions one type while its sibling shares publicly", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(time.Minute),
					Types: map[string]cacheconfig.TypeCacheConfig{
						"Product": {MaxAge: 30 * time.Second, Scope: cacheconfig.CacheScopePrivate},
					},
				},
			},
		}
		// The same sequential chain, reaching the products subgraph as User and
		// then as Product.
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		productsUser := Rule(`favoriteProduct`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		productsProduct := Rule(``, `{"data":{"_entities":[{"__typename":"Product","name":"Table"}]}}`)
		products := Rules(productsUser, productsProduct)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"2"}}]}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "reviews": reviews})
		query := `{ me { favoriteProduct { upc reviews { product { name } } } } }`
		expected := `{"data":{"me":{"favoriteProduct":{"upc":"1","reviews":[{"product":{"name":"Table"}}]}}}}`

		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-a"))))

		ops := store.Ops()
		require.Len(t, ops, 3)
		require.Len(t, ops[1].Keys, 1)
		privateProductKey := ops[1].Keys[0]
		// The declaration moved the Product entry out of the public keyspace the
		// undeclared chain writes it into.
		assert.NotEqual(t, "v1:0:f9ab765f03bb381a", privateProductKey)
		assert.Equal(t, []cachetesting.StoreOp{
			// The User fetch: undeclared, so it keeps the subgraph's public key.
			{
				Kind: "GetMany",
				Keys: []string{"v1:0:ae7bd989bf833fe9"},
				Hits: []bool{false},
			},
			// The Product fetch: partitioned by the hook's identity.
			{
				Kind: "GetMany",
				Keys: []string{privateProductKey},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   "v1:0:ae7bd989bf833fe9",
						Value: `{"data":{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:User",
							"entity:0:User:19ce389448be3351", // id "u1"
						},
					},
					{
						Key:   privateProductKey,
						Value: `{"data":{"__typename":"Product","name":"Table"},"cc":{"ttl":30,"created":-1,"scope":"private"}}`,
						TTL:   30 * time.Second,
						// The partition lives in the key alone, so a purge of the
						// product clears every requester's copy.
						Tags: []string{
							"subgraph:0",
							"type:0:Product",
							"entity:0:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))

		// A second requester hits the shared User entry and misses the Product
		// one, which belongs to user-a alone.
		store.ResetOps()
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-b"))))
		ops = store.Ops()
		require.Len(t, ops, 3)
		require.Len(t, ops[1].Keys, 1)
		assert.NotEqual(t, privateProductKey, ops[1].Keys[0])
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:0:ae7bd989bf833fe9"},
				Hits: []bool{true},
			},
			{
				Kind: "GetMany",
				Keys: []string{ops[1].Keys[0]},
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   ops[1].Keys[0],
						Value: `{"data":{"__typename":"Product","name":"Table"},"cc":{"ttl":30,"created":-1,"scope":"private"}}`,
						TTL:   30 * time.Second,
						Tags: []string{
							"subgraph:0",
							"type:0:Product",
							"entity:0:Product:4f93140518d68e67", // upc "2"
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))
	})

	t.Run("a response max-age still beats the declaration", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {
					DefaultTTL: ptr(5 * time.Minute),
					Types: map[string]cacheconfig.TypeCacheConfig{
						"Product": {MaxAge: 90 * time.Second},
					},
				},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "max-age=60")
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// The declaration only replaces the STATIC tier; the runtime truth on the
		// wire wins over it exactly as it wins over a subgraph DefaultTTL.
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
}
