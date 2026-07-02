package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// productsEntityCaching enables entity caching for the PRODUCTS subgraph,
// whose Product declares two @key sets (upc, sku) — the multi-key entity.
func productsEntityCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"products": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute},
			},
		},
	}
}

// TestMultiKeyCrossKeyHitEndToEnd is the plan-driven cross-key row: request 1
// reaches Product through the upc-keyed reviews path and — because the fresh
// response carries sku — BACKFILLS the sku key; request 2 reaches the same
// entity through the sku-keyed deals path, renders ONLY the sku key, and is
// served from the cache with ZERO network to products.
func TestMultiKeyCrossKeyHitEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()

	// Request 1: featuredReview.product — reviews provides upc; the products
	// entity fetch renders the upc candidate, the sku candidate is pending and
	// backfills from the fresh response (which includes sku).
	prime := Plan(t, `{ featuredReview { product { name } } }`, productsEntityCaching(), map[string]string{
		"reviews":                         `{"data":{"featuredReview":{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}}}`,
		"products:featuredReview.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table","sku":"S1"}]}}`,
	})
	primeBody := ResolveResponse(t, prime.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"featuredReview":{"product":{"name":"Table"}}}}`, primeBody)
	assert.Equal(t, int64(1), prime.LoadCount("products", "featuredReview.product"))

	primeOps := store.Ops()
	require.Len(t, primeOps, 3)
	upcKey := primeOps[0].Key
	skuKey := primeOps[2].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: upcKey},
		{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","sku":"S1"}`, TTL: time.Minute},
		{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","sku":"S1"}`, TTL: time.Minute},
	}, primeOps)
	assert.NotEqual(t, upcKey, skuKey)

	// Request 2: deal.product — deals provides ONLY sku, so the entity fetch
	// renders only the sku candidate: the cross-key hit. Its ops assert in
	// isolation.
	store.ResetOps()
	serve := Plan(t, `{ deal(id: "d1") { product { name } } }`, productsEntityCaching(), map[string]string{
		"deals": `{"data":{"deal":{"__typename":"Deal","product":{"__typename":"Product","sku":"S1"}}}}`,
	})
	serveBody := ResolveResponse(t, serve.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"deal":{"product":{"name":"Table"}}}}`, serveBody)
	assert.Equal(t, int64(1), serve.LoadCount("deals", ""))
	assert.Equal(t, int64(0), serve.LoadCount("products", "deal.product"))

	// The serve request performed exactly one Get, under the BACKFILLED sku key.
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: skuKey},
	}, store.Ops())
}
