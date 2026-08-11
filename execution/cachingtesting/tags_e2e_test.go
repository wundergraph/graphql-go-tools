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

// TestDefaultTagsEndToEnd drives the default tag vocabulary through the REAL
// ExecutionEngine over REAL HTTP upstreams. Keys are identity and tags are
// addressing, so the rows below all probe the same property from different
// sides: entries whose KEYS must differ still have to be reachable by ONE
// entity tag. The committed fixture names its datasources by subgraph id — "1"
// is inventory, "2" is reviews.
func TestDefaultTagsEndToEnd(t *testing.T) {
	t.Run("a batch write tags every entity of the batch on its own @key subset", func(t *testing.T) {
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

		ops := store.Ops()
		require.Len(t, ops, 2)
		require.Len(t, ops[0].Keys, 2)
		key1, key2 := ops[0].Keys[0], ops[0].Keys[1]
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{key1, key2},
				Hits: []bool{false, false}, // cold store: both entities miss
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   key1,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Solid"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						// One batch, one subgraph, one type: the two items'
						// tag sets differ in the entity digest ALONE.
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:d3cc039c7a9789e7", // xxhash64 of `{"upc":"1"}`
						},
					},
					{
						Key:   key2,
						Value: `{"data":{"__typename":"Product","reviews":[{"body":"Wobbly"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags: []string{
							"subgraph:2",
							"type:2:Product",
							"entity:2:Product:4f93140518d68e67", // xxhash64 of `{"upc":"2"}`
						},
					},
				},
			},
		}, NormalizeStoreOpsClock(ops))
	})

	t.Run("argument variants are two entries under ONE entity tag", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		// One rule per argument value (the engine renames $days to $a in the
		// rendered body).
		inventory := Rules(
			Rule(`"a":3`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[1,2,3]}]}}`),
			Rule(`"a":1`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[7]}]}}`),
		)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})
		query := `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`

		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`,
			ExecuteWithVariables(t, executionEngine, query, `{"days":3}`, controller))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[7]}}}}`,
			ExecuteWithVariables(t, executionEngine, query, `{"days":1}`, controller))

		wantTags := []string{
			"subgraph:1",
			"type:1:Product",
			"entity:1:Product:d3cc039c7a9789e7", // xxhash64 of `{"upc":"1"}` — the args digest is not in it
		}
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:1ea890ef1fe73466"},
				Hits: []bool{false}, // request 1: the days:3 entry does not exist yet
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   "v1:1:1ea890ef1fe73466",
						Value: `{"data":{"stockHistory_f123752fc2272dfc":[1,2,3],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags:  wantTags,
					},
				},
			},
			{
				Kind: "GetMany",
				Keys: []string{"v1:1:e58401b99256d16d"},
				Hits: []bool{false}, // request 2: days:1 looks under its OWN key and misses
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						// A SECOND entry under a different key, carrying the
						// IDENTICAL tag set: one purge of the product clears
						// every argument variant of it.
						Key:   "v1:1:e58401b99256d16d",
						Value: `{"data":{"stockHistory_b487e21691ea4c86":[7],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
						TTL:   time.Minute,
						Tags:  wantTags,
					},
				},
			},
		}, NormalizeStoreOpsClock(store.Ops()))
	})

	t.Run("private entries of two requesters carry no identity material in their tags", func(t *testing.T) {
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

		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-a"))))
		assert.Equal(t, expected, Execute(t, executionEngine, query, controller,
			engine.WithPrivatePartitionProvider(requesterIdentity("user-b"))))

		ops := NormalizeStoreOpsClock(store.Ops())
		require.Len(t, ops, 4)
		keyA, keyB := ops[1].Items[0].Key, ops[3].Items[0].Key
		// The requester identity moved the KEYS apart — and away from the key a
		// public inventory fetch would derive.
		assert.NotEqual(t, keyA, keyB)
		assert.NotEqual(t, "v1:1:4f796e3bbd360fce", keyA)
		assert.NotEqual(t, "v1:1:4f796e3bbd360fce", keyB)

		// ... while both partitions are addressed by the very tags the public
		// entry of this entity would carry.
		wantTags := []string{
			"subgraph:1",
			"type:1:Product",
			"entity:1:Product:d3cc039c7a9789e7", // xxhash64 of `{"upc":"1"}` — no partition, no header, no identity
		}
		assert.Equal(t, []cachetesting.StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{keyA},
				Hits: []bool{false}, // user-a's partition is cold
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   keyA,
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"private"}}`,
						TTL:   time.Minute,
						Tags:  wantTags,
					},
				},
			},
			{
				Kind: "GetMany",
				Keys: []string{keyB},
				Hits: []bool{false}, // user-b derives another key, so it misses too
			},
			{
				Kind: "SetMany",
				Items: []cachetesting.StoreOpItem{
					{
						Key:   keyB,
						Value: `{"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":-1,"scope":"private"}}`,
						TTL:   time.Minute,
						Tags:  wantTags,
					},
				},
			},
		}, ops)
	})
}
