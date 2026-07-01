package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// productL1Caching enables the products Product entity policy; ttl 0 keeps the
// config L1-only (L2 = TTL > 0), ttl > 0 enables both layers.
func productL1Caching(ttl time.Duration) map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"products": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: ttl},
			},
		},
	}
}

// l1ChainQuery produces a dependency-ordered same-type pair: the deals root
// resolves the product by SKU (products fetch A), the reviews hop needs UPC
// from A's response, and each review's product resolves through a SECOND
// products fetch B that transitively depends on A — exactly the shape
// optimizeL1Cache keeps L1 on for.
const l1ChainQuery = `{ deal(id: "d1") { product { name reviews { product { name } } } } }`

func l1ChainResponses() map[string]string {
	return map[string]string{
		"deals":                 `{"data":{"deal":{"__typename":"Deal","id":"d1","product":{"__typename":"Product","sku":"S1"}}}}`,
		"products:deal.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table","upc":"1"}]}}`,
		"reviews":               `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`,
		"products:deal.product.reviews.@.product": `{"data":{"_entities":[{"__typename":"Product","name":"NETWORK-MUST-NOT-SERVE"}]}}`,
	}
}

const l1ChainExpected = `{"data":{"deal":{"product":{"name":"Table","reviews":[{"product":{"name":"Table"}}]}}}}`

// TestL1InRequestReuseEndToEnd: fetch A (deal.product) populates L1 under both
// entity keys (upc backfilled from the response); the DEPENDENT fetch B
// (review.product, known only by upc) is served from L1 with ZERO network and
// — the policy being L1-only — ZERO store ops for the whole request.
func TestL1InRequestReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	result := Plan(t, l1ChainQuery, productL1Caching(0), l1ChainResponses())
	body := ResolveResponse(t, result.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, l1ChainExpected, body)
	assert.Equal(t, int64(1), result.LoadCount("products", "deal.product"))
	assert.Equal(t, int64(0), result.LoadCount("products", "deal.product.reviews.@.product"))
	assert.Empty(t, store.Ops())
}

// TestL1ModeMatrixEndToEnd (J rows): the same operation under NO-OP / L1-only /
// L1+L2 produces byte-identical data; the modes differ ONLY in network and
// store traffic. Fetch B's canned response matches the cached value here (the
// tampered variant is TestL1InRequestReuseEndToEnd's job), so byte equality
// across modes is meaningful.
func TestL1ModeMatrixEndToEnd(t *testing.T) {
	matrixResponses := func() map[string]string {
		responses := l1ChainResponses()
		responses["products:deal.product.reviews.@.product"] = `{"data":{"_entities":[{"__typename":"Product","name":"Table"}]}}`
		return responses
	}
	// NO-OP: no caching config, no controller — the baseline bytes.
	noop := Plan(t, l1ChainQuery, nil, matrixResponses())
	noopBody := ResolveResponse(t, noop.Response, nil)
	assert.Equal(t, l1ChainExpected, noopBody)
	assert.Equal(t, int64(1), noop.LoadCount("products", "deal.product.reviews.@.product")) // the baseline really fetches B

	// L1-only: identical bytes, fetch B off the network, zero store traffic.
	l1Store := cachetesting.NewFakeStore()
	l1Only := Plan(t, l1ChainQuery, productL1Caching(0), matrixResponses())
	l1Body := ResolveResponse(t, l1Only.Response, cachetesting.NewRealishCache(l1Store, nil))
	assert.Equal(t, noopBody, l1Body)
	assert.Equal(t, int64(0), l1Only.LoadCount("products", "deal.product.reviews.@.product"))
	assert.Empty(t, l1Store.Ops())

	// L1+L2: identical bytes; fetch A misses L2 once and flushes its writes;
	// fetch B rides L1 and touches NEITHER the network NOR the store.
	bothStore := cachetesting.NewFakeStore()
	both := Plan(t, l1ChainQuery, productL1Caching(time.Minute), matrixResponses())
	bothBody := ResolveResponse(t, both.Response, cachetesting.NewRealishCache(bothStore, nil))
	assert.Equal(t, noopBody, bothBody)
	assert.Equal(t, int64(0), both.LoadCount("products", "deal.product.reviews.@.product"))
	ops := bothStore.Ops()
	require.Len(t, ops, 3) // Get (A's sku miss) + Set sku + Set upc backfill
	assert.Equal(t, "Get", ops[0].Kind)
	assert.Equal(t, "Set", ops[1].Kind)
	assert.Equal(t, "Set", ops[2].Kind)
	assert.Equal(t, `{"__typename":"Product","name":"Table","upc":"1"}`, ops[1].Value)

	// A SECOND request over a fresh plan hits L2 on fetch A and L1 on fetch B.
	second := Plan(t, l1ChainQuery, productL1Caching(time.Minute), matrixResponses())
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(bothStore, nil))
	assert.Equal(t, noopBody, secondBody)
	assert.Equal(t, int64(0), second.LoadCount("products", "deal.product"))
	assert.Equal(t, int64(0), second.LoadCount("products", "deal.product.reviews.@.product"))
}

// TestL1LazyInitAndParallelWrites (M1 + M2): two parallel eligible entity
// fetches trigger exactly ONE BeginRequest, and their concurrent writes to the
// shared request cache produce an uncorrupted response (run under -race).
func TestL1LazyInitAndParallelWrites(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ me { favoriteProduct { stock } } products(first: 2) { stock } }`
	caching := map[string]cacheconfig.CachingConfiguration{
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "inventory", TTL: time.Minute},
			},
		},
	}
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"products":                     `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		"inventory:products":           `{"data":{"_entities":[{"__typename":"Product","stock":5},{"__typename":"Product","stock":7}]}}`,
	}
	result := Plan(t, query, caching, responses)
	controller := &countingController{inner: cachetesting.NewRealishCache(store, nil)}
	body := ResolveResponse(t, result.Response, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"stock":5}},"products":[{"stock":5},{"stock":7}]}}`, body)
	// M1: exactly one BeginRequest despite two parallel eligible fetches.
	assert.Equal(t, int64(1), controller.begins.Load())
	// M2: both fetches wrote (single write + batch writes) without corruption.
	assert.Equal(t, int64(1), result.LoadCount("inventory", "me.favoriteProduct"))
	assert.Equal(t, int64(1), result.LoadCount("inventory", "products"))
}
