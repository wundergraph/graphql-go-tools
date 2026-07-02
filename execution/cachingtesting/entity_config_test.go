package cachingtesting

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
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
// REAL sync execution: the reviews batch entity fetch is configured, the
// products root fetch stays uncached (root-field caching is task 13). The lone
// entity fetch has no L1 provider/consumer pair, so optimizeL1Cache (task 16)
// narrows L1 off. Runs through the REAL ExecutionEngine with the QueryPlan in
// the response extensions; the ENTIRE body (data + fetch tree with cache
// configs) is pinned.
func TestEntityCacheConfigSyncPlan(t *testing.T) {
	controller := cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil)
	products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
	reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
	subgraphs := Subgraphs{"products": products, "reviews": reviews}
	executionEngine := NewEngine(t, entityCachingFor("reviews"), subgraphs)
	body := ExecutePlanned(t, executionEngine, `{ products(first: 2) { upc reviews { body } } }`, controller)
	var buf bytes.Buffer
	require.NoError(t, json.Indent(&buf, []byte(subgraphs.NormalizeURLs(body)), "", "  "))
	assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1",
        "reviews": [
          {
            "body": "Solid"
          }
        ]
      },
      {
        "upc": "2",
        "reviews": [
          {
            "body": "Wobbly"
          }
        ]
      }
    ]
  },
  "extensions": {
    "queryPlan": {
      "version": "1",
      "kind": "Sequence",
      "children": [
        {
          "kind": "Single",
          "fetch": {
            "kind": "Single",
            "subgraphName": "0",
            "subgraphId": "0",
            "fetchId": 0
          }
        },
        {
          "kind": "Single",
          "fetch": {
            "kind": "BatchEntity",
            "path": "products",
            "subgraphName": "2",
            "subgraphId": "2",
            "fetchId": 1,
            "dependsOnFetchIds": [
              0
            ],
            "dependencies": [
              {
                "coordinate": {
                  "typeName": "Product",
                  "fieldName": "reviews"
                },
                "isUserRequested": true,
                "dependsOn": [
                  {
                    "fetchId": 0,
                    "subgraph": "0",
                    "coordinate": {
                      "typeName": "Product",
                      "fieldName": "upc"
                    },
                    "isKey": true,
                    "isRequires": false
                  }
                ]
              }
            ],
            "cache": "{l1:false l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}"
          }
        }
      ]
    }
  }
}`, buf.String())
}

// TestEntityCacheConfigDeferPlan pins that DEFER-GROUP entity fetches carry
// config too: the initial inventory fetch and the deferred inventory fetch are
// both configured from the same policy. This is a planner-level test that pins
// plan-internal state not visible in a client response: the queryPlan response
// extension only covers the synchronous fetch tree, so the deferred group's
// fetch (and its cache config) can only be asserted on the Plan() result.
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

// TestEntityCacheConfigDeterminism executes the same operation through TWO
// independent engines (one engine would serve the repeat from its execution
// plan cache) and asserts the two full response bodies — data plus the
// queryPlan extension with its rendered cache configs — are byte-identical.
func TestEntityCacheConfigDeterminism(t *testing.T) {
	query := `{ products(first: 2) { upc reviews { body } } }`

	run := func(t *testing.T) string {
		controller := cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil)
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`)
		subgraphs := Subgraphs{"products": products, "reviews": reviews}
		executionEngine := NewEngine(t, entityCachingFor("reviews", "inventory"), subgraphs)
		return subgraphs.NormalizeURLs(ExecutePlanned(t, executionEngine, query, controller))
	}

	first := run(t)
	second := run(t)
	assert.Equal(t, first, second)
}
