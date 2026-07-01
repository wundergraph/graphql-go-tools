package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// reuseCaching enables the products ENTITY policy plus a by-key root-field
// policy for Query.product; rootCacheName decides whether the root field
// shares the entity key space.
func reuseCaching(rootCacheName string) map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"products": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute},
			},
			RootFields: []cacheconfig.RootFieldCachePolicy{
				{TypeName: "Query", FieldName: "product", CacheName: rootCacheName, TTL: time.Minute},
			},
		},
	}
}

// TestEntityCacheReuseEndToEnd: an entity entry primed by ANOTHER path (the
// reviews->products entity fetch) serves the by-key root field with zero
// network; a mismatched CacheName does NOT reuse (the constraint is enforced,
// not accidental).
func TestEntityCacheReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()

	// Prime the Product entity through featuredReview.product; the fresh
	// response carries sku, so BOTH entity keys are written (upc refresh via
	// the representation + sku backfill).
	prime := Plan(t, `{ featuredReview { product { name } } }`, reuseCaching("entities"), map[string]string{
		"reviews":                         `{"data":{"featuredReview":{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}}}`,
		"products:featuredReview.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table","sku":"S1"}]}}`,
	})
	primeBody := ResolveResponse(t, prime.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"featuredReview":{"product":{"name":"Table"}}}}`, primeBody)
	require.Len(t, store.Ops(), 3) // Get upc (miss), Set upc, Set sku

	// The by-key root field is served FROM THE ENTITY ENTRY: zero products loads.
	serveQuery := `query($upc: String!) { product(upc: $upc) { name } }`
	serve := Plan(t, serveQuery, reuseCaching("entities"), map[string]string{
		"products": `{"data":{"product":{"__typename":"Product","name":"Network"}}}`,
	})
	serveBody := resolveWithVariables(t, serve, `{"upc":"1"}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"product":{"name":"Table"}}}`, serveBody)
	assert.Equal(t, int64(0), serve.LoadCount("products", ""))

	// A MISMATCHED CacheName renders a different prefix: no reuse, the network
	// answers.
	mismatched := Plan(t, serveQuery, reuseCaching("root-fields"), map[string]string{
		"products": `{"data":{"product":{"__typename":"Product","name":"Network"}}}`,
	})
	mismatchedBody := resolveWithVariables(t, mismatched, `{"upc":"1"}`, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, `{"data":{"product":{"name":"Network"}}}`, mismatchedBody)
	assert.Equal(t, int64(1), mismatched.LoadCount("products", ""))
}
