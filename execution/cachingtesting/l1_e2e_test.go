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

// l1ChainDoubles builds the doubles for the dependency-ordered deal ->
// product(sku) -> reviews(upc) -> product(upc) chain: the deals root resolves
// the product by SKU (products fetch A), the reviews hop needs UPC from A's
// response, and each review's product resolves through a SECOND products
// fetch B that transitively depends on A — exactly the shape optimizeL1Cache
// keeps L1 on for. fetchBName parameterizes fetch B's canned product name so
// tests can TAMPER it (accidental network use fails loudly).
func l1ChainDoubles(fetchBName string) (deals, reviews *Subgraph, fetchA, fetchB *SubgraphRule) {
	deals = Respond(`{"data":{"deal":{"__typename":"Deal","id":"d1","product":{"__typename":"Product","sku":"S1"}}}}`)
	reviews = Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`)
	fetchA = Rule(`"sku":"S1"`, `{"data":{"_entities":[{"__typename":"Product","name":"Table","upc":"1"}]}}`)
	fetchB = Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","name":"`+fetchBName+`"}]}}`)
	return deals, reviews, fetchA, fetchB
}

// TestL1InRequestReuseEndToEnd: fetch A (deal.product) populates L1 under both
// entity keys (upc backfilled from the response); the DEPENDENT fetch B
// (review.product, known only by upc) is served from L1 with ZERO network and
// — the policy being L1-only — ZERO store ops for the whole request. Runs
// through the REAL ExecutionEngine; the old per-path load counts become
// per-rule request counts on the products double.
func TestL1InRequestReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ deal(id: "d1") { product { name reviews { product { name } } } } }`
	deals, reviews, fetchA, fetchB := l1ChainDoubles("NETWORK-MUST-NOT-SERVE")
	executionEngine := NewEngine(t, productL1Caching(0), Subgraphs{"deals": deals, "reviews": reviews, "products": Rules(fetchA, fetchB)})

	body := Execute(t, executionEngine, query, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"deal":{"product":{"name":"Table","reviews":[{"product":{"name":"Table"}}]}}}}`, body)
	assert.Equal(t, int64(1), fetchA.Count.Load())
	assert.Equal(t, int64(0), fetchB.Count.Load())
	assert.Empty(t, store.Ops())
}

// TestL1ModeMatrixEndToEnd (J rows): the same operation under NO-OP / L1-only /
// L1+L2 produces byte-identical data; the modes differ ONLY in network and
// store traffic. Fetch B's canned response matches the cached value here (the
// tampered variant is TestL1InRequestReuseEndToEnd's job), so byte equality
// across modes is meaningful. Three caching configurations → three engines,
// each over its own doubles.
func TestL1ModeMatrixEndToEnd(t *testing.T) {
	query := `{ deal(id: "d1") { product { name reviews { product { name } } } } }`

	// NO-OP: no caching config, no controller — the baseline bytes.
	noopDeals, noopReviews, noopA, noopB := l1ChainDoubles("Table")
	noopEngine := NewEngine(t, nil, Subgraphs{"deals": noopDeals, "reviews": noopReviews, "products": Rules(noopA, noopB)})
	noopBody := Execute(t, noopEngine, query, nil)
	assert.Equal(t, `{"data":{"deal":{"product":{"name":"Table","reviews":[{"product":{"name":"Table"}}]}}}}`, noopBody)
	assert.Equal(t, int64(1), noopB.Count.Load()) // the baseline really fetches B

	// L1-only: identical bytes, fetch B off the network, zero store traffic.
	l1Store := cachetesting.NewFakeStore()
	l1Deals, l1Reviews, l1A, l1B := l1ChainDoubles("Table")
	l1Engine := NewEngine(t, productL1Caching(0), Subgraphs{"deals": l1Deals, "reviews": l1Reviews, "products": Rules(l1A, l1B)})
	l1Body := Execute(t, l1Engine, query, cachetesting.NewRealishCache(l1Store, nil))
	assert.Equal(t, noopBody, l1Body)
	assert.Equal(t, int64(0), l1B.Count.Load())
	assert.Empty(t, l1Store.Ops())

	// L1+L2: identical bytes; fetch A misses L2 once and flushes its writes;
	// fetch B rides L1 and touches NEITHER the network NOR the store.
	bothStore := cachetesting.NewFakeStore()
	bothDeals, bothReviews, bothA, bothB := l1ChainDoubles("Table")
	bothEngine := NewEngine(t, productL1Caching(time.Minute), Subgraphs{"deals": bothDeals, "reviews": bothReviews, "products": Rules(bothA, bothB)})
	controller := cachetesting.NewRealishCache(bothStore, nil)
	bothBody := Execute(t, bothEngine, query, controller)
	assert.Equal(t, noopBody, bothBody)
	assert.Equal(t, int64(0), bothB.Count.Load())
	ops := bothStore.Ops()
	require.Len(t, ops, 3) // Get (A's sku miss) + Set sku + Set upc backfill
	assert.Equal(t, "Get", ops[0].Kind)
	assert.Equal(t, "Set", ops[1].Kind)
	assert.Equal(t, "Set", ops[2].Kind)
	assert.Equal(t, `{"__typename":"Product","name":"Table","upc":"1"}`, ops[1].Value)

	// A SECOND request through the same engine hits L2 on fetch A and L1 on
	// fetch B; its ops assert in isolation.
	bothStore.ResetOps()
	secondBody := Execute(t, bothEngine, query, controller)
	assert.Equal(t, noopBody, secondBody)
	assert.Equal(t, int64(1), bothA.Count.Load()) // no NEW fetch A request
	assert.Equal(t, int64(0), bothB.Count.Load())
	// Exactly three ops: fetch A's sku-key Get (L2 hit), fetch B's upc-key Get
	// (L2 hit — request 2's L1 only holds A's RENDERED key), and fetch A's
	// pending upc candidate re-rendered from the served value (backfill Set at
	// the request-end flush).
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: ops[0].Key},
		{Kind: "Get", Key: ops[2].Key},
		{Kind: "Set", Key: ops[2].Key, Value: `{"__typename":"Product","name":"Table","upc":"1"}`, TTL: time.Minute},
	}, bothStore.Ops())
}

// TestL1LazyInitAndParallelWrites (M1 + M2): two parallel eligible entity
// fetches trigger exactly ONE BeginRequest, and their concurrent writes to the
// shared request cache produce an uncorrupted response (run under -race).
// One request through the engine, so the countingController's begins count is
// exactly this request's.
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
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Rules(
		Rule(`"representations"`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`),
		Rule(`"variables":{"a":2}`, `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`),
	)
	// The batch fetch is the only one whose representations carry upc 2, so it
	// must be matched first; the remaining upc-1 body is the single fetch.
	batchFetch := Rule(`"upc":"2"`, `{"data":{"_entities":[{"__typename":"Product","stock":5},{"__typename":"Product","stock":7}]}}`)
	singleFetch := Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	inventory := Rules(batchFetch, singleFetch)
	executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

	controller := &countingController{inner: cachetesting.NewRealishCache(store, nil)}
	body := Execute(t, executionEngine, query, controller)
	assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"stock":5}},"products":[{"stock":5},{"stock":7}]}}`, body)
	// M1: exactly one BeginRequest despite two parallel eligible fetches.
	assert.Equal(t, int64(1), controller.begins.Load())
	// M2: both fetches wrote (single write + batch writes) without corruption.
	assert.Equal(t, int64(1), singleFetch.Count.Load())
	assert.Equal(t, int64(1), batchFetch.Count.Load())
}
