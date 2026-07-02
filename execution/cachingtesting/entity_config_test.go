package cachingtesting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

func entityCachingFor(subgraphs ...string) map[string]cacheconfig.CachingConfiguration {
	caching := make(map[string]cacheconfig.CachingConfiguration, len(subgraphs))
	for _, name := range subgraphs {
		caching[name] = cacheconfig.CachingConfiguration{
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "products", TTL: time.Minute},
			},
		}
	}
	return caching
}

// TestEntityCacheConfigSyncPlan pins the full per-fetch cache config over a
// REAL sync plan: the reviews batch entity fetch is configured, the products
// root fetch stays nil (root-field caching is task 13). The lone entity fetch
// has no L1 provider/consumer pair, so optimizeL1Cache (task 16) narrows L1
// off.
func TestEntityCacheConfigSyncPlan(t *testing.T) {
	result := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, entityCachingFor("reviews"), nil)
	assert.Equal(t, `QueryPlan {
  Sequence {
    Fetch(service: "0") {
      {
          products(first: $a){
              upc
              __typename
          }
      }
    }
    Fetch(service: "2") {
      {
        fragment Key on Product {
            __typename
            upc
        }
      } =>
      Cache: {l1:false l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
      {
          _entities(representations: $representations){
              ... on Product {
                  __typename
                  reviews {
                      body
                  }
              }
          }
      }
    }
  }
}
`, PrettyPlan(result))
}

// TestEntityCacheConfigDeferPlan pins that DEFER-GROUP entity fetches carry
// config too: the initial inventory fetch and the deferred inventory fetch are
// both configured from the same policy.
func TestEntityCacheConfigDeferPlan(t *testing.T) {
	query := `
		query {
			me { favoriteProduct { upc stock warehouse { id location } } }
			products(first: 1) {
				upc
				... @defer { stock }
			}
		}`
	result := Plan(t, query, entityCachingFor("inventory"), nil)
	require.NotNil(t, result.DeferResponse)

	assert.Equal(t, `QueryPlan {
  Sequence {
    Parallel {
      Fetch(service: "3") {
        {
            me {
                __typename
                id
            }
        }
      }
      Fetch(service: "0") {
        {
            products(first: $a){
                upc
                __typename
            }
        }
      }
    }
    Fetch(service: "0") {
      {
        fragment Key on User {
            __typename
            id
        }
      } =>
      {
          _entities(representations: $representations){
              ... on User {
                  __typename
                  favoriteProduct {
                      upc
                      __typename
                  }
              }
          }
      }
    }
    Flatten(path: "me.favoriteProduct") {
      Fetch(service: "1") {
        {
          fragment Key on Product {
              __typename
              upc
          }
        } =>
        Cache: {l1:true l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
        {
            _entities(representations: $representations){
                ... on Product {
                    __typename
                    stock
                    warehouse {
                        id
                        location
                    }
                }
            }
        }
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
    Cache: {l1:true l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
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
}

// TestEntityCacheConfigDeterminism plans the same operation twice and asserts
// identical rendered configs.
func TestEntityCacheConfigDeterminism(t *testing.T) {
	query := `{ products(first: 2) { upc reviews { body } } }`
	first := Plan(t, query, entityCachingFor("reviews", "inventory"), nil)
	second := Plan(t, query, entityCachingFor("reviews", "inventory"), nil)
	assert.Equal(t, PrettyPlan(first), PrettyPlan(second))
}
