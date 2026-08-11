package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// l1ChainQuery is the same-representation inventory chain: the INITIAL tree
// resolves product(upc:"1") through inventory selecting {stock, warehouse}, and
// the DEFERRED group resolves products(first:1)[0] — the very same product —
// through inventory selecting {stock} alone. Both fetches send the identical
// representation {__typename:"Product", upc:"1"}, so single-key L1 shares one
// entry between them, the deferred selection is covered by the initial value,
// and the defer-group ancestry orders provider before consumer.
const l1ChainQuery = `{ product(upc: "1") { stock warehouse { id location } } products(first: 1) { upc ... @defer { stock } } }`

// l1ChainDoubles builds the doubles for l1ChainQuery. deferredStock
// parameterizes the DEFERRED fetch's canned stock so tests can TAMPER it
// (accidental network use then fails loudly).
func l1ChainDoubles(deferredStock string) (products *Subgraph, initialFetch, deferredFetch *SubgraphRule, inventory *Subgraph) {
	products = Rules(
		Rule(`product(upc: $a)`, `{"data":{"product":{"__typename":"Product","upc":"1"},"products":[{"__typename":"Product","upc":"1"}]}}`),
	)
	// The initial fetch is the only one selecting warehouse; the deferred fetch
	// selects stock alone and falls through to the second rule.
	initialFetch = Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`)
	deferredFetch = Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":`+deferredStock+`}]}}`)
	return products, initialFetch, deferredFetch, Rules(initialFetch, deferredFetch)
}

// TestL1InRequestReuseEndToEnd: the initial inventory fetch populates L1 under
// the representation both fetches send; the DEFERRED fetch of the same product
// is served from L1 with ZERO network and without a second store lookup. Runs
// through the REAL ExecutionEngine.
func TestL1InRequestReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	products, initialFetch, deferredFetch, inventory := l1ChainDoubles("999")
	executionEngine := NewEngine(t, inventoryL1Caching(time.Minute), Subgraphs{"products": products, "inventory": inventory})

	frames := ExecuteDefer(t, executionEngine, l1ChainQuery, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, []string{
		`{"data":{"product":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		// stock 5, not the tampered 999: the deferred group read L1.
		`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, frames)
	assert.Equal(t, int64(1), initialFetch.Count.Load())
	assert.Equal(t, int64(0), deferredFetch.Count.Load())
	ops := store.Ops()
	require.Len(t, ops, 2)
	require.Len(t, ops[0].Keys, 1)
	key := ops[0].Keys[0]
	assert.Equal(t, []cachetesting.StoreOp{
		// The initial fetch's lookup (miss); the deferred fetch rode L1 and
		// never reached the store.
		{
			Kind: "GetMany",
			Keys: []string{key},
			Hits: []bool{false},
		},
		// Its request-end flush.
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					Key:   key,
					Value: `{"data":{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
	}, NormalizeStoreOpsClock(ops))
}

// TestL1ModeMatrixEndToEnd (J rows): the same operation under NO-OP and under
// caching produces byte-identical frames; the modes differ ONLY in network and
// store traffic. The deferred fetch's canned response matches the cached value
// here (the tampered variant is TestL1InRequestReuseEndToEnd's job), so byte
// equality across modes is meaningful. Two caching configurations → two
// engines, each over its own doubles.
func TestL1ModeMatrixEndToEnd(t *testing.T) {
	// NO-OP: no caching config, no controller — the baseline frames.
	noopProducts, _, noopDeferred, noopInventory := l1ChainDoubles("5")
	noopEngine := NewEngine(t, nil, Subgraphs{"products": noopProducts, "inventory": noopInventory})
	noopFrames := ExecuteDefer(t, noopEngine, l1ChainQuery, nil)
	assert.Equal(t, []string{
		`{"data":{"product":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, noopFrames)
	assert.Equal(t, int64(1), noopDeferred.Count.Load()) // the baseline really fetches it

	// L1+L2: identical frames; the initial fetch misses L2 once and flushes its
	// single write, and the deferred fetch rides L1 — neither network nor store.
	bothStore := cachetesting.NewFakeStore()
	bothProducts, bothInitial, bothDeferred, bothInventory := l1ChainDoubles("5")
	bothEngine := NewEngine(t, inventoryL1Caching(time.Minute), Subgraphs{"products": bothProducts, "inventory": bothInventory})
	controller := cachetesting.NewRealishCache(bothStore, nil)
	bothFrames := ExecuteDefer(t, bothEngine, l1ChainQuery, controller)
	assert.Equal(t, noopFrames, bothFrames)
	assert.Equal(t, int64(0), bothDeferred.Count.Load())
	ops := bothStore.Ops()
	require.Len(t, ops, 2)
	require.Len(t, ops[0].Keys, 1)
	key := ops[0].Keys[0]
	assert.Equal(t, []cachetesting.StoreOp{
		// The initial fetch's lookup (miss).
		{
			Kind: "GetMany",
			Keys: []string{key},
			Hits: []bool{false},
		},
		// Its request-end flush; ONE key per item, so one written item.
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					Key:   key,
					Value: `{"data":{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					Tags: []string{
						"subgraph:1",
						"type:1:Product",
						"entity:1:Product:d3cc039c7a9789e7", // upc "1"
					},
				},
			},
		},
	}, NormalizeStoreOpsClock(ops))

	// A SECOND request through the same engine hits L2 on the initial fetch and
	// L1 on the deferred one; its ops assert in isolation.
	bothStore.ResetOps()
	secondFrames := ExecuteDefer(t, bothEngine, l1ChainQuery, controller)
	assert.Equal(t, noopFrames, secondFrames)
	assert.Equal(t, int64(1), bothInitial.Count.Load()) // no NEW network fetch
	assert.Equal(t, int64(0), bothDeferred.Count.Load())
	// Exactly one op: the initial fetch's L2 hit. A served hit owes no write
	// (read key == write key), and the deferred fetch rides that request's L1.
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{key},
			Hits: []bool{true},
		},
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
	caching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"inventory": {DefaultTTL: ptr(time.Minute)},
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
