package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// resolveWithVariables drives the public sync entry point with request
// variables set (argument-suffix keys derive from them).
func resolveWithVariables(t *testing.T, result PlanResult, variables string, controller resolve.CacheController) string {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(variables))
	ctx.SetCacheController(controller)
	return resolveWithContext(t, ctx, result.Response)
}

// TestAliasIndependentReuseEndToEnd: operation A caches the inventory entity
// under one alias; operation B selects the same field under a DIFFERENT alias
// and is served from the cache (zero inventory loads), each with its complete
// response asserted.
func TestAliasIndependentReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()

	first := Plan(t, `{ me { favoriteProduct { upc inStock: stock } } }`, inventoryCaching(), map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","inStock":5}]}}`,
	})
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","inStock":5}}}}`, firstBody)
	assert.Equal(t, int64(1), first.LoadCount("inventory", "me.favoriteProduct"))

	// The STORED form is normalized to the schema name.
	ops := store.Ops()
	require.Len(t, ops, 2)
	assert.Equal(t, cachetesting.StoreOp{
		Kind:  "Set",
		Key:   ops[1].Key,
		Value: `{"stock":5,"__typename":"Product"}`,
		TTL:   time.Minute,
	}, ops[1])

	second := Plan(t, `{ me { favoriteProduct { upc availability: stock } } }`, inventoryCaching(), map[string]string{
		"users":       `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
		"products:me": `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
	})
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","availability":5}}}}`, secondBody)
	assert.Equal(t, int64(0), second.LoadCount("inventory", "me.favoriteProduct"))
}

// TestArgumentMismatchEndToEnd: the same entity field with DIFFERENT argument
// values must never share a cache entry — request B misses and fetches.
func TestArgumentMismatchEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()

	responsesFor := func(history string) map[string]string {
		return map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stockHistory":` + history + `}]}}`,
		}
	}

	first := Plan(t, `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`,
		inventoryCaching(), responsesFor(`[1,2,3]`))
	firstBody := resolveWithVariables(t, first, `{"days":3}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`, firstBody)
	assert.Equal(t, int64(1), first.LoadCount("inventory", "me.favoriteProduct"))

	// Same variables → HIT (the suffix matches).
	sameArgs := Plan(t, `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`,
		inventoryCaching(), responsesFor(`[9,9,9]`))
	sameBody := resolveWithVariables(t, sameArgs, `{"days":3}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[1,2,3]}}}}`, sameBody)
	assert.Equal(t, int64(0), sameArgs.LoadCount("inventory", "me.favoriteProduct"))

	// Different variables → MISS: the fetch runs and returns the new data.
	differentArgs := Plan(t, `query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }`,
		inventoryCaching(), responsesFor(`[7]`))
	differentBody := resolveWithVariables(t, differentArgs, `{"days":1}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stockHistory":[7]}}}}`, differentBody)
	assert.Equal(t, int64(1), differentArgs.LoadCount("inventory", "me.favoriteProduct"))
}
