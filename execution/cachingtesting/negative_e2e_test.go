package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// inventoryNegativeCaching enables entity caching with a negative TTL.
func inventoryNegativeCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute, NegativeCacheTTL: 10 * time.Second},
			},
		},
	}
}

// TestNegativeCachingEndToEnd: request 1 fetches a nonexistent entity (the
// subgraph SUCCESSFULLY returns a null entity) and writes the sentinel;
// request 2 serves the same null response with ZERO network to inventory.
// Runs through the REAL ExecutionEngine over HTTP subgraph doubles.
func TestNegativeCachingEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	query := `{ me { favoriteProduct { upc stock } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"404"}}]}}`)
	inventory := Respond(`{"data":{"_entities":[null]}}`)
	executionEngine := NewEngine(t, inventoryNegativeCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

	firstBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, int64(1), inventory.Requests())

	// The sentinel was written with the NEGATIVE TTL.
	ops := store.Ops()
	require.Len(t, ops, 2)
	key := ops[0].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: "null", TTL: 10 * time.Second},
	}, ops)

	secondBody := Execute(t, executionEngine, query, controller)
	// ZERO new network to inventory: the negative sentinel served.
	assert.Equal(t, int64(1), inventory.Requests())

	// The negative hit reproduces the empty-fetch response BYTE-IDENTICALLY —
	// the same null bubble AND the same non-null error the uncached path
	// renders; caching never changes the response (G6 end to end).
	assert.Equal(t, firstBody, secondBody)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.me.favoriteProduct.stock'.","path":["me","favoriteProduct","stock"]}],"data":{"me":{"favoriteProduct":null}}}`, firstBody)
}
