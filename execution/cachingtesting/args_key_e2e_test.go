package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
)

// TestArgumentVariantsAreIndependentEntriesEndToEnd: interleaved traffic over
// two argument variants of one entity field warms TWO stable entries instead of
// replacing one shared entry on every request. Four requests, alternating the
// argument, give the hit pattern miss, miss, hit, hit — and the subgraph is
// asked exactly once per variant.
func TestArgumentVariantsAreIndependentEntriesEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	// One rule per argument value (the engine renames $days to $a in the
	// rendered body); each rule's count proves how often its variant fetched.
	days3 := Rule(`"a":3`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[1,2,3]}]}}`)
	days1 := Rule(`"a":1`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[7]}]}}`)
	inventory := Rules(days3, days1)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	query := `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`

	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`,
		ExecuteWithVariables(t, executionEngine, query, `{"days":3}`, controller))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[7]}}}}`,
		ExecuteWithVariables(t, executionEngine, query, `{"days":1}`, controller))
	// The repeats are served from the entries the first two requests wrote —
	// neither variant evicted the other.
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`,
		ExecuteWithVariables(t, executionEngine, query, `{"days":3}`, controller))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[7]}}}}`,
		ExecuteWithVariables(t, executionEngine, query, `{"days":1}`, controller))

	assert.Equal(t, int64(1), days3.Count.Load())
	assert.Equal(t, int64(1), days1.Count.Load())
	assert.Equal(t, int64(2), inventory.Requests())

	// Two distinct keys — the argument value rides in the preimage — each read
	// twice and written once, with its own envelope.
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
					// request 1's write: the days:3 value under the days:3 key
					Key:   "v1:1:1ea890ef1fe73466",
					Value: `{"data":{"stockHistory_f123752fc2272dfc":[1,2,3],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
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
					// request 2's write: a SECOND entry, not a replacement
					Key:   "v1:1:e58401b99256d16d",
					Value: `{"data":{"stockHistory_b487e21691ea4c86":[7],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					// A DIFFERENT key, the SAME entity tag as the days:3 entry:
					// the tag hashes the @key subset alone, so one purge of the
					// product clears every argument variant of it.
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:1ea890ef1fe73466"},
			Hits: []bool{true}, // request 3: days:3 survived — days:1 wrote elsewhere
		},
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:e58401b99256d16d"},
			Hits: []bool{true}, // request 4: and days:1 survived too
		},
	}, NormalizeStoreOpsClock(store.Ops()))
}

// TestAliasedArgumentVariantsShareOneEntryEndToEnd: two variants of one
// parameterized field selected under aliases in a SINGLE operation stay fully
// cacheable — one key, one entry carrying both argument-suffixed names — and a
// second operation selecting the same two variants under DIFFERENT aliases is
// served from it.
func TestAliasedArgumentVariantsShareOneEntryEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	// The subgraph answers under the aliases the fetch sends.
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","recent":[1,2,3],"older":[7]}]}}`)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	firstBody := ExecuteWithVariables(t, executionEngine,
		`query($long: Int!, $short: Int!) { me { favoriteProduct { upc recent: stockHistory(days: $long) older: stockHistory(days: $short) } } }`,
		`{"long":3,"short":1}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","recent":[1,2,3],"older":[7]}}}}`, firstBody)
	assert.Equal(t, int64(1), inventory.Requests())

	// ONE entry for the whole selection, its aliases normalized away and both
	// argument variants of stockHistory living side by side inside it.
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:b26f0f685a0b663d"},
			Hits: []bool{false}, // first request: one key for both variants, nothing stored yet
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					// both variants in ONE entry, under their suffixed names
					Key:   "v1:1:b26f0f685a0b663d",
					Value: `{"data":{"stockHistory_f123752fc2272dfc":[1,2,3],"stockHistory_b487e21691ea4c86":[7],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
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

	// A differently-aliased operation with the SAME argument values renders the
	// same digest, hence the same key, and is served without a fetch.
	store.ResetOps()
	secondBody := ExecuteWithVariables(t, executionEngine,
		`query($a: Int!, $b: Int!) { me { favoriteProduct { upc fresh: stockHistory(days: $a) stale: stockHistory(days: $b) } } }`,
		`{"a":3,"b":1}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","fresh":[1,2,3],"stale":[7]}}}}`, secondBody)
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:b26f0f685a0b663d"},
			Hits: []bool{true}, // the other aliases derive the same digest, hence the same key
		},
	}, store.Ops())
}

// TestArgumentlessSelectionKeepsItsKeyEndToEnd pins that a fetch selecting no
// parameterized field keys exactly as it did before the argument segment
// existed: the literal store key of the plain stock selection on the inventory
// subgraph, unchanged.
func TestArgumentlessSelectionKeepsItsKeyEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	body := Execute(t, executionEngine, `{ me { favoriteProduct { upc stock } } }`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)

	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:4f796e3bbd360fce"},
			Hits: []bool{false}, // cold store: the plain stock selection misses
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					// the write lands on the read's key — no argument segment anywhere
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
}

// TestMatchingDigestStillRunsTheCoverageWalkEndToEnd: only PARAMETERIZED fields
// enter the digest, so two selections differing in a plain field share one key.
// The narrower entry is therefore still gated by the coverage walk — found in
// the store, rejected as not covering, refetched — and the widened entry then
// serves.
func TestMatchingDigestStillRunsTheCoverageWalkEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	// The warehouse selection identifies the first operation's fetch; the
	// catch-all serves the second one, which asks for stock instead.
	withWarehouse := Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","warehouse":{"location":"Berlin"},"stockHistory":[1,2,3]}]}}`)
	withStock := Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":5,"stockHistory":[1,2,3]}]}}`)
	inventory := Rules(withWarehouse, withStock)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	warehouseBody := ExecuteWithVariables(t, executionEngine,
		`query($days: Int!) { me { favoriteProduct { upc warehouse { location } stockHistory(days: $days) } } }`,
		`{"days":3}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","warehouse":{"location":"Berlin"},"stockHistory":[1,2,3]}}}}`, warehouseBody)
	assert.Equal(t, int64(1), withWarehouse.Count.Load())

	// Same entity, same argument value, a plain field swapped: the SAME key,
	// found in the store, but the entry does not cover stock — so the fetch runs
	// and rewrites the entry.
	stockBody := ExecuteWithVariables(t, executionEngine,
		`query($days: Int!) { me { favoriteProduct { upc stock stockHistory(days: $days) } } }`,
		`{"days":3}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5,"stockHistory":[1,2,3]}}}}`, stockBody)
	assert.Equal(t, int64(1), withStock.Count.Load())

	// The rewritten entry covers the second selection: a repeat serves without a
	// fetch.
	repeatBody := ExecuteWithVariables(t, executionEngine,
		`query($days: Int!) { me { favoriteProduct { upc stock stockHistory(days: $days) } } }`,
		`{"days":3}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5,"stockHistory":[1,2,3]}}}}`, repeatBody)
	assert.Equal(t, int64(1), withStock.Count.Load())
	assert.Equal(t, int64(2), inventory.Requests())

	// One key throughout: Hits reports STORE presence, not servability — read 2
	// found the entry and the coverage walk still rejected it.
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:1ea890ef1fe73466"},
			Hits: []bool{false}, // request 1: nothing written yet
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					// request 1's write: warehouse, no stock
					Key:   "v1:1:1ea890ef1fe73466",
					Value: `{"data":{"warehouse":{"location":"Berlin"},"stockHistory_f123752fc2272dfc":[1,2,3],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:1ea890ef1fe73466"},
			Hits: []bool{true}, // request 2: present, but it carries no stock
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					// request 2's write: the refetched value REPLACES the entry under the same key
					Key:   "v1:1:1ea890ef1fe73466",
					Value: `{"data":{"stock":5,"stockHistory_f123752fc2272dfc":[1,2,3],"__typename":"Product"},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
		{
			Kind: "GetMany",
			Keys: []string{"v1:1:1ea890ef1fe73466"},
			Hits: []bool{true}, // request 3: covered this time, served
		},
	}, NormalizeStoreOpsClock(store.Ops()))
}
