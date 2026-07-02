package cachingtesting

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// resolveTraced resolves with ART enabled and the production TraceObserver
// composed into the controller, returning the response body.
func resolveTraced(t *testing.T, resp *resolve.GraphQLResponse, store *cachetesting.FakeStore) string {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	ctx.TracingOptions.Enable = true
	ctx.SetCacheController(cache.NewController(cachetesting.StoreAdapter{Store: store}, cache.NewTraceObserver()))
	return resolveWithContext(t, ctx, resp)
}

// fullTraces renders EVERY fetch's COMPLETE ART trace as path-labeled,
// indented JSON — the whole DataSourceLoadTrace, cache section included — with
// the real-clock fields normalized: durations zeroed, TTL and cache-age nanos
// set to -1 when positive (their exact values are pinned by the synctest
// observer unit rows).
func fullTraces(t *testing.T, node *resolve.FetchTreeNode) string {
	t.Helper()
	var b strings.Builder
	var walk func(node *resolve.FetchTreeNode)
	walk = func(node *resolve.FetchTreeNode) {
		if node == nil {
			return
		}
		if node.Item != nil && node.Item.Fetch != nil {
			if trace := loadTraceOf(node.Item.Fetch); trace != nil {
				normalized := *trace
				normalized.DurationSinceStartNano = 0
				normalized.DurationLoadNano = 0
				if normalized.DurationSinceStartPretty != "" {
					normalized.DurationSinceStartPretty = "0s"
				}
				if normalized.DurationLoadPretty != "" {
					normalized.DurationLoadPretty = "0s"
				}
				if trace.CacheTrace != nil {
					cacheTrace := *trace.CacheTrace
					cacheTrace.Items = append([]resolve.CacheItemTrace(nil), trace.CacheTrace.Items...)
					for i := range cacheTrace.Items {
						if cacheTrace.Items[i].RemainingTTLNano > 0 {
							cacheTrace.Items[i].RemainingTTLNano = -1
						}
					}
					cacheTrace.ShadowCompares = append([]resolve.CacheShadowCompareTrace(nil), trace.CacheTrace.ShadowCompares...)
					for i := range cacheTrace.ShadowCompares {
						if cacheTrace.ShadowCompares[i].CacheAgeNano > 0 {
							cacheTrace.ShadowCompares[i].CacheAgeNano = -1
						}
					}
					normalized.CacheTrace = &cacheTrace
				}
				raw, err := json.MarshalIndent(&normalized, "", "  ")
				require.NoError(t, err)
				fmt.Fprintf(&b, "=== path %q ===\n%s\n", node.Item.ResponsePath, raw)
			}
		}
		for _, child := range node.ChildNodes {
			walk(child)
		}
	}
	walk(node)
	return b.String()
}

func loadTraceOf(fetch resolve.Fetch) *resolve.DataSourceLoadTrace {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Trace
	case *resolve.EntityFetch:
		return f.Trace
	case *resolve.BatchEntityFetch:
		return f.Trace
	default:
		return nil
	}
}

// TestARTCacheTraceEndToEnd asserts the COMPLETE cache sections of the ART
// trace for the representative scenarios.
func TestARTCacheTraceEndToEnd(t *testing.T) {
	t.Run("L2 miss then hit", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		query := `{ me { favoriteProduct { upc stock } } }`
		caching := map[string]cacheconfig.CachingConfiguration{
			"inventory": {Entities: []cacheconfig.EntityCachePolicy{{TypeName: "Product", CacheName: "inventory", TTL: time.Minute}}},
		}
		responses := map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		}

		first := Plan(t, query, caching, responses)
		firstBody := resolveTraced(t, first.Response, store)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, firstBody)
		assert.Equal(t, `=== path "" ===
{
  "raw_input_data": {},
  "input": {
    "body": {
      "query": "{me {__typename id}}"
    },
    "header": {},
    "method": "POST",
    "url": "http://users.service"
  },
  "output": {
    "data": {
      "me": {
        "__typename": "User",
        "id": "u1"
      }
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me" ===
{
  "raw_input_data": {
    "__typename": "User",
    "id": "u1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename favoriteProduct {upc __typename}}}}",
      "variables": {
        "representations": [
          {
            "__typename": "User",
            "id": "u1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://products.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "User",
          "favoriteProduct": {
            "__typename": "Product",
            "upc": "1"
          }
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me.favoriteProduct" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "upc": "1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename stock}}}",
      "variables": {
        "representations": [
          {
            "__typename": "Product",
            "upc": "1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://inventory.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "Product",
          "stock": 5
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  },
  "cache": {
    "decision": "Fetch",
    "hit": false,
    "items": [
      {
        "keys": [
          "inventory:22ec9a28afcdd0bf"
        ],
        "hit": false,
        "write_reason": "refresh"
      }
    ]
  }
}
`, fullTraces(t, first.Response.Fetches))

		second := Plan(t, query, caching, responses)
		secondBody := resolveTraced(t, second.Response, store)
		assert.Equal(t, firstBody, secondBody)
		assert.Equal(t, `=== path "" ===
{
  "raw_input_data": {},
  "input": {
    "body": {
      "query": "{me {__typename id}}"
    },
    "header": {},
    "method": "POST",
    "url": "http://users.service"
  },
  "output": {
    "data": {
      "me": {
        "__typename": "User",
        "id": "u1"
      }
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me" ===
{
  "raw_input_data": {
    "__typename": "User",
    "id": "u1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename favoriteProduct {upc __typename}}}}",
      "variables": {
        "representations": [
          {
            "__typename": "User",
            "id": "u1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://products.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "User",
          "favoriteProduct": {
            "__typename": "Product",
            "upc": "1"
          }
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me.favoriteProduct" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "upc": "1"
  },
  "single_flight_used": false,
  "single_flight_shared_response": false,
  "load_skipped": true,
  "cache": {
    "decision": "SkipFullHit",
    "hit": true,
    "items": [
      {
        "keys": [
          "inventory:22ec9a28afcdd0bf"
        ],
        "served_from": "l2",
        "hit": true,
        "remaining_ttl_nanoseconds": -1
      }
    ]
  }
}
`, fullTraces(t, second.Response.Fetches))
	})

	t.Run("L1 hit within one request", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		// The dependency-ordered deal -> product(sku) -> reviews(upc) ->
		// product(upc) chain: fetch B is served from L1 within the request
		// (its canned response is TAMPERED so network use fails loudly).
		query := `{ deal(id: "d1") { product { name reviews { product { name } } } } }`
		responses := map[string]string{
			"deals":                 `{"data":{"deal":{"__typename":"Deal","id":"d1","product":{"__typename":"Product","sku":"S1"}}}}`,
			"products:deal.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table","upc":"1"}]}}`,
			"reviews":               `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`,
			"products:deal.product.reviews.@.product": `{"data":{"_entities":[{"__typename":"Product","name":"NETWORK-MUST-NOT-SERVE"}]}}`,
		}
		result := Plan(t, query, productL1Caching(0), responses)
		body := resolveTraced(t, result.Response, store)
		assert.Equal(t, `{"data":{"deal":{"product":{"name":"Table","reviews":[{"product":{"name":"Table"}}]}}}}`, body)
		// The key hashes are deterministic (xxhash64 of the canonical key
		// preimage), so the WHOLE cache sections pin as literals: fetch A is a
		// plain miss+refresh (its sku-derived key; the upc candidate pending),
		// fetch B is the in-request L1 hit under the upc-derived key.
		assert.Equal(t, `=== path "" ===
{
  "raw_input_data": {},
  "input": {
    "body": {
      "query": "query($a: ID!){deal(id: $a){product {__typename sku}}}",
      "variables": {
        "a": null
      }
    },
    "header": {},
    "method": "POST",
    "undefined": [
      "a"
    ],
    "url": "http://deals.service"
  },
  "output": {
    "data": {
      "deal": {
        "__typename": "Deal",
        "id": "d1",
        "product": {
          "__typename": "Product",
          "sku": "S1"
        }
      }
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "deal.product" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "sku": "S1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename name upc}}}",
      "variables": {
        "representations": [
          {
            "__typename": "Product",
            "sku": "S1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://products.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "Product",
          "name": "Table",
          "upc": "1"
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  },
  "cache": {
    "decision": "Fetch",
    "hit": false,
    "items": [
      {
        "keys": [
          "entities:f58e5d95ce10e65e"
        ],
        "hit": false,
        "write_reason": "refresh",
        "pending_candidates": 1
      }
    ]
  }
}
=== path "deal.product" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "sku": "S1",
    "name": "Table",
    "upc": "1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {product {__typename upc}}}}}",
      "variables": {
        "representations": [
          {
            "__typename": "Product",
            "upc": "1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://reviews.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "Product",
          "reviews": [
            {
              "__typename": "Review",
              "product": {
                "__typename": "Product",
                "upc": "1"
              }
            }
          ]
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "deal.product.reviews.@.product" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "upc": "1"
  },
  "single_flight_used": false,
  "single_flight_shared_response": false,
  "load_skipped": true,
  "cache": {
    "decision": "SkipFullHit",
    "hit": true,
    "items": [
      {
        "keys": [
          "entities:153cc511c85713fb"
        ],
        "served_from": "l1",
        "hit": true,
        "pending_candidates": 1
      }
    ]
  }
}
`, fullTraces(t, result.Response.Fetches))
	})

	t.Run("shadow compare", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		query := `{ me { favoriteProduct { upc stock } } }`
		responses := map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		}
		prime := Plan(t, query, inventoryShadowCaching(), responses)
		resolveTraced(t, prime.Response, store)
		second := Plan(t, query, inventoryShadowCaching(), responses)
		secondBody := resolveTraced(t, second.Response, store)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, secondBody)
		assert.Equal(t, `=== path "" ===
{
  "raw_input_data": {},
  "input": {
    "body": {
      "query": "{me {__typename id}}"
    },
    "header": {},
    "method": "POST",
    "url": "http://users.service"
  },
  "output": {
    "data": {
      "me": {
        "__typename": "User",
        "id": "u1"
      }
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me" ===
{
  "raw_input_data": {
    "__typename": "User",
    "id": "u1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename favoriteProduct {upc __typename}}}}",
      "variables": {
        "representations": [
          {
            "__typename": "User",
            "id": "u1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://products.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "User",
          "favoriteProduct": {
            "__typename": "Product",
            "upc": "1"
          }
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "me.favoriteProduct" ===
{
  "raw_input_data": {
    "__typename": "Product",
    "upc": "1"
  },
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename stock}}}",
      "variables": {
        "representations": [
          {
            "__typename": "Product",
            "upc": "1"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://inventory.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "Product",
          "stock": 5
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  },
  "cache": {
    "decision": "FetchShadow",
    "hit": false,
    "shadow": true,
    "items": [
      {
        "keys": [
          "entities:153cc511c85713fb"
        ],
        "hit": false,
        "write_reason": "refresh"
      }
    ],
    "shadow_compares": [
      {
        "key": "entities:153cc511c85713fb",
        "is_fresh": true,
        "cache_age_nanoseconds": -1
      }
    ]
  }
}
`, fullTraces(t, second.Response.Fetches))
	})

	t.Run("partial batch", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		prime := Plan(t, `{ products(first: 1) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`,
		})
		resolveTraced(t, prime.Response, store)

		second := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"sturdy chair"}]}]}}`,
		})
		body := resolveTraced(t, second.Response, store)
		assert.Equal(t,
			`{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]},{"upc":"2","reviews":[{"body":"sturdy chair"}]}]}}`,
			body)
		// One partial fetch: bucket 1 (upc "1") served from L2 under the primed
		// key, bucket 2 (upc "2") fetched over the reduced batch and refreshed.
		assert.Equal(t, `=== path "" ===
{
  "raw_input_data": {},
  "input": {
    "body": {
      "query": "query($a: Int){products(first: $a){upc __typename}}",
      "variables": {
        "a": null
      }
    },
    "header": {},
    "method": "POST",
    "undefined": [
      "a"
    ],
    "url": "http://products.service"
  },
  "output": {
    "data": {
      "products": [
        {
          "__typename": "Product",
          "upc": "1"
        },
        {
          "__typename": "Product",
          "upc": "2"
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  }
}
=== path "products" ===
{
  "raw_input_data": [
    {
      "__typename": "Product",
      "upc": "1"
    },
    {
      "__typename": "Product",
      "upc": "2"
    }
  ],
  "input": {
    "body": {
      "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body}}}}",
      "variables": {
        "representations": [
          {
            "__typename": "Product",
            "upc": "2"
          }
        ]
      }
    },
    "header": {},
    "method": "POST",
    "url": "http://reviews.service"
  },
  "output": {
    "data": {
      "_entities": [
        {
          "__typename": "Product",
          "reviews": [
            {
              "__typename": "Review",
              "body": "sturdy chair"
            }
          ]
        }
      ]
    }
  },
  "duration_since_start_pretty": "0s",
  "duration_load_pretty": "0s",
  "single_flight_used": true,
  "single_flight_shared_response": false,
  "load_skipped": false,
  "load_stats": {
    "get_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host_port": ""
    },
    "got_conn": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "reused": false,
      "was_idle": false,
      "idle_time_nanoseconds": 0,
      "idle_time_pretty": ""
    },
    "got_first_response_byte": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "dns_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "host": ""
    },
    "dns_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "connect_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "connect_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": "",
      "network": "",
      "addr": ""
    },
    "tls_handshake_start": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "tls_handshake_done": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_headers": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    },
    "wrote_request": {
      "duration_since_start_nanoseconds": 0,
      "duration_since_start_pretty": ""
    }
  },
  "cache": {
    "decision": "FetchPartial",
    "hit": false,
    "items": [
      {
        "keys": [
          "reviews:aa052522cb64dff0"
        ],
        "served_from": "l2",
        "hit": true,
        "remaining_ttl_nanoseconds": -1
      },
      {
        "keys": [
          "reviews:258590a53d3c528e"
        ],
        "hit": false,
        "write_reason": "refresh"
      }
    ]
  }
}
`, fullTraces(t, second.Response.Fetches))
	})

	t.Run("regression: tracing off leaves fetch traces nil", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		result := Plan(t, `{ me { favoriteProduct { upc stock } } }`, map[string]cacheconfig.CachingConfiguration{
			"inventory": {Entities: []cacheconfig.EntityCachePolicy{{TypeName: "Product", CacheName: "inventory", TTL: time.Minute}}},
		}, map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		body := ResolveResponse(t, result.Response, cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)
		assert.Empty(t, fullTraces(t, result.Response.Fetches))
	})
}
