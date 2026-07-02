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
func productsRootFieldCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"products": {
			RootFields: []cacheconfig.RootFieldCachePolicy{
				{TypeName: "Query", FieldName: "products", CacheName: "root-fields", TTL: time.Minute},
			},
		},
	}
}

// TestRootFieldL2EndToEnd: a cached root field is served from L2 on the second
// request with ZERO network, and an ALIAS-VARIANT operation over the same
// field and variables is served from the SAME entry (task 09 reuse); a
// different-arguments operation misses. Runs through the REAL ExecutionEngine;
// the old per-path load counts become per-rule request counts (one rule per
// distinct argument value).
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
	key := ops[0].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}`, TTL: time.Minute},
	}, ops)

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
