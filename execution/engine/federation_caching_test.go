package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFederationCaching(t *testing.T) {
	t.Run("two subgraphs - miss then hit", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable caching for L2 tests (opt-in per-subgraph)
		// Explicitly configure which subgraphs cache which root fields and entity types
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: Query.topProducts (get/set), Product entities (get/set), User entities (get/set)
		// With root field caching enabled, Query.topProducts is now cached too.
		assert.Equal(t, 6, len(logAfterFirst), "Should have exactly 6 cache operations (get+set for root field, Products, Users)")

		// Verify the exact cache access log (order may vary for keys within each operation)
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			// Product entity fetches (reviews data for each product)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity fetches (author data)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match expected")

		// Verify subgraph calls for first query
		// First query should call products (topProducts), reviews (reviews), and accounts (User entity)
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - should hit cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All cache operations should be gets with hits: Query.topProducts, Product entities, User entities
		// With root field caching enabled, all 3 types should hit cache
		assert.Equal(t, 3, len(logAfterSecond), "Second query should have 3 cache get operations (all hits)")

		// Verify the exact cache access log for second query (all hits)
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true},
			},
			// Product entity fetches - HITS
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true},
			},
			// User entity fetches - HITS
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query cache log should match expected (all hits)")

		// Verify subgraph calls for second query
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		// With root field caching enabled, all subgraphs should be skipped on second query
		assert.Equal(t, 0, productsCallsSecond, "Second query should skip products subgraph (root field cache hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should skip reviews subgraph (entity cache hit)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should skip accounts subgraph (entity cache hit)")
	})

	t.Run("two subgraphs - partial fields then full fields", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable caching for L2 tests (opt-in per-subgraph)
		// Configure root field caching for products and entity caching for reviews/accounts
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - only ask for name field (products subgraph only)
		defaultCache.ClearLog()
		tracker.Reset()
		firstQuery := `query {
			topProducts {
				name
			}
		}`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, firstQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// With root field caching enabled: get miss + set for Query.topProducts
		assert.Equal(t, 2, len(logAfterFirst), "First query should have 2 cache operations (get miss + set for root field)")

		// Verify the exact cache access log for first query
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match expected")

		// Verify first query calls products subgraph only
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query calls products subgraph once")
		assert.Equal(t, 0, reviewsCallsFirst, "First query does not call reviews subgraph")
		assert.Equal(t, 0, accountsCallsFirst, "First query does not call accounts subgraph")

		// Second query - ask for full fields including reviews (products + reviews + accounts)
		defaultCache.ClearLog()
		tracker.Reset()
		secondQuery := `query {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, secondQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Cache operations with root field caching:
		// - Root field Query.topProducts: get (miss - different query shape) + set
		// - Product entities: get miss + set
		// - User entities: get miss + set
		// Note: The first query only requested 'name', second query requests 'name' and 'reviews'.
		// These are different query operations, so different cache keys.
		assert.Equal(t, 6, len(logAfterSecond), "Second query should have 6 cache operations")

		// Verify the exact cache access log for second query
		// Note: Root field Query.topProducts is a HIT because cache key doesn't include selected fields
		// The first query already cached this root field, so the second query reuses it
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (same cache key, different selection doesn't matter)
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true},
			},
			// Still need to set because cache returns partial data that needs merging
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			// Product entity fetches - MISS (first time fetching these)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity fetches - MISS (first time fetching these)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query cache log should match expected")

		// Verify second query subgraph calls
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsSecond, "Second query calls products subgraph once (different query shape)")
		assert.Equal(t, 1, reviewsCallsSecond, "Second query calls reviews subgraph once (for reviews data)")
		assert.Equal(t, 1, accountsCallsSecond, "Second query calls accounts subgraph for User entity resolution")

		// Third query - repeat the second query (full fields)
		defaultCache.ClearLog()
		tracker.Reset()
		thirdQuery := `query {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, thirdQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterThird := defaultCache.GetLog()
		// All cache operations should be gets with hits: root field, Product entities, User entities
		// Third query is same as second query, so all should hit cache
		assert.Equal(t, 3, len(logAfterThird), "Third query should have 3 cache get operations (all hits)")

		// Verify the exact cache access log for third query (all hits)
		wantLogThird := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true},
			},
			// Product entity fetches - HITS
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true},
			},
			// User entity fetches - HITS
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Third query cache log should match expected (all hits)")

		// Verify third query: all data is cached, no subgraph calls needed
		productsCallsThird := tracker.GetCount(productsHost)
		reviewsCallsThird := tracker.GetCount(reviewsHost)
		accountsCallsThird := tracker.GetCount(accountsHost)

		// With root field caching enabled, all subgraphs should be skipped
		assert.Equal(t, 0, productsCallsThird, "Third query skips products subgraph (root field cache hit)")
		assert.Equal(t, 0, reviewsCallsThird, "Third query skips reviews subgraph (entity cache hits)")
		assert.Equal(t, 0, accountsCallsThird, "Third query skips accounts subgraph (entity cache hits)")
	})

	t.Run("two subgraphs - with subgraph header prefix", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Create mock SubgraphHeadersBuilder that returns a fixed hash for each subgraph
		// Subgraph names are used as keys for the header hash lookup:
		// - "accounts" -> prefix 33333 for User entity cache keys
		// - "products" -> prefix 11111 for Query cache keys
		// - "reviews" -> prefix 22222 for Product entity cache keys
		mockHeadersBuilder := &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{
				"accounts": 33333,
				"products": 11111,
				"reviews":  22222,
			},
		}

		// Enable root field and entity caching with subgraph header prefix for L2 tests (opt-in per-subgraph caching)
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withSubgraphHeadersBuilder(mockHeadersBuilder),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache and then set with prefixed keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: products (get/set), reviews (get/set), accounts User entity (get/set)
		assert.Equal(t, 6, len(logAfterFirst))

		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity resolution from accounts (author.username requires entity fetch)
			{
				Operation: "get",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst))

		// Verify subgraph calls for first query
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		// Accounts IS called for User entity resolution (author.username requires entity fetch)
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - should hit cache with prefixed keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Root field, Product entities, and User entities should all hit L2 cache with prefixed keys
		assert.Equal(t, 3, len(logAfterSecond), "Second query should have 3 cache get operations (all hits)")

		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT with prefix
			{
				Operation: "get",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true},
			},
			// Product entities - HIT with prefix
			{
				Operation: "get",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true},
			},
			// User entities - HIT with prefix
			{
				Operation: "get",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify subgraph calls for second query - all should be skipped due to cache hits
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 0, productsCallsSecond, "Second query should skip products subgraph (root field cache hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should skip reviews subgraph (entity cache hit)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should skip accounts subgraph (entity cache hit)")
	})
}

func TestRootFieldCachingWithArgs(t *testing.T) {
	t.Run("root field with args - miss then hit", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - cache miss
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "First query should have 2 cache operations (get miss + set)")
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts subgraph once")

		// Second query - cache hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have 1 cache get (hit)")
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query should hit cache")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts subgraph (cache hit)")
	})

	t.Run("root field with args - different args different keys", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query with id=1234
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query should miss cache and set")

		// Second query with id=5678 - different cache key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "5678"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"5678","username":"User 5678"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query with different id should call accounts once")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterSecond), "Second query with different id should have get miss + set")
		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"5678"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"5678"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterSecond), "Different args should produce different cache keys")

		// Third query with id=1234 - should hit cache from first query
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Third query (same as first) should hit cache")

		logAfterThird := defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Third query should hit cache from first query")
	})

	t.Run("entity key mapping - uses entity key format", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Query with entity key mapping - should use entity key format
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set")
		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst), "Should use entity key format, not root field format")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		// Second query - should hit cache using entity key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should hit cache")
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query should hit entity cache key")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")
	})

	t.Run("entity key mapping - invalidation via entity key", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - cache miss, populate
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts")

		// Delete the entity key from cache
		err := defaultCache.Delete(ctx, []string{`{"__typename":"User","key":{"id":"1234"}}`})
		require.NoError(t, err)

		// Third query - should be a miss after deletion
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "After deletion, should call accounts again")

		logAfterDelete := defaultCache.GetLog()
		wantLogDelete := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogDelete), sortCacheLogKeys(logAfterDelete), "After deletion: get miss + set")
	})

	t.Run("entity key mapping - cross-lookup from entity fetch", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Configure both root field entity key mapping AND entity caching for same type
		// Both use same cache key format: {"__typename":"User","key":{"id":"1234"}}
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First: Query user by ID (root field with entity key mapping)
		// This caches under entity key {"__typename":"User","key":{"id":"1234"}}
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Root field query should call accounts once")

		// Verify root field used entity key format
		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Root field query should use entity key format")

		// Second: Query that triggers entity fetch for same User 1234
		// Both root field and entity fetch use the same cache key format.
		// The root field stored entity-level data (extracted at merge path) thanks to EntityMergePath,
		// so the entity fetch finds {"id":"1234","username":"Me"} → validation passes → cache HIT.
		// No re-fetch needed, no SET operation.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Entity fetch should skip accounts (cross-lookup hit: root field stored entity-level data)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`},
				Hits:      []bool{false, false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`},
			},
			{
				// Cross-lookup hit: root field stored entity-level data,
				// entity fetch reads it and validation passes.
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Entity fetch should use same key format as root field entity key mapping")
	})

	t.Run("entity key mapping - cross-lookup from root field", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Configure both root field entity key mapping AND entity caching for same type
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First: Query that triggers entity fetch for User 1234 (via topProducts → reviews → authorWithoutProvides)
		// Entity fetch stores entity-level data: {"id":"1234","username":"Me"}
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once for entity resolution")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query should miss all caches and set")

		// Second: Root field query with entity key mapping for same User 1234
		// Root field generates entity key {"__typename":"User","key":{"id":"1234"}} (same as entity fetch).
		// Cache has entity-level data → EntityMergePath wraps it to response-level → validation passes → HIT.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Root field query should skip accounts (cross-lookup hit from entity fetch)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				// Cross-lookup hit: entity fetch stored entity-level data,
				// root field wraps it at merge path and validation passes.
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Root field should hit cache from entity fetch data")
	})

	t.Run("entity key mapping + header prefix", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		mockHeadersBuilder := &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{
				"accounts": 33333,
			},
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withSubgraphHeadersBuilder(mockHeadersBuilder),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		defaultCache.ClearLog()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set")
		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`33333:{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`33333:{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst), "Entity key should have header prefix")
	})

	t.Run("root field without args - regression", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		productsHost := productsURLParsed.Host

		// First query
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { topProducts { name } }`, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(productsHost), "First query should call products once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Should use root field key format (no entity key mapping)")

		// Second query - hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { topProducts { name } }`, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query should hit cache")
	})

	t.Run("root field caching + entity caching nested", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "product",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
					},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// Query product with nested reviews
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { product(upc: "top-1") { name reviews { body } } }`, queryVariables{"upc": "top-1"}, t)
		assert.Equal(t, `{"data":{"product":{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(productsHost), "First query should call products once")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "First query should call reviews once")

		logAfterFirst := defaultCache.GetLog()
		// Should have root field get/set + entity get/set
		assert.Equal(t, 4, len(logAfterFirst), "Should have 4 cache operations (root field get/set + entity get/set)")
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`},
			},
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-1"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-1"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query should miss both root field and entity cache")

		// Second identical query - all from cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { product(upc: "top-1") { name reviews { body } } }`, queryVariables{"upc": "top-1"}, t)
		assert.Equal(t, `{"data":{"product":{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products (root field cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "Second query should skip reviews (entity cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`},
				Hits:      []bool{true},
			},
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-1"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query should hit both root field and entity cache")
	})

	t.Run("TTL expiry", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 100 * time.Millisecond, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - cache miss
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts")

		// Second query immediately - cache hit
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Immediate second query should hit cache")

		// Wait for TTL to expire
		time.Sleep(200 * time.Millisecond)

		// Third query after expiry - cache miss
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Query after TTL expiry should call accounts")
	})

	t.Run("concurrency with different IDs", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Run 10 concurrent queries with different IDs
		var wg sync.WaitGroup
		results := make([]string, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				id := strconv.Itoa(idx + 1000)
				resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": id}, t)
				results[idx] = string(resp)
			}(i)
		}
		wg.Wait()

		// Verify all results
		for i := 0; i < 10; i++ {
			id := strconv.Itoa(i + 1000)
			expected := fmt.Sprintf(`{"data":{"user":{"id":"%s","username":"User %s"}}}`, id, id)
			assert.Equal(t, expected, results[i], "Concurrent query %d should return correct result", i)
		}
	})

	t.Run("two args - reversed argument order hits cache", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "userByIdAndName", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query: arguments in schema-defined order (id, username)
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`, queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match")

		// Second query: arguments in REVERSED order (username, id)
		// The cache key should be identical because the planner always adds arguments
		// in the order defined by the field configuration (schema order), not query order.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($username: String!, $id: ID!) { userByIdAndName(username: $username, id: $id) { username id } }`, queryVariables{"username": "Me", "id": "1234"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"username":"Me","id":"1234"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query (reversed args) should hit cache with identical key")
	})

	t.Run("root field more fields then fewer fields - cache hit (superset)", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query: fetch MORE fields (username + realName) - cache miss
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username realName } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me","realName":"Real Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match")

		// Second query: fetch FEWER fields (username only) - should be cache HIT
		// The cached data has {username, realName}, the query only needs {username} → superset → hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query (fewer fields) should be a cache HIT because cached data is a superset")
	})

	t.Run("root field fewer fields then more fields - cache miss (subset)", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query: fetch FEWER fields (username only) - cache miss
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First query cache log should match")

		// Second query: fetch MORE fields (username + realName) - should be cache MISS
		// The cached data only has {username}, the query needs {username, realName} → subset → miss
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username realName } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me","realName":"Real Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query should call accounts (cache miss - needs more fields)")

		logAfterSecond := defaultCache.GetLog()
		// The cache GET returns a hit (key exists), but validateItemHasRequiredData fails
		// because the cached data is missing realName. This causes a re-fetch (tracker=1) and cache update.
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second query should find stale cache entry but re-fetch because cached data is only a subset")

		// Third query: same more-fields query - should now hit cache (re-fetch populated it)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username realName } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me","realName":"Real Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Third query should skip accounts (cache hit after re-fetch)")

		logAfterThird := defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"user","args":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Third query should hit cache with full data from re-fetch")
	})

	t.Run("entity key mapping - multiple keys single mapping", func(t *testing.T) {
		// User has @key(fields: "id") @key(fields: "username"), but root field user(id)
		// only maps to the "id" key. Adding a second @key doesn't change behavior
		// when only one key is mapped.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - miss, stores under single entity key
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set")
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Single mapping: only id key, not combined id+username")

		// Second query - hit via entity key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Should hit cache via entity key")
	})

	t.Run("entity key mapping - multiple keys multiple mappings", func(t *testing.T) {
		// User has @key(fields: "id") @key(fields: "username").
		// Root field userByIdAndName(id, username) maps to BOTH keys.
		// Data is stored under 2 entity keys, one per mapping.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "userByIdAndName",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - miss, stores under BOTH entity keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set (both keys)")
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Multiple mappings: data stored under both id and username keys")

		// Second query - hit (via either key)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
				Hits: []bool{true, true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Both keys should hit cache")
	})

	t.Run("entity key mapping - multiple mappings partial args", func(t *testing.T) {
		// Two entity key mappings configured (id and username),
		// but only the id variable is provided. The username mapping
		// cannot resolve → only a single entity cache key is generated.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "user",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								},
							},
						},
					},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - miss, only id mapping resolves → single cache key
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set (single key only)")
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Only id mapping resolves, username mapping skipped (missing variable)")

		// Second query - hit via id key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Single id key should hit cache")
	})

	t.Run("entity key mapping - multiple mappings cross-lookup", func(t *testing.T) {
		// Root field userByIdAndName stores under BOTH entity keys.
		// Entity fetch for User uses @key(fields: "id") → finds data stored by root field.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:                    "Query",
						FieldName:                   "userByIdAndName",
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						IncludeSubgraphHeaderPrefix: false,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								},
							},
							{
								EntityTypeName: "User",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								},
							},
						},
					},
				},
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First: Root field stores user under both entity keys (id and username)
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Root field query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Root field should store under both id and username entity keys")

		// Second: Entity fetch for User 1234 via topProducts → reviews → authorWithoutProvides
		// Entity fetch uses @key(fields: "id") → finds data stored under id key by root field
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Entity fetch should skip accounts (cross-lookup hit: root field stored under id key)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			{
				// Cross-lookup hit: root field stored entity-level data under id key,
				// entity fetch finds it via @key(fields: "id").
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Entity fetch should cross-lookup User via id key stored by root field")
	})

	t.Run("root field not configured - still calls subgraph", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Only configure products - not accounts
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), withSubgraphEntityCachingConfigs(subgraphCachingConfigs)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts (not cached)")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 0, len(logAfterFirst), "Unconfigured root field should produce no cache operations")

		// Second query - not cached, should call again
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query should also call accounts (not cached)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 0, len(logAfterSecond), "Unconfigured root field should produce no cache operations on second query either")
	})
}

func TestFederationCaching_MutationSkipsL2Read(t *testing.T) {
	// Shared caching config for all subtests: only entity caching for User on accounts
	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
		{
			SubgraphName: "accounts",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
	}

	mutationVars := queryVariables{
		"authorID": "1234",
		"upc":      "top-1",
		"review":   "Great!",
	}

	t.Run("mutation skips L2 cache read and writes updated entity", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: Query populates L2 cache.
		// The query fetches me.reviews.authorWithoutProvides.username, which triggers
		// User entity resolution from accounts. L2 cache is empty → miss → fetch → set.
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery1 := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterQuery1), "Step 1: should have exactly 2 cache operations (get miss + set for User)")
		wantLogQuery1 := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogQuery1), sortCacheLogKeys(logAfterQuery1), "Step 1: cache log should show get miss then set for User")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once for User entity resolution")

		// Step 2: Mutation skips L2 read, still writes to L2.
		// The mutation guard in tryL2CacheLoad checks l.info.OperationType != Query,
		// so L2 read is bypassed. After the entity fetch completes, updateL2Cache
		// writes fresh data (cacheMustBeUpdated=true).
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterMutation), "Step 2: should have exactly 1 cache operation (set only, NO get)")
		wantLogMutation := []CacheLogEntry{
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogMutation), sortCacheLogKeys(logAfterMutation), "Step 2: mutation should only set to L2, never get")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: mutation should call accounts subgraph (not served from cache)")

		// Step 3: Query reads from L2 (hit).
		// Same query as step 1. User entity is in L2 from the mutation's write → HIT.
		// No accounts call needed (entity resolution fully served from L2).
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery2 := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterQuery2), "Step 3: should have exactly 1 cache operation (get hit)")
		wantLogQuery2 := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogQuery2), sortCacheLogKeys(logAfterQuery2), "Step 3: query should hit L2 cache for User")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 3: query should NOT call accounts subgraph (L2 cache hit)")
	})

	t.Run("mutation with no prior cache writes to L2 for subsequent query", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: Mutation first (no prior cache)
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterMutation), "Step 1: should have exactly 1 cache operation (set only)")
		wantLogMutation := []CacheLogEntry{
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogMutation), sortCacheLogKeys(logAfterMutation), "Step 1: mutation should only set to L2")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Step 2: Query reads from L2 (hit from mutation's write)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterQuery), "Step 2: should have exactly 1 cache operation (get hit)")
		wantLogQuery := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogQuery), sortCacheLogKeys(logAfterQuery), "Step 2: query should hit L2 cache for User")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 2: query should NOT call accounts subgraph (L2 cache hit)")
	})

	t.Run("consecutive mutations never read from L2 cache", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: First mutation
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation1 := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterMutation1), "Step 1: should have exactly 1 cache operation (set only)")
		wantLogMutation1 := []CacheLogEntry{
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogMutation1), sortCacheLogKeys(logAfterMutation1), "Step 1: first mutation should only set to L2")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Step 2: Second mutation (same author, different review)
		defaultCache.ClearLog()
		tracker.Reset()
		mutation2Vars := queryVariables{
			"authorID": "1234",
			"upc":      "top-2",
			"review":   "Also great!",
		}
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutation2Vars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Also great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation2 := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterMutation2), "Step 2: should have exactly 1 cache operation (set only, NO get even though L2 has data)")
		wantLogMutation2 := []CacheLogEntry{
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogMutation2), sortCacheLogKeys(logAfterMutation2), "Step 2: second mutation should only set to L2, never get")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: should call accounts subgraph exactly once (not from cache)")
	})

	t.Run("query with different fields after mutation hits L2 cache", func(t *testing.T) {
		// Entity fetches store complete entity data from the subgraph (all fields the subgraph provides),
		// not just the fields selected in the current query. So a mutation that triggers entity resolution
		// for User populates L2 with full User data, and a subsequent query selecting different fields
		// (e.g., nickname) will still get a cache HIT.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
			withDebugMode(true),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: Mutation writes User entity data to L2 (skips L2 read).
		// The mutation guard in tryL2CacheLoad bypasses L2 reads for non-query operations.
		// After entity resolution, updateL2Cache writes fresh User data to L2.
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation := defaultCache.GetLogWithCaller()
		assert.Equal(t, 1, len(logAfterMutation), "Step 1: should have exactly 1 cache operation (set only)")
		wantLogMutation := []CacheLogEntry{
			// updateL2Cache writes fresh User data after entity resolution (mutation skipped L2 read).
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Caller:    "accounts: entity(User)",
			},
		}
		assert.Equal(t, sortCacheLogKeysWithCaller(wantLogMutation), sortCacheLogKeysWithCaller(logAfterMutation), "Step 1: mutation should only set to L2")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Step 2: Query requests different fields (username + nickname).
		// The query plan has two fetch nodes in a serial chain that both use the User entity cache key:
		//   (a) Entity resolution for authorWithoutProvides User → tryL2CacheLoad → HIT (from mutation's write)
		//   (b) A separate fetch to accounts (for the `me` root query) → fetches from accounts → updateL2Cache writes to L2
		// Entity fetches store complete entity data from the subgraph, so even though the mutation
		// only selected username, the cached data includes all User fields (username, nickname, etc.),
		// and the entity resolution for authorWithoutProvides gets a full HIT.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides_with_nickname.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}}]}}}`, string(resp))

		logAfterQuery := defaultCache.GetLogWithCaller()
		assert.Equal(t, 2, len(logAfterQuery), "Step 2: should have exactly 2 cache operations (get hit + set)")
		wantLogQuery := []CacheLogEntry{
			// Entity resolution for authorWithoutProvides checks L2 → HIT (data from mutation's write).
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Hits:      []bool{true},
				Caller:    "accounts: entity(User)",
			},
			// A separate fetch to accounts (me root query) fetches User data and writes it to L2.
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"User","key":{"id":"1234"}}`},
				Caller:    "accounts: entity(User)",
			},
		}
		assert.Equal(t, sortCacheLogKeysWithCaller(wantLogQuery), sortCacheLogKeysWithCaller(logAfterQuery), "Step 2: query should hit L2 cache (entity stores complete data)")
		// Accounts is called once for the me root query (not cached), but NOT for entity resolution (L2 hit)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: accounts called once for me root query, entity resolution served from L2 cache")
	})
}

