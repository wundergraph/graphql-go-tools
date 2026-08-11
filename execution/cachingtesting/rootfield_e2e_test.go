package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// productsRootFieldCaching enables root-field caching for Query.products on
// the products subgraph.
func productsRootFieldCaching() *cacheconfig.CachingConfiguration {
	return &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"products": {
				RootFields: []cacheconfig.RootFieldCacheConfig{
					{TypeName: "Query", FieldName: "products", TTL: time.Minute},
				},
			},
		},
	}
}

// TestRootFieldL2EndToEnd: a cached root field is served from L2 on the second
// request with ZERO network, and an ALIAS-VARIANT operation over the same
// field and variables is served from the SAME entry; a different-arguments
// operation misses. Runs through the REAL ExecutionEngine, with one subgraph
// rule per distinct argument value.
func TestRootFieldL2EndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	query := `query($first: Int!) { products(first: $first) { upc name } }`
	// The engine renames $first to $a in the rendered body: one rule per
	// distinct argument value.
	first1 := Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`)
	first5 := Rule(`"variables":{"a":5}`, `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`)
	products := Rules(first1, first5)
	executionEngine := NewEngine(t, productsRootFieldCaching(), Subgraphs{"products": products})

	firstBody := ExecuteWithVariables(t, executionEngine, query, `{"first":1}`, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`, firstBody)
	assert.Equal(t, int64(1), first1.Count.Load())

	ops := store.Ops()
	require.Len(t, ops, 2)
	require.Len(t, ops[0].Keys, 1)
	key := ops[0].Keys[0]
	assert.Equal(t, []cachetesting.StoreOp{
		{
			Kind: "GetMany",
			Keys: []string{key},
			Hits: []bool{false},
		},
		{
			Kind: "SetMany",
			Items: []cachetesting.StoreOpItem{
				{
					Key:   key,
					Value: `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]},"cc":{"ttl":60,"created":-1,"scope":"public"}}`,
					TTL:   time.Minute,
					// A whole-response root-field entry is not one entity, so it
					// carries the two coarse tags only.
					Tags: []string{
						"subgraph:0",
						"type:0:Query",
					},
				},
			},
		},
	}, NormalizeStoreOpsClock(ops))

	// Same operation again: L2 hit, zero network.
	secondBody := ExecuteWithVariables(t, executionEngine, query, `{"first":1}`, controller)
	assert.Equal(t, firstBody, secondBody)
	assert.Equal(t, int64(1), first1.Count.Load())

	// ALIAS VARIANT over the same field + variables: served from the SAME entry.
	aliasQuery := `query($first: Int!) { items: products(first: $first) { code: upc title: name } }`
	aliasBody := ExecuteWithVariables(t, executionEngine, aliasQuery, `{"first":1}`, controller)
	assert.Equal(t, `{"data":{"items":[{"code":"1","title":"Table"}]}}`, aliasBody)
	assert.Equal(t, int64(1), first1.Count.Load())
	assert.Equal(t, int64(1), products.Requests())

	// Different ARGUMENTS: a different key — miss, network runs.
	differentBody := ExecuteWithVariables(t, executionEngine, query, `{"first":5}`, controller)
	assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`, differentBody)
	assert.Equal(t, int64(1), first5.Count.Load())
}
