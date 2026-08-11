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
func reviewsPartialCaching() *cacheconfig.CachingConfiguration {
	return &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"reviews": {
				DefaultTTL:             ptr(time.Minute),
				EnablePartialCacheLoad: ptr(true),
			},
		},
	}
}

// TestPartialBatchEndToEnd: with one of two products' reviews already cached,
// the reviews subgraph receives EXACTLY the missing representation and the
// response is complete — cached and fetched reviews at their original
// positions. Runs through the REAL ExecutionEngine; the reduced batch is
// proven by pinning the reviews double's COMPLETE second request body.
func TestPartialBatchEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	products := Rules(
		Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
		Rule(`"variables":{"a":2}`, `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`),
	)
	// The second rule carries ONE entity — it can only answer a REDUCED
	// single-representation batch; the Bodies() pin below proves the reduced
	// request is what actually went over the wire.
	reviews := Rules(
		Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`),
		Rule(`"upc":"2"`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"sturdy chair"}]}]}}`),
	)
	executionEngine := NewEngine(t, reviewsPartialCaching(), Subgraphs{"products": products, "reviews": reviews})

	// Prime product 1's reviews.
	primeBody := Execute(t, executionEngine, `{ products(first: 1) { upc reviews { body } } }`, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]}]}}`, primeBody)

	// Two products now: product 1 covered, product 2 missing.
	secondBody := Execute(t, executionEngine, `{ products(first: 2) { upc reviews { body } } }`, controller)
	assert.Equal(t,
		`{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]},{"upc":"2","reviews":[{"body":"sturdy chair"}]}]}}`,
		secondBody)

	// The EXACT subgraph input: only the missing representation was sent.
	bodies := reviews.Bodies()
	require.Len(t, bodies, 2)
	assert.Equal(t, `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body}}}}","variables":{"representations":[{"__typename":"Product","upc":"2"}]}}`, bodies[1])
}

// TestPartialExpiryEndToEnd: mixed TTLs across subgraphs — when one policy's
// entry expires, only THAT subgraph is re-fetched; the still-fresh portion is
// served from cache.
func TestPartialExpiryEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	caching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"reviews":   {DefaultTTL: ptr(5 * time.Minute)},
			"inventory": {DefaultTTL: ptr(30 * time.Second)},
		},
	}
	query := `{ products(first: 1) { upc stock reviews { body } } }`
	products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`)
	executionEngine := NewEngine(t, caching, Subgraphs{"products": products, "inventory": inventory, "reviews": reviews})
	expected := `{"data":{"products":[{"upc":"1","stock":5,"reviews":[{"body":"great table"}]}]}}`

	firstBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, firstBody)
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, int64(1), reviews.Requests())

	// Age ONLY the short-TTL inventory entry past its TTL (store-double aging;
	// the exact TTL arithmetic is pinned by the synctest unit rows).
	var inventoryKey string
	for _, op := range store.Ops() {
		for _, item := range op.Items {
			if item.TTL == 30*time.Second {
				inventoryKey = item.Key
			}
		}
	}
	require.NotEmpty(t, inventoryKey)
	value, ok := store.Value(inventoryKey)
	require.True(t, ok)
	store.Seed(inventoryKey, value, -time.Second)

	secondBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, secondBody)
	// ONLY the expired portion hit the network: inventory refetched, reviews
	// still served from its fresh entry.
	assert.Equal(t, int64(2), inventory.Requests())
	assert.Equal(t, int64(1), reviews.Requests())
}
