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
// not accidental). Two caching configurations → two engines over the SAME
// store; the root-field canned responses are TAMPERED ("Network") so serving
// them instead of the cached "Table" fails loudly.
func TestEntityCacheReuseEndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)

	reviews := Respond(`{"data":{"featuredReview":{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}}}`)
	entityFetch := Rule(`"representations"`, `{"data":{"_entities":[{"__typename":"Product","name":"Table","sku":"S1"}]}}`)
	rootFetch := Rule(`"product`, `{"data":{"product":{"__typename":"Product","name":"Network"}}}`)
	products := Rules(entityFetch, rootFetch)
	matchedEngine := NewEngine(t, reuseCaching("entities"), Subgraphs{"reviews": reviews, "products": products})

	// Prime the Product entity through featuredReview.product; the fresh
	// response carries sku, so BOTH entity keys are written (upc refresh via
	// the representation + sku backfill).
	primeBody := Execute(t, matchedEngine, `{ featuredReview { product { name } } }`, controller)
	assert.Equal(t, `{"data":{"featuredReview":{"product":{"name":"Table"}}}}`, primeBody)
	require.Len(t, store.Ops(), 3) // Get upc (miss), Set upc, Set sku

	// The by-key root field is served FROM THE ENTITY ENTRY: zero root-field
	// requests to products.
	serveQuery := `query($upc: String!) { product(upc: $upc) { name } }`
	serveBody := ExecuteWithVariables(t, matchedEngine, serveQuery, `{"upc":"1"}`, controller)
	assert.Equal(t, `{"data":{"product":{"name":"Table"}}}`, serveBody)
	assert.Equal(t, int64(0), rootFetch.Count.Load())

	// A MISMATCHED CacheName renders a different prefix: no reuse, the network
	// answers. This is a different caching configuration, so a second engine.
	mismatchedProducts := Respond(`{"data":{"product":{"__typename":"Product","name":"Network"}}}`)
	mismatchedEngine := NewEngine(t, reuseCaching("root-fields"), Subgraphs{"products": mismatchedProducts})
	mismatchedBody := ExecuteWithVariables(t, mismatchedEngine, serveQuery, `{"upc":"1"}`, controller)
	assert.Equal(t, `{"data":{"product":{"name":"Network"}}}`, mismatchedBody)
	assert.Equal(t, int64(1), mismatchedProducts.Requests())
}
