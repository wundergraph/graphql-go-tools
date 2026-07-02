package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// reviewsPartialCaching enables partial cache loading for the Product entity
// on the reviews subgraph.
func reviewsPartialCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"reviews": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "reviews", TTL: time.Minute, EnablePartialCacheLoad: true},
			},
		},
	}
}

// TestPartialBatchEndToEnd: with one of two products' reviews already cached,
// the reviews subgraph receives EXACTLY the missing representation and the
// response is complete — cached and fetched reviews at their original
// positions.
func TestPartialBatchEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()

	// Prime product 1's reviews.
	prime := Plan(t, `{ products(first: 1) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`,
	})
	primeBody := ResolveResponse(t, prime.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]}]}}`, primeBody)

	// Two products now: product 1 covered, product 2 missing. The canned
	// reviews response carries ONE entity — it only matches a REDUCED request;
	// a full two-representation batch would fail the resolve loudly.
	second := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"sturdy chair"}]}]}}`,
	})
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t,
		`{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]},{"upc":"2","reviews":[{"body":"sturdy chair"}]}]}}`,
		secondBody)

	// The EXACT subgraph input: only the missing representation was sent.
	inputs := second.Inputs("reviews", "products")
	require.Len(t, inputs, 1)
	assert.Equal(t, `{"method":"POST","url":"http://reviews.service","header":{},"body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body}}}}","variables":{"representations":[{"__typename":"Product","upc":"2"}]}}}`, inputs[0])
}

// TestPartialExpiryEndToEnd: mixed TTLs across subgraphs — when one policy's
// entry expires, only THAT subgraph is re-fetched; the still-fresh portion is
// served from cache.
func TestPartialExpiryEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	caching := map[string]cacheconfig.CachingConfiguration{
		"reviews": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "reviews", TTL: 5 * time.Minute},
			},
		},
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "inventory", TTL: 30 * time.Second},
			},
		},
	}
	query := `{ products(first: 1) { upc stock reviews { body } } }`
	responses := map[string]string{
		"products":  `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"inventory": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		"reviews":   `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`,
	}
	expected := `{"data":{"products":[{"upc":"1","stock":5,"reviews":[{"body":"great table"}]}]}}`

	first := Plan(t, query, caching, responses)
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, firstBody)

	// Age ONLY the short-TTL inventory entry past its TTL (store-double aging;
	// the exact TTL arithmetic is pinned by the synctest unit rows).
	ops := store.Ops()
	var inventoryKey string
	for _, op := range ops {
		if op.Kind == "Set" && op.TTL == 30*time.Second {
			inventoryKey = op.Key
		}
	}
	require.NotEmpty(t, inventoryKey)
	entry, ok := store.Get(inventoryKey)
	require.True(t, ok)
	store.Seed(inventoryKey, entry.Value, -time.Second)

	second := Plan(t, query, caching, responses)
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, secondBody)
	// ONLY the expired portion hit the network: inventory refetched, reviews
	// still served from its fresh entry.
	assert.Equal(t, int64(1), second.LoadCount("inventory", "products"))
	assert.Equal(t, int64(0), second.LoadCount("reviews", "products"))
}
