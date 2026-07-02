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

// TestRootFieldIsolationPlans covers the isolation rows through the REAL
// ExecutionEngine: each scenario executes like a client with the QueryPlan in
// the response extensions, pinning the ENTIRE body — data plus the sync fetch
// tree with its per-fetch cache configs.
func TestRootFieldIsolationPlans(t *testing.T) {
	t.Run("two cached siblings with different policies become two parallel fetches", func(t *testing.T) {
		controller := cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil)
		products := Rules(
			Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
			Rule(`promotions`, `{"data":{"promotions":[{"__typename":"Product","upc":"9"}]}}`),
		)
		subgraphs := Subgraphs{"products": products}
		executionEngine := NewEngine(t, isolationCaching(), subgraphs)
		body := ExecutePlanned(t, executionEngine, `{ products(first: 1) { upc } promotions { upc } }`, controller)
		var buf bytes.Buffer
		require.NoError(t, json.Indent(&buf, []byte(subgraphs.NormalizeURLs(body)), "", "  "))
		assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1"
      }
    ],
    "promotions": [
      {
        "upc": "9"
      }
    ]
  },
  "extensions": {
    "queryPlan": {
      "version": "1",
      "kind": "Sequence",
      "children": [
        {
          "kind": "Parallel",
          "children": [
            {
              "kind": "Single",
              "fetch": {
                "kind": "Single",
                "subgraphName": "0",
                "subgraphId": "0",
                "fetchId": 0,
                "cache": "{l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}"
              }
            },
            {
              "kind": "Single",
              "fetch": {
                "kind": "Single",
                "subgraphName": "0",
                "subgraphId": "0",
                "fetchId": 1,
                "cache": "{l1:false l2:true cacheName:promotions-cache ttl:5m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:promotions candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}"
              }
            }
          ]
        }
      ]
    }
  }
}`, buf.String())
	})

	t.Run("cached + uncached sibling: cached isolated, uncached separate without config", func(t *testing.T) {
		controller := cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil)
		caching := map[string]cacheconfig.CachingConfiguration{
			"products": {
				RootFields: []cacheconfig.RootFieldCachePolicy{
					{TypeName: "Query", FieldName: "products", CacheName: "products-cache", TTL: time.Minute},
				},
			},
		}
		products := Rules(
			Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
			Rule(`promotions`, `{"data":{"promotions":[{"__typename":"Product","upc":"9"}]}}`),
		)
		subgraphs := Subgraphs{"products": products}
		executionEngine := NewEngine(t, caching, subgraphs)
		body := ExecutePlanned(t, executionEngine, `{ products(first: 1) { upc } promotions { upc } }`, controller)
		var buf bytes.Buffer
		require.NoError(t, json.Indent(&buf, []byte(subgraphs.NormalizeURLs(body)), "", "  "))
		assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1"
      }
    ],
    "promotions": [
      {
        "upc": "9"
      }
    ]
  },
  "extensions": {
    "queryPlan": {
      "version": "1",
      "kind": "Sequence",
      "children": [
        {
          "kind": "Parallel",
          "children": [
            {
              "kind": "Single",
              "fetch": {
                "kind": "Single",
                "subgraphName": "0",
                "subgraphId": "0",
                "fetchId": 0,
                "cache": "{l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}"
              }
            },
            {
              "kind": "Single",
              "fetch": {
                "kind": "Single",
                "subgraphName": "0",
                "subgraphId": "0",
                "fetchId": 1
              }
            }
          ]
        }
      ]
    }
  }
}`, buf.String())
	})

	t.Run("caching off: one merged fetch, no cache config on it", func(t *testing.T) {
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}],"promotions":[{"__typename":"Product","upc":"9"}]}}`)
		subgraphs := Subgraphs{"products": products}
		executionEngine := NewEngine(t, nil, subgraphs)
		body := ExecutePlanned(t, executionEngine, `{ products(first: 1) { upc } promotions { upc } }`, nil)
		var buf bytes.Buffer
		require.NoError(t, json.Indent(&buf, []byte(subgraphs.NormalizeURLs(body)), "", "  "))
		assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1"
      }
    ],
    "promotions": [
      {
        "upc": "9"
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
        }
      ]
    }
  }
}`, buf.String())
	})

	t.Run("entity-root-node trap: a nested entity under an isolated root still merges into its subtree", func(t *testing.T) {
		// TWO fetches only: the isolated products root (its own subtree intact)
		// and the inventory entity fetch sequenced after it — the products
		// subtree was NOT torn apart into more fetches.
		controller := cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil)
		products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		subgraphs := Subgraphs{"products": products, "inventory": inventory}
		executionEngine := NewEngine(t, isolationCaching(), subgraphs)
		body := ExecutePlanned(t, executionEngine, `{ products(first: 1) { upc stock } }`, controller)
		var buf bytes.Buffer
		require.NoError(t, json.Indent(&buf, []byte(subgraphs.NormalizeURLs(body)), "", "  "))
		assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1",
        "stock": 5
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
            "fetchId": 0,
            "cache": "{l1:false l2:true cacheName:products-cache ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type:Query field:products candidates:0 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}"
          }
        },
        {
          "kind": "Single",
          "fetch": {
            "kind": "BatchEntity",
            "path": "products",
            "subgraphName": "1",
            "subgraphId": "1",
            "fetchId": 1,
            "dependsOnFetchIds": [
              0
            ],
            "dependencies": [
              {
                "coordinate": {
                  "typeName": "Product",
                  "fieldName": "stock"
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
            ]
          }
        }
      ]
    }
  }
}`, buf.String())
	})

	// This subtest is a planner-level test that pins plan-internal state not
	// visible in a client response: the queryPlan response extension only
	// covers the synchronous fetch tree, so the deferred group's plan (and its
	// fetch placement) can only be asserted on the Plan() result itself.
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
// Runs through the REAL ExecutionEngine: the products double routes each
// isolated fetch by its request body, so the old per-DS load counts become
// per-rule request counts.
func TestRootFieldIsolationIndependentServing(t *testing.T) {
	store := cachetesting.NewFakeStore()
	controller := cachetesting.NewRealishCache(store, nil)
	query := `{ products(first: 1) { upc } promotions { upc } }`
	productsRule := Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
	promotionsRule := Rule(`promotions`, `{"data":{"promotions":[{"__typename":"Product","upc":"9"}]}}`)
	products := Rules(productsRule, promotionsRule)
	executionEngine := NewEngine(t, isolationCaching(), Subgraphs{"products": products})
	expected := `{"data":{"products":[{"upc":"1"}],"promotions":[{"upc":"9"}]}}`

	firstBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, firstBody)
	// TWO isolated fetches, one per root field, both to the products double.
	assert.Equal(t, int64(1), productsRule.Count.Load())
	assert.Equal(t, int64(1), promotionsRule.Count.Load())

	ops := store.Ops()
	require.Len(t, ops, 4)
	// The two fetches run in parallel: identify the products entry by its
	// EXACT stored value instead of relying on op order.
	var productsKey, promotionsKey string
	for _, op := range ops {
		if op.Kind != "Set" {
			continue
		}
		switch op.Value {
		case `{"products":[{"__typename":"Product","upc":"1"}]}`:
			productsKey = op.Key
		case `{"promotions":[{"__typename":"Product","upc":"9"}]}`:
			promotionsKey = op.Key
		}
	}
	require.NotEmpty(t, productsKey)
	require.NotEmpty(t, promotionsKey)
	assert.NotEqual(t, productsKey, promotionsKey)

	// Force-expire ONLY the products entry (the store double lets tests age an
	// entry without real sleeping; TTL expiry semantics are pinned by the
	// synctest unit rows).
	entry, ok := store.Get(productsKey)
	require.True(t, ok)
	store.Seed(productsKey, entry.Value, -time.Second)

	secondBody := Execute(t, executionEngine, query, controller)
	assert.Equal(t, expected, secondBody)
	// The products fetch expired and refetched; promotions still hit: exactly
	// ONE of the two isolated fetches touched the network again.
	assert.Equal(t, int64(2), productsRule.Count.Load())
	assert.Equal(t, int64(1), promotionsRule.Count.Load())
}
