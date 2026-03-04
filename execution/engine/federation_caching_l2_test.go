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

		logAfterFirst := defaultCache.GetLog()
		productKeys := []string{
			`{"__typename":"Product","key":{"upc":"top-1"}}`,
			`{"__typename":"Product","key":{"upc":"top-2"}}`,
		}
		userKeys := []string{
			`{"__typename":"User","key":{"id":"1234"}}`,
		}
		wantFirstLog := []CacheLogEntry{
			// reviews subgraph _entities(Product) — L2 miss, first time seeing these products
			{Operation: "get", Keys: productKeys, Hits: []bool{false, false}},
			// reviews subgraph _entities(Product) — store fetched product data in L2
			{Operation: "set", Keys: productKeys},
			// accounts subgraph _entities(User) — L2 miss, first time seeing this user
			{Operation: "get", Keys: userKeys, Hits: []bool{false}},
			// accounts subgraph _entities(User) — store fetched user data in L2
			{Operation: "set", Keys: userKeys},
		}
		assert.Equal(t, sortCacheLogKeys(wantFirstLog), sortCacheLogKeys(logAfterFirst), "First request: L2 miss + set for Product and User")

		// Second request - L1 is fresh (new request), but L2 should provide data
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		wantSecondLog := []CacheLogEntry{
			// reviews subgraph _entities(Product) — L2 hit, both products cached from first request
			{Operation: "get", Keys: productKeys, Hits: []bool{true, true}},
			// accounts subgraph _entities(User) — L2 hit, user cached from first request (deduplicated: 1 unique user)
			{Operation: "get", Keys: userKeys, Hits: []bool{true}},
			// No set operations — all data served from cache
		}
		assert.Equal(t, sortCacheLogKeys(wantSecondLog), sortCacheLogKeys(logAfterSecond), "Second request: all L2 cache hits, no sets")

		// No subgraph calls on second request — all entity data served from L2 cache
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		assert.Equal(t, 0, tracker.GetCount(reviewsURLParsed.Host), "Second request should skip reviews subgraph (Product L2 cache hit)")
		assert.Equal(t, 0, tracker.GetCount(accountsURLParsed.Host), "Second request should skip accounts subgraph (User L2 cache hit)")
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

		// Only Product has L2 caching configured (reviews subgraph); User (accounts) does NOT.
		// So we expect cache operations for Product only — no User cache activity at all.
		productKeys := []string{
			`{"__typename":"Product","key":{"upc":"top-1"}}`,
			`{"__typename":"Product","key":{"upc":"top-2"}}`,
		}
		logAfterFirst := defaultCache.GetLog()
		wantFirstLog := []CacheLogEntry{
			// reviews subgraph _entities(Product) — L2 miss, first time seeing these products
			{Operation: "get", Keys: productKeys, Hits: []bool{false, false}},
			// reviews subgraph _entities(Product) — store fetched product data in L2
			{Operation: "set", Keys: productKeys},
			// No User operations — accounts subgraph has no caching configured
		}
		assert.Equal(t, sortCacheLogKeys(wantFirstLog), sortCacheLogKeys(logAfterFirst), "First request: only Product entities have cache operations")

		// Both subgraphs called on first request (no cache to serve from)
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "First query should call reviews subgraph")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts subgraph")

		// Second query - Product should hit cache, User should still be fetched from subgraph
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		wantSecondLog := []CacheLogEntry{
			// reviews subgraph _entities(Product) — L2 hit, both products cached from first request
			{Operation: "get", Keys: productKeys, Hits: []bool{true, true}},
			// No User operations — accounts subgraph still has no caching configured
			// No set operations — Product data served from cache
		}
		assert.Equal(t, sortCacheLogKeys(wantSecondLog), sortCacheLogKeys(logAfterSecond), "Second request: Product cache hits only")

		// Reviews subgraph skipped (Product served from cache), accounts still called (User not cached)
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "Second query should skip reviews subgraph (Product cache hit)")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query should still call accounts subgraph (User NOT cached)")
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
		wantSecondLog := []CacheLogEntry{
			// products subgraph Query.topProducts — root field L2 hit, cached from first request
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// reviews subgraph _entities(Product) — L2 hit, both products cached from first request
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// accounts subgraph _entities(User) — L2 hit, user cached from first request (1 unique user)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			// No set operations — all data served from cache
		}
		assert.Equal(t, sortCacheLogKeys(wantSecondLog), sortCacheLogKeys(logAfterSecond), "Second query: all cache hits, no sets")

		// All subgraphs skipped on second query (everything served from cache)
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products subgraph (root field cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "Second query should skip reviews subgraph (entity cache hit)")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts subgraph (entity cache hit)")
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
