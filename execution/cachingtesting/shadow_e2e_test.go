package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// inventoryShadowCaching enables entity caching in SHADOW mode.
func inventoryShadowCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute, ShadowMode: true},
			},
		},
	}
}

// TestShadowModeEndToEnd: the response is ALWAYS the fresh network value even
// on an L2 hit; the shadow compare records the staleness probe.
func TestShadowModeEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ me { favoriteProduct { upc stock } } }`
	responsesFor := func(stock string) map[string]string {
		return map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":` + stock + `}]}}`,
		}
	}

	// Request 1: shadow MISS — plain fetch + write.
	first := Plan(t, query, inventoryShadowCaching(), responsesFor("5"))
	firstObserver := &cachetesting.RecordingObserver{}
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, firstObserver))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, firstBody)
	assert.Equal(t, int64(1), first.LoadCount("inventory", "me.favoriteProduct"))
	assert.Empty(t, firstObserver.Compares())

	ops := store.Ops()
	require.Len(t, ops, 2)
	key := ops[0].Key

	// Request 2: L2 HIT under shadow — the subgraph now says stock=7 and the
	// response MUST show 7 (fresh served, never the cached 5); the compare
	// records the mismatch and L2 is overwritten with the fresh value.
	second := Plan(t, query, inventoryShadowCaching(), responsesFor("7"))
	secondObserver := &cachetesting.RecordingObserver{}
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, secondObserver))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":7}}}}`, secondBody)
	assert.Equal(t, int64(1), second.LoadCount("inventory", "me.favoriteProduct"))
	assert.Equal(t, []cachetesting.ShadowCompare{
		{CacheKey: key, IsFresh: false},
	}, normalizeCompareAges(secondObserver.Compares()))

	entry, ok := store.Get(key)
	require.True(t, ok)
	assert.Equal(t, `{"__typename":"Product","stock":7}`, string(entry.Value))

	// Request 3: unchanged data — the compare records IsFresh true; the
	// response is still the network value.
	third := Plan(t, query, inventoryShadowCaching(), responsesFor("7"))
	thirdObserver := &cachetesting.RecordingObserver{}
	thirdBody := ResolveResponse(t, third.Response, cachetesting.NewRealishCache(store, thirdObserver))
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":7}}}}`, thirdBody)
	assert.Equal(t, int64(1), third.LoadCount("inventory", "me.favoriteProduct"))
	assert.Equal(t, []cachetesting.ShadowCompare{
		{CacheKey: key, IsFresh: true},
	}, normalizeCompareAges(thirdObserver.Compares()))
}

// normalizeCompareAges zeroes the real-clock CacheAge before the structural
// assert; the EXACT age is pinned by the synctest H2 unit row.
func normalizeCompareAges(compares []cachetesting.ShadowCompare) []cachetesting.ShadowCompare {
	for i := range compares {
		compares[i].CacheAge = 0
	}
	return compares
}
