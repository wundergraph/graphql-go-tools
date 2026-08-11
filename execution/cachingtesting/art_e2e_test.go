package cachingtesting

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// normalizedTraceBody makes a traced response body pinnable: subgraph double
// URLs become their stable fixture form, real-clock cache-trace nanos become
// -1, and the whole body is indented for readable assertions.
func normalizedTraceBody(t *testing.T, subgraphs Subgraphs, body string) string {
	t.Helper()
	normalized := NormalizeCacheTraceClock(subgraphs.NormalizeURLs(body))
	var buf bytes.Buffer
	require.NoError(t, json.Indent(&buf, []byte(normalized), "", "  "))
	return buf.String()
}

// TestARTCacheTraceEndToEnd asserts the COMPLETE response bodies — data plus
// the full ART trace in extensions, cache sections included — for the
// representative scenarios. Runs through the REAL ExecutionEngine over HTTP
// subgraph doubles.
func TestARTCacheTraceEndToEnd(t *testing.T) {
	t.Run("L2 miss then hit", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cache.NewController(store, cache.NewTraceObserver())
		query := `{ me { favoriteProduct { upc stock } } }`
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		subgraphs := Subgraphs{"users": users, "products": products, "inventory": inventory}
		executionEngine := NewEngine(t, caching, subgraphs)

		firstBody := normalizedTraceBody(t, subgraphs, ExecuteTraced(t, executionEngine, query, controller))
		assert.Equal(t, `{
  "data": {
    "me": {
      "favoriteProduct": {
        "upc": "1",
        "stock": 5
      }
    }
  },
  "extensions": {
    "trace": {
      "version": "1",
      "info": {
        "trace_start_time": "",
        "trace_start_unix": 0,
        "parse_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "normalize_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "validate_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "planner_stats": {
          "duration_nanoseconds": 5,
          "duration_pretty": "5ns",
          "duration_since_start_nanoseconds": 20,
          "duration_since_start_pretty": "20ns"
        }
      },
      "fetches": {
        "kind": "Sequence",
        "children": [
          {
            "kind": "Single",
            "fetch": {
              "kind": "Single",
              "path": "",
              "source_id": "3",
              "source_name": "3",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://users.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "47"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 47
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me",
              "source_id": "0",
              "source_name": "0",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "99"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 99
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me.favoriteProduct",
              "source_id": "1",
              "source_name": "1",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://inventory.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "59"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 59
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false,
                "cache": {
                  "decision": "Fetch",
                  "hit": false,
                  "items": [
                    {
                      "key": "v1:1:4f796e3bbd360fce",
                      "hit": false
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  }
}`, firstBody)

		// The second request is a FULL L2 hit: the inventory fetch is skipped
		// (no input/output in its trace) and the cache section records the hit.
		secondBody := normalizedTraceBody(t, subgraphs, ExecuteTraced(t, executionEngine, query, controller))
		assert.Equal(t, `{
  "data": {
    "me": {
      "favoriteProduct": {
        "upc": "1",
        "stock": 5
      }
    }
  },
  "extensions": {
    "trace": {
      "version": "1",
      "info": {
        "trace_start_time": "",
        "trace_start_unix": 0,
        "parse_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "normalize_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "validate_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "planner_stats": {
          "duration_nanoseconds": 5,
          "duration_pretty": "5ns",
          "duration_since_start_nanoseconds": 20,
          "duration_since_start_pretty": "20ns"
        }
      },
      "fetches": {
        "kind": "Sequence",
        "children": [
          {
            "kind": "Single",
            "fetch": {
              "kind": "Single",
              "path": "",
              "source_id": "3",
              "source_name": "3",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://users.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "47"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 47
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me",
              "source_id": "0",
              "source_name": "0",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "99"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 99
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me.favoriteProduct",
              "source_id": "1",
              "source_name": "1",
              "trace": {
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
                      "key": "v1:1:4f796e3bbd360fce",
                      "served_from": "l2",
                      "hit": true,
                      "remaining_ttl_nanoseconds": -1
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  }
}`, secondBody)
		assert.Equal(t, int64(1), inventory.Requests())
	})

	t.Run("two representations of one entity are two independent entries", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cache.NewController(store, cache.NewTraceObserver())
		// The deal -> product(sku) -> reviews(upc) -> product(upc) chain reaches
		// ONE Product twice, but through two different representations: the
		// first fetch sends {sku}, the second {upc}. The key IS the
		// representation, so neither fetch can serve the other — both miss and
		// both write, under different keys.
		query := `{ deal(id: "d1") { product { name reviews { product { name } } } } }`
		deals := Respond(`{"data":{"deal":{"__typename":"Deal","id":"d1","product":{"__typename":"Product","sku":"S1"}}}}`)
		reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`)
		products := Rules(
			Rule(`"sku":"S1"`, `{"data":{"_entities":[{"__typename":"Product","name":"Table","upc":"1"}]}}`),
			Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","name":"Table"}]}}`),
		)
		subgraphs := Subgraphs{"deals": deals, "reviews": reviews, "products": products}
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {DefaultTTL: ptr(time.Minute)},
			},
		}
		executionEngine := NewEngine(t, caching, subgraphs)

		// The key hashes are deterministic (xxhash64 of the canonical
		// representation preimage), so the WHOLE cache sections pin as literals.
		body := normalizedTraceBody(t, subgraphs, ExecuteTraced(t, executionEngine, query, controller))
		assert.Equal(t, `{
  "data": {
    "deal": {
      "product": {
        "name": "Table",
        "reviews": [
          {
            "product": {
              "name": "Table"
            }
          }
        ]
      }
    }
  },
  "extensions": {
    "trace": {
      "version": "1",
      "info": {
        "trace_start_time": "",
        "trace_start_unix": 0,
        "parse_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "normalize_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "validate_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "planner_stats": {
          "duration_nanoseconds": 5,
          "duration_pretty": "5ns",
          "duration_since_start_nanoseconds": 20,
          "duration_since_start_pretty": "20ns"
        }
      },
      "fetches": {
        "kind": "Sequence",
        "children": [
          {
            "kind": "Single",
            "fetch": {
              "kind": "Single",
              "path": "",
              "source_id": "4",
              "source_name": "4",
              "trace": {
                "raw_input_data": {},
                "input": {
                  "body": {
                    "query": "query($a: ID!){deal(id: $a){product {__typename sku}}}",
                    "variables": {
                      "a": "d1"
                    }
                  },
                  "header": {},
                  "method": "POST",
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://deals.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "95"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 95
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "deal.product",
              "source_id": "0",
              "source_name": "0",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "74"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 74
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false,
                "cache": {
                  "decision": "Fetch",
                  "hit": false,
                  "items": [
                    {
                      "key": "v1:0:6ad3afa747e670b2",
                      "hit": false
                    }
                  ]
                }
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "deal.product",
              "source_id": "2",
              "source_name": "2",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://reviews.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "130"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 130
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "BatchEntity",
              "path": "deal.product.reviews.@.product",
              "source_id": "0",
              "source_name": "0",
              "trace": {
                "raw_input_data": {
                  "__typename": "Product",
                  "upc": "1"
                },
                "input": {
                  "body": {
                    "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename name}}}",
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
                  "url": "http://products.service"
                },
                "output": {
                  "data": {
                    "_entities": [
                      {
                        "__typename": "Product",
                        "name": "Table"
                      }
                    ]
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "64"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 64
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false,
                "cache": {
                  "decision": "Fetch",
                  "hit": false,
                  "items": [
                    {
                      "key": "v1:0:b9d0094b63d1f2a8",
                      "hit": false
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  }
}`, body)
	})

	t.Run("shadow compare", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cache.NewController(store, cache.NewTraceObserver())
		query := `{ me { favoriteProduct { upc stock } } }`
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		subgraphs := Subgraphs{"users": users, "products": products, "inventory": inventory}
		executionEngine := NewEngine(t, inventoryShadowCaching(), subgraphs)

		primeBody := Execute(t, executionEngine, query, controller)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, primeBody)

		secondBody := normalizedTraceBody(t, subgraphs, ExecuteTraced(t, executionEngine, query, controller))
		assert.Equal(t, `{
  "data": {
    "me": {
      "favoriteProduct": {
        "upc": "1",
        "stock": 5
      }
    }
  },
  "extensions": {
    "trace": {
      "version": "1",
      "info": {
        "trace_start_time": "",
        "trace_start_unix": 0,
        "parse_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "normalize_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "validate_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "planner_stats": {
          "duration_nanoseconds": 5,
          "duration_pretty": "5ns",
          "duration_since_start_nanoseconds": 20,
          "duration_since_start_pretty": "20ns"
        }
      },
      "fetches": {
        "kind": "Sequence",
        "children": [
          {
            "kind": "Single",
            "fetch": {
              "kind": "Single",
              "path": "",
              "source_id": "3",
              "source_name": "3",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://users.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "47"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 47
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me",
              "source_id": "0",
              "source_name": "0",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "99"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 99
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "Entity",
              "path": "me.favoriteProduct",
              "source_id": "1",
              "source_name": "1",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://inventory.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "59"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 59
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false,
                "cache": {
                  "decision": "FetchShadow",
                  "hit": false,
                  "shadow": true,
                  "items": [
                    {
                      "key": "v1:1:4f796e3bbd360fce",
                      "hit": false
                    }
                  ],
                  "shadow_compares": [
                    {
                      "key": "v1:1:4f796e3bbd360fce",
                      "is_fresh": true,
                      "cache_age_nanoseconds": -1
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  }
}`, secondBody)
		// Shadow mode never serves from cache: BOTH requests hit inventory.
		assert.Equal(t, int64(2), inventory.Requests())
	})

	t.Run("partial batch", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cache.NewController(store, cache.NewTraceObserver())
		// The products double routes by the rendered first-argument variable;
		// the reviews double by the representations: the priming run sends upc
		// "1", the partial batch afterwards sends ONLY the missing upc "2".
		products := Rules(
			Rule(`"variables":{"a":1}`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
			Rule(`"variables":{"a":2}`, `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`),
		)
		reviews := Rules(
			Rule(`"upc":"1"`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`),
			Rule(`"upc":"2"`, `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"sturdy chair"}]}]}}`),
		)
		subgraphs := Subgraphs{"products": products, "reviews": reviews}
		executionEngine := NewEngine(t, reviewsPartialCaching(), subgraphs)

		primeBody := Execute(t, executionEngine, `{ products(first: 1) { upc reviews { body } } }`, controller)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]}]}}`, primeBody)

		// One partial fetch: bucket 1 (upc "1") served from L2 under the primed
		// key, bucket 2 (upc "2") fetched over the reduced batch and refreshed.
		body := normalizedTraceBody(t, subgraphs, ExecuteTraced(t, executionEngine, `{ products(first: 2) { upc reviews { body } } }`, controller))
		assert.Equal(t, `{
  "data": {
    "products": [
      {
        "upc": "1",
        "reviews": [
          {
            "body": "great table"
          }
        ]
      },
      {
        "upc": "2",
        "reviews": [
          {
            "body": "sturdy chair"
          }
        ]
      }
    ]
  },
  "extensions": {
    "trace": {
      "version": "1",
      "info": {
        "trace_start_time": "",
        "trace_start_unix": 0,
        "parse_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "normalize_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "validate_stats": {
          "duration_nanoseconds": 0,
          "duration_pretty": "",
          "duration_since_start_nanoseconds": 0,
          "duration_since_start_pretty": ""
        },
        "planner_stats": {
          "duration_nanoseconds": 5,
          "duration_pretty": "5ns",
          "duration_since_start_nanoseconds": 20,
          "duration_since_start_pretty": "20ns"
        }
      },
      "fetches": {
        "kind": "Sequence",
        "children": [
          {
            "kind": "Single",
            "fetch": {
              "kind": "Single",
              "path": "",
              "source_id": "0",
              "source_name": "0",
              "trace": {
                "raw_input_data": {},
                "input": {
                  "body": {
                    "query": "query($a: Int){products(first: $a){upc __typename}}",
                    "variables": {
                      "a": 2
                    }
                  },
                  "header": {},
                  "method": "POST",
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://products.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "93"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 93
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false
              }
            }
          },
          {
            "kind": "Single",
            "fetch": {
              "kind": "BatchEntity",
              "path": "products",
              "source_id": "2",
              "source_name": "2",
              "trace": {
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
                  },
                  "extensions": {
                    "trace": {
                      "request": {
                        "method": "POST",
                        "url": "http://reviews.service",
                        "headers": {
                          "Accept": [
                            "application/json"
                          ],
                          "Accept-Encoding": [
                            "gzip",
                            "deflate"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        }
                      },
                      "response": {
                        "status_code": 200,
                        "status": "200 OK",
                        "headers": {
                          "Content-Length": [
                            "107"
                          ],
                          "Content-Type": [
                            "application/json"
                          ]
                        },
                        "body_size": 107
                      }
                    }
                  }
                },
                "single_flight_used": true,
                "single_flight_shared_response": false,
                "load_skipped": false,
                "cache": {
                  "decision": "FetchPartial",
                  "hit": false,
                  "items": [
                    {
                      "key": "v1:2:48d227258abb2a74",
                      "served_from": "l2",
                      "hit": true,
                      "remaining_ttl_nanoseconds": -1
                    },
                    {
                      "key": "v1:2:898c0bf202240771",
                      "hit": false
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  }
}`, body)
	})

	t.Run("regression: tracing off leaves fetch traces nil", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		query := `{ me { favoriteProduct { upc stock } } }`
		caching := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"inventory": {DefaultTTL: ptr(time.Minute)},
			},
		}
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, caching, Subgraphs{"users": users, "products": products, "inventory": inventory})

		// Tracing OFF: the response is the bare data — no extensions, no trace.
		body := Execute(t, executionEngine, query, cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)
	})
}
