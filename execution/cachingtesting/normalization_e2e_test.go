package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
)

// TestAliasIndependentReuseEndToEnd: operation A caches the inventory entity
// under one alias; operation B selects the same field under a DIFFERENT alias
// and is served from the cache (zero inventory requests), each with its
// complete response asserted. Runs through the REAL ExecutionEngine.
func TestAliasIndependentReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","inStock":5}]}}`)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	firstBody := Execute(t, executionEngine, `{ me { favoriteProduct { upc inStock: stock } } }`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","inStock":5}}}}`, firstBody)
	assert.Equal(t, int64(1), inventory.Requests())

	// The STORED form is normalized to the schema name; the write key equals
	// the miss-lookup's key (read key == write key — ops[0] is the Get).
	ops := store.Ops()
	require.Len(t, ops, 2)
	assert.Equal(t, cachetesting.StoreOp{
		Kind: "Get",
		Key:  ops[0].Key,
	}, ops[0])
	assert.Equal(t, cachetesting.StoreOp{
		Kind:  "Set",
		Key:   ops[0].Key,
		Value: `{"stock":5,"__typename":"Product"}`,
		TTL:   time.Minute,
	}, ops[1])

	secondBody := Execute(t, executionEngine, `{ me { favoriteProduct { upc availability: stock } } }`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","availability":5}}}}`, secondBody)
	// Served from the cache: ZERO new inventory requests.
	assert.Equal(t, int64(1), inventory.Requests())
}

// TestArgumentMismatchEndToEnd: the same entity field with DIFFERENT argument
// values must never share a cache entry — request B misses and fetches.
func TestArgumentMismatchEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	// One rule per distinct argument value (the engine renames $days to $a in
	// the rendered body): the days:3 rule serves the first request only (the
	// repeat must be a cache hit — its count pins that); the days:1 rule
	// proves the different-arguments miss really fetched.
	days3 := Rule(`"a":3`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[1,2,3]}]}}`)
	days1 := Rule(`"a":1`, `{"data":{"_entities":[{"__typename":"Product","stockHistory":[7]}]}}`)
	inventory := Rules(days3, days1)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	query := `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`

	firstBody := ExecuteWithVariables(t, executionEngine, query, `{"days":3}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`, firstBody)
	assert.Equal(t, int64(1), days3.Count.Load())

	// Same variables → HIT (the suffix matches): no new days:3 request.
	sameBody := ExecuteWithVariables(t, executionEngine, query, `{"days":3}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`, sameBody)
	assert.Equal(t, int64(1), days3.Count.Load())
	assert.Equal(t, int64(0), days1.Count.Load())

	// Different variables → MISS: the fetch runs and returns the new data.
	differentBody := ExecuteWithVariables(t, executionEngine, query, `{"days":1}`, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[7]}}}}`, differentBody)
	assert.Equal(t, int64(1), days1.Count.Load())
}
