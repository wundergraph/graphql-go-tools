package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// toWSAddr converts an HTTP URL to a WebSocket URL.
func toWSAddr(httpURL string) string {
	return strings.ReplaceAll(httpURL, "http://", "ws://")
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// collectSubscriptionMessages subscribes and collects exactly count messages.
func collectSubscriptionMessages(ctx context.Context, gqlClient *GraphqlClient, setup *federationtesting.FederationSetup, wsAddr, queryPath string,
	variables queryVariables, count int, t *testing.T) []string {
	t.Helper()

	messages, closeSubscription := gqlClient.Subscription(ctx, wsAddr, queryPath, variables, t)
	defer closeSubscription()

	trigger, err := setup.NextProductSubscription(ctx)
	require.NoError(t, err)

	var result []string
	for i := range count {
		trigger.Emit()

		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatalf("subscription channel closed after %d messages, expected %d", i, count)
			}
			result = append(result, string(msg))
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for subscription message %d of %d", i+1, count)
		}
	}

	return result
}

//nolint:tparallel // Timing-sensitive subscription cache tests need a few subtests to run before parallel siblings.
// TestFederationSubscriptionCaching verifies subscription-driven entity cache population:
// subscription events write entity data to L2, which subsequent queries can hit.
func TestFederationSubscriptionCaching(t *testing.T) {
	// =====================================================================
	// Category 1: Child fetch L2 read/write within subscription events
	// =====================================================================

	t.Run("child entity fetch - L2 miss then hit across events", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Configure entity caching for User entities in accounts subgraph
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe to product "top-4" which has 2 reviews by different authors
		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 2, t)

		// Event 1: should resolve User entities (L2 miss → fetch → L2 set)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])

		// Event 2: should hit L2 cache for User entities
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":2,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[1])

		// Verify accounts was called exactly once (event 1 fetched, event 2 hit cache)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "accounts should be called exactly once (L2 miss on event 1, hit on event 2)")

		// Verify cache log
		cacheLog := defaultCache.GetLog()

		// Event 1: get (miss for User 1234 and 7777), set (both users)
		// Event 2: get (hit for User 1234 and 7777)
		// Total: 3 operations
		assert.Equal(t, 3, len(cacheLog), "should have exactly 3 cache operations")

		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationGet,
				Keys: []string{
					`{"__typename":"User","key":{"id":"5678"}}`,
					`{"__typename":"User","key":{"id":"8888"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: CacheOperationSet,
				Keys: []string{
					`{"__typename":"User","key":{"id":"5678"}}`,
					`{"__typename":"User","key":{"id":"8888"}}`,
				},
			},
			{
				Operation: CacheOperationGet,
				Keys: []string{
					`{"__typename":"User","key":{"id":"5678"}}`,
					`{"__typename":"User","key":{"id":"8888"}}`,
				},
				Hits: []bool{true, true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(cacheLog), "cache log should show miss+set on event 1, hit on event 2")
	})

	t.Run("L2 pre-populated - subscription child fetch hits L2", func(t *testing.T) {
		t.Parallel()
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
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		// Pre-populate L2 with User entities that match top-4's review authors
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: `{"__typename":"User","key":{"id":"5678"}}`, Value: []byte(`{"id":"5678","username":"User 5678"}`)},
			{Key: `{"__typename":"User","key":{"id":"8888"}}`, Value: []byte(`{"id":"8888","username":"User 8888"}`)},
		}, 30*time.Second)
		require.NoError(t, err)

		// Subscribe - User entities should hit L2 from pre-populated cache
		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, toWSAddr(setup.GatewayServer.URL),
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 1, t)

		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])

		// Accounts should NOT be called during subscription (L2 hit)
		subAccountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, subAccountsCalls, "subscription should not call accounts (L2 pre-populated)")

		// Cache log should show L2 get with hits
		cacheLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationGet,
				Keys: []string{
					`{"__typename":"User","key":{"id":"5678"}}`,
					`{"__typename":"User","key":{"id":"8888"}}`,
				},
				Hits: []bool{true, true},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(cacheLog), "cache log should show L2 hits for pre-populated users")
	})

	t.Run("child entity fetch L2 TTL expiry across events", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Short TTL for testing expiry
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 150 * time.Millisecond},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Collect 3 events:
		// Event 1 (~100ms): L2 miss → accounts called → L2 set
		// Event 2 (~200ms): Within TTL → L2 hit → no call
		// Event 3 (~300ms): After TTL expiry → L2 miss → accounts called again
		tracker.Reset()
		messages, closeSubscription := gqlClient.Subscription(ctx, wsAddr, cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"), queryVariables{"upc": "top-4"}, t)
		t.Cleanup(closeSubscription)

		trigger, err := setup.NextProductSubscription(ctx)
		require.NoError(t, err)

		trigger.Emit()
		first := <-messages
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, string(first))

		trigger.Emit()
		second := <-messages
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":2,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, string(second))

		// Wait for 150ms TTL to expire on the cached user entities (deterministic via Peek)
		assert.Eventually(t, func() bool {
			_, ok1 := defaultCache.Peek(`{"__typename":"User","key":{"id":"5678"}}`)
			_, ok2 := defaultCache.Peek(`{"__typename":"User","key":{"id":"8888"}}`)
			return !ok1 && !ok2
		}, 2*time.Second, 10*time.Millisecond, "user L2 entries should expire after TTL")
		trigger.Emit()
		third := <-messages
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":3,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, string(third))

		// Accounts should be called exactly 2 times (event 1 and event 3)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls, "accounts should be called exactly twice (miss, hit, miss after TTL expiry)")
	})

	t.Run("entity caching not configured - no cache operations", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// No entity caching configured for accounts
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 2, t)

		require.Equal(t, 2, len(messages))

		// Accounts should be called on every event (no caching)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls, "accounts should be called on every event (no caching configured)")

		// Cache log should be empty for entity operations
		cacheLog := defaultCache.GetLog()
		assert.Equal(t, 0, len(cacheLog), "no cache operations expected when caching not configured")
	})

	// =====================================================================
	// Category 2: Subscription root entity populates L2
	// =====================================================================

	t.Run("subscription entity populates L2 - verified via cache", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe to product updates - selects name, price beyond @key(upc) → populate mode
		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify L2 was populated by subscription via cache log
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription should populate L2 with Product entity")

		// Verify the cached data directly
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("subscription populates L2 - cached data has only selected fields", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe with subscription_product_only.query which selects {upc, name, price}
		// but NOT inStock. The subscription should populate L2 with only these fields.
		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify L2 was populated
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription should populate L2")

		// Verify the cached entity has upc, name, price but NOT inStock
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("subscription entity list populates L2 - multiple entities cached", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updatedPrices", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe to updatedPrices which returns a list of products (top-1, top-2, top-3)
		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_all_prices_with_reviews.query"),
			nil, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updatedPrices":[{"upc":"top-1","name":"Trilby","price":1,"reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"upc":"top-2","name":"Fedora","price":2,"reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]},{"upc":"top-3","name":"Boater","price":3,"reviews":[{"body":"This is the last straw. Hat you will wear. 11/10","authorWithoutProvides":{"username":"User 7777"}}]}]}}}`, messages[0])

		// Verify L2 was populated with all 3 product entities
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationSet, Keys: []string{
				`{"__typename":"Product","key":{"upc":"top-1"}}`,
				`{"__typename":"Product","key":{"upc":"top-2"}}`,
				`{"__typename":"Product","key":{"upc":"top-3"}}`,
			}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription should populate L2 with Product entities")

		// Verify exact cached values for all 3 products
		entityKeys := []string{
			`{"__typename":"Product","key":{"upc":"top-1"}}`,
			`{"__typename":"Product","key":{"upc":"top-2"}}`,
			`{"__typename":"Product","key":{"upc":"top-3"}}`,
		}
		entries, err := defaultCache.Get(ctx, entityKeys)
		require.NoError(t, err)
		require.Equal(t, 3, len(entries))
		require.NotNil(t, entries[0])
		assert.Equal(t, `{"upc":"top-1","name":"Trilby","price":1,"__typename":"Product"}`, string(entries[0].Value))
		require.NotNil(t, entries[1])
		assert.Equal(t, `{"upc":"top-2","name":"Fedora","price":2,"__typename":"Product"}`, string(entries[1].Value))
		require.NotNil(t, entries[2])
		assert.Equal(t, `{"upc":"top-3","name":"Boater","price":3,"__typename":"Product"}`, string(entries[2].Value))
	})

	t.Run("subscription entity population not configured - no L2 writes from subscription", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// No SubscriptionEntityPopulation configured
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		productsHost := productsURLParsed.Host

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe without entity population config
		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// No cache operations from subscription (entity population not configured)
		subLog := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry(nil)), sortCacheLogKeys(subLog), "no cache operations when entity population not configured")

		// Query should miss L2 and call products subgraph
		defaultCache.ClearLog()
		tracker.Reset()

		productQuery := `query { product(upc: "top-4") { upc name price } }`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, productQuery, nil, t)
		assert.Equal(t, `{"data":{"product":{"upc":"top-4","name":"Bowler","price":64}}}`, string(resp))

		productsCallsQuery := tracker.GetCount(productsHost)
		assert.Equal(t, 1, productsCallsQuery, "products should be called (no subscription entity population)")
	})

	t.Run("subscription entity + child fetch caching combined", func(t *testing.T) {
		t.Parallel()
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
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		// Subscribe with product entity population AND child entity caching for User
		// Collect 2 events to verify both Product population and User L2 caching
		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 2, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":2,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[1])

		// Accounts called once (event 1 L2 miss, event 2 L2 hit for User entities)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "accounts called once (event 2 hits L2 from event 1)")

		// Verify Product entity was populated in L2 by subscription
		productEntries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(productEntries))
		require.NotNil(t, productEntries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":2,"__typename":"Product"}`, string(productEntries[0].Value))

		// Verify User entities were populated in L2 by child entity caching
		userEntries, err := defaultCache.Get(ctx, []string{
			`{"__typename":"User","key":{"id":"5678"}}`,
			`{"__typename":"User","key":{"id":"8888"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(userEntries))
		require.NotNil(t, userEntries[0], "User 5678 should be in L2 cache")
		require.NotNil(t, userEntries[1], "User 8888 should be in L2 cache")
	})

	t.Run("subscription entity population with header prefix", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		mockHeadersBuilder := &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{
				"products": 11111,
				"accounts": 33333,
				"reviews":  22222,
			},
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify the L2 set used a prefixed key
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`11111:{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription should populate L2 with prefixed key")

		// Verify the cached data directly using the prefixed key
		entries, err := defaultCache.Get(ctx, []string{`11111:{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache with prefixed key")
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`, string(entries[0].Value))
	})

	// =====================================================================
	// Category 3: Subscription entity invalidation (key-only mode)
	// =====================================================================

	t.Run("key-only subscription invalidates L2 cache", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second, EnableInvalidationOnKeyOnly: true},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		entityKey := `{"__typename":"Product","key":{"upc":"top-4"}}`

		// Pre-populate L2 directly with entity cache key
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: entityKey, Value: []byte(`{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`)},
		}, 30*time.Second)
		require.NoError(t, err)

		// Verify product is in cache
		entries, err := defaultCache.Get(ctx, []string{entityKey})
		require.NoError(t, err)
		require.NotNil(t, entries[0], "Product should be in L2 cache before subscription")

		// Subscribe with key-only query → invalidation mode
		defaultCache.ClearLog()

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_key_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])

		// Verify cache delete + User entity resolution
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationDelete, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{false, false}},
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription should delete Product and resolve Users")

		// Verify Product is gone from cache
		entries, err = defaultCache.Get(ctx, []string{entityKey})
		require.NoError(t, err)
		assert.Nil(t, entries[0], "Product should be deleted from L2 cache after invalidation")

		// Verify User entities are cached
		userEntries, err := defaultCache.Get(ctx, []string{
			`{"__typename":"User","key":{"id":"5678"}}`,
			`{"__typename":"User","key":{"id":"8888"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(userEntries))
		require.NotNil(t, userEntries[0])
		assert.Equal(t, `{"__typename":"User","id":"5678","username":"User 5678"}`, string(userEntries[0].Value))
		require.NotNil(t, userEntries[1])
		assert.Equal(t, `{"__typename":"User","id":"8888","username":"User 8888"}`, string(userEntries[1].Value))
	})

	t.Run("key-only subscription WITHOUT invalidation flag - no cache operation", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second, EnableInvalidationOnKeyOnly: false},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		entityKey := `{"__typename":"Product","key":{"upc":"top-4"}}`

		// Pre-populate L2 directly with entity cache key
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: entityKey, Value: []byte(`{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`)},
		}, 30*time.Second)
		require.NoError(t, err)

		// Subscribe with key-only query but invalidation disabled
		defaultCache.ClearLog()

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_key_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])

		// No delete for Product (invalidation disabled), only User entity resolution
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{false, false}},
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "no delete for Product, only User entity resolution")

		// Verify Product is still in cache (not invalidated)
		entries, err := defaultCache.Get(ctx, []string{entityKey})
		require.NoError(t, err)
		require.NotNil(t, entries[0])
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`, string(entries[0].Value))

		// Verify User entities are cached
		userEntries, err := defaultCache.Get(ctx, []string{
			`{"__typename":"User","key":{"id":"5678"}}`,
			`{"__typename":"User","key":{"id":"8888"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(userEntries))
		require.NotNil(t, userEntries[0])
		assert.Equal(t, `{"__typename":"User","id":"5678","username":"User 5678"}`, string(userEntries[0].Value))
		require.NotNil(t, userEntries[1])
		assert.Equal(t, `{"__typename":"User","id":"8888","username":"User 8888"}`, string(userEntries[1].Value))
	})

	t.Run("invalidation on every event", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second, EnableInvalidationOnKeyOnly: true},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		entityKey := `{"__typename":"Product","key":{"upc":"top-4"}}`
		entityValue := []byte(`{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`)

		// Pre-populate L2
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: entityKey, Value: entityValue},
		}, 30*time.Second)
		require.NoError(t, err)

		// Subscribe with key-only query → invalidation mode, collect 2 events
		defaultCache.ClearLog()

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		messages, closeSubscription := gqlClient.Subscription(ctx, wsAddr, cachingTestQueryPath("subscriptions/subscription_product_key_only.query"), queryVariables{"upc": "top-4"}, t)
		t.Cleanup(closeSubscription)

		handle, err := setup.NextProductSubscription(ctx)
		require.NoError(t, err)

		handle.Emit()
		firstMessage := <-messages
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, string(firstMessage))

		handle.Emit()
		secondMessage := <-messages
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, string(secondMessage))

		// Verify 2 delete operations (one per event) + User entity resolution
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationDelete, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{false, false}},
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}},
			{Operation: CacheOperationDelete, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{true, true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "should have 2 delete operations (one per event) + User entity resolution")

		// Verify Product is gone after both events
		entries, err := defaultCache.Get(ctx, []string{entityKey})
		require.NoError(t, err)
		assert.Nil(t, entries[0], "Product should be deleted from L2 after invalidation events")

		// Verify User entities are still cached (set on event 1, hit on event 2)
		userEntries, err := defaultCache.Get(ctx, []string{
			`{"__typename":"User","key":{"id":"5678"}}`,
			`{"__typename":"User","key":{"id":"8888"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(userEntries))
		require.NotNil(t, userEntries[0])
		assert.Equal(t, `{"__typename":"User","id":"5678","username":"User 5678"}`, string(userEntries[0].Value))
		require.NotNil(t, userEntries[1])
		assert.Equal(t, `{"__typename":"User","id":"8888","username":"User 8888"}`, string(userEntries[1].Value))
	})

	// =====================================================================
	// Category 4: Root field caching NOT applied to subscriptions
	// =====================================================================

	t.Run("root field cache config does not apply to subscription root", func(t *testing.T) {
		t.Parallel()
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
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])

		// Verify no root field cache operations for subscription trigger
		// No root field cache operations, only User entity caching
		cacheLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{false, false}},
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(cacheLog), "no root field cache, only User entity caching")

		// Verify User entities are cached with correct values
		userEntries, err := defaultCache.Get(ctx, []string{
			`{"__typename":"User","key":{"id":"5678"}}`,
			`{"__typename":"User","key":{"id":"8888"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(userEntries))
		require.NotNil(t, userEntries[0])
		assert.Equal(t, `{"__typename":"User","id":"5678","username":"User 5678"}`, string(userEntries[0].Value))
		require.NotNil(t, userEntries[1])
		assert.Equal(t, `{"__typename":"User","id":"8888","username":"User 8888"}`, string(userEntries[1].Value))
	})

	// =====================================================================
	// Category 5: Edge cases
	// =====================================================================

	t.Run("multiple subscription events share L2 - second event skips fetch", func(t *testing.T) {
		t.Parallel()
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
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()
		tracker.Reset()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_reviews.query"),
			queryVariables{"upc": "top-4"}, 2, t)

		require.Equal(t, 2, len(messages))
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[0])
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":2,"reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, messages[1])

		// Accounts called exactly once (event 1), event 2 uses L2
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls, "accounts called once (event 2 uses L2 from event 1)")
	})

	t.Run("subscription with @provides skips entity resolution", func(t *testing.T) {
		t.Parallel()
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
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
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

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()
		tracker.Reset()

		// Uses author (with @provides) - no entity resolution for User
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_with_provides.query"),
			queryVariables{"upc": "top-4"}, 2, t)

		require.Equal(t, 2, len(messages))

		// Accounts should never be called (@provides means reviews subgraph provides username)
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 0, accountsCalls, "accounts never called with @provides")

		// No cache operations at all (no entity resolution with @provides)
		cacheLog := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry(nil)), sortCacheLogKeys(cacheLog), "no cache operations with @provides")
	})

	// =====================================================================
	// Category 6: Alias and union edge cases
	// =====================================================================

	t.Run("subscription root field alias - entity population works", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		// Uses alias: "priceUpdate: updateProductPrice(upc: $upc)"
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_alias.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"priceUpdate":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify L2 was populated by subscription (alias doesn't break entity population)
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription with alias should populate L2 with Product entity")

		// Verify cached data
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("subscription union return type - entity population works", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				// Configure for concrete type "Product", not the union "ProductUpdate"
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPriceUnion", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		// Uses union return type: updateProductPriceUnion returns ProductUpdate union
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_union.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPriceUnion":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify L2 was populated (planner resolves union → Product member)
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription with union return type should populate L2 with Product entity")

		// Verify cached data
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"__typename":"Product","upc":"top-4","name":"Bowler","price":1}`, string(entries[0].Value))
	})

	t.Run("subscription interface return type - entity population works", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				// Configure for concrete type "Product", not the interface "ProductInterface"
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPriceInterface", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		// Uses interface return type: updateProductPriceInterface returns ProductInterface
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_product_interface.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPriceInterface":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		// Verify L2 was populated (planner resolves interface → Product implementor)
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "subscription with interface return type should populate L2 with Product entity")

		// Verify cached data
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0], "Product entity should be in L2 cache")
		assert.Equal(t, `{"__typename":"Product","upc":"top-4","name":"Bowler","price":1}`, string(entries[0].Value))
	})

	t.Run("subscription union return type - unconfigured type not cached", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Configure entity population for Product only, NOT DigitalProduct.
		// The union ProductUpdate = Product | DigitalProduct, but the planner picks
		// Product's config. At runtime, DigitalProduct is returned and its __typename
		// doesn't match → filtered out → no L2 cache write.
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateDigitalProductPriceUnion", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		// Subscribe via union field that returns DigitalProduct (not Product)
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_digital_product_union.query"),
			queryVariables{"upc": "digital-1"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateDigitalProductPriceUnion":{"upc":"digital-1","name":"eBook: GraphQL in Action","price":1}}}}`, messages[0])

		// No cache operations: DigitalProduct's __typename doesn't match configured "Product"
		subLog := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry(nil)), sortCacheLogKeys(subLog), "no cache operations for unconfigured DigitalProduct type")

		// Verify neither Product nor DigitalProduct keys are in cache
		productEntries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"digital-1"}}`})
		require.NoError(t, err)
		assert.Nil(t, productEntries[0], "Product key should not be in cache")

		digitalEntries, err := defaultCache.Get(ctx, []string{`{"__typename":"DigitalProduct","key":{"upc":"digital-1"}}`})
		require.NoError(t, err)
		assert.Nil(t, digitalEntries[0], "DigitalProduct key should not be in cache")
	})

	t.Run("subscription interface return type - unconfigured type not cached", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Configure entity population for Product only, NOT DigitalProduct.
		// The interface ProductInterface is implemented by Product and DigitalProduct,
		// but the planner picks Product's config. At runtime, DigitalProduct is returned
		// and its __typename doesn't match → filtered out → no L2 cache write.
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateDigitalProductPriceInterface", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)

		defaultCache.ClearLog()

		// Subscribe via interface field that returns DigitalProduct (not Product)
		messages := collectSubscriptionMessages(ctx, gqlClient, setup, wsAddr,
			cachingTestQueryPath("subscriptions/subscription_digital_product_interface.query"),
			queryVariables{"upc": "digital-1"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateDigitalProductPriceInterface":{"upc":"digital-1","name":"eBook: GraphQL in Action","price":1}}}}`, messages[0])

		// No cache operations: DigitalProduct's __typename doesn't match configured "Product"
		subLog := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry(nil)), sortCacheLogKeys(subLog), "no cache operations for unconfigured DigitalProduct type")

		// Verify neither Product nor DigitalProduct keys are in cache
		productEntries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"digital-1"}}`})
		require.NoError(t, err)
		assert.Nil(t, productEntries[0], "Product key should not be in cache")

		digitalEntries, err := defaultCache.Get(ctx, []string{`{"__typename":"DigitalProduct","key":{"upc":"digital-1"}}`})
		require.NoError(t, err)
		assert.Nil(t, digitalEntries[0], "DigitalProduct key should not be in cache")
	})

	// =====================================================================
	// Category 7: Trigger-level cache deduplication
	// =====================================================================

	t.Run("entity population happens once per trigger event with multiple subscriptions", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		queryPath := cachingTestQueryPath("subscriptions/subscription_product_only.query")
		vars := queryVariables{"upc": "top-4"}

		// Start 2 subscriptions to the same query/variables (same trigger)
		messages1, close1 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close1)
		messages2, close2 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close2)

		handle, err := setup.NextProductSubscription(ctx)
		require.NoError(t, err)

		handle.Emit()

		var msg1, msg2 string
		for msg1 == "" || msg2 == "" {
			select {
			case m := <-messages1:
				msg1 = string(m)
			case m := <-messages2:
				msg2 = string(m)
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for first messages")
			}
		}

		assert.Equal(t, msg1, msg2, "both clients should receive the same event")
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, msg1)

		// ClearLog and collect second event to measure deduplication
		defaultCache.ClearLog()
		setNotification := defaultCache.WaitForOperation(CacheOperationSet, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})

		handle.Emit()

		var msg1b, msg2b string
		for msg1b == "" || msg2b == "" {
			select {
			case m := <-messages1:
				msg1b = string(m)
			case m := <-messages2:
				msg2b = string(m)
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for second messages")
			}
		}

		assert.Equal(t, msg1b, msg2b, "both clients should receive the same event")
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":2}}}}`, msg1b)

		// Close subscriptions before cache log assertions
		close1()
		close2()

		select {
		case entry, ok := <-setNotification:
			require.True(t, ok, "set notification channel should be closed after delivery")
			assert.Equal(t, CacheLogEntry{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
				Hits:      nil,
				TTL:       30 * time.Second,
			}, entry)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Product cache population")
		}

		// Verify exactly 1 set operation (deduplicated, not 2)
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "should have exactly 1 L2 set for Product (deduplicated, not 2)")

		// Verify cached Product value
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0])
		assert.Equal(t, `{"upc":"top-4","name":"Bowler","price":2,"__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("entity invalidation happens once per trigger event with multiple subscriptions", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second, EnableInvalidationOnKeyOnly: true},
				},
			},
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		entityKey := `{"__typename":"Product","key":{"upc":"top-4"}}`

		// Pre-populate L2
		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: entityKey, Value: []byte(`{"upc":"top-4","name":"Bowler","price":64,"__typename":"Product"}`)},
		}, 30*time.Second)
		require.NoError(t, err)

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		queryPath := cachingTestQueryPath("subscriptions/subscription_product_key_only.query")
		vars := queryVariables{"upc": "top-4"}

		// Start 2 subscriptions to the same key-only query (same trigger)
		messages1, close1 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close1)
		messages2, close2 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close2)

		handle, err := setup.NextProductSubscription(ctx)
		require.NoError(t, err)

		handle.Emit()

		var msg1, msg2 string
		for msg1 == "" || msg2 == "" {
			select {
			case m := <-messages1:
				msg1 = string(m)
			case m := <-messages2:
				msg2 = string(m)
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for first messages")
			}
		}

		assert.Equal(t, msg1, msg2, "both clients should receive the same event")
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, msg1)

		// ClearLog and collect second event to measure deduplication
		defaultCache.ClearLog()
		deleteNotification := defaultCache.WaitForOperation(CacheOperationDelete, []string{entityKey})

		handle.Emit()

		var msg1b, msg2b string
		for msg1b == "" || msg2b == "" {
			select {
			case m := <-messages1:
				msg1b = string(m)
			case m := <-messages2:
				msg2b = string(m)
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for second messages")
			}
		}

		assert.Equal(t, msg1b, msg2b, "both clients should receive the same event")
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","reviews":[{"body":"Perfect summer hat.","authorWithoutProvides":{"username":"User 5678"}},{"body":"A bit too fancy for my taste.","authorWithoutProvides":{"username":"User 8888"}}]}}}}`, msg1b)

		// Close subscriptions before cache log assertions
		close1()
		close2()

		select {
		case entry, ok := <-deleteNotification:
			require.True(t, ok, "delete notification channel should be closed after delivery")
			assert.Equal(t, CacheLogEntry{
				Operation: CacheOperationDelete,
				Keys:      []string{entityKey},
				Hits:      nil,
				TTL:       0,
			}, entry)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Product cache invalidation")
		}

		// Verify exactly 1 delete (deduplicated) + User entity resolution with L2 hits
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationDelete, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{true, true}},
			{Operation: CacheOperationGet, Keys: []string{`{"__typename":"User","key":{"id":"5678"}}`, `{"__typename":"User","key":{"id":"8888"}}`}, Hits: []bool{true, true}},
		}
		subLog := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "should have exactly 1 L2 delete for Product (deduplicated, not 2)")

		// Verify entity is gone from cache
		entries, err := defaultCache.Get(ctx, []string{entityKey})
		require.NoError(t, err)
		assert.Nil(t, entries[0], "Product should be deleted from L2 cache after invalidation")
	})

	t.Run("three clients - cache operations still happen once", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "products",
				SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
					{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		wsAddr := toWSAddr(setup.GatewayServer.URL)
		queryPath := cachingTestQueryPath("subscriptions/subscription_product_only.query")
		vars := queryVariables{"upc": "top-4"}

		// Start 3 subscriptions to the same query/variables (same trigger)
		messages1, close1 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close1)
		messages2, close2 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close2)
		messages3, close3 := gqlClient.Subscription(ctx, wsAddr, queryPath, vars, t)
		t.Cleanup(close3)

		handle, err := setup.NextProductSubscription(ctx)
		require.NoError(t, err)

		// Shared-trigger subscriptions are attached asynchronously after the upstream
		// handle is created. On Windows, the third client can miss an immediate first
		// emit, so warm up until all three clients have observed at least one event.
		firstSeen := [3]bool{}
		warmupEmits := 0
		warmupCtx, warmupCancel := context.WithTimeout(ctx, 5*time.Second)
		defer warmupCancel()
		for !firstSeen[0] || !firstSeen[1] || !firstSeen[2] {
			handle.Emit()
			warmupEmits++

			settleTimer := time.NewTimer(200 * time.Millisecond)
		collectWarmup:
			for {
				select {
				case <-messages1:
					firstSeen[0] = true
					if !settleTimer.Stop() {
						select {
						case <-settleTimer.C:
						default:
						}
					}
					settleTimer.Reset(200 * time.Millisecond)
				case <-messages2:
					firstSeen[1] = true
					if !settleTimer.Stop() {
						select {
						case <-settleTimer.C:
						default:
						}
					}
					settleTimer.Reset(200 * time.Millisecond)
				case <-messages3:
					firstSeen[2] = true
					if !settleTimer.Stop() {
						select {
						case <-settleTimer.C:
						default:
						}
					}
					settleTimer.Reset(200 * time.Millisecond)
				case <-settleTimer.C:
					break collectWarmup
				case <-warmupCtx.Done():
					t.Fatalf("timeout waiting for first messages, received %d of 3", boolToInt(firstSeen[0])+boolToInt(firstSeen[1])+boolToInt(firstSeen[2]))
				}
			}
		}

		// Drain any extra warm-up messages from already-attached clients so the next
		// emit is the only source of messages in the measured phase.
		drainTimer := time.NewTimer(200 * time.Millisecond)
	drainWarmup:
		for {
			select {
			case <-messages1:
				if !drainTimer.Stop() {
					select {
					case <-drainTimer.C:
					default:
					}
				}
				drainTimer.Reset(200 * time.Millisecond)
			case <-messages2:
				if !drainTimer.Stop() {
					select {
					case <-drainTimer.C:
					default:
					}
				}
				drainTimer.Reset(200 * time.Millisecond)
			case <-messages3:
				if !drainTimer.Stop() {
					select {
					case <-drainTimer.C:
					default:
					}
				}
				drainTimer.Reset(200 * time.Millisecond)
			case <-drainTimer.C:
				break drainWarmup
			}
		}

		// ClearLog and collect second event to measure deduplication
		defaultCache.ClearLog()
		setNotification := defaultCache.WaitForOperation(CacheOperationSet, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})

		handle.Emit()

		received := 0
		for received < 3 {
			select {
			case <-messages1:
				received++
			case <-messages2:
				received++
			case <-messages3:
				received++
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout waiting for second messages, received %d of 3", received)
			}
		}

		// Close subscriptions before cache log assertions
		close1()
		close2()
		close3()

		select {
		case entry, ok := <-setNotification:
			require.True(t, ok, "set notification channel should be closed after delivery")
			assert.Equal(t, CacheLogEntry{
				Operation: CacheOperationSet,
				Keys:      []string{`{"__typename":"Product","key":{"upc":"top-4"}}`},
				Hits:      nil,
				TTL:       30 * time.Second,
			}, entry)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Product cache population")
		}

		// Verify exactly 1 set operation (deduplicated, not 3)
		subLog := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(subLog), "should have exactly 1 L2 set for Product (deduplicated, not 3)")

		// Verify cached Product value
		entries, err := defaultCache.Get(ctx, []string{`{"__typename":"Product","key":{"upc":"top-4"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0])
		assert.Equal(t, fmt.Sprintf(`{"upc":"top-4","name":"Bowler","price":%d,"__typename":"Product"}`, warmupEmits+1), string(entries[0].Value))
	})

	// =====================================================================
	// Category 5: Tier 1 field-name disambiguation
	// =====================================================================

	t.Run("subscription field-name disambiguation - updateProductPrice uses 30s TTL", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
						// Two configs for the same entity type, disambiguated by FieldName (Tier 1)
						{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
						{TypeName: "Product", FieldName: "updatedPrice", CacheName: "default", TTL: 60 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, toWSAddr(setup.GatewayServer.URL),
			cachingTestQueryPath("subscriptions/subscription_product_only.query"),
			queryVariables{"upc": "top-4"}, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}}`, messages[0])

		log := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"Product","key":{"upc":"top-4"}}`}, TTL: 30 * time.Second}, // Tier 1 match: updateProductPrice config selected (30s), not updatedPrice (60s)
		}, log)
	})

	t.Run("subscription field-name disambiguation - updatedPrice uses 60s TTL", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()

		setup := federationtesting.NewManualFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					SubscriptionEntityPopulation: plan.SubscriptionEntityPopulationConfigurations{
						// Same two configs — this time exercising the updatedPrice field
						{TypeName: "Product", FieldName: "updateProductPrice", CacheName: "default", TTL: 30 * time.Second},
						{TypeName: "Product", FieldName: "updatedPrice", CacheName: "default", TTL: 60 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		defaultCache.ClearLog()

		messages := collectSubscriptionMessages(ctx, gqlClient, setup, toWSAddr(setup.GatewayServer.URL),
			cachingTestQueryPath("subscriptions/subscription_updated_price.query"),
			nil, 1, t)
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updatedPrice":{"upc":"top-3","name":"Boater","price":10}}}}`, messages[0])

		log := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: CacheOperationSet, Keys: []string{`{"__typename":"Product","key":{"upc":"top-3"}}`}, TTL: 60 * time.Second}, // Tier 1 match: updatedPrice config selected (60s), not updateProductPrice (30s)
		}, log)
	})
}
