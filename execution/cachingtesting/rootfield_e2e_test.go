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
// different-arguments operation misses.
func TestRootFieldL2EndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `query($first: Int!) { products(first: $first) { upc name } }`
	responses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`,
	}

	first := Plan(t, query, productsRootFieldCaching(), responses)
	firstBody := resolveWithVariables(t, first, `{"first":1}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`, firstBody)
	assert.Equal(t, int64(1), first.LoadCount("products", ""))

	ops := store.Ops()
	require.Len(t, ops, 2)
	key := ops[0].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}`, TTL: time.Minute},
	}, ops)

	// Same operation again: L2 hit, zero network.
	second := Plan(t, query, productsRootFieldCaching(), responses)
	secondBody := resolveWithVariables(t, second, `{"first":1}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, firstBody, secondBody)
	assert.Equal(t, int64(0), second.LoadCount("products", ""))

	// ALIAS VARIANT over the same field + variables: served from the SAME entry.
	aliasQuery := `query($first: Int!) { items: products(first: $first) { code: upc title: name } }`
	alias := Plan(t, aliasQuery, productsRootFieldCaching(), responses)
	aliasBody := resolveWithVariables(t, alias, `{"first":1}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"items":[{"code":"1","title":"Table"}]}}`, aliasBody)
	assert.Equal(t, int64(0), alias.LoadCount("products", ""))

	// Different ARGUMENTS: a different key — miss, network runs.
	differentArgs := Plan(t, query, productsRootFieldCaching(), responses)
	differentBody := resolveWithVariables(t, differentArgs, `{"first":5}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"products":[{"upc":"1","name":"Table"}]}}`, differentBody)
	assert.Equal(t, int64(1), differentArgs.LoadCount("products", ""))
}
