package engine_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFederationCaching_BasicMissThenHit verifies the fundamental L2 cache flow:
// first request misses cache and populates it, second request hits cache and skips subgraph calls.
func TestFederationCaching_BasicMissThenHit(t *testing.T) {
	t.Parallel()
	t.Run("two subgraphs - miss then hit", func(t *testing.T) {
		t.Parallel()
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
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
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
		// Cache operations: get+set for root field, Products, Users = 6 total
		assert.Equal(t, 6, len(logAfterFirst))

		// Verify the exact cache access log (order may vary for keys within each operation)
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			// Product entity fetches (reviews data for each product)
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			// User entity fetches (author data)
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))

		// Subgraph calls: each called once (cold cache)
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst)
		assert.Equal(t, 1, reviewsCallsFirst)
		assert.Equal(t, 1, accountsCallsFirst)

		// Second query - should hit cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All cache operations should be gets with hits: Query.topProducts, Product entities, User entities
		// With root field caching enabled, all 3 types should hit cache
		// All cache operations should be gets with hits
		assert.Equal(t, 3, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: true}}},
			// Product entity fetches - HITS
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
			}},
			// User entity fetches - HITS
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// Subgraph calls: all skipped (warm cache)
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, productsCallsSecond)
		assert.Equal(t, 0, reviewsCallsSecond)
		assert.Equal(t, 0, accountsCallsSecond)
	})

	t.Run("two subgraphs - partial fields then full fields", func(t *testing.T) {
		t.Parallel()
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
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
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
		// Root field caching: get miss + set = 2 operations
		assert.Equal(t, 2, len(logAfterFirst))

		// Verify the exact cache access log for first query
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))

		// Subgraph calls: only products called (name-only query)
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst)
		assert.Equal(t, 0, reviewsCallsFirst)
		assert.Equal(t, 0, accountsCallsFirst)

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
		// Root field hit + re-set, Products miss + set, Users miss + set = 6 operations
		assert.Equal(t, 6, len(logAfterSecond))

		// Verify the exact cache access log for second query
		// Note: Root field Query.topProducts is a HIT because cache key doesn't include selected fields
		// The first query already cached this root field, so the second query reuses it
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (same cache key, different selection doesn't matter)
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: true}}},
			// Still need to set because cache returns partial data that needs merging
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			// Product entity fetches - MISS (first time fetching these)
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			// User entity fetches - MISS (first time fetching these)
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// Subgraph calls: all called (new entity types needed)
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsSecond)
		assert.Equal(t, 1, reviewsCallsSecond)
		assert.Equal(t, 1, accountsCallsSecond)

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
		// All hits: 3 get operations
		assert.Equal(t, 3, len(logAfterThird))

		// Verify the exact cache access log for third query (all hits)
		wantLogThird := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: true}}},
			// Product entity fetches - HITS
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
			}},
			// User entity fetches - HITS
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogThird), sortCacheLogEntries(logAfterThird))

		// Subgraph calls: all skipped (warm cache)
		productsCallsThird := tracker.GetCount(productsHost)
		reviewsCallsThird := tracker.GetCount(reviewsHost)
		accountsCallsThird := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, productsCallsThird)
		assert.Equal(t, 0, reviewsCallsThird)
		assert.Equal(t, 0, accountsCallsThird)
	})

	t.Run("two subgraphs - with subgraph header prefix", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `11111:{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `11111:{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			{Operation: "get", Items: []CacheLogItem{
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			// User entity resolution from accounts (author.username requires entity fetch)
			{Operation: "get", Items: []CacheLogItem{{Key: `33333:{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `33333:{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLog), sortCacheLogEntries(logAfterFirst))

		// Verify subgraph calls for first query
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		// Subgraph calls: each called once (cold cache)
		assert.Equal(t, 1, productsCallsFirst)
		assert.Equal(t, 1, reviewsCallsFirst)
		assert.Equal(t, 1, accountsCallsFirst)

		// Second query - should hit cache with prefixed keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All hits: 3 get operations with prefixed keys
		assert.Equal(t, 3, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT with prefix
			{Operation: "get", Items: []CacheLogItem{{Key: `11111:{"__typename":"Query","field":"topProducts"}`, Hit: true}}},
			// Product entities - HIT with prefix
			{Operation: "get", Items: []CacheLogItem{
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
				{Key: `22222:{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
			}},
			// User entities - HIT with prefix
			{Operation: "get", Items: []CacheLogItem{{Key: `33333:{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// Verify subgraph calls for second query - all should be skipped due to cache hits
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		// Subgraph calls: all skipped (warm cache)
		assert.Equal(t, 0, productsCallsSecond)
		assert.Equal(t, 0, reviewsCallsSecond)
		assert.Equal(t, 0, accountsCallsSecond)
	})
}

// TestFederationCaching_MutationSkipsL2Read verifies that mutations never read from L2 cache
// (always fetch fresh data) and optionally populate L2 when EnableEntityL2CachePopulation is set.
func TestFederationCaching_MutationSkipsL2Read(t *testing.T) {
	t.Parallel()
	// Shared caching config: entity caching for User on accounts + opt-in L2 population for addReview on reviews.
	// Mutations do NOT populate L2 by default; subtests that expect L2 population need EnableEntityL2CachePopulation.
	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
		{
			SubgraphName: "accounts",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
		{
			SubgraphName: "reviews",
			MutationFieldCaching: plan.MutationFieldCacheConfigurations{
				{FieldName: "addReview", EnableEntityL2CachePopulation: true},
			},
		},
	}

	mutationVars := queryVariables{
		"authorID": "1234",
		"upc":      "top-1",
		"review":   "Great!",
	}

	t.Run("mutation skips L2 cache read and writes updated entity", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery1), sortCacheLogEntries(logAfterQuery1), "Step 1: cache log should show get miss then set for User")
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
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogMutation), sortCacheLogEntries(logAfterMutation), "Step 2: mutation should only set to L2, never get")
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery2), sortCacheLogEntries(logAfterQuery2), "Step 3: query should hit L2 cache for User")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 3: query should NOT call accounts subgraph (L2 cache hit)")
	})

	t.Run("mutation with no prior cache writes to L2 for subsequent query", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogMutation), sortCacheLogEntries(logAfterMutation), "Step 1: mutation should only set to L2")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Step 2: Query reads from L2 (hit from mutation's write)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterQuery), "Step 2: should have exactly 1 cache operation (get hit)")
		wantLogQuery := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery), sortCacheLogEntries(logAfterQuery), "Step 2: query should hit L2 cache for User")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 2: query should NOT call accounts subgraph (L2 cache hit)")
	})

	t.Run("consecutive mutations never read from L2 cache", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogMutation1), sortCacheLogEntries(logAfterMutation1), "Step 1: first mutation should only set to L2")
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
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogMutation2), sortCacheLogEntries(logAfterMutation2), "Step 2: second mutation should only set to L2, never get")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: should call accounts subgraph exactly once (not from cache)")
	})

	t.Run("query with different fields after mutation hits L2 cache", func(t *testing.T) {
		t.Parallel()
		// A mutation that triggers entity resolution for User populates L2 with the fields
		// the mutation selected. A subsequent query selecting a superset of fields gets a
		// PARTIAL hit on L2 (the cached key is present but missing some requested fields),
		// and the loader still fetches from accounts to fill the missing fields.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
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
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterMutation), "Step 1: should have exactly 1 cache operation (set only)")
		wantLogMutation := []CacheLogEntry{
			// updateL2Cache writes fresh User data after entity resolution (mutation skipped L2 read).
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogMutation), sortCacheLogEntries(logAfterMutation), "Step 1: mutation should only set to L2")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Analytics snapshot attributes the L2 write to the accounts subgraph / User entity
		// (this is the documented attribution channel; the old Caller field has been removed).
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Writes: []resolve.CacheWriteEvent{
				{
					CacheKey:   `{"__typename":"User","key":{"id":"1234"}}`,
					EntityType: "User",
					ByteSize:   49,
					DataSource: "accounts",
					CacheLevel: resolve.CacheLevelL2,
					TTL:        30 * time.Second,
					Source:     resolve.CacheSourceMutation, // Mutation-triggered L2 write after User entity resolution
				},
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`}, // addReview.authorWithoutProvides.username = "Me"
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation resolved 1 User entity
			},
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Step 2: Query requests different fields (username + nickname).
		// The query plan has two fetch nodes for the User cache key: one entity resolution for
		// `authorWithoutProvides` and one root fetch for `me`. The entity L2 read is a PARTIAL
		// hit (cached key present but missing `nickname`), and the `me` fetch to accounts
		// (called once) provides the full User data which `updateL2Cache` writes back.
		defaultCache.ClearLog()
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides_with_nickname.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}}]}}}`, string(resp))

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterQuery), "Step 2: should have exactly 2 cache operations (get hit + set)")
		wantLogQuery := []CacheLogEntry{
			// Entity resolution for authorWithoutProvides checks L2 → cache key present (FakeLoaderCache
			// only tracks key presence; the analytics layer classifies this as a PartialHit because the
			// cached entry is missing the `nickname` field).
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
			// A separate fetch to accounts (me root query) fetches User data and writes it to L2.
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery), sortCacheLogEntries(logAfterQuery), "Step 2: cache key is present (partial hit) plus writeback")
		// Accounts is called once for the me root query (not cached), but NOT for entity resolution (L2 hit)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: accounts called once for me root query, entity resolution served from L2 cache")

		// Analytics snapshot attributes both the L2 read (partial hit) and the L2 writeback to
		// accounts / User — this is the documented attribution channel replacing the old Caller field.
		// The L2 hit is a PARTIAL hit: the mutation's cache entry only contains `username`, but this
		// query also selects `nickname`, so the fetch still needs to go to accounts for the missing field.
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyPartialHit, DataSource: "accounts"}, // Cached entity has username but not nickname
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", ByteSize: 70, DataSource: "accounts", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second, Source: resolve.CacheSourceQuery}, // Writeback includes both username and nickname after the accounts fetch
			},
			FieldHashes: []resolve.EntityFieldHash{
				// Three nickname values (one per review's author) and three username values.
				{EntityType: "User", FieldName: "nickname", FieldHash: 10005559372589796850, KeyRaw: `{"id":"1234"}`},
				{EntityType: "User", FieldName: "nickname", FieldHash: 10005559372589796850, KeyRaw: `{"id":"1234"}`},
				{EntityType: "User", FieldName: "nickname", FieldHash: 10005559372589796850, KeyRaw: `{"id":"1234"}`},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 4, UniqueKeys: 2}, // me User + 3 authors
			},
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("mutation skips L2 write by default without EnableEntityL2CachePopulation", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Entity caching for accounts (User) only. No MutationFieldCaching config for reviews,
		// so addReview does NOT populate L2 (default behavior).
		noMutationPopulateConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(noMutationPopulateConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: Query populates L2 cache (flag does not affect queries).
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery1 := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterQuery1), "Step 1: should have exactly 2 cache operations (get miss + set)")
		wantLogQuery1 := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery1), sortCacheLogEntries(logAfterQuery1), "Step 1: query should miss then set")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph exactly once")

		// Step 2: Mutation produces zero cache operations (read skipped because mutation, write skipped because flag).
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("mutations/add_review_without_provides.query"), mutationVars, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"Great!","authorWithoutProvides":{"username":"Me"}}}}`, string(resp))

		logAfterMutation := defaultCache.GetLog()
		assert.Equal(t, 0, len(logAfterMutation), "Step 2: should have zero cache operations (no read AND no write)")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 2: should call accounts subgraph (not cached)")

		// Step 3: Query still hits L2 from step 1's write (mutation didn't overwrite it).
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/me_reviews_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}},{"body":"Great!","authorWithoutProvides":{"username":"Me"}}]}}}`, string(resp))

		logAfterQuery2 := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterQuery2), "Step 3: should have exactly 1 cache operation (get hit)")
		wantLogQuery2 := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogQuery2), sortCacheLogEntries(logAfterQuery2), "Step 3: query should hit L2 from step 1's write")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 3: should NOT call accounts subgraph (L2 cache hit)")
	})
}

// TestFederationCaching_PlanTimeTypeName verifies that entity cache keys use the type name
// from the query plan when __typename is missing from the subgraph response data.
// This tests the fallback path: a non-compliant subgraph omits __typename from its response,
// but the cache key should still use the correct entity type name (e.g. "Product")
// rather than a generic fallback.
func TestFederationCaching_PlanTimeTypeName(t *testing.T) {
	t.Parallel()
	defaultCache := NewFakeLoaderCache()

	// Transport that strips __typename from the products subgraph response.
	// This simulates a non-compliant subgraph that omits __typename from entity data.
	// The resolver should fall back to the plan-time entity type name for cache keys.
	strippingTransport := &typenameStrippingTransport{
		inner: http.DefaultTransport,
	}
	trackingClient := &http.Client{Transport: strippingTransport}

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)

	// Record the products URL so the transport knows which responses to strip
	productsURL, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	strippingTransport.targetHost = productsURL.Host

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	defaultCache.ClearLog()
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query { topProducts { name reviews { body } } }`, nil, t)

	// The query should still succeed — missing __typename doesn't crash resolution.
	// reviews is null because stripping __typename from the products response means
	// the planner cannot build an Entity representation to fetch reviews.
	assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":null},{"name":"Fedora","reviews":null}]}}`, string(resp))

	// Cache keys should use "Product" from the query plan, not "Entity".
	// Only entity caching for reviews/Product is configured, so we get a single L2 get
	// with both product cache keys using the plan-time type name as fallback.
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
			{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
		}},
	}), sortCacheLogEntries(defaultCache.GetLog()))
}
