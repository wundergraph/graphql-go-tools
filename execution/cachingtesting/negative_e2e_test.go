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
func TestNegativeCachingEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ me { favoriteProduct { upc stock } } }`
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"404"}}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[null]}}`,
	}

	first := Plan(t, query, inventoryNegativeCaching(), responses)
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, int64(1), first.LoadCount("inventory", "me.favoriteProduct"))

	// The sentinel was written with the NEGATIVE TTL.
	ops := store.Ops()
	require.Len(t, ops, 2)
	key := ops[0].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: "null", TTL: 10 * time.Second},
	}, ops)

	second := Plan(t, query, inventoryNegativeCaching(), responses)
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, int64(0), second.LoadCount("inventory", "me.favoriteProduct"))

	// The negative hit reproduces the empty-fetch response BYTE-IDENTICALLY —
	// the same null bubble AND the same non-null error the uncached path
	// renders; caching never changes the response (G6 end to end).
	assert.Equal(t, firstBody, secondBody)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.me.favoriteProduct.stock'.","path":["me","favoriteProduct","stock"]}],"data":{"me":{"favoriteProduct":null}}}`, firstBody)
}
