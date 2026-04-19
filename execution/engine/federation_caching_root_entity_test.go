package engine_test

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	reviews "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestRootFieldEntityKeyMappingCacheSharing tests that a root field with EntityKeyMappings
// shares cache keys with entity fetches from another subgraph.
//
// Scenario (mirrors failing cosmo router test):
//   - "products" subgraph: root field product(upc: "top-1") → {upc, name, price}
//   - "reviews" subgraph: entity fetch Product._entities(upc: "top-1") → {reviews: [...]}
//   - Root field uses EntityKeyMappings so L2 key = {"__typename":"Product","key":{"upc":"top-1"}}
//   - Second request should hit L2 cache for both fetches (no subgraph calls)
//
// Root cause: EntityKeyMapping.ArgumentPath used the schema argument name ("upc"),
// but after variable extraction the actual variable in ctx.Variables has a normalized
// sequential name ("a"). The planner resolves this mismatch by looking up the actual
// ContextVariable path from the root field's tracked arguments.
func TestRootFieldEntityKeyMappingCacheSharing(t *testing.T) {
	t.Parallel()

	t.Run("root field with EntityKeyMappings L2 hit on second request", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "product",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
		require.NoError(t, err)
		reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
		require.NoError(t, err)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		productKey := `{"__typename":"Product","key":{"upc":"top-1"}}`

		// Request 1: cache miss → both subgraphs called
		defaultCache.ClearLog()
		tracker.Reset()
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, `{"data":{"product":{"upc":"top-1","name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}}}`, string(resp))

		assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph once")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "first request should call reviews subgraph once")
		assert.Equal(t, sortCacheLogKeysWithTTL([]CacheLogEntry{
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{false}}, // Products root field: cold cache, cache miss
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Products root field: write products payload under shared key
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},   // Reviews entity fetch: hits the shared root payload written above
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Reviews entity fetch: merge reviews payload into shared key
		}), sortCacheLogKeysWithTTL(defaultCache.GetLog()))

		// Request 2: should hit cache → neither subgraph called
		defaultCache.ClearLog()
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, string(resp), string(resp2), "both requests should return identical responses")

		assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should NOT call products subgraph (root field entity cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "second request should NOT call reviews subgraph (entity cache hit)")
		assert.Equal(t, []CacheLogEntry{
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}}, // Products root field: cache hit, skip subgraph
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}}, // Reviews entity fetch: cache hit on shared key, skip subgraph
		}, defaultCache.GetLog())
	})

	t.Run("shadow mode with EntityKeyMappings always calls subgraph", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:   "Query",
							FieldName:  "product",
							CacheName:  "default",
							TTL:        30 * time.Second,
							ShadowMode: true,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
		require.NoError(t, err)
		reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
		require.NoError(t, err)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		productKey := `{"__typename":"Product","key":{"upc":"top-1"}}`

		// Request 1: cache miss → subgraph called, shadow write populates cache
		defaultCache.ClearLog()
		tracker.Reset()
		gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "first request should call reviews subgraph")
		assert.Equal(t, sortCacheLogKeysWithTTL([]CacheLogEntry{
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{false}}, // Products root field (shadow): cold cache shadow read, miss
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Products root field (shadow): shadow write of products payload
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},   // Reviews entity fetch (non-shadow): hits the shared shadow-written key
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Reviews entity fetch (non-shadow): merge reviews payload under shared key
		}), sortCacheLogKeysWithTTL(defaultCache.GetLog()))

		// Request 2: shadow mode → subgraph MUST be called again (shadow read happens but is not served)
		defaultCache.ClearLog()
		tracker.Reset()
		gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, 1, tracker.GetCount(productsHost), "shadow mode should always call products subgraph")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "reviews entity cache is non-shadow, so second request should hit cache")
		assert.Equal(t, sortCacheLogKeysWithTTL([]CacheLogEntry{
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},   // Products root field (shadow): hit, but shadow mode ignores the cached value
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Products root field (shadow): shadow re-write after subgraph call
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},   // Reviews entity fetch (non-shadow): cache hit, skip subgraph
		}), sortCacheLogKeysWithTTL(defaultCache.GetLog()))
	})

	t.Run("root field with EntityKeyMappings caches nullable negative entity response without nulling root object", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		reviewsInterceptor := newSubgraphResponseInterceptor(reviews.GraphQLEndpointHandler(reviews.TestOptions))
		reviewsInterceptor.SetModifier(func(body []byte) []byte {
			if bytes.Contains(body, []byte(`"_service"`)) {
				return body
			}
			return []byte(`{"data":{"_entities":[null]}}`)
		})

		setup := newFederationSetupWithReviewInterceptor(reviewsInterceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "product",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{
							TypeName:         "Product",
							CacheName:        "default",
							TTL:              30 * time.Second,
							NegativeCacheTTL: 10 * time.Second,
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
		require.NoError(t, err)
		reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
		require.NoError(t, err)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		query := `query { product(upc: "top-1") { upc name reviews { body } } }`
		expected := `{"data":{"product":{"upc":"top-1","name":"Trilby","reviews":null}}}`
		productKey := `{"__typename":"Product","key":{"upc":"top-1"}}`

		defaultCache.ClearLog()
		tracker.Reset()
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expected, string(resp))
		assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "first request should call reviews subgraph")

		storedValue, exists := defaultCache.Peek(productKey)
		assert.True(t, exists, "shared entity/root cache key should be populated")
		assert.Equal(t, compactJSONForAssert(t, `{"__typename":"Product","upc":"top-1","name":"Trilby","reviews":null}`), compactJSONForAssert(t, string(storedValue)))
		assert.Equal(t, sortCacheLogKeysWithTTL([]CacheLogEntry{
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{false}}, // Products root field: cold cache, cache miss
			{Operation: "set", Keys: []string{productKey}, TTL: 30 * time.Second}, // Products root field: write positive payload under shared key with 30s TTL
			{Operation: "get", Keys: []string{productKey}, Hits: []bool{true}},   // Reviews entity fetch: hits the shared root payload written above
			{Operation: "set", Keys: []string{productKey}, TTL: 10 * time.Second}, // Reviews entity fetch: merge reviews:null negative payload with 10s NegativeCacheTTL
		}), sortCacheLogKeysWithTTL(defaultCache.GetLog()))

		defaultCache.ClearLog()
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expected, string(resp2))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should skip products subgraph on shared-key root cache hit")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "second request should skip reviews subgraph: reviews:null lives inside the shared root payload, so this is an object-shaped cache hit, not a TypeNull negative-sentinel hit")
		assert.Equal(t, []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
		}, defaultCache.GetLog())
	})

	t.Run("root field with EntityKeyMappings reuses cached nullable negative field for narrower follow-up query", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		reviewsInterceptor := newSubgraphResponseInterceptor(reviews.GraphQLEndpointHandler(reviews.TestOptions))
		reviewsInterceptor.SetModifier(func(body []byte) []byte {
			if bytes.Contains(body, []byte(`"_service"`)) {
				return body
			}
			return []byte(`{"data":{"_entities":[null]}}`)
		})

		setup := newFederationSetupWithReviewInterceptor(reviewsInterceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "product",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{
							TypeName:         "Product",
							CacheName:        "default",
							TTL:              30 * time.Second,
							NegativeCacheTTL: 10 * time.Second,
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
		require.NoError(t, err)
		reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
		require.NoError(t, err)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		seedQuery := `query { product(upc: "top-1") { upc name reviews { body } } }`
		followUpQuery := `query { product(upc: "top-1") { upc reviews { body } } }`
		productKey := `{"__typename":"Product","key":{"upc":"top-1"}}`

		defaultCache.ClearLog()
		tracker.Reset()
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, seedQuery, nil, t)
		assert.Equal(t, `{"data":{"product":{"upc":"top-1","name":"Trilby","reviews":null}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(productsHost), "seed request should call products subgraph")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "seed request should call reviews subgraph")
		assert.Equal(t, sortCacheLogKeysWithTTL([]CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{productKey},
				TTL:       30 * time.Second,
			},
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
			{
				Operation: "set",
				Keys:      []string{productKey},
				TTL:       10 * time.Second,
			},
		}), sortCacheLogKeysWithTTL(defaultCache.GetLog()))
		storedValue, exists := defaultCache.Peek(productKey)
		assert.True(t, exists, "shared entity/root cache key should be populated after the seed request")
		assert.Equal(t, compactJSONForAssert(t, `{"__typename":"Product","upc":"top-1","name":"Trilby","reviews":null}`), compactJSONForAssert(t, string(storedValue)))

		defaultCache.ClearLog()
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, followUpQuery, nil, t)
		assert.Equal(t, `{"data":{"product":{"upc":"top-1","reviews":null}}}`, string(resp2))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "follow-up query should skip products subgraph on shared-key root cache hit")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "follow-up query should skip reviews subgraph: reviews:null is already stored as a field inside the shared root payload (object-shaped hit, not a TypeNull sentinel)")
		assert.Equal(t, []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
		}, defaultCache.GetLog())
	})

	t.Run("root field with EntityKeyMappings does not cache nullable negative entity response when NegativeCacheTTL is unset", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		reviewsInterceptor := newSubgraphResponseInterceptor(reviews.GraphQLEndpointHandler(reviews.TestOptions))
		reviewsInterceptor.SetModifier(func(body []byte) []byte {
			if bytes.Contains(body, []byte(`"_service"`)) {
				return body
			}
			return []byte(`{"data":{"_entities":[null]}}`)
		})

		setup := newFederationSetupWithReviewInterceptor(reviewsInterceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "product",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{
							TypeName:  "Product",
							CacheName: "default",
							TTL:       30 * time.Second,
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
		require.NoError(t, err)
		reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
		require.NoError(t, err)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		query := `query { product(upc: "top-1") { upc name reviews { body } } }`
		expected := `{"data":{"product":{"upc":"top-1","name":"Trilby","reviews":null}}}`
		productKey := `{"__typename":"Product","key":{"upc":"top-1"}}`

		defaultCache.ClearLog()
		tracker.Reset()
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expected, string(resp))
		assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "first request should call reviews subgraph")
		assert.Equal(t, []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{productKey},
				TTL:       30 * time.Second,
			},
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
		}, defaultCache.GetLog())

		storedValue, exists := defaultCache.Peek(productKey)
		assert.True(t, exists, "shared entity/root cache key should still hold the positive root payload")
		assert.Equal(t, compactJSONForAssert(t, `{"__typename":"Product","upc":"top-1","name":"Trilby"}`), compactJSONForAssert(t, string(storedValue)))

		defaultCache.ClearLog()
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expected, string(resp2))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should skip products subgraph on shared-key root cache hit")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "second request should call reviews subgraph again when negative caching is disabled")
		assert.Equal(t, []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
			{
				Operation: "get",
				Keys:      []string{productKey},
				Hits:      []bool{true},
			},
		}, defaultCache.GetLog())
	})
}
