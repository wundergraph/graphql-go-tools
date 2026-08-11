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

// TestClientCacheAnswerEndToEnd drives the client cache answer through the REAL
// ExecutionEngine over REAL HTTP upstreams: what the integrator reads back
// after an execution is folded from the same fetches that produced the
// response. The committed fixture names its datasources by subgraph id — "0" is
// products, "1" is inventory.
func TestClientCacheAnswerEndToEnd(t *testing.T) {
	t.Run("the shortest-lived part decides the answer, and a warm request answers what is left", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})
		query := `{ products(first: 1) { upc stock } }`

		var cold resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, query, controller, engine.WithCacheResponseInfo(&cold)))
		// Both parts were written in this request, so both contribute the
		// lifetime they were written under: the inventory minute wins over the
		// root field's five, with no clock in the arithmetic.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, cold)

		var warm resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, query, controller, engine.WithCacheResponseInfo(&warm)))
		// Neither subgraph was touched again: both parts came from their entries
		// and contributed what is LEFT of them.
		assert.Equal(t, int64(1), products.Requests())
		assert.Equal(t, int64(1), inventory.Requests())
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    -1,
		}, NormalizeServedFreshness(warm))
	})

	t.Run("a no-store part makes the whole response no-store", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := RespondCacheControl(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`, "no-store")
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		// The root field's five minutes are not reported alongside the part the
		// origin refused to have stored.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, info)
	})

	t.Run("a fetch of an unconfigured subgraph makes the response no-store", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		// users and products carry no policy at all: their data is as dynamic as
		// this response gets, whatever the inventory entry is good for.
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`,
			Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, info)

		// The store side is untouched by the client answer: the inventory entry
		// is written and serves the next request as usual.
		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		key := ops[0].Keys[0]
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
	})

	t.Run("an operation with no cache-configured fetch answers nothing at all", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`)
		executionEngine := NewEngine(t, nil, Subgraphs{"products": products})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc name } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		// The EMPTY answer, not a no-store one: the integrator emits no cache
		// header rather than forbidding a cache that was never consulted.
		assert.Equal(t, resolve.CacheResponseInfo{}, info)
		// Not one store round trip either: no fetch of this operation is the
		// cache's business.
		assert.Equal(t, []cachetesting.StoreOp(nil), store.Ops())
	})

	t.Run("a private part marks the response private", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {
					DefaultTTL: ptr(time.Minute),
					Scope:      ptr(cacheconfig.CacheScopePrivate),
				},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller,
				engine.WithCacheResponseInfo(&info),
				engine.WithPrivatePartitionProvider(requesterIdentity("user-a"))))
		// The private entry WAS written — into this requester's partition — so
		// the response stays fresh for a minute, for them alone. One private
		// part is enough to mark the response the public root field shares it
		// with.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
			Private:   true,
		}, info)
	})

	t.Run("a @defer operation answers nothing: its parts resolve after the headers ship", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		frames := ExecuteDefer(t, executionEngine,
			`{ products(first: 1) { upc ... @defer { stock } } }`, controller,
			engine.WithCacheResponseInfo(&info))
		assert.Equal(t, []string{
			`{"data":{"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
			`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
		}, frames)
		// The deferred inventory entry was still written — only the CLIENT
		// answer is suppressed, because it would have shipped with the initial
		// frame while that fetch was outstanding.
		assert.Equal(t, resolve.CacheResponseInfo{}, info)
		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 1)
		key := ops[0].Keys[0]
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key},
				Hits: []bool{false}, // the deferred group's lookup on a cold store
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key,
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
	})

	t.Run("CDN tags are the union of every contributing entry, sorted", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{EmitCdnTags: true},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		controller := cachetesting.NewRealishCache(store, nil, cache.WithGlobalConfig(caching.Global))
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		// One address per contributing entry, coarse and fine: purging
		// "subgraph:1" or the single product's entity tag both reach this
		// response in the CDN.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
			Tags: []string{
				"entity:1:Product:d3cc039c7a9789e7", // upc "1"
				"subgraph:0",
				"subgraph:1",
				"type:0:Query",
				"type:1:Product",
			},
		}, info)
	})

	t.Run("CDN tags stay unaccumulated while their emission knob is off", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		// The entries carry their tags into the store all the same — only the
		// response-side union is off.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			MaxAge:    time.Minute,
		}, info)
		ops := store.Ops()
		writes := ops[len(ops)-1]
		assert.Equal(t, "SetMany", writes.Kind)
		assert.Equal(t, [][]string{
			{
				"subgraph:0",
				"type:0:Query",
			},
			{
				"subgraph:1",
				"type:1:Product",
				"entity:1:Product:d3cc039c7a9789e7", // upc "1"
			},
		}, [][]string{writes.Items[0].Tags, writes.Items[1].Tags})
	})

	t.Run("the client answer is off while its emission knob is off", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		caching := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{EmitClientCacheControl: ptr(false)},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		controller := cachetesting.NewRealishCache(store, nil, cache.WithGlobalConfig(caching.Global))
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`,
			Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		assert.Equal(t, resolve.CacheResponseInfo{}, info)
	})

	t.Run("a subgraph failure makes the response no-store", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		// Every fetch of the operation is cache-configured, so the no-store can
		// come from nothing but the failed result itself.
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					DefaultTTL: ptr(5 * time.Minute),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Rules(&SubgraphRule{
			Response: `{"errors":[{"message":"inventory is down"}]}`,
			Headers:  http.Header{"Cache-Control": []string{"max-age=600"}},
		})
		executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory})

		var info resolve.CacheResponseInfo
		assert.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph '1' at Path 'products'."},{"message":"Cannot return null for non-nullable field 'Query.products.stock'.","path":["products",0,"stock"]}],"data":null}`,
			Execute(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller,
				engine.WithCacheResponseInfo(&info)))
		// The origin's own max-age never applies to an errored result: nothing
		// was stored, so nothing may be.
		assert.Equal(t, resolve.CacheResponseInfo{
			HasPolicy: true,
			NoStore:   true,
		}, info)
		// Both lookups happened; the flush carries the root field that answered
		// and nothing of the subgraph that errored.
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:0:013d56d080493bfa"}, // Query.products
				Hits: []bool{false},
			},
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:4f796e3bbd360fce"}, // the Product entity
				Hits: []bool{false},
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   "v1:0:013d56d080493bfa",
						Value: `{"data":{"products":[{"__typename":"Product","upc":"1"}]},"cc":{"ttl":300,"created":-1,"scope":"public"}}`,
						TTL:   5 * time.Minute,
						Tags: []string{
							"subgraph:0",
							"type:0:Query",
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(store.Ops()))
	})
}
