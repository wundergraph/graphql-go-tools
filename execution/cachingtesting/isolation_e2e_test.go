package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// isolationCaching configures two sibling root fields on the products
// subgraph with DIFFERENT policies.
func isolationCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"products": {
			RootFields: []cacheconfig.RootFieldCachePolicy{
				{TypeName: "Query", FieldName: "products", CacheName: "products-cache", TTL: time.Minute},
				{TypeName: "Query", FieldName: "promotions", CacheName: "promotions-cache", TTL: 5 * time.Minute},
			},
		},
	}
}

// TestRootFieldIsolationPlans covers the plan-level isolation rows.
func TestRootFieldIsolationPlans(t *testing.T) {
	t.Run("two cached siblings with different policies become two parallel fetches", func(t *testing.T) {
		result := Plan(t, `{ products(first: 1) { upc } promotions { upc } }`, isolationCaching(), nil)
		assert.Equal(t, `QueryPlan {
  Parallel {
    Fetch(service: "0") {
      Cache: {l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
      {
          products(first: $a){
              upc
          }
      }
    }
    Fetch(service: "0") {
      Cache: {l1:false l2:true cacheName:promotions-cache ttl:5m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:promotions candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
      {
          promotions {
              upc
          }
      }
    }
  }
}
`, PrettyPlan(result))
	})

	t.Run("cached + uncached sibling: cached isolated, uncached separate without config", func(t *testing.T) {
		caching := map[string]cacheconfig.CachingConfiguration{
			"products": {
				RootFields: []cacheconfig.RootFieldCachePolicy{
					{TypeName: "Query", FieldName: "products", CacheName: "products-cache", TTL: time.Minute},
				},
			},
		}
		result := Plan(t, `{ products(first: 1) { upc } promotions { upc } }`, caching, nil)
		assert.Equal(t, `QueryPlan {
  Parallel {
    Fetch(service: "0") {
      Cache: {l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
      {
          products(first: $a){
              upc
          }
      }
    }
    Fetch(service: "0") {
      {
          promotions {
              upc
          }
      }
    }
  }
}
`, PrettyPlan(result))
	})

	t.Run("caching off: one merged fetch, byte-identical to the pre-isolation plan", func(t *testing.T) {
		result := Plan(t, `{ products(first: 1) { upc } promotions { upc } }`, nil, nil)
		assert.Equal(t, `QueryPlan {
  Fetch(service: "0") {
    {
        products(first: $a){
            upc
        }
        promotions {
            upc
        }
    }
  }
}
`, PrettyPlan(result))
	})

	t.Run("entity-root-node trap: a nested entity under an isolated root still merges into its subtree", func(t *testing.T) {
		result := Plan(t, `{ products(first: 1) { upc stock } }`, isolationCaching(), map[string]string{
			"products":  `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
			"inventory": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		// TWO fetches only: the isolated products root (its own subtree intact)
		// and the inventory entity fetch sequenced after it — the products
		// subtree was NOT torn apart into more fetches.
		assert.Equal(t, `QueryPlan {
  Sequence {
    Fetch(service: "0") {
      Cache: {l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
      {
          products(first: $a){
              upc
              __typename
          }
      }
    }
    Fetch(service: "1") {
      {
        fragment Key on Product {
            __typename
            upc
        }
      } =>
      {
          _entities(representations: $representations){
              ... on Product {
                  __typename
                  stock
              }
          }
      }
    }
  }
}
`, PrettyPlan(result))

		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","stock":5}]}}`, body)
	})

	t.Run("defer composition: an isolated root's deferred sub-fetch lands in its defer group", func(t *testing.T) {
		result := Plan(t, `{ products(first: 1) { upc ... @defer { stock } } }`, isolationCaching(), nil)
		require.NotNil(t, result.DeferResponse)
		assert.Equal(t, `QueryPlan {
  Fetch(service: "0") {
    Cache: {l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
    {
        products(first: $a){
            upc
            __typename
        }
    }
  }
}
Deferred (id: 1) QueryPlan {
  Fetch(service: "1") {
    {
      fragment Key on Product {
          __typename
          upc
      }
    } =>
    {
        _entities(representations: $representations){
            ... on Product {
                __typename
                stock
            }
        }
    }
  }
}
`, PrettyPlan(result))
	})
}

// TestRootFieldIsolationIndependentServing: the two isolated siblings cache
// and expire INDEPENDENTLY — one entry expires (forced), the other still hits.
func TestRootFieldIsolationIndependentServing(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ products(first: 1) { upc } promotions { upc } }`
	responses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}],"promotions":[{"__typename":"Product","upc":"9"}]}}`,
	}
	expected := `{"data":{"products":[{"upc":"1"}],"promotions":[{"upc":"9"}]}}`

	first := Plan(t, query, isolationCaching(), responses)
	firstBody := ResolveResponse(t, first.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, firstBody)
	// TWO isolated fetches, one per root field, both to the products DS.
	assert.Equal(t, int64(2), first.LoadCount("products", ""))

	ops := store.Ops()
	require.Len(t, ops, 4)
	productsKey, promotionsKey := ops[0].Key, ops[1].Key
	assert.NotEqual(t, productsKey, promotionsKey)

	// Force-expire ONLY the products entry (the store double lets tests age an
	// entry without real sleeping; TTL expiry semantics are pinned by the
	// synctest unit rows).
	entry, ok := store.Get(productsKey)
	require.True(t, ok)
	store.Seed(productsKey, entry.Value, -time.Second)

	second := Plan(t, query, isolationCaching(), responses)
	secondBody := ResolveResponse(t, second.Response, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, expected, secondBody)
	// The products fetch expired and refetched; promotions still hit: exactly
	// ONE of the two isolated fetches touched the network.
	assert.Equal(t, int64(1), second.LoadCount("products", ""))
}
