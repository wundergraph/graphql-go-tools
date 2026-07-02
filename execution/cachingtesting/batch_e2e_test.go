package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// reviewsEntityCaching enables entity caching for the reviews subgraph's
// Product (the batch entity fetch under products).
func reviewsEntityCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"reviews": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute},
			},
		},
	}
}

// TestBatchEntityL2EndToEnd: request 1 populates one entry PER entity from the
// batch response; request 2 is a full-batch hit with ZERO network to reviews.
func TestBatchEntityL2EndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ products(first: 2) { upc reviews { body } } }`
	responses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`,
	}
	expected := `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`

	first := Plan(t, query, reviewsEntityCaching(), responses)
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, firstBody)
	assert.Equal(t, int64(1), first.LoadCount("reviews", "products"))

	firstOps := store.Ops()
	require.Len(t, firstOps, 4)
	key1, key2 := firstOps[0].Key, firstOps[1].Key
	assert.NotEqual(t, key1, key2)
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key1},
		{Kind: "Get", Key: key2},
		{Kind: "Set", Key: key1, Value: `{"__typename":"Product","reviews":[{"body":"Solid"}]}`, TTL: time.Minute},
		{Kind: "Set", Key: key2, Value: `{"__typename":"Product","reviews":[{"body":"Wobbly"}]}`, TTL: time.Minute},
	}, firstOps)

	second := Plan(t, query, reviewsEntityCaching(), responses)
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, secondBody)
	assert.Equal(t, int64(0), second.LoadCount("reviews", "products"))
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key1},
		{Kind: "Get", Key: key2},
		{Kind: "Set", Key: key1, Value: `{"__typename":"Product","reviews":[{"body":"Solid"}]}`, TTL: time.Minute},
		{Kind: "Set", Key: key2, Value: `{"__typename":"Product","reviews":[{"body":"Wobbly"}]}`, TTL: time.Minute},
		{Kind: "Get", Key: key1},
		{Kind: "Get", Key: key2},
	}, store.Ops())
}

// TestBatchEntityMixedRun: only ONE of two entities is primed; the batch must
// fully refetch (no partial serving in this task), produce the correct
// response, and write entries for ALL entities afterwards.
func TestBatchEntityMixedRun(t *testing.T) {
	store := cachetesting.NewFakeStore()

	// Prime ONLY upc 1 through a single-product run.
	prime := Plan(t, `{ products(first: 1) { upc reviews { body } } }`, reviewsEntityCaching(), map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`,
	})
	ResolveResponse(t, prime.Response, cachetesting.NewRealishCache(store, nil))
	require.Equal(t, int64(1), prime.LoadCount("reviews", "products"))

	// The mixed batch (upc 1 primed, upc 3 not) refetches EVERYTHING.
	query := `{ products(first: 2) { upc reviews { body } } }`
	mixed := Plan(t, query, reviewsEntityCaching(), map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"New"}]}]}}`,
	})
	mixedBody := ResolveResponse(t, mixed.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"3","reviews":[{"body":"New"}]}]}}`, mixedBody)
	assert.Equal(t, int64(1), mixed.LoadCount("reviews", "products"))

	// Afterwards BOTH entities are cached: a repeat of the mixed batch hits.
	repeat := Plan(t, query, reviewsEntityCaching(), map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}`,
		"reviews":  ``,
	})
	repeatBody := ResolveResponse(t, repeat.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"3","reviews":[{"body":"New"}]}]}}`, repeatBody)
	assert.Equal(t, int64(0), repeat.LoadCount("reviews", "products"))
}
