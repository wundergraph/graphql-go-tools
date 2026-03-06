package engine_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	accounts "github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCacheAnalyticsE2E(t *testing.T) {
	// Common cache key constants used across subtests
	const (
		keyProductTop1 = `{"__typename":"Product","key":{"upc":"top-1"}}`
		keyProductTop2 = `{"__typename":"Product","key":{"upc":"top-2"}}`
		keyTopProducts = `{"__typename":"Query","field":"topProducts"}`
		keyUser1234    = `{"__typename":"User","key":{"id":"1234"}}`
		keyMe          = `{"__typename":"Query","field":"me"}`
		dsAccounts     = "accounts"
		dsProducts     = "products"
		dsReviews      = "reviews"
	)

	// Field hash constants — xxhash of the rendered scalar field values.
	// These are deterministic because xxhash is seeded identically each time.
	const (
		hashProductNameTrilby uint64 = 1032923585965781586 // xxhash("Trilby")
		hashProductNameFedora uint64 = 2432227032303632641 // xxhash("Fedora")
		hashUserUsernameMe    uint64 = 4957449860898447395 // xxhash("Me")
	)

	// Entity key constants for field hash assertions
	const (
		entityKeyProductTop1 = `{"upc":"top-1"}`
		entityKeyProductTop2 = `{"upc":"top-2"}`
		entityKeyUser1234    = `{"id":"1234"}`
	)

	// Byte sizes of cached entities (measured from actual JSON marshalling)
	const (
		byteSizeProductTop1  = 177 // Product top-1 entity (reviews subgraph response)
		byteSizeProductTop2  = 233 // Product top-2 entity (reviews subgraph response)
		byteSizeTopProducts  = 127 // Query.topProducts root field (products subgraph response)
		byteSizeUser1234     = 49  // User 1234 entity (accounts subgraph response)
		byteSizeUser1234Full = 105 // User 1234 entity from L1 (includes sameUserReviewers data)
		byteSizeQueryMe      = 56  // Query.me root field (accounts subgraph response)
	)

	// Shared field hashes for the multi-upstream query (topProducts with reviews).
	// Product.name: 2 products (Trilby, Fedora) → 2 distinct hashes
	// User.username: 2 reviews both by "Me" → 2 identical hashes
	// All FieldSourceSubgraph by default (overridden in specific tests)
	multiUpstreamFieldHashes := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceSubgraph},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceSubgraph},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceSubgraph},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceSubgraph},
	}

	// L2 hit field hashes — same data but all sourced from L2 cache
	multiUpstreamFieldHashesL2 := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceL2},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
	}

	multiUpstreamEntityTypes := []resolve.EntityTypeInfo{
		{TypeName: "Product", Count: 2, UniqueKeys: 2},
		{TypeName: "User", Count: 2, UniqueKeys: 1},
	}

	// Standard subgraph caching configs used by L2 and L1+L2 tests
	multiUpstreamCachingConfigs := engine.SubgraphCachingConfigs{
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

	expectedResponseBody := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`

	t.Run("L2 miss then hit with analytics", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(multiUpstreamCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First query — all L2 misses, populates L2 cache
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		expected1 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews}, // L2 miss: first request, cache empty
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews}, // L2 miss: first request, cache empty
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts},  // L2 miss: root field not yet cached
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: dsAccounts},      // L2 miss: User entity not yet cached (second review's User 1234 deduplicated in batch)
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written after subgraph fetch on miss
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written after subgraph fetch on miss
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},  // Root field written to L2 after fetch
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // User entity written after accounts fetch
			},
			FieldHashes: multiUpstreamFieldHashes,
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected1, normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Second query — all L2 hits from populated cache
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		expected2 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1}, // L2 hit: populated by Request 1
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2}, // L2 hit: populated by Request 1
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},  // L2 hit: root field cached by Request 1
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234},         // L2 hit: User entity cached by Request 1 (second review's User 1234 deduplicated)
			},
			// No L2Writes: all served from cache, no fetches needed
			FieldHashes: multiUpstreamFieldHashesL2,
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected2, normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("L1 cache analytics with entity reuse", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache:        true,
				EnableL2Cache:        false,
				EnableCacheAnalytics: true,
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Query that triggers L1 entity reuse:
		// 1. Query.me -> accounts subgraph -> returns User 1234 -> populates L1
		// 2. User.sameUserReviewers -> reviews subgraph -> returns [User 1234]
		// 3. Entity fetch for User 1234 -> L1 HIT (no subgraph call)
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

		tracker.Reset()
		resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}}`, string(resp))

		expected := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L1Reads: []resolve.CacheKeyEvent{
				// L1 hit: User 1234 was populated by Query.me root fetch, reused for sameUserReviewers
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234Full},
			},
			L1Writes: []resolve.CacheWriteEvent{
				// Query.me root field written to L1 after accounts subgraph fetch
				{CacheKey: keyMe, EntityType: "Query", ByteSize: byteSizeQueryMe, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL1},
			},
			FieldHashes: []resolve.EntityFieldHash{
				// Both username entries show L1 source because the entity key resolves to
				// the L1 source recorded during the entity fetch L1 HIT
				{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL1}, // me.username: entity came from L1
				{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL1}, // sameUserReviewers[0].username: same L1 entity
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 2, UniqueKeys: 1}, // 2 User instances, but only 1 unique key (1234)
			},
		})
		assert.Equal(t, expected, normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("L1+L2 combined analytics", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache:        true,
				EnableL2Cache:        true,
				EnableCacheAnalytics: true,
			}),
			withSubgraphEntityCachingConfigs(multiUpstreamCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First query — L2 misses (L1 is per-request, always fresh)
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		expected1 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews}, // L2 miss: first request, cache empty
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews}, // L2 miss: first request, cache empty
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts},  // L2 miss: root field not yet cached
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: dsAccounts},      // L2 miss: User entity not yet cached (second review's User 1234 hits L1 after this fetch)
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written after reviews subgraph fetch
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written after reviews subgraph fetch
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},  // Root field written after products fetch
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // User entity written after accounts fetch
			},
			FieldHashes: multiUpstreamFieldHashes,
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected1, normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Second query — L2 hits (L1 is per-request, reset between requests)
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		expected2 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1}, // L2 hit: populated by Request 1
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2}, // L2 hit: populated by Request 1
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},  // L2 hit: root field cached by Request 1
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234},         // L2 hit: User entity cached by Request 1 (second review's User 1234 hits L1)
			},
			// No L2Writes: all entities served from L2 cache
			FieldHashes: multiUpstreamFieldHashesL2,
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected2, normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("root field with args - L2 analytics", func(t *testing.T) {
		// Tests that root field caching with arguments properly records L2 analytics events.
		// This covers the root field path in tryL2CacheLoad (no L1 keys branch).
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		rootFieldArgsCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "user", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(rootFieldArgsCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		const (
			keyUserById1234  = `{"__typename":"Query","field":"user","args":{"id":"1234"}}`
			keyUserById5678  = `{"__typename":"Query","field":"user","args":{"id":"5678"}}`
			dsAccountsLocal  = "accounts"
			byteSizeUser1234 = 38 // {"user":{"id":"1234","username":"Me"}}
			byteSizeUser5678 = 45 // {"user":{"id":"5678","username":"User 5678"}}

			hashUsernameMeLocal    uint64 = 4957449860898447395  // xxhash("Me")
			hashUsername5678Local  uint64 = 15512417390573333165 // xxhash("User 5678")
			entityKeyUser1234Local        = `{"id":"1234"}`
			entityKeyUser5678Local        = `{"id":"5678"}`
		)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// First query (id=1234) — L2 miss, populates cache
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts subgraph")

		expected1 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyUserById1234, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsAccountsLocal}, // L2 miss: first request, cache empty
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyUserById1234, EntityType: "Query", ByteSize: byteSizeUser1234, DataSource: dsAccountsLocal, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Root field written after accounts fetch
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "User", FieldName: "username", FieldHash: hashUsernameMeLocal, KeyRaw: entityKeyUser1234Local, Source: resolve.FieldSourceSubgraph}, // User returned by root field, data from subgraph
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // 1 User entity from root field response
			},
		})
		assert.Equal(t, expected1, normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Second query (same id=1234) — L2 hit
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		expected2 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyUserById1234, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsAccountsLocal, ByteSize: byteSizeUser1234}, // L2 hit: populated by first request
			},
			// No L2Writes: data served from cache
			FieldHashes: []resolve.EntityFieldHash{
				// Source is FieldSourceSubgraph (default) because entity source tracking operates at
				// entity cache level, not root field cache level — no entity caching configured for User
				{EntityType: "User", FieldName: "username", FieldHash: hashUsernameMeLocal, KeyRaw: entityKeyUser1234Local, Source: resolve.FieldSourceSubgraph},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1},
			},
		})
		assert.Equal(t, expected2, normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Third query (different id=5678) — L2 miss (different args = different cache key)
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "5678"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"5678","username":"User 5678"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Third query should call accounts (different args)")

		expected3 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyUserById5678, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsAccountsLocal}, // L2 miss: different args, not cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyUserById5678, EntityType: "Query", ByteSize: byteSizeUser5678, DataSource: dsAccountsLocal, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // New args written to L2
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "User", FieldName: "username", FieldHash: hashUsername5678Local, KeyRaw: entityKeyUser5678Local, Source: resolve.FieldSourceSubgraph}, // User 5678 data from subgraph
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1},
			},
		})
		assert.Equal(t, expected3, normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("root field only - L2 analytics without entity caching", func(t *testing.T) {
		// Tests root field caching analytics in isolation — only root field caching configured,
		// no entity caching. Verifies that only root field events appear in analytics.
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Only configure root field caching for products — no entity caching at all
		rootOnlyConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(rootOnlyConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		reviewsHost := reviewsURLParsed.Host
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		const (
			keyTopProductsLocal = `{"__typename":"Query","field":"topProducts"}`
			dsProductsLocal     = "products"
			byteSizeTP          = 127 // Query.topProducts root field response
		)

		// First query — L2 miss for root field, no events for entities (not configured)
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		// Products subgraph called (root field miss), reviews + accounts always called (no entity caching)
		assert.Equal(t, 1, tracker.GetCount(productsHost), "First query should call products subgraph")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "First query should call reviews subgraph")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts subgraph")

		expected1 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyTopProductsLocal, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProductsLocal}, // L2 miss: first request, cache empty
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyTopProductsLocal, EntityType: "Query", ByteSize: byteSizeTP, DataSource: dsProductsLocal, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Root field written after products fetch
			},
			// Only entity types tracked during resolution (not caching-dependent)
			FieldHashes: multiUpstreamFieldHashes,
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected1, normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Second query — L2 hit for root field, entities still fetched (not cached)
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		// Products subgraph skipped (root field cache hit), reviews + accounts still called
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products (root field cache hit)")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "Second query should call reviews (no entity caching)")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query should call accounts (no entity caching)")

		expected2 := normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyTopProductsLocal, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProductsLocal, ByteSize: byteSizeTP}, // L2 hit: root field cached by first request
			},
			// No L2Writes: root field served from cache, entities have no caching configured
			FieldHashes: multiUpstreamFieldHashes, // Entity field hashes still tracked (resolution, not caching)
			EntityTypes: multiUpstreamEntityTypes,
		})
		assert.Equal(t, expected2, normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("subgraph fetch records HTTPStatusCode and ResponseBytes", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(multiUpstreamCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First request — all L2 misses, subgraph fetches happen
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		snap := parseCacheAnalytics(t, headers)

		// Filter to subgraph fetch events only (exclude L2 read events)
		var subgraphTimings []resolve.FetchTimingEvent
		for _, ft := range snap.FetchTimings {
			if ft.Source == resolve.FieldSourceSubgraph {
				subgraphTimings = append(subgraphTimings, ft)
			}
		}
		timings := normalizeFetchTimings(subgraphTimings)

		assert.Equal(t, []resolve.FetchTimingEvent{
			{DataSource: dsAccounts, EntityType: "User", Source: resolve.FieldSourceSubgraph, ItemCount: 1, IsEntityFetch: true, HTTPStatusCode: 200, ResponseBytes: 62},    // _entities fetch for User 1234
			{DataSource: dsProducts, EntityType: "Query", Source: resolve.FieldSourceSubgraph, ItemCount: 1, IsEntityFetch: false, HTTPStatusCode: 200, ResponseBytes: 136}, // topProducts root field fetch
			{DataSource: dsReviews, EntityType: "Product", Source: resolve.FieldSourceSubgraph, ItemCount: 1, IsEntityFetch: true, HTTPStatusCode: 200, ResponseBytes: 376}, // _entities fetch for Product top-1 and top-2
		}, timings)
	})

	t.Run("cache hit has zero HTTPStatusCode and ResponseBytes", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(multiUpstreamCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First request — populates L2 cache
		resp, _ := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		// Second request — all L2 hits
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		snap := parseCacheAnalytics(t, headers)
		timings := normalizeFetchTimings(snap.FetchTimings)

		assert.Equal(t, []resolve.FetchTimingEvent{
			{DataSource: dsAccounts, EntityType: "User", Source: resolve.FieldSourceL2, ItemCount: 1, IsEntityFetch: true},   // L2 hit for User 1234 entity
			{DataSource: dsProducts, EntityType: "Query", Source: resolve.FieldSourceL2, ItemCount: 1, IsEntityFetch: true},  // L2 hit for topProducts root field
			{DataSource: dsReviews, EntityType: "Product", Source: resolve.FieldSourceL2, ItemCount: 2, IsEntityFetch: true}, // L2 hit for Product top-1 and top-2 entities
		}, timings)
	})
}

func TestShadowCacheE2E(t *testing.T) {
	// Cache key constants (same as TestCacheAnalyticsE2E — same federation setup)
	const (
		keyProductTop1 = `{"__typename":"Product","key":{"upc":"top-1"}}`
		keyProductTop2 = `{"__typename":"Product","key":{"upc":"top-2"}}`
		keyTopProducts = `{"__typename":"Query","field":"topProducts"}`
		keyUser1234    = `{"__typename":"User","key":{"id":"1234"}}`
		dsAccounts     = "accounts"
		dsProducts     = "products"
		dsReviews      = "reviews"
	)

	// Field hash constants
	const (
		hashProductNameTrilby uint64 = 1032923585965781586
		hashProductNameFedora uint64 = 2432227032303632641
		hashUserUsernameMe    uint64 = 4957449860898447395
	)

	// Entity key constants
	const (
		entityKeyProductTop1 = `{"upc":"top-1"}`
		entityKeyProductTop2 = `{"upc":"top-2"}`
		entityKeyUser1234    = `{"id":"1234"}`
	)

	// Byte sizes
	const (
		byteSizeProductTop1 = 177
		byteSizeProductTop2 = 233
		byteSizeTopProducts = 127
		byteSizeUser1234    = 49
	)

	// Shadow comparison hash constants
	const (
		shadowHashProductTop1  uint64 = 8656108128396512717
		shadowHashProductTop2  uint64 = 4671066427758823003
		shadowHashUser1234     uint64 = 188937276969638005
		shadowBytesProductTop1        = 124
		shadowBytesProductTop2        = 180
		shadowBytesUser1234           = 17
	)

	// Shadow cached field hash constants (ProvidesData fields hashed from cached value during shadow comparison)
	const (
		shadowFieldHashProductReviewsTop1 uint64 = 13894521258004960943 // xxhash of Product reviews field for top-1
		shadowFieldHashProductReviewsTop2 uint64 = 3182276346310063647  // xxhash of Product reviews field for top-2
	)

	// Field hashes when all data comes from subgraph (first request, all misses)
	fieldHashesSubgraph := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceSubgraph},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceSubgraph},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceSubgraph},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceSubgraph},
	}

	// Field hashes when all data comes from L2 (second request, all hits — no shadow entities)
	fieldHashesL2 := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceL2},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
	}

	// Field hashes when all entities are in shadow mode (second request):
	// L2 source hashes from resolution + ShadowCached hashes from compareShadowValues
	fieldHashesL2AllShadow := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceL2},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceL2},
		{EntityType: "Product", FieldName: "reviews", FieldHash: shadowFieldHashProductReviewsTop1, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceShadowCached}, // Cached Product reviews field for per-field staleness detection
		{EntityType: "Product", FieldName: "reviews", FieldHash: shadowFieldHashProductReviewsTop2, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceShadowCached}, // Cached Product reviews field for per-field staleness detection
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceShadowCached},                     // Cached User username for per-field staleness detection
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceShadowCached},                     // Cached User username (second review)
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
	}

	// Field hashes when only User is in shadow mode (mixed mode, second request):
	// Product/root L2 source hashes + User L2 + User ShadowCached hashes
	fieldHashesL2MixedShadow := []resolve.EntityFieldHash{
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: entityKeyProductTop1, Source: resolve.FieldSourceL2},
		{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: entityKeyProductTop2, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceShadowCached}, // Cached User username for per-field staleness detection
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceShadowCached}, // Cached User username (second review)
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
		{EntityType: "User", FieldName: "username", FieldHash: hashUserUsernameMe, KeyRaw: entityKeyUser1234, Source: resolve.FieldSourceL2},
	}

	entityTypes := []resolve.EntityTypeInfo{
		{TypeName: "Product", Count: 2, UniqueKeys: 2},
		{TypeName: "User", Count: 2, UniqueKeys: 1},
	}

	expectedResponseBody := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`

	t.Run("shadow all entities - always fetches", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Shadow mode for all entity types, real caching for root fields
		shadowConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, ShadowMode: true},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, ShadowMode: true},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(shadowConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)
		productsHost := mustParseHost(setup.ProductsUpstreamServer.URL)
		reviewsHost := mustParseHost(setup.ReviewsUpstreamServer.URL)

		// Request 1: All L2 misses → all 3 subgraphs called
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		assert.Equal(t, 1, tracker.GetCount(productsHost), "request 1: should call products exactly once")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "request 1: should call reviews exactly once")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "request 1: should call accounts exactly once")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews, Shadow: true}, // Shadow L2 miss: cache empty, subgraph fetched
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews, Shadow: true}, // Shadow L2 miss: cache empty, subgraph fetched
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts},                // Real L2 miss: root field not shadow, fetched normally
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: dsAccounts, Shadow: true},      // Shadow L2 miss: User not yet cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written to L2 even in shadow (populates for comparison)
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Written to L2 even in shadow
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},  // Root field written normally (not shadow)
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // User entity written for future shadow comparison
			},
			// No ShadowComparisons: nothing cached yet to compare against
			FieldHashes: fieldHashesSubgraph,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Request 2: Entity L2 hits (shadow) → entity subgraphs STILL called
		// Root field L2 hit → products NOT called (real caching)
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		assert.Equal(t, 0, tracker.GetCount(productsHost), "request 2: products should NOT be called (root field real cache hit)")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "request 2: reviews should be called (Product entity shadow)")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "request 2: accounts should be called (User entity shadow)")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1, Shadow: true}, // Shadow L2 hit: cached by Req 1, but subgraph still called
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2, Shadow: true}, // Shadow L2 hit: cached by Req 1, but subgraph still called
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},                // Real L2 hit: root field served from cache (not shadow)
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234, Shadow: true},         // Shadow L2 hit: accounts still called for comparison
			},
			L2Writes: []resolve.CacheWriteEvent{
				// Only shadow entities re-written (refreshed from subgraph); root field NOT re-written (real cache hit)
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Shadow re-write: fresh data from subgraph
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Shadow re-write: fresh data from subgraph
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // Shadow re-write: fresh User from accounts
			},
			ShadowComparisons: []resolve.ShadowComparisonEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", IsFresh: true, CachedHash: shadowHashProductTop1, FreshHash: shadowHashProductTop1, CachedBytes: shadowBytesProductTop1, FreshBytes: shadowBytesProductTop1, DataSource: dsReviews, ConfiguredTTL: 30 * time.Second}, // Fresh: cached matches subgraph (data unchanged)
				{CacheKey: keyProductTop2, EntityType: "Product", IsFresh: true, CachedHash: shadowHashProductTop2, FreshHash: shadowHashProductTop2, CachedBytes: shadowBytesProductTop2, FreshBytes: shadowBytesProductTop2, DataSource: dsReviews, ConfiguredTTL: 30 * time.Second}, // Fresh: cached matches subgraph (data unchanged)
				{CacheKey: keyUser1234, EntityType: "User", IsFresh: true, CachedHash: shadowHashUser1234, FreshHash: shadowHashUser1234, CachedBytes: shadowBytesUser1234, FreshBytes: shadowBytesUser1234, DataSource: dsAccounts, ConfiguredTTL: 30 * time.Second},                  // Fresh: cached User matches subgraph (no mutation)
			},
			FieldHashes: fieldHashesL2AllShadow,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("mixed mode - shadow User, real cache Product", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Shadow mode for User only, real caching for Product and root fields
		mixedConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "reviews",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second}, // real caching
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, ShadowMode: true}, // shadow
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(mixedConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)
		productsHost := mustParseHost(setup.ProductsUpstreamServer.URL)
		reviewsHost := mustParseHost(setup.ReviewsUpstreamServer.URL)

		// Request 1: All L2 misses → all 3 subgraphs called
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		assert.Equal(t, 1, tracker.GetCount(productsHost), "request 1: should call products exactly once")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "request 1: should call reviews exactly once")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "request 1: should call accounts exactly once")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews},          // Real L2 miss: Product entity not yet cached
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews},          // Real L2 miss: Product entity not yet cached
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts},           // Real L2 miss: root field not yet cached
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: dsAccounts, Shadow: true}, // Shadow L2 miss: User entity not yet cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Product written for real caching
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Product written for real caching
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},  // Root field written for real caching
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // User written (shadow still populates L2)
			},
			FieldHashes: fieldHashesSubgraph,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Request 2: Product real cache hit, User shadow → still fetched
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		assert.Equal(t, 0, tracker.GetCount(productsHost), "request 2: products should NOT be called (root field real cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "request 2: reviews should NOT be called (Product entity real cache hit)")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "request 2: accounts SHOULD be called (User entity shadow)")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1},       // Real L2 hit: Product served from cache (no subgraph call)
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2},       // Real L2 hit: Product served from cache (no subgraph call)
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},        // Real L2 hit: root field served from cache
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234, Shadow: true}, // Shadow L2 hit: accounts still called for comparison
			},
			L2Writes: []resolve.CacheWriteEvent{
				// Only User re-written (shadow always fetches fresh); Product/root NOT re-written (real hit)
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Shadow re-write: fresh data from accounts
			},
			ShadowComparisons: []resolve.ShadowComparisonEvent{
				// Only User has shadow comparisons; Product uses real caching
				{CacheKey: keyUser1234, EntityType: "User", IsFresh: true, CachedHash: shadowHashUser1234, FreshHash: shadowHashUser1234, CachedBytes: shadowBytesUser1234, FreshBytes: shadowBytesUser1234, DataSource: dsAccounts, ConfiguredTTL: 30 * time.Second}, // Fresh: cached User matches subgraph
			},
			FieldHashes: fieldHashesL2MixedShadow,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("shadow mode without analytics - safety only", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		shadowConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, ShadowMode: true},
				},
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}), // analytics NOT enabled
			withSubgraphEntityCachingConfigs(shadowConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Request 1: Populate cache
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))
		// No stats when analytics is disabled
		assert.Empty(t, headers.Get("X-Cache-Analytics"), "analytics header should not be set when analytics disabled")

		// Request 2: Shadow mode — accounts still fetched (data not served from cache)
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "request 2: accounts should be called (shadow mode)")
		// No stats when analytics is disabled
		assert.Empty(t, headers.Get("X-Cache-Analytics"), "analytics header should not be set when analytics disabled")
	})

	t.Run("graduation - shadow to real", func(t *testing.T) {
		// Same FakeLoaderCache shared across both engine setups
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// Phase 1: Shadow mode for User
		shadowConfigs := engine.SubgraphCachingConfigs{
			{SubgraphName: "products", RootFieldCaching: plan.RootFieldCacheConfigurations{
				{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
			}},
			{SubgraphName: "reviews", EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
			}},
			{SubgraphName: "accounts", EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, ShadowMode: true},
			}},
		}

		setup1 := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(shadowConfigs),
		))

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost1 := mustParseHost(setup1.AccountsUpstreamServer.URL)

		// Phase 1, Request 1: Populate L2 cache
		tracker.Reset()
		resp, headers := gqlClient.QueryWithHeaders(ctx, setup1.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews},          // Real L2 miss: first request, cache empty
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: dsReviews},          // Real L2 miss: first request, cache empty
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts},           // Real L2 miss: root field not yet cached
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: dsAccounts, Shadow: true}, // Shadow L2 miss: User not yet cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", ByteSize: byteSizeProductTop1, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Product written for real caching
				{CacheKey: keyProductTop2, EntityType: "Product", ByteSize: byteSizeProductTop2, DataSource: dsReviews, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Product written for real caching
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},  // Root field written for real caching
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},         // User written (shadow still populates L2)
			},
			FieldHashes: fieldHashesSubgraph,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Phase 1, Request 2: Shadow — accounts still called
		tracker.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup1.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost1), "phase 1 request 2: accounts should be called (shadow mode)")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1},       // Real L2 hit: Product served from cache
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2},       // Real L2 hit: Product served from cache
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},        // Real L2 hit: root field from cache
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234, Shadow: true}, // Shadow L2 hit: cached but accounts still called
			},
			L2Writes: []resolve.CacheWriteEvent{
				// Only shadow User re-written; Product/root use real caching (no re-write on hit)
				{CacheKey: keyUser1234, EntityType: "User", ByteSize: byteSizeUser1234, DataSource: dsAccounts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Shadow re-write with fresh data from accounts
			},
			ShadowComparisons: []resolve.ShadowComparisonEvent{
				{CacheKey: keyUser1234, EntityType: "User", IsFresh: true, CachedHash: shadowHashUser1234, FreshHash: shadowHashUser1234, CachedBytes: shadowBytesUser1234, FreshBytes: shadowBytesUser1234, DataSource: dsAccounts, ConfiguredTTL: 30 * time.Second}, // Fresh: cached User matches subgraph (safe to graduate)
			},
			FieldHashes: fieldHashesL2MixedShadow,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		setup1.Close()

		// Phase 2: Graduated to real caching (same cache, new engine)
		realConfigs := engine.SubgraphCachingConfigs{
			{SubgraphName: "products", RootFieldCaching: plan.RootFieldCacheConfigurations{
				{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
			}},
			{SubgraphName: "reviews", EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
			}},
			{SubgraphName: "accounts", EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second}, // No ShadowMode!
			}},
		}

		tracker2 := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient2 := &http.Client{Transport: tracker2}

		setup2 := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches), // SAME cache
			withHTTPClient(trackingClient2),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(realConfigs),
		))
		t.Cleanup(setup2.Close)

		accountsHost2 := mustParseHost(setup2.AccountsUpstreamServer.URL)

		// Phase 2, Request 3: Real L2 hit — accounts NOT called
		tracker2.Reset()
		resp, headers = gqlClient.QueryWithHeaders(ctx, setup2.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, expectedResponseBody, string(resp))
		assert.Equal(t, 0, tracker2.GetCount(accountsHost2), "phase 2: accounts should NOT be called (real L2 hit)")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyProductTop1, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop1}, // Real L2 hit: cached by Phase 1
				{CacheKey: keyProductTop2, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: dsReviews, ByteSize: byteSizeProductTop2}, // Real L2 hit: cached by Phase 1
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts},  // Real L2 hit: root field cached by Phase 1
				{CacheKey: keyUser1234, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: dsAccounts, ByteSize: byteSizeUser1234},         // Real L2 hit: graduated from shadow, no longer calls accounts
			},
			// No L2Writes: all real cache hits, no fetches needed
			// No ShadowComparisons: User is no longer in shadow mode
			FieldHashes: fieldHashesL2,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})
}

func TestMutationImpactE2E(t *testing.T) {
	accounts.ResetUsers()
	t.Cleanup(accounts.ResetUsers)

	// Configure entity caching for User on accounts subgraph
	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
		{
			SubgraphName: "accounts",
			EntityCaching: plan.EntityCacheConfigurations{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
		},
	}

	mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`

	// Uses a simple query that causes an entity fetch for User 1234
	// me { id username } triggers: accounts root fetch for Query.me, no entity fetch
	// We need a query that triggers entity caching for User - topProducts with reviews + authorWithoutProvides
	entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`

	t.Run("mutation with prior cache shows stale entity", func(t *testing.T) {
		accounts.ResetUsers()
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

		// Request 1: Query to populate L2 cache with User entity
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)

		// Request 2: Mutation — should detect stale cached entity
		tracker.Reset()
		respMut, headersMut := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)
		assert.Contains(t, string(respMut), `"UpdatedMe"`)

		snap := normalizeSnapshot(parseCacheAnalytics(t, headersMut))
		require.NotNil(t, snap.MutationEvents, "should have mutation impact events")
		require.Equal(t, 1, len(snap.MutationEvents), "should have exactly 1 mutation impact event")

		event := snap.MutationEvents[0]
		assert.Equal(t, "updateUsername", event.MutationRootField)
		assert.Equal(t, "User", event.EntityType)
		assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, event.EntityCacheKey)
		assert.Equal(t, true, event.HadCachedValue, "should have found cached value")
		assert.Equal(t, true, event.IsStale, "cached value should be stale (username changed)")

		// Record discovered values for exact assertion
		t.Logf("MutationImpact event: %+v", event)

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			FieldHashes: []resolve.EntityFieldHash{
				// Hash of "UpdatedMe" (post-mutation username)
				{EntityType: "User", FieldName: "username", FieldHash: 16932466035575627600, KeyRaw: `{"id":"1234"}`},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation returned 1 User entity
			},
			MutationEvents: []resolve.MutationEvent{
				{
					MutationRootField: "updateUsername",
					EntityType:        "User",
					EntityCacheKey:    `{"__typename":"User","key":{"id":"1234"}}`,
					HadCachedValue:    true, // L2 had cached value from Request 1 query
					IsStale:           true, // Cached "Me" differs from fresh "UpdatedMe"
					CachedHash:        event.CachedHash,
					FreshHash:         event.FreshHash,
					CachedBytes:       event.CachedBytes,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)
	})

	t.Run("mutation without prior cache shows no-cache event", func(t *testing.T) {
		accounts.ResetUsers()
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

		// NO prior query — L2 cache is empty
		// Send mutation directly
		tracker.Reset()
		respMut, headersMut := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)
		assert.Contains(t, string(respMut), `"UpdatedMe"`)

		snap := normalizeSnapshot(parseCacheAnalytics(t, headersMut))
		require.NotNil(t, snap.MutationEvents, "should have mutation impact events")
		require.Equal(t, 1, len(snap.MutationEvents), "should have exactly 1 mutation impact event")

		event := snap.MutationEvents[0]
		assert.Equal(t, "updateUsername", event.MutationRootField)
		assert.Equal(t, "User", event.EntityType)
		assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, event.EntityCacheKey)
		assert.Equal(t, false, event.HadCachedValue, "should NOT have found cached value")
		assert.Equal(t, false, event.IsStale, "cannot be stale without cached value")
		assert.Equal(t, uint64(0), event.CachedHash, "no cached value = no hash")
		assert.Equal(t, 0, event.CachedBytes, "no cached value = no bytes")

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			FieldHashes: []resolve.EntityFieldHash{
				// Hash of "UpdatedMe" (post-mutation username)
				{EntityType: "User", FieldName: "username", FieldHash: 16932466035575627600, KeyRaw: `{"id":"1234"}`},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation returned 1 User entity
			},
			MutationEvents: []resolve.MutationEvent{
				{
					MutationRootField: "updateUsername",
					EntityType:        "User",
					EntityCacheKey:    `{"__typename":"User","key":{"id":"1234"}}`,
					HadCachedValue:    false, // No prior query, L2 cache was empty
					IsStale:           false, // Cannot be stale without a cached value to compare
					FreshHash:         event.FreshHash,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)
	})
}

func TestFederationCachingAliases(t *testing.T) {
	// Helper to create a standard setup for alias caching tests
	setupAliasCachingTest := func(t *testing.T) (
		*federationtesting.FederationSetup,
		*GraphqlClient,
		context.Context,
		context.CancelFunc,
		*subgraphCallTracker,
		*FakeLoaderCache,
		string, // accountsHost
	) {
		t.Helper()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}
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
		return setup, gqlClient, ctx, cancel, tracker, defaultCache, accountsHost
	}

	t.Run("L2 hit - alias then no alias", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, accountsHost := setupAliasCachingTest(t)

		// Request 1: Use alias userName for username
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { topProducts { name reviews { body authorWithoutProvides { userName: username } } } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"userName":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"userName":"Me"}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1 should call accounts subgraph once")

		// Request 2: No alias (original field name)
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2 should skip accounts (L2 hit from normalized cache)")
	})

	t.Run("L2 hit - two different aliases for same field", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, accountsHost := setupAliasCachingTest(t)

		// Request 1: alias u1 for username
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { topProducts { name reviews { body authorWithoutProvides { u1: username } } } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"u1":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"u1":"Me"}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1 should call accounts subgraph once")

		// Request 2: alias u2 for username
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name reviews { body authorWithoutProvides { u2: username } } } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"u2":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"u2":"Me"}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2 should skip accounts (L2 hit - same underlying field)")
	})

	t.Run("no collision - alias matches another field name", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, accountsHost := setupAliasCachingTest(t)

		// Request 1: alias realName for username (realName is another real field on User)
		// This triggers an accounts entity fetch for username, stores normalized {"username":"Me"} in L2
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { topProducts { name reviews { body authorWithoutProvides { realName: username } } } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"realName":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"realName":"Me"}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1 should call accounts subgraph once for username")

		// Request 2: actual username field (no alias) - same underlying field
		// Should be an L2 hit because both resolve username from accounts
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2 should skip accounts (L2 hit - same underlying field username)")
	})

	t.Run("no collision - field name used as alias for another field", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, accountsHost := setupAliasCachingTest(t)

		// Request 1: username field (no alias) - triggers accounts entity fetch for username
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1 should call accounts subgraph once")

		// Request 2: different alias (u1) for same field (username)
		// Should be an L2 hit because the underlying field is the same
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name reviews { body authorWithoutProvides { u1: username } } } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"u1":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"u1":"Me"}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2 should skip accounts (L2 hit - same underlying field)")
	})

	t.Run("L2 hit - multiple fields some aliased some not", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, accountsHost := setupAliasCachingTest(t)

		// Request 1: alias username and include realName (realName comes from reviews, not accounts)
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { topProducts { name reviews { body authorWithoutProvides { userName: username realName } } } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"userName":"Me","realName":"User Usington"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"userName":"Me","realName":"User Usington"}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1 should call accounts subgraph once")

		// Request 2: no alias on username, different alias on realName
		// accounts entity cache should be L2 hit (same username field)
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name reviews { body authorWithoutProvides { username name: realName } } } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","name":"User Usington"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","name":"User Usington"}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2 should skip accounts (L2 hit - same underlying username field)")
	})

	t.Run("L1 hit within single request with aliases", func(t *testing.T) {
		// Tests L1 cache with aliased fields across entity fetches within the same request.
		// Flow:
		// 1. topProducts -> products
		// 2. reviews -> reviews (entity fetch for Products)
		// 3. authorWithoutProvides -> accounts (entity fetch for User 1234, aliased userName: username)
		//    -> User 1234 stored in L1 with normalized field names
		// 4. sameUserReviewers -> reviews (returns [User 1234] reference)
		// 5. Entity resolution for sameUserReviewers -> accounts
		//    -> User 1234 is L1 HIT (already fetched in step 3), entire accounts call skipped
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL1Cache: true, EnableL2Cache: false}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Query with alias on username - sameUserReviewers returns same user,
		// should be L1 hit from the first entity fetch
		tracker.Reset()
		query := `query {
			topProducts {
				reviews {
					authorWithoutProvides {
						id
						userName: username
						sameUserReviewers {
							id
							userName: username
						}
					}
				}
			}
		}`
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","userName":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","userName":"Me"}]}}]}]}}`,
			string(resp))

		// With L1 enabled: first accounts call fetches User 1234 for authorWithoutProvides
		// sameUserReviewers entity resolution hits L1 -> accounts call skipped
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "Should call accounts subgraph once (sameUserReviewers skipped via L1)")
	})

	t.Run("L1 hit within single request with mixed alias and no alias", func(t *testing.T) {
		// Same as above, but the nested sameUserReviewers uses the original field name (no alias)
		// while the outer authorWithoutProvides uses an alias. L1 cache stores normalized data,
		// so the nested fetch should still hit L1 despite the different field naming.
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL1Cache: true, EnableL2Cache: false}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Outer authorWithoutProvides uses alias "userName: username"
		// Nested sameUserReviewers uses plain "username" (no alias)
		// L1 should still hit because cache stores normalized (original) field names
		tracker.Reset()
		query := `query {
			topProducts {
				reviews {
					authorWithoutProvides {
						id
						userName: username
						sameUserReviewers {
							id
							username
						}
					}
				}
			}
		}`
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`,
			string(resp))

		// With L1 enabled: first accounts call fetches User 1234 for authorWithoutProvides
		// sameUserReviewers entity resolution hits L1 -> accounts call skipped
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "Should call accounts subgraph once (sameUserReviewers skipped via L1)")
	})

	t.Run("L2 hit - aliased root field then original root field", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, _ := setupAliasCachingTest(t)
		productsHost := mustParseHost(setup.ProductsUpstreamServer.URL)

		// Request 1: alias the root field topProducts as tp
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { tp: topProducts { name } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"tp":[{"name":"Trilby"},{"name":"Fedora"}]}}`,
			string(resp))

		productsCalls1 := tracker.GetCount(productsHost)
		assert.Equal(t, 1, productsCalls1, "Request 1 should call products subgraph once")

		// Request 2: same root field without alias — should L2 hit (same cache key)
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { topProducts { name } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`,
			string(resp))

		productsCalls2 := tracker.GetCount(productsHost)
		assert.Equal(t, 0, productsCalls2, "Request 2 should skip products (L2 hit from aliased root field)")
	})

	t.Run("L2 hit - two different root field aliases", func(t *testing.T) {
		setup, gqlClient, ctx, _, tracker, defaultCache, _ := setupAliasCachingTest(t)
		productsHost := mustParseHost(setup.ProductsUpstreamServer.URL)

		// Request 1: alias p1 for topProducts
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query { p1: topProducts { name } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"p1":[{"name":"Trilby"},{"name":"Fedora"}]}}`,
			string(resp))

		productsCalls1 := tracker.GetCount(productsHost)
		assert.Equal(t, 1, productsCalls1, "Request 1 should call products subgraph once")

		// Request 2: different alias p2 for same root field
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query { p2: topProducts { name } }`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"p2":[{"name":"Trilby"},{"name":"Fedora"}]}}`,
			string(resp))

		productsCalls2 := tracker.GetCount(productsHost)
		assert.Equal(t, 0, productsCalls2, "Request 2 should skip products (L2 hit - same underlying root field)")
	})

	t.Run("L1+L2 combined - alias entity caching across both layers", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
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
			withCachingOptionsFunc(resolve.CachingOptions{EnableL1Cache: true, EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Request 1: alias on username, sameUserReviewers triggers L1 hit within request
		// L2 is also populated on the first entity fetch
		defaultCache.ClearLog()
		tracker.Reset()
		query1 := `query {
			topProducts {
				reviews {
					authorWithoutProvides {
						id
						userName: username
						sameUserReviewers {
							id
							userName: username
						}
					}
				}
			}
		}`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","userName":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","userName":"Me","sameUserReviewers":[{"id":"1234","userName":"Me"}]}}]}]}}`,
			string(resp))

		accountsCalls1 := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls1, "Request 1: accounts called once (sameUserReviewers skipped via L1)")

		// Request 2: same query without alias — L2 hit for User entity, no accounts calls
		defaultCache.ClearLog()
		tracker.Reset()
		query2 := `query {
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
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`,
			string(resp))

		accountsCalls2 := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls2, "Request 2: accounts skipped (L2 hit from normalized cache)")
	})

	t.Run("L2 analytics - aliased root field", func(t *testing.T) {
		const (
			keyTopProducts        = `{"__typename":"Query","field":"topProducts"}`
			dsProducts            = "products"
			byteSizeTopProducts   = 53
			hashProductNameTrilby = uint64(1032923585965781586)
			hashProductNameFedora = uint64(2432227032303632641)
		)

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
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}
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

		// Shared field hashes: Product.name for Trilby and Fedora from root field response
		// Products are not entity-resolved (no @key fetch), so KeyRaw is empty
		fieldHashes := []resolve.EntityFieldHash{
			{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameTrilby, KeyRaw: "{}"}, // xxhash("Trilby"), no entity key (root field)
			{EntityType: "Product", FieldName: "name", FieldHash: hashProductNameFedora, KeyRaw: "{}"}, // xxhash("Fedora"), no entity key (root field)
		}
		entityTypes := []resolve.EntityTypeInfo{
			{TypeName: "Product", Count: 2, UniqueKeys: 1}, // 2 products from root field, no entity keys
		}

		// Request 1: aliased root field — L2 miss, populates cache
		tracker.Reset()
		query1 := `query { tp: topProducts { name } }`
		resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query1, nil, t)
		assert.Equal(t, `{"data":{"tp":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))

		// Cache key must use original field name "topProducts", NOT the alias "tp"
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: dsProducts}, // L2 miss: first request, cache empty
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: keyTopProducts, EntityType: "Query", ByteSize: byteSizeTopProducts, DataSource: dsProducts, CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second}, // Root field written after products fetch
			},
			FieldHashes: fieldHashes,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))

		// Request 2: original root field (no alias) — L2 hit from Request 1
		tracker.Reset()
		query2 := `query { topProducts { name } }`
		resp, headers = gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query2, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))

		// Same cache key hit regardless of alias difference
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: keyTopProducts, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: dsProducts, ByteSize: byteSizeTopProducts}, // L2 hit: populated by aliased Request 1
			},
			// No L2Writes: served from cache
			FieldHashes: fieldHashes,
			EntityTypes: entityTypes,
		}), normalizeSnapshot(parseCacheAnalytics(t, headers)))
	})

	t.Run("L1 dedup - two aliases for same entity field in single request", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL1Cache: true, EnableL2Cache: false}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Two aliases (a1, a2) for the same entity field (authorWithoutProvides)
		// Both resolve the same User 1234 — second should be L1 hit
		tracker.Reset()
		query := `query {
			topProducts {
				reviews {
					a1: authorWithoutProvides {
						id
						username
					}
					a2: authorWithoutProvides {
						id
						username
					}
				}
			}
		}`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"reviews":[{"a1":{"id":"1234","username":"Me"},"a2":{"id":"1234","username":"Me"}}]},{"reviews":[{"a1":{"id":"1234","username":"Me"},"a2":{"id":"1234","username":"Me"}}]}]}}`,
			string(resp))

		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "Should call accounts once (second alias L1 hit for same User entity)")
	})
}

func TestHeaderImpactAnalyticsE2E(t *testing.T) {
	t.Run("shadow mode with header prefix - same response different headers", func(t *testing.T) {
		mockHeaders := &headerForwardingMock{
			headers: map[string]http.Header{
				"products": {"Authorization": {"Bearer token-A"}},
				"reviews":  {"Authorization": {"Bearer token-A"}},
				"accounts": {"Authorization": {"Bearer token-A"}},
			},
		}
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": NewFakeLoaderCache()}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withSubgraphHeadersBuilder(mockHeaders),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true, ShadowMode: true},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true, ShadowMode: true},
					},
				},
				{
					SubgraphName: "accounts",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true, ShadowMode: true},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Request 1: L2 miss → fetch → write with token-A header hash prefix
		tracker.Reset()
		resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body authorWithoutProvides { username } } } }`, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		snap1 := normalizeSnapshot(parseCacheAnalytics(t, headers))

		// Capture response hashes from first request (deterministic subgraph responses)
		responseHashes := make(map[string]uint64, len(snap1.HeaderImpactEvents))
		for _, ev := range snap1.HeaderImpactEvents {
			responseHashes[ev.BaseKey] = ev.ResponseHash
		}

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews", Shadow: true}, // Shadow L2 miss: cache empty
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews", Shadow: true}, // Shadow L2 miss: cache empty
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: "products", Shadow: false},    // L2 miss: shadow mode not implemented for root fields
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: "accounts", Shadow: true},         // Shadow L2 miss: User not yet cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: `11945571715631340836:{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", ByteSize: 177, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", ByteSize: 233, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", ByteSize: 127, DataSource: "products", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "Product", FieldName: "name", FieldHash: 1032923585965781586, KeyRaw: `{"upc":"top-1"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "Product", FieldName: "name", FieldHash: 2432227032303632641, KeyRaw: `{"upc":"top-2"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "Product", Count: 2, UniqueKeys: 2},
				{TypeName: "User", Count: 2, UniqueKeys: 1},
			},
			HeaderImpactEvents: []resolve.HeaderImpactEvent{
				// Authorization: Bearer token-A → header hash 11945571715631340836
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-1"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-2"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Query","field":"topProducts"}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Query","field":"topProducts"}`], EntityType: "Query", DataSource: "products"},
				{BaseKey: `{"__typename":"User","key":{"id":"1234"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"User","key":{"id":"1234"}}`], EntityType: "User", DataSource: "accounts"},
			},
		}), snap1)

		// Request 2: Switch to token-B headers (actually different headers forwarded to subgraphs)
		mockHeaders.setAll(http.Header{"Authorization": {"Bearer token-B"}})

		tracker.Reset()
		resp, headers = gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body authorWithoutProvides { username } } } }`, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		snap2 := normalizeSnapshot(parseCacheAnalytics(t, headers))

		// Key insight: different headers (token-B) → SAME ResponseHash → headers are irrelevant
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews", Shadow: true}, // token-B prefix not in cache
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews", Shadow: true}, // token-B prefix not in cache
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: "products", Shadow: false},    // shadow mode not implemented for root fields
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: "accounts", Shadow: true},         // token-B prefix not in cache
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: `4753115417090238877:{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", ByteSize: 177, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `4753115417090238877:{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", ByteSize: 233, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `4753115417090238877:{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", ByteSize: 127, DataSource: "products", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `4753115417090238877:{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "Product", FieldName: "name", FieldHash: 1032923585965781586, KeyRaw: `{"upc":"top-1"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "Product", FieldName: "name", FieldHash: 2432227032303632641, KeyRaw: `{"upc":"top-2"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "Product", Count: 2, UniqueKeys: 2},
				{TypeName: "User", Count: 2, UniqueKeys: 1},
			},
			HeaderImpactEvents: []resolve.HeaderImpactEvent{
				// Authorization: Bearer token-B → header hash 4753115417090238877; SAME ResponseHash → headers irrelevant
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, HeaderHash: 4753115417090238877, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-1"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, HeaderHash: 4753115417090238877, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-2"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Query","field":"topProducts"}`, HeaderHash: 4753115417090238877, ResponseHash: responseHashes[`{"__typename":"Query","field":"topProducts"}`], EntityType: "Query", DataSource: "products"},
				{BaseKey: `{"__typename":"User","key":{"id":"1234"}}`, HeaderHash: 4753115417090238877, ResponseHash: responseHashes[`{"__typename":"User","key":{"id":"1234"}}`], EntityType: "User", DataSource: "accounts"},
			},
		}), snap2)
	})

	t.Run("non-shadow mode - events on L2 miss, no events on L2 hit", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": NewFakeLoaderCache()}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withSubgraphHeadersBuilder(&headerForwardingMock{
				headers: map[string]http.Header{
					"products": {"Authorization": {"Bearer token-A"}},
					"reviews":  {"Authorization": {"Bearer token-A"}},
					"accounts": {"Authorization": {"Bearer token-A"}},
				},
			}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
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
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Request 1: L2 miss → fetch → HeaderImpactEvents recorded
		tracker.Reset()
		resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body authorWithoutProvides { username } } } }`, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		snap1 := normalizeSnapshot(parseCacheAnalytics(t, headers))

		// Capture response hashes (deterministic)
		responseHashes := make(map[string]uint64, len(snap1.HeaderImpactEvents))
		for _, ev := range snap1.HeaderImpactEvents {
			responseHashes[ev.BaseKey] = ev.ResponseHash
		}

		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews"}, // L2 miss: cache empty
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews"}, // L2 miss: cache empty
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: "products"},     // L2 miss: root field not yet cached
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: "accounts"},         // L2 miss: User not yet cached
			},
			L2Writes: []resolve.CacheWriteEvent{
				// Authorization: Bearer token-A → header hash prefix 11945571715631340836
				{CacheKey: `11945571715631340836:{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", ByteSize: 177, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", ByteSize: 233, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", ByteSize: 127, DataSource: "products", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `11945571715631340836:{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "Product", FieldName: "name", FieldHash: 1032923585965781586, KeyRaw: `{"upc":"top-1"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "Product", FieldName: "name", FieldHash: 2432227032303632641, KeyRaw: `{"upc":"top-2"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "Product", Count: 2, UniqueKeys: 2},
				{TypeName: "User", Count: 2, UniqueKeys: 1},
			},
			HeaderImpactEvents: []resolve.HeaderImpactEvent{
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-1"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Product","key":{"upc":"top-2"}}`], EntityType: "Product", DataSource: "reviews"},
				{BaseKey: `{"__typename":"Query","field":"topProducts"}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"Query","field":"topProducts"}`], EntityType: "Query", DataSource: "products"},
				{BaseKey: `{"__typename":"User","key":{"id":"1234"}}`, HeaderHash: 11945571715631340836, ResponseHash: responseHashes[`{"__typename":"User","key":{"id":"1234"}}`], EntityType: "User", DataSource: "accounts"},
			},
		}), snap1)

		// Request 2: Same headers → L2 hit → no fetch → empty analytics (except L2 reads)
		tracker.Reset()
		resp, headers = gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body authorWithoutProvides { username } } } }`, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		snap2 := normalizeSnapshot(parseCacheAnalytics(t, headers))
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: "reviews", ByteSize: 177}, // L2 hit: populated by request 1
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", Kind: resolve.CacheKeyHit, DataSource: "reviews", ByteSize: 233}, // L2 hit: populated by request 1
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", Kind: resolve.CacheKeyHit, DataSource: "products", ByteSize: 127},     // L2 hit: root field cached by request 1
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyHit, DataSource: "accounts", ByteSize: 49},          // L2 hit: User cached by request 1
			},
			// No L2Writes, no HeaderImpactEvents: all served from cache, no fresh fetches
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "Product", FieldName: "name", FieldHash: 1032923585965781586, KeyRaw: `{"upc":"top-1"}`, Source: resolve.FieldSourceL2},
				{EntityType: "Product", FieldName: "name", FieldHash: 2432227032303632641, KeyRaw: `{"upc":"top-2"}`, Source: resolve.FieldSourceL2},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceL2},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceL2},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "Product", Count: 2, UniqueKeys: 2},
				{TypeName: "User", Count: 2, UniqueKeys: 1},
			},
		}), snap2)
	})

	t.Run("no events when IncludeSubgraphHeaderPrefix is false", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": NewFakeLoaderCache()}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true, EnableCacheAnalytics: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
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
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tracker.Reset()
		resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body authorWithoutProvides { username } } } }`, nil, t)
		assert.Equal(t,
			`{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`,
			string(resp))

		snap := normalizeSnapshot(parseCacheAnalytics(t, headers))
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			L2Reads: []resolve.CacheKeyEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews"},
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews"},
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", Kind: resolve.CacheKeyMiss, DataSource: "products"},
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", Kind: resolve.CacheKeyMiss, DataSource: "accounts"},
			},
			L2Writes: []resolve.CacheWriteEvent{
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", ByteSize: 177, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `{"__typename":"Product","key":{"upc":"top-2"}}`, EntityType: "Product", ByteSize: 233, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `{"__typename":"Query","field":"topProducts"}`, EntityType: "Query", ByteSize: 127, DataSource: "products", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
				{CacheKey: `{"__typename":"User","key":{"id":"1234"}}`, EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
			},
			FieldHashes: []resolve.EntityFieldHash{
				{EntityType: "Product", FieldName: "name", FieldHash: 1032923585965781586, KeyRaw: `{"upc":"top-1"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "Product", FieldName: "name", FieldHash: 2432227032303632641, KeyRaw: `{"upc":"top-2"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
				{EntityType: "User", FieldName: "username", FieldHash: 4957449860898447395, KeyRaw: `{"id":"1234"}`, Source: resolve.FieldSourceSubgraph},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "Product", Count: 2, UniqueKeys: 2},
				{TypeName: "User", Count: 2, UniqueKeys: 1},
			},
			// No HeaderImpactEvents: IncludeSubgraphHeaderPrefix is false
		}), snap)
	})
}
