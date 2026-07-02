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
// Runs through the REAL ExecutionEngine over HTTP subgraph doubles.
func TestBatchEntityL2EndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	query := `{ products(first: 2) { upc reviews { body } } }`
	products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
	reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
	executionEngine := NewEngine(t, reviewsEntityCaching(), Subgraphs{"products": products, "reviews": reviews})
	expected := `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`

	firstBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, firstBody)
	assert.Equal(t, int64(1), reviews.Requests())

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

	// Request 2's ops assert in isolation: a full-batch hit — the reviews
	// upstream is NOT hit again, and exactly one Get per entity, no writes.
	store.ResetOps()
	secondBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, secondBody)
	assert.Equal(t, int64(1), reviews.Requests())
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key1},
		{Kind: "Get", Key: key2},
	}, store.Ops())
}

// TestBatchEntityMixedRun: only ONE of two entities is primed; the batch must
// fully refetch (no partial serving in this task), produce the correct
// response, and write entries for ALL entities afterwards.
func TestBatchEntityMixedRun(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)

	// The subgraph doubles route by REQUEST BODY: the one-product request
	// primes upc 1; the two-product request returns both entities. The reviews
	// double distinguishes the batches by their representations.
	products := Rules(
		Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
		Rule(`"variables":{"a":2}`, `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}`),
	)
	reviews := Rules(
		Rule(`{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"New"}]}]}}`),
		Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`),
	)
	executionEngine := NewEngine(t, reviewsEntityCaching(), Subgraphs{"products": products, "reviews": reviews})

	// Prime ONLY upc 1 through a single-product run.
	primeBody := Execute(t, executionEngine, `{ products(first: 1) { upc reviews { body } } }`, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]}]}}`, primeBody)
	require.Equal(t, int64(1), reviews.Requests())

	// The mixed batch (upc 1 primed, upc 3 not) refetches EVERYTHING.
	query := `{ products(first: 2) { upc reviews { body } } }`
	mixedBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"3","reviews":[{"body":"New"}]}]}}`, mixedBody)
	assert.Equal(t, int64(2), reviews.Requests())

	// Afterwards BOTH entities are cached: a repeat of the mixed batch hits
	// with no further reviews request.
	repeatBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"3","reviews":[{"body":"New"}]}]}}`, repeatBody)
	assert.Equal(t, int64(2), reviews.Requests())
}
