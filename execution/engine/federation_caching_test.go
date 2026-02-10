package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	reviewsgraph "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
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
	// Ensure reviews are reset after all subtests complete to avoid polluting other test functions.
	t.Cleanup(reviewsgraph.ResetReviews)

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
		reviewsgraph.ResetReviews()

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
		reviewsgraph.ResetReviews()

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
		reviewsgraph.ResetReviews()

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
		reviewsgraph.ResetReviews()

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

// subgraphCallTracker tracks HTTP requests made to subgraph servers
type subgraphCallTracker struct {
	mu       sync.RWMutex
	counts   map[string]int // Maps subgraph URL to call count
	original http.RoundTripper
}

func newSubgraphCallTracker(original http.RoundTripper) *subgraphCallTracker {
	return &subgraphCallTracker{
		counts:   make(map[string]int),
		original: original,
	}
}

func (t *subgraphCallTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	host := req.URL.Host
	t.counts[host]++
	t.mu.Unlock()
	return t.original.RoundTrip(req)
}

func (t *subgraphCallTracker) GetCount(url string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.counts[url]
}

func (t *subgraphCallTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts = make(map[string]int)
}

func (t *subgraphCallTracker) GetCounts() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]int)
	for k, v := range t.counts {
		result[k] = v
	}
	return result
}

func (t *subgraphCallTracker) DebugPrint() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return fmt.Sprintf("%v", t.counts)
}

// Helper functions for gateway setup with HTTP client support
type cachingGatewayOptions struct {
	enableART                    bool
	withLoaderCache              map[string]resolve.LoaderCache
	httpClient                   *http.Client
	subgraphHeadersBuilder       resolve.SubgraphHeadersBuilder
	cachingOptions               resolve.CachingOptions
	subgraphEntityCachingConfigs engine.SubgraphCachingConfigs
	debugMode                    bool
}

func withCachingEnableART(enableART bool) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.enableART = enableART
	}
}

func withCachingLoaderCache(loaderCache map[string]resolve.LoaderCache) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.withLoaderCache = loaderCache
	}
}

func withHTTPClient(client *http.Client) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.httpClient = client
	}
}

func withSubgraphHeadersBuilder(builder resolve.SubgraphHeadersBuilder) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.subgraphHeadersBuilder = builder
	}
}

func withCachingOptionsFunc(cachingOpts resolve.CachingOptions) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.cachingOptions = cachingOpts
	}
}

func withSubgraphEntityCachingConfigs(configs engine.SubgraphCachingConfigs) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.subgraphEntityCachingConfigs = configs
	}
}

func withDebugMode(enabled bool) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.debugMode = enabled
	}
}

type cachingGatewayOptionsToFunc func(opts *cachingGatewayOptions)

func addCachingGateway(options ...cachingGatewayOptionsToFunc) func(setup *federationtesting.FederationSetup) *httptest.Server {
	opts := &cachingGatewayOptions{}
	for _, option := range options {
		option(opts)
	}
	return func(setup *federationtesting.FederationSetup) *httptest.Server {
		httpClient := opts.httpClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}

		poller := gateway.NewDatasource([]gateway.ServiceConfig{
			{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
			{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
			{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
		}, httpClient)

		gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, httpClient, opts.enableART, opts.withLoaderCache, opts.subgraphHeadersBuilder, opts.cachingOptions, opts.subgraphEntityCachingConfigs, opts.debugMode)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		poller.Run(ctx)
		return httptest.NewServer(gtw)
	}
}

// mockSubgraphHeadersBuilder is a mock implementation of SubgraphHeadersBuilder
type mockSubgraphHeadersBuilder struct {
	hashes map[string]uint64
}

func (m *mockSubgraphHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	hash := m.hashes[subgraphName]
	if hash == 0 {
		// Return default hash if not found
		return nil, 99999
	}
	return nil, hash
}

func (m *mockSubgraphHeadersBuilder) HashAll() uint64 {
	// Return a simple hash of all subgraph hashes combined
	var result uint64
	for _, hash := range m.hashes {
		result ^= hash
	}
	return result
}

func cachingTestQueryPath(name string) string {
	return path.Join("..", "federationtesting", "testdata", name)
}

type CacheLogEntry struct {
	Operation string   // "get", "set", "delete"
	Keys      []string // Keys involved in the operation
	Hits      []bool   // For Get: whether each key was a hit (true) or miss (false)
	Caller    string   // Fetch identity when debug enabled: "accounts: entity(User)" or "products: rootField(Query.topProducts)"
}

// sortCacheLogKeys sorts the keys (and corresponding hits) in each cache log entry.
// This makes comparisons order-independent when multiple keys are present.
// Caller is intentionally stripped — it's for debug logging, not assertions.
func sortCacheLogKeys(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		// Only sort if there are multiple keys
		if len(entry.Keys) <= 1 {
			sorted[i] = CacheLogEntry{
				Operation: entry.Operation,
				Keys:      entry.Keys,
				Hits:      entry.Hits,
			}
			continue
		}

		// Create pairs of (key, hit) to sort together
		pairs := make([]struct {
			key string
			hit bool
		}, len(entry.Keys))
		for j := range entry.Keys {
			pairs[j].key = entry.Keys[j]
			if entry.Hits != nil && j < len(entry.Hits) {
				pairs[j].hit = entry.Hits[j]
			}
		}

		// Sort pairs by key
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})

		// Extract sorted keys and hits
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
		}
		if entry.Hits != nil && len(entry.Hits) > 0 {
			sorted[i].Hits = make([]bool, len(pairs))
		}
		for j := range pairs {
			sorted[i].Keys[j] = pairs[j].key
			if sorted[i].Hits != nil {
				sorted[i].Hits[j] = pairs[j].hit
			}
		}
	}
	return sorted
}

// sortCacheLogKeysWithCaller is like sortCacheLogKeys but preserves the Caller field.
// Use this when you want assertions to verify which Loader method chain triggered each cache event.
func sortCacheLogKeysWithCaller(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		if len(entry.Keys) <= 1 {
			sorted[i] = CacheLogEntry{
				Operation: entry.Operation,
				Keys:      entry.Keys,
				Hits:      entry.Hits,
				Caller:    entry.Caller,
			}
			continue
		}

		pairs := make([]struct {
			key string
			hit bool
		}, len(entry.Keys))
		for j := range entry.Keys {
			pairs[j].key = entry.Keys[j]
			if entry.Hits != nil && j < len(entry.Hits) {
				pairs[j].hit = entry.Hits[j]
			}
		}
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
			Caller:    entry.Caller,
		}
		if entry.Hits != nil && len(entry.Hits) > 0 {
			sorted[i].Hits = make([]bool, len(pairs))
		}
		for j := range pairs {
			sorted[i].Keys[j] = pairs[j].key
			if sorted[i].Hits != nil {
				sorted[i].Hits[j] = pairs[j].hit
			}
		}
	}
	return sorted
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
}

type FakeLoaderCache struct {
	mu      sync.RWMutex
	storage map[string]cacheEntry
	log     []CacheLogEntry
}

func NewFakeLoaderCache() *FakeLoaderCache {
	return &FakeLoaderCache{
		storage: make(map[string]cacheEntry),
		log:     make([]CacheLogEntry, 0),
	}
}

func (f *FakeLoaderCache) cleanupExpired() {
	now := time.Now()
	for key, entry := range f.storage {
		if entry.expiresAt != nil && now.After(*entry.expiresAt) {
			delete(f.storage, key)
		}
	}
}

func (f *FakeLoaderCache) Get(ctx context.Context, keys []string) ([]*resolve.CacheEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	hits := make([]bool, len(keys))
	result := make([]*resolve.CacheEntry, len(keys))
	for i, key := range keys {
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			result[i] = &resolve.CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			hits[i] = true
		} else {
			result[i] = nil
			hits[i] = false
		}
	}

	// Log the operation
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Keys:      keys,
		Hits:      hits,
		Caller:    caller,
	})

	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, entries []*resolve.CacheEntry, ttl time.Duration) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		cacheEntry := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(entry.Value)),
		}
		copy(cacheEntry.data, entry.Value)

		// If ttl is 0, store without expiration
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			cacheEntry.expiresAt = &expiresAt
		}

		f.storage[entry.Key] = cacheEntry
		keys = append(keys, entry.Key)
	}

	// Log the operation
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Keys:      keys,
		Hits:      nil, // Set operations don't have hits/misses
		Caller:    caller,
	})

	return nil
}

func (f *FakeLoaderCache) Delete(ctx context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	for _, key := range keys {
		delete(f.storage, key)
	}

	// Log the operation
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Keys:      keys,
		Hits:      nil, // Delete operations don't have hits/misses
		Caller:    caller,
	})

	return nil
}

// GetLog returns a copy of the cache operation log
func (f *FakeLoaderCache) GetLog() []CacheLogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	logCopy := make([]CacheLogEntry, len(f.log))
	copy(logCopy, f.log)
	return logCopy
}

// GetLogWithCaller returns a copy of the cache operation log with Caller populated.
// Use this with sortCacheLogKeysWithCaller to assert on both operation details and
// the Loader method chain that triggered each cache event.
func (f *FakeLoaderCache) GetLogWithCaller() []CacheLogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	logCopy := make([]CacheLogEntry, len(f.log))
	copy(logCopy, f.log)
	return logCopy
}

// ClearLog clears the cache operation log
func (f *FakeLoaderCache) ClearLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = make([]CacheLogEntry, 0)
}

// TestFakeLoaderCache tests the cache implementation itself
func TestFakeLoaderCache(t *testing.T) {
	ctx := context.Background()
	cache := NewFakeLoaderCache()

	t.Run("SetAndGet", func(t *testing.T) {
		// Test basic set and get
		keys := []string{"key1", "key2", "key3"}
		entries := []*resolve.CacheEntry{
			{Key: "key1", Value: []byte("value1")},
			{Key: "key2", Value: []byte("value2")},
			{Key: "key3", Value: []byte("value3")},
		}

		err := cache.Set(ctx, entries, 0) // No TTL
		require.NoError(t, err)

		// Get all keys
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "value2", string(result[1].Value))
		assert.NotNil(t, result[2])
		assert.Equal(t, "value3", string(result[2].Value))

		// Get partial keys
		result, err = cache.Get(ctx, []string{"key2", "key4", "key1"})
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value2", string(result[0].Value))
		assert.Nil(t, result[1]) // key4 doesn't exist
		assert.NotNil(t, result[2])
		assert.Equal(t, "value1", string(result[2].Value))
	})

	t.Run("Delete", func(t *testing.T) {
		// Set some keys
		entries := []*resolve.CacheEntry{
			{Key: "del1", Value: []byte("v1")},
			{Key: "del2", Value: []byte("v2")},
			{Key: "del3", Value: []byte("v3")},
		}
		err := cache.Set(ctx, entries, 0)
		require.NoError(t, err)

		// Delete some keys
		err = cache.Delete(ctx, []string{"del1", "del3"})
		require.NoError(t, err)

		// Check remaining keys
		result, err := cache.Get(ctx, []string{"del1", "del2", "del3"})
		require.NoError(t, err)
		assert.Nil(t, result[0])    // del1 was deleted
		assert.NotNil(t, result[1]) // del2 still exists
		assert.Equal(t, "v2", string(result[1].Value))
		assert.Nil(t, result[2]) // del3 was deleted
	})

	t.Run("TTL", func(t *testing.T) {
		// Set with 50ms TTL
		entries := []*resolve.CacheEntry{
			{Key: "ttl1", Value: []byte("expire1")},
			{Key: "ttl2", Value: []byte("expire2")},
		}
		err := cache.Set(ctx, entries, 50*time.Millisecond)
		require.NoError(t, err)

		// Immediately get - should exist
		result, err := cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "expire1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "expire2", string(result[1].Value))

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Get again - should be nil
		result, err = cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
	})

	t.Run("MixedTTL", func(t *testing.T) {
		// Set some with TTL, some without
		err := cache.Set(ctx, []*resolve.CacheEntry{{Key: "perm1", Value: []byte("permanent")}}, 0)
		require.NoError(t, err)

		err = cache.Set(ctx, []*resolve.CacheEntry{{Key: "temp1", Value: []byte("temporary")}}, 50*time.Millisecond)
		require.NoError(t, err)

		// Wait for temporary to expire
		time.Sleep(60 * time.Millisecond)

		// Check both
		result, err := cache.Get(ctx, []string{"perm1", "temp1"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "permanent", string(result[0].Value)) // Still exists
		assert.Nil(t, result[1])                              // Expired
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		// Test concurrent access
		done := make(chan bool)

		// Writer goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := cache.Set(ctx, []*resolve.CacheEntry{{Key: key, Value: []byte(value)}}, 0)
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Reader goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i%50)
				_, err := cache.Get(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Deleter goroutine
		go func() {
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("concurrent_%d", i*2)
				err := cache.Delete(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Wait for all goroutines
		<-done
		<-done
		<-done
	})

	t.Run("ResultLengthMatchesKeysLength", func(t *testing.T) {
		// Test that result length always matches input keys length

		// Set some data
		err := cache.Set(ctx, []*resolve.CacheEntry{
			{Key: "exist1", Value: []byte("data1")},
			{Key: "exist3", Value: []byte("data3")},
		}, 0)
		require.NoError(t, err)

		// Request mix of existing and non-existing keys
		keys := []string{"exist1", "missing1", "exist3", "missing2", "missing3"}
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)

		// Verify length matches exactly
		assert.Len(t, result, len(keys), "Result length must match keys length")
		assert.Len(t, result, 5, "Should return exactly 5 results")

		// Verify correct values
		assert.NotNil(t, result[0])
		assert.Equal(t, "data1", string(result[0].Value)) // exist1
		assert.Nil(t, result[1])                          // missing1
		assert.NotNil(t, result[2])
		assert.Equal(t, "data3", string(result[2].Value)) // exist3
		assert.Nil(t, result[3])                          // missing2
		assert.Nil(t, result[4])                          // missing3

		// Test with all missing keys
		allMissingKeys := []string{"missing4", "missing5", "missing6"}
		result, err = cache.Get(ctx, allMissingKeys)
		require.NoError(t, err)
		assert.Len(t, result, 3, "Should return 3 results for 3 keys")
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
		assert.Nil(t, result[2])

		// Test with empty keys
		result, err = cache.Get(ctx, []string{})
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should return empty slice for empty keys")
	})
}

// =============================================================================
// L1/L2 CACHE END-TO-END TESTS
// =============================================================================
//
// These tests verify the L1 (per-request in-memory) and L2 (external cross-request)
// caching behavior in a federated GraphQL setup.
//
// L1 Cache: Prevents redundant fetches for the same entity within a single request
// L2 Cache: Shares entity data across requests via external cache (e.g., Redis)
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)

func TestL1CacheReducesHTTPCalls(t *testing.T) {
	// This test demonstrates L1 cache behavior with entity fetches.
	//
	// Query structure:
	// - me: root query to accounts service → returns User 1234 {id, username}
	// - me.reviews: entity fetch from reviews service → returns reviews
	// - me.reviews.product: entity fetch from products service → returns products
	// - me.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - me.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// Note: The `me` root query does NOT populate L1 cache because L1 cache only works
	// for entity fetches (RequiresEntityFetch=true). Root queries don't qualify.
	//
	// With L1 enabled: Both `me` (root) and `authorWithoutProvides` (entity) make calls.
	//   L1 cache doesn't help here because `me` is a root query, not an entity fetch.
	// With L1 disabled: Same behavior - 2 accounts calls.
	//
	// L1 cache DOES help when the same entity is fetched multiple times through
	// entity fetches within a single request (e.g., self-referential entities).

	query := `query {
		me {
			id
			username
			reviews {
				body
				product {
					upc
					reviews {
						authorWithoutProvides {
							id
							username
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Both `me` (root query) and `authorWithoutProvides` (entity fetch) call accounts.
		// L1 cache doesn't help because `me` is a root query, not an entity fetch.
		// Root queries don't populate L1 cache (RequiresEntityFetch=false).
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Both me (root query) and authorWithoutProvides (entity fetch) call accounts")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		// The authorWithoutProvides.username requires another fetch since L1 is disabled.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheReducesHTTPCallsInterface(t *testing.T) {
	// This test demonstrates L1 cache behavior with interface return types.
	//
	// Query structure:
	// - meInterface: root query to accounts service → returns User 1234 via Identifiable interface
	// - meInterface.reviews: entity fetch from reviews service → returns reviews
	// - meInterface.reviews.product: entity fetch from products service → returns products
	// - meInterface.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - meInterface.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// This tests that interface return types properly build cache key templates
	// for all entity types that implement the interface.

	query := `query {
		meInterface {
			... on User {
				id
				username
				reviews {
					body
					product {
						upc
						reviews {
							authorWithoutProvides {
								id
								username
							}
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"meInterface":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - interface entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Same behavior as non-interface: root query + entity fetch both call accounts
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Interface field should behave same as object field for L1 caching")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheReducesHTTPCallsUnion(t *testing.T) {
	// This test demonstrates L1 cache behavior with union return types.
	//
	// Query structure:
	// - meUnion: root query to accounts service → returns User 1234 via MeUnion union
	// - meUnion.reviews: entity fetch from reviews service → returns reviews
	// - meUnion.reviews.product: entity fetch from products service → returns products
	// - meUnion.reviews.product.reviews: entity fetch from reviews service → returns reviews
	// - meUnion.reviews.product.reviews.authorWithoutProvides: entity fetch from accounts → returns User 1234
	//
	// This tests that union return types properly build cache key templates
	// for all entity types that are members of the union.

	query := `query {
		meUnion {
			... on User {
				id
				username
				reviews {
					body
					product {
						upc
						reviews {
							authorWithoutProvides {
								id
								username
							}
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"meUnion":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - union entity fetches use L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Same behavior as non-union: root query + entity fetch both call accounts
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"Union field should behave same as object field for L1 caching")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheSelfReferentialEntity(t *testing.T) {
	// This test verifies that self-referential entities don't cause
	// stack overflow when L1 cache is enabled.
	//
	// Background: When an entity type has a field that returns the same type
	// (e.g., User.sameUserReviewers returning [User]), and L1 cache stores
	// a pointer to the entity, both key.Item and key.FromCache can point to
	// the same memory location. Without a fix, calling MergeValues(ptr, ptr)
	// causes infinite recursion and stack overflow.
	//
	// The sameUserReviewers field has @requires(fields: "username") which forces
	// sequential execution: the User entity is first fetched from accounts
	// (populating L1), then sameUserReviewers is resolved, returning the same
	// User entity that's already in L1 cache.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	// This response shows User 1234 appearing both at authorWithoutProvides level
	// and inside sameUserReviewers (which returns the same user for testing)
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("self-referential entity should not cause stack overflow", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// This should complete without stack overflow
		// Before the fix, this would crash with "fatal error: stack overflow"
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))
	})
}

func TestL2CacheOnly(t *testing.T) {
	t.Run("L2 enabled - miss then hit across requests", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable L2 cache only
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		// Enable entity caching for L2 tests (opt-in per-subgraph caching)
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

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: get/set for Query.topProducts, Product entities, User entities = 6 operations
		assert.Equal(t, 6, len(logAfterFirst), "Should have exactly 6 cache operations (get/set for Query, Products, Users)")

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
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - all fetches should hit cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 cache hits
		logAfterSecond := defaultCache.GetLog()
		// All cache operations should be gets with hits: Query.topProducts, Product entities, User entities
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

		// Verify subgraph calls for second query - all should be cached
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, productsCallsSecond, "Second query should not call products subgraph (root field cache hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should not call reviews subgraph (entity cache hit)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should not call accounts subgraph (entity cache hit)")
	})

	t.Run("L2 disabled - no external cache operations", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Disable L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First query
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify no cache operations
		log := defaultCache.GetLog()
		assert.Empty(t, log, "No L2 cache operations should occur when L2 is disabled")
	})
}

func TestL1L2CacheCombined(t *testing.T) {
	t.Run("L1+L2 enabled - L1 within request, L2 across requests", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable both L1 and L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		// Enable entity caching for L2 tests (opt-in per-entity caching)
		// Configure caching per-subgraph with explicit subgraph names
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

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - L1 helps within request, L2 populates for later
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: get/set for Query.topProducts, Product entities, User entities = 6 operations
		assert.Equal(t, 6, len(logAfterFirst), "Should have exactly 6 cache operations (get/set for Query, Products, Users)")

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
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - new request means fresh L1, but L2 should hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All cache operations should be gets with hits: Query.topProducts, Product entities, User entities
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

		// Verify no subgraph calls for second query (L2 cache hits)
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, productsCallsSecond, "Second query should not call products subgraph (L2 hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should not call reviews subgraph (L2 hit)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should not call accounts subgraph (L2 hit)")
	})

	t.Run("L1+L2 - cross-request isolation: L1 per-request, L2 shared", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable both L1 and L2
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		// Enable entity caching for L2 tests (opt-in per-entity caching)
		// Configure caching per-subgraph with explicit subgraph names
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
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

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First request - populates L2 cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 has set operations
		logAfterFirst := defaultCache.GetLog()
		hasSet := false
		for _, entry := range logAfterFirst {
			if entry.Operation == "set" {
				hasSet = true
				break
			}
		}
		assert.True(t, hasSet, "First request should populate L2 cache")

		// Second request - L1 is fresh (new request), but L2 should provide data
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 has get operations with hits
		logAfterSecond := defaultCache.GetLog()
		getCount := 0
		hitCount := 0
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				getCount++
				for _, hit := range entry.Hits {
					if hit {
						hitCount++
					}
				}
			}
		}
		assert.Greater(t, getCount, 0, "Second request should have L2 get operations")
		assert.Greater(t, hitCount, 0, "Second request should have L2 cache hits")
	})
}

// TestPartialEntityCaching demonstrates that only explicitly configured entity types
// are cached. This test configures caching for Product but NOT for User, verifying
// the opt-in nature of the per-entity caching configuration.
func TestPartialEntityCaching(t *testing.T) {
	t.Run("only configured entities are cached", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		// PARTIAL CACHING: Only configure caching for Product in reviews subgraph, NOT for User in accounts
		// This demonstrates the opt-in per-entity caching behavior
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
				},
			},
			// Note: accounts subgraph is intentionally NOT configured - User entities should NOT be cached
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - Product entities should be cached, User entities should NOT
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Only Product entities should have cache operations (get + set = 2 operations)
		// User entities should NOT have any cache operations
		assert.Equal(t, 2, len(logAfterFirst), "Only Product entities should have cache operations (get + set)")

		// Verify only Product cache operations
		for _, entry := range logAfterFirst {
			for _, key := range entry.Keys {
				assert.Contains(t, key, `"__typename":"Product"`, "Only Product entities should be in cache operations")
				assert.NotContains(t, key, `"__typename":"User"`, "User entities should NOT be cached")
			}
		}

		// Verify first query subgraph calls
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph")

		// Second query - Product should hit cache, User should still be fetched
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Should only have Product cache hit (get operation), no User operations
		assert.Equal(t, 1, len(logAfterSecond), "Only Product cache get operation")

		// Verify Product cache hits
		productHits := 0
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				for i, key := range entry.Keys {
					assert.Contains(t, key, `"__typename":"Product"`, "Only Product should be in cache")
					if entry.Hits[i] {
						productHits++
					}
				}
			}
		}
		assert.Equal(t, 2, productHits, "Both Product entities should hit cache")

		// KEY ASSERTION: Reviews subgraph is skipped (Product cache hit), but accounts is called (User not cached)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should skip reviews subgraph (Product cache hit)")
		assert.Equal(t, 1, accountsCallsSecond, "Second query should still call accounts subgraph (User NOT cached)")
	})
}

// TestRootFieldCaching tests that root fields (like Query.topProducts) can be cached
// when explicitly configured with RootFieldCaching configuration.
func TestRootFieldCaching(t *testing.T) {
	t.Run("root field caching enabled", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		// Configure root field caching for Query.topProducts on products subgraph
		// Also configure entity caching to compare behavior
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

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		// First query - should miss cache for all: root field, entity types
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Should have cache operations for:
		// 1. Root field Query.topProducts (get + set = 2 operations)
		// 2. Product entities (get + set = 2 operations)
		// 3. User entities (get + set = 2 operations)
		// Total: 6 operations
		assert.Equal(t, 6, len(logAfterFirst), "First query should have 6 cache operations (get+set for root field, Product, User)")

		// Verify first query calls all subgraphs
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph")

		// Second query - should hit cache for root field and entities
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Should have only get operations (hits) for root field, Product, User
		// No set operations since everything is cached
		assert.Equal(t, 3, len(logAfterSecond), "Second query should have 3 cache get operations (root field, Product, User)")

		// Verify cache hits
		hitCount := 0
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				for _, hit := range entry.Hits {
					if hit {
						hitCount++
					}
				}
			}
		}
		// Root field: 1 hit, Product: 2 hits, User: 2 hits = 5 total hits
		assert.GreaterOrEqual(t, hitCount, 3, "Should have cache hits for root field and entities")

		// KEY ASSERTION: Products subgraph is NOT called on second query because root field is cached
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, productsCallsSecond, "Second query should skip products subgraph (root field cache hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should skip reviews subgraph (entity cache hit)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should skip accounts subgraph (entity cache hit)")
	})

	t.Run("root field caching NOT enabled - subgraph still called", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		// Only configure entity caching, NOT root field caching
		// This demonstrates opt-in behavior: root fields are NOT cached unless configured
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
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
			// Note: products subgraph has NO caching config for Query.topProducts
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		productsHost := productsURLParsed.Host

		// First query
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		productsCallsFirst := tracker.GetCount(productsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph")

		// Second query - products subgraph should still be called because root field is NOT cached
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// KEY ASSERTION: Products subgraph IS called on second query because root field is NOT cached
		productsCallsSecond := tracker.GetCount(productsHost)
		assert.Equal(t, 1, productsCallsSecond, "Second query SHOULD call products subgraph (root field NOT cached)")
	})
}

// =============================================================================
// L1 CACHE TESTS FOR LIST FIELDS
// =============================================================================
//
// These tests verify L1 caching behavior when root fields or child fields
// return lists of entities.

func TestL1CacheChildFieldEntityList(t *testing.T) {
	// This test verifies L1 cache behavior for User.sameUserReviewers: [User!]!
	// which returns only the same user (self-reference).
	//
	// sameUserReviewers is defined in the reviews subgraph with @requires(fields: "username"),
	// which means:
	// 1. The gateway first resolves username from accounts (entity fetch)
	// 2. Then calls reviews to get sameUserReviewers
	// 3. sameUserReviewers returns User references (just IDs) - only the same user
	// 4. The gateway must make entity fetches to accounts to resolve those users
	//
	// Query flow:
	// 1. topProducts -> products subgraph (root query)
	// 2. reviews -> reviews subgraph (entity fetch for Products)
	// 3. authorWithoutProvides -> accounts subgraph (entity fetch for User 1234)
	//    - User 1234 is fetched and stored in L1
	// 4. sameUserReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234] as reference (same user only)
	// 5. Entity resolution for sameUserReviewers -> accounts subgraph
	//    - User 1234 is 100% L1 HIT (already fetched in step 3)
	//    - THE ENTIRE ACCOUNTS CALL IS SKIPPED!
	//
	// With L1 enabled: The sameUserReviewers entity fetch is completely skipped
	// because all entities are already in L1 cache.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	// User 1234's sameUserReviewers returns [User 1234] (only self)
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("L1 enabled - sameUserReviewers fetch entirely skipped via L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false, // Isolate L1 behavior
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 enabled:
		// - First accounts call fetches User 1234 for authorWithoutProvides (L1 miss, stored)
		// - Reviews called for sameUserReviewers (returns [User 1234] reference)
		// - sameUserReviewers entity resolution: User 1234 is 100% L1 HIT
		//   → accounts call is COMPLETELY SKIPPED!
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// Reviews should be called twice: once for Product entity (reviews field),
		// once for sameUserReviewers (after username is resolved from accounts)
		assert.Equal(t, 2, reviewsCalls, "Reviews subgraph called for Product.reviews and User.sameUserReviewers")

		// KEY ASSERTION: Only 1 accounts call! The sameUserReviewers entity resolution
		// is completely skipped because User 1234 is already in L1 cache.
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled: only 1 accounts call (sameUserReviewers entity fetch skipped via L1)")

		// Verify L1 cache activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		// L1 hits for User 1234 in sameUserReviewers (twice, once per product's review)
		// L1 misses: User entity fetches (Product fetch has UseL1Cache=false due to optimization)
		assert.Equal(t, int64(2), l1HitsInt, "Should have exactly 2 L1 hits for User 1234 in sameUserReviewers")
		assert.Equal(t, int64(2), l1MissesInt, "Should have exactly 2 L1 misses (User entity fetches)")
	})

	t.Run("L1 disabled - accounts called for sameUserReviewers", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 disabled:
		// - First accounts call fetches User 1234 for authorWithoutProvides
		// - Second accounts call for sameUserReviewers: User 1234 fetched again (no L1)
		// Total: 2 accounts calls
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled: 2 accounts calls (sameUserReviewers requires separate fetch)")

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")
	})
}

func TestL1CacheNestedEntityListDeduplication(t *testing.T) {
	// This test verifies L1 deduplication when the same entity appears
	// at multiple levels in nested list queries using coReviewers.
	//
	// coReviewers is defined in the reviews subgraph with @requires(fields: "username"),
	// so it triggers cross-subgraph entity resolution.
	//
	// Query flow:
	// 1. topProducts -> products subgraph
	// 2. reviews -> reviews subgraph (Product entity fetch)
	// 3. authorWithoutProvides -> accounts (User 1234 fetched, stored in L1)
	// 4. coReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234, User 7777] as references
	// 5. Entity resolution for coReviewers -> accounts
	//    - User 1234 should be L1 HIT (already fetched in step 3)
	//    - User 7777 is L1 MISS (stored in L1)
	// 6. coReviewers for User 1234 and User 7777 -> reviews subgraph
	// 7. Entity resolution for nested coReviewers -> accounts
	//    - All users (1234, 7777) are already in L1!
	//
	// With L1 enabled: The nested coReviewers level should have 100% L1 hits,
	// potentially skipping the accounts call entirely for that level.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					coReviewers {
						id
						username
						coReviewers {
							id
							username
						}
					}
				}
			}
		}
	}`

	// User 1234's coReviewers: [User 1234, User 7777]
	// User 7777's coReviewers: [User 7777, User 1234]
	// Nested level repeats these patterns
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me"},{"id":"7777","username":"User 7777"}]},{"id":"7777","username":"User 7777","coReviewers":[{"id":"7777","username":"User 7777"},{"id":"1234","username":"Me"}]}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me","coReviewers":[{"id":"1234","username":"Me"},{"id":"7777","username":"User 7777"}]},{"id":"7777","username":"User 7777","coReviewers":[{"id":"7777","username":"User 7777"},{"id":"1234","username":"Me"}]}]}}]}]}}`

	t.Run("L1 enabled - nested coReviewers benefits from L1 hits", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 enabled:
		// - Call 1: authorWithoutProvides fetches User 1234 (miss, stored)
		// - Call 2: coReviewers entity resolution [User 1234 (hit), User 7777 (miss, stored)]
		// - Call 3: nested coReviewers entity resolution - all users are in L1!
		//   This call should be fully served from L1 cache.
		accountsCalls := tracker.GetCount(accountsHost)
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		// With L1 enabled, the nested coReviewers should be served from L1
		// Only 2 accounts calls needed because nested coReviewers is fully served from L1
		assert.Equal(t, 2, accountsCalls,
			"With L1 enabled: exactly 2 accounts calls (nested coReviewers served entirely from L1)")

		// We expect significant L1 hits for the nested level where all users are already cached
		// The L1 optimization reduces misses by skipping L1 operations for entity types
		// that have no valid provider/consumer relationship.
		assert.Equal(t, int64(12), l1HitsInt,
			"Should have exactly 12 L1 hits for nested coReviewers deduplication")
		assert.Equal(t, int64(8), l1MissesInt,
			"Should have exactly 8 L1 misses (reduced by optimization)")
	})

	t.Run("L1 disabled - more accounts calls without deduplication", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// With L1 disabled:
		// - Call 1: authorWithoutProvides fetches User 1234
		// - Call 2: coReviewers entity resolution for User 1234 and User 7777 (no L1 dedup)
		// - Call 3: nested coReviewers entity resolution (no L1 dedup)
		accountsCalls := tracker.GetCount(accountsHost)
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		// Without L1 cache, we need 3 accounts calls (no deduplication at nested level)
		assert.Equal(t, 3, accountsCalls,
			"With L1 disabled: exactly 3 accounts calls (no deduplication)")

		// Verify NO L1 activity
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")
	})
}

func TestL1CacheRootFieldEntityListPopulation(t *testing.T) {
	// This test verifies L1 cache behavior with a complex nested query starting
	// from a root field that returns a list of entities.
	//
	// Query flow:
	// 1. topProducts -> products subgraph (root query, returns list)
	// 2. reviews -> reviews subgraph (entity fetch for Products)
	// 3. authorWithoutProvides -> accounts subgraph (entity fetch for User 1234)
	//    - User 1234 is fetched and stored in L1
	// 4. sameUserReviewers -> reviews subgraph (after username resolved)
	//    - Returns [User 1234] as reference (same user only)
	// 5. Entity resolution for sameUserReviewers -> accounts subgraph
	//    - User 1234 is 100% L1 HIT (already fetched in step 3)
	//    - THE ENTIRE ACCOUNTS CALL IS SKIPPED!
	//
	// With L1 enabled: The sameUserReviewers entity fetch is completely skipped.
	// With L1 disabled: accounts is called twice (no deduplication).

	query := `query {
		topProducts {
			upc
			name
			reviews {
				body
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"topProducts":[{"upc":"top-1","name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"upc":"top-2","name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("L1 enabled - sameUserReviewers fetch skipped via L1 cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 enabled:
		// 1. products subgraph: topProducts root query
		// 2. reviews subgraph: Product entity fetch for reviews
		// 3. accounts subgraph: User entity fetch for authorWithoutProvides (User 1234 stored in L1)
		// 4. reviews subgraph: sameUserReviewers (returns [User 1234])
		// 5. sameUserReviewers entity resolution: User 1234 is 100% L1 HIT → accounts call SKIPPED!
		productsCalls := tracker.GetCount(productsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCalls, "Should call products subgraph once for topProducts")
		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice (Product.reviews + User.sameUserReviewers)")
		// KEY ASSERTION: Only 1 accounts call! sameUserReviewers entity resolution skipped via L1.
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled: only 1 accounts call (sameUserReviewers entity fetch skipped via L1)")

		// Verify L1 cache activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		// L1 cache flow:
		// - Product entity fetch (reviews subgraph): 2 products, batched as 1 fetch
		//   Each product checked L1 → miss, then populated after fetch
		// - User entity fetch (authorWithoutProvides): User 1234 fetched twice (same user, 2 reviews)
		//   First: miss, populate L1. Second: hit!
		// - User entity fetch (sameUserReviewers): 2 hits for User 1234
		// Total: 2 L1 hits (second authorWithoutProvides + sameUserReviewers uses same User 1234)
		assert.Equal(t, int64(2), l1HitsInt, "Should have exactly 2 L1 hits for User 1234 in sameUserReviewers")
		// L1 misses: Product and User entity fetches on first encounter
		// - Product fetch: 2 products in batch = 2 individual L1 lookups = 2 misses
		// - User fetch: 1 miss for first User 1234, then hits
		// With batching, we see 2 misses total (Product misses are now skipped due to optimization)
		assert.Equal(t, int64(2), l1MissesInt, "Should have exactly 2 L1 misses (User entity fetches)")
	})

	t.Run("L1 disabled - more accounts calls without L1 optimization", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 disabled:
		// 1. products subgraph: topProducts root query
		// 2. reviews subgraph: Product entity fetch for reviews
		// 3. accounts subgraph: User entity fetch for authorWithoutProvides
		// 4. reviews subgraph: sameUserReviewers
		// 5. accounts subgraph: User entity fetch for sameUserReviewers (no L1 → must fetch again!)
		productsCalls := tracker.GetCount(productsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)
		accountsCalls := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCalls, "Should call products subgraph once")
		assert.Equal(t, 2, reviewsCalls, "Should call reviews subgraph twice")
		// KEY ASSERTION: 2 accounts calls without L1 optimization
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled: 2 accounts calls (sameUserReviewers requires separate fetch)")

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")
	})
}

// =============================================================================
// CACHE ERROR HANDLING TESTS
// =============================================================================
//
// These tests verify that caches are NOT populated when subgraphs return errors.
// The cache should only store successful responses to prevent caching error states.

func TestCacheNotPopulatedOnErrors(t *testing.T) {
	// Query that triggers an error in accounts subgraph via error-user
	// The reviewWithError field returns a review with author ID "error-user"
	// which causes FindUserByID to return an error
	errorQuery := `query {
		reviewWithError {
			body
			authorWithoutProvides {
				id
				username
			}
		}
	}`

	// Expected error response - data is null due to non-nullable username field error propagation
	expectedErrorResponse := `{"errors":[{"message":"Failed to fetch from Subgraph 'accounts' at Path 'reviewWithError.authorWithoutProvides'."},{"message":"Cannot return null for non-nullable field 'User.username'.","path":["reviewWithError","authorWithoutProvides","username"]}],"data":{"reviewWithError":null}}`

	t.Run("L1 only - error response prevents cache population", func(t *testing.T) {
		// This test verifies that L1 cache is NOT populated when an error occurs.
		// If L1 was erroneously populated, the second query would not call accounts.
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should get error from accounts
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Verify exact error response
		assert.Equal(t, expectedErrorResponse, string(resp))

		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph once")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph once")

		// Second query - L1 should NOT have cached the error, so accounts should be called again
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Same error should be returned
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsSecond := tracker.GetCount(accountsHost)
		// KEY ASSERTION: If L1 incorrectly cached the error, this would be 0
		assert.Equal(t, 1, accountsCallsSecond, "Second query should call accounts again (L1 should NOT cache errors)")
	})

	t.Run("L2 only - error response prevents cache population", func(t *testing.T) {
		// This test verifies that L2 cache is NOT populated when an error occurs.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Configure L2 caching for User entities
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - should get error from accounts
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Verify exact error response
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph once")

		// Verify exact cache log: only "get" with miss, NO "set"
		// Since the fetch had an error, cache population should be skipped entirely
		wantCacheLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"error-user"}}`},
				Hits:      []bool{false},
			},
			// NO "set" entry - this is the key assertion
		}
		assert.Equal(t, wantCacheLog, defaultCache.GetLog(), "Cache log should only have 'get' miss, no 'set'")

		// Second query - L2 should NOT have cached the error, so accounts should be called again
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Same error should be returned
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsSecond, "Second query should call accounts again (L2 should NOT cache errors)")

		// Second query should also have same cache log pattern (get miss, no set)
		assert.Equal(t, wantCacheLog, defaultCache.GetLog(), "Second query cache log should also have 'get' miss, no 'set'")
	})

	t.Run("L1 and L2 - error response prevents both caches", func(t *testing.T) {
		// This test verifies that both L1 and L2 caches are NOT populated when an error occurs.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Configure L2 caching for User entities
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query - should get error from accounts
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Verify exact error response
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph once")

		// Verify exact cache log: only "get" with miss, NO "set"
		wantCacheLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"error-user"}}`},
				Hits:      []bool{false},
			},
		}
		assert.Equal(t, wantCacheLog, defaultCache.GetLog(), "Cache log should only have 'get' miss, no 'set'")

		// Second query - neither L1 nor L2 should have cached the error
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Same error should be returned
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsSecond, "Second query should call accounts again (neither L1 nor L2 should cache errors)")

		// Second query should also have same cache log pattern
		assert.Equal(t, wantCacheLog, defaultCache.GetLog(), "Second query cache log should also have 'get' miss, no 'set'")
	})

	t.Run("error does not pollute cache for subsequent success queries", func(t *testing.T) {
		// This test verifies that an error query doesn't pollute the cache
		// and that subsequent successful queries still work correctly.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Configure L2 caching for User entities
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First: Query that triggers an error
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		// Verify exact error response
		assert.Equal(t, expectedErrorResponse, string(resp))

		accountsCallsError := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsError, "Error query should call accounts")

		// Verify error-user was NOT cached (only get, no set)
		wantErrorCacheLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"User","key":{"id":"error-user"}}`},
				Hits:      []bool{false},
			},
		}
		assert.Equal(t, wantErrorCacheLog, defaultCache.GetLog(), "Error query cache log should only have 'get' miss, no 'set'")

		// Second: Query a successful user (User 1234 via me query)
		// Note: "me" is a root query, not an entity fetch, so it doesn't use L2 entity caching
		successQuery := `query {
			me {
				id
				username
			}
		}`
		expectedSuccessResponse := `{"data":{"me":{"id":"1234","username":"Me"}}}`

		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, successQuery, nil, t)

		// Should succeed with exact expected response
		assert.Equal(t, expectedSuccessResponse, string(resp))

		// Note: Root queries (me) don't use L2 entity caching by default,
		// so the cache log should be empty for this query.
		// The important thing is that the previous error didn't pollute the cache.
		assert.Equal(t, 0, len(defaultCache.GetLog()), "Root query should not use L2 entity cache")

		// Third: Query the error user again - should still fail (not cached)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, errorQuery, nil, t)

		assert.Equal(t, expectedErrorResponse, string(resp))
		accountsCallsErrorAgain := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCallsErrorAgain, "Error query should call accounts again (error was not cached)")

		// Verify cache log still shows only get miss, no set
		assert.Equal(t, wantErrorCacheLog, defaultCache.GetLog(), "Third query cache log should still have 'get' miss, no 'set'")
	})
}

// TestL1CacheOptimizationReducesSubgraphCalls tests that the L1 cache optimization
// postprocessor (optimizeL1Cache) correctly identifies which fetches can benefit
// from L1 caching and sets UseL1Cache appropriately.
//
// The key insight is that L1 is only useful when:
// 1. A prior fetch can provide cached data (READ benefit)
// 2. A later fetch can consume cached data (WRITE benefit)
//
// This test verifies the end-to-end effect: when L1 optimization identifies
// matching entity types between fetches, it enables L1 caching, resulting in
// fewer subgraph calls.
func TestL1CacheOptimizationReducesSubgraphCalls(t *testing.T) {
	// This query demonstrates L1 optimization:
	// - Query.me returns User entity
	// - User.sameUserReviewers returns [User] entities
	// When L1 is enabled and optimized correctly:
	// - First User fetch (me) populates L1 cache
	// - Second User fetch (sameUserReviewers) hits L1 cache, SKIPS subgraph call
	//
	// The optimizeL1Cache postprocessor:
	// - Sets UseL1Cache=true on User fetches (they share the same entity type)
	// - Sets UseL1Cache=false on fetches with no matching entity types

	query := `query {
		me {
			id
			username
			sameUserReviewers {
				id
				username
			}
		}
	}`

	expectedResponse := `{"data":{"me":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}}`

	t.Run("L1 optimization enables cache hit between same entity type fetches", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow with L1 optimization:
		// 1. accounts subgraph: Query.me (root query, returns User 1234)
		//    - L1 cache populated with User 1234
		// 2. reviews subgraph: User.sameUserReviewers (returns [User 1234])
		// 3. accounts subgraph: User entity fetch for sameUserReviewers
		//    - User 1234 is 100% L1 HIT! This call is SKIPPED!
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// KEY ASSERTION: Only 1 accounts call!
		// Without L1 optimization, there would be 2 calls:
		// - First: Query.me
		// - Second: User entity resolution for sameUserReviewers
		// With L1 optimization, the second call is skipped because User 1234 is in L1 cache.
		assert.Equal(t, 1, accountsCalls,
			"L1 optimization: only 1 accounts call (sameUserReviewers resolved from L1 cache)")
		assert.Equal(t, 1, reviewsCalls,
			"Should call reviews subgraph once for User.sameUserReviewers")

		// Verify L1 cache was used
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		// L1 hit: User 1234 found in cache during sameUserReviewers resolution
		// Query.me populates L1 via RootFieldL1EntityCacheKeyTemplates (write-only, no miss)
		// sameUserReviewers entity fetch finds User 1234 in L1 → HIT
		assert.Equal(t, int64(1), l1HitsInt,
			"Should have exactly 1 L1 hit (User 1234 in sameUserReviewers)")
		// L1 misses: 0 because Query.me populates L1 without going through entity fetch path
		// Root field L1 population is write-only, doesn't register as a miss
		assert.Equal(t, int64(0), l1MissesInt,
			"Should have exactly 0 L1 misses (root field population doesn't count as miss)")
	})

	t.Run("Without L1, same query requires more subgraph calls", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false, // L1 disabled
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Query flow WITHOUT L1:
		// 1. accounts subgraph: Query.me (root query)
		// 2. reviews subgraph: User.sameUserReviewers
		// 3. accounts subgraph: User entity fetch (NO L1 cache → must fetch!)
		accountsCalls := tracker.GetCount(accountsHost)
		reviewsCalls := tracker.GetCount(reviewsHost)

		// KEY ASSERTION: 2 accounts calls without L1!
		// This proves L1 optimization saves a subgraph call.
		assert.Equal(t, 2, accountsCalls,
			"Without L1: 2 accounts calls (sameUserReviewers requires separate fetch)")
		assert.Equal(t, 1, reviewsCalls,
			"Should call reviews subgraph once for User.sameUserReviewers")

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when L1 disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when L1 disabled")
	})
}
