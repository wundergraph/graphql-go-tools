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

// TestRootFieldCachingWithArgs verifies L2 caching for root fields with arguments,
// including EntityKeyMappings that derive entity-level cache keys from argument values.
func TestRootFieldCachingWithArgs(t *testing.T) {
	t.Parallel()
	t.Run("root field with args - miss then hit", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query cache log should match")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts subgraph once")

		// Second query - cache hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have 1 cache get (hit)")
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query should hit cache")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts subgraph (cache hit)")
	})

	t.Run("root field with args - different args different keys", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query should miss cache and set")

		// Second query with id=5678 - different cache key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "5678"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"5678","username":"User 5678"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Second query with different id should call accounts once")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterSecond), "Second query with different id should have get miss + set")
		wantLog := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"5678"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"5678"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLog), sortCacheLogEntries(logAfterSecond), "Different args should produce different cache keys")

		// Third query with id=1234 - should hit cache from first query
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Third query (same as first) should hit cache")

		logAfterThird := defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogThird), sortCacheLogEntries(logAfterThird), "Third query should hit cache from first query")
	})

	t.Run("entity key mapping - uses entity key format", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLog), sortCacheLogEntries(logAfterFirst), "Should use entity key format, not root field format")
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		// Second query - should hit cache using entity key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should hit cache")
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query should hit entity cache key")
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")
	})

	t.Run("entity key mapping - invalidation via entity key", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogDelete), sortCacheLogEntries(logAfterDelete), "After deletion: get miss + set")
	})

	t.Run("entity key mapping - cross-lookup from entity fetch", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Root field query should use entity key format")

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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Entity fetch should use same key format as root field entity key mapping")
	})

	t.Run("entity key mapping - cross-lookup from root field", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query should miss all caches and set")

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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Root field should hit cache from entity fetch data")
	})

	t.Run("entity key mapping + header prefix", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `33333:{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `33333:{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLog), sortCacheLogEntries(logAfterFirst), "Entity key should have header prefix")
	})

	t.Run("root field without args - regression", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Should use root field key format (no entity key mapping)")

		// Second query - hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { topProducts { name } }`, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query should hit cache")
	})

	t.Run("root field caching + entity caching nested", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`, TTL: 30 * time.Second}}},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query should miss both root field and entity cache")

		// Second identical query - all from cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query { product(upc: "top-1") { name reviews { body } } }`, queryVariables{"upc": "top-1"}, t)
		assert.Equal(t, `{"data":{"product":{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(productsHost), "Second query should skip products (root field cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "Second query should skip reviews (entity cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"product","args":{"upc":"top-1"}}`, Hit: true}}},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query should hit both root field and entity cache")
	})

	t.Run("TTL expiry", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		for i := range 10 {
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
		for i := range 10 {
			id := strconv.Itoa(i + 1000)
			expected := fmt.Sprintf(`{"data":{"user":{"id":"%s","username":"User %s"}}}`, id, id)
			assert.Equal(t, expected, results[i], "Concurrent query %d should return correct result", i)
		}
	})

	t.Run("two args - reversed argument order hits cache", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query cache log should match")

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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"userByIdAndName","args":{"id":"1234","username":"Me"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query (reversed args) should hit cache with identical key")
	})

	t.Run("root field more fields then fewer fields - cache hit (superset)", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query cache log should match")

		// Second query: fetch FEWER fields (username only) - should be cache HIT
		// The cached data has {username, realName}, the query only needs {username} → superset → hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query (fewer fields) should be a cache HIT because cached data is a superset")
	})

	t.Run("root field fewer fields then more fields - cache miss (subset)", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First query cache log should match")

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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: true}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second query should find stale cache entry but re-fetch because cached data is only a subset")

		// Third query: same more-fields query - should now hit cache (re-fetch populated it)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query($id: ID!) { user(id: $id) { username realName } }`, queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"username":"Me","realName":"Real Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Third query should skip accounts (cache hit after re-fetch)")

		logAfterThird := defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"user","args":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogThird), sortCacheLogEntries(logAfterThird), "Third query should hit cache with full data from re-fetch")
	})

	t.Run("entity key mapping - multiple keys single mapping", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Single mapping: only id key, not combined id+username")

		// Second query - hit via entity key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Should hit cache via entity key")
	})

	t.Run("entity key mapping - multiple keys multiple mappings", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Multiple mappings: data stored under both id and username keys")

		// Second query - hit (via either key)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: true},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Both keys should hit cache")
	})

	t.Run("entity key mapping - multiple mappings partial args", func(t *testing.T) {
		t.Parallel()
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

		// First query - miss on id key, then response data backfills the sibling username key too
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst), "Should have get miss + set (id key plus response-derived username key)")
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "The response supplies username, so both entity keys are written")

		// Second query - hit via id key
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 1, len(logAfterSecond), "Second query should have single get hit")
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Single id key should hit cache")
	})

	t.Run("entity key mapping - multiple mappings cross-lookup", func(t *testing.T) {
		t.Parallel()
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
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Root field should store under both id and username entity keys")

		// Second: Entity fetch for User 1234 via topProducts → reviews → authorWithoutProvides
		// Entity fetch uses @key(fields: "id") → finds data stored under id key by root field
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Entity fetch should skip accounts (cross-lookup hit: root field stored under id key)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second}}},
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			}},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Entity fetch should cross-lookup User via id key stored by root field")
	})

	t.Run("root field not configured - still calls subgraph", func(t *testing.T) {
		t.Parallel()
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

	t.Run("entity key mapping - two root fields asymmetric key coverage", func(t *testing.T) {
		t.Parallel()
		// userByIdAndName provides both args → 2 cache keys (id + username).
		// user(id) provides only id → 1 cache key.
		// Step 1: userByIdAndName writes under both keys.
		// Step 2: user(id) reads via id key → hit from step 1.
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
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
						{
							TypeName:  "Query",
							FieldName: "user",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: userByIdAndName — both mappings resolve → 2 reads (miss), 2 writes
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Both mappings resolved: data stored under id and username keys")

		// Step 2: user(id) — only id mapping resolves → 1 read (hit via id key)
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip accounts (cache hit via id key)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "user(id) should hit cache via id key stored by userByIdAndName")
	})
}

// TestRootFieldCachingWithArgs_PartialKeyWrite verifies that when only some EntityKeyMappings
// match the request arguments, only those matching keys are written to L2.
func TestRootFieldCachingWithArgs_PartialKeyWrite(t *testing.T) {
	t.Parallel()
	t.Run("entity key mapping - partial key write does not generate extra keys from response", func(t *testing.T) {
		t.Parallel()
		// Documents current behavior: when user(id) is queried with only the id
		// mapping matching, the write stores under the id key only.
		// The username key is NOT generated from the fetched response data.
		// Verified via Peek: id key exists, username key does not.
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
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "user",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// user(id) — id mapping resolves from args, username key is derived from the fetched response
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Fetched response should backfill the username key too")

		// Direct cache inspection: both keys present
		_, idExists := defaultCache.Peek(`{"__typename":"User","key":{"id":"1234"}}`)
		assert.True(t, idExists, "id key should be in cache")
		_, usernameExists := defaultCache.Peek(`{"__typename":"User","key":{"username":"Me"}}`)
		assert.True(t, usernameExists, "username key should be in cache once the response reveals it")
	})

	t.Run("entity key mapping - flat key cross-lookup from composite key write", func(t *testing.T) {
		t.Parallel()
		// userByIdAndName configured with flat @key(fields: "id") + composite key
		// using id+username together as a single mapping.
		// user(id) configured with flat @key(fields: "id") only.
		// Step 1: userByIdAndName writes under both keys (flat id + composite id+username).
		// Step 2: user(id) reads via flat id key → hit from step 1.
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
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
						{
							TypeName:  "Query",
							FieldName: "user",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Step 1: userByIdAndName — both mappings resolve → 2 reads (miss), 2 writes
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should call accounts once")

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"id":"1234","username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"id":"1234","username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "Both flat id and composite id+username keys written")

		// Step 2: user(id) — flat id mapping only → hit via flat id key from step 1
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id.query"), queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Should skip accounts (flat id key hit)")

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Flat id key cross-lookup succeeds from composite key write")
	})
}

// TestRootFieldCachingWithArgs_BothKeysHit verifies that when both EntityKeyMappings
// are populated, a second request hits both keys and skips the subgraph entirely.
func TestRootFieldCachingWithArgs_BothKeysHit(t *testing.T) {
	t.Parallel()

	t.Run("both entity key mappings hit on second request", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`,
			queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "First query should fetch from subgraph")

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}), sortCacheLogEntries(logAfterFirst))

		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`,
			queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second query should skip subgraph (cache hit)")

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: true},
			}},
		}), sortCacheLogEntries(logAfterSecond))
	})
}

// TestRootFieldCachingWithArgs_SeededDifferentData verifies that when L2 has conflicting
// data under different entity key mappings, the fresher entry wins during merge.
func TestRootFieldCachingWithArgs_SeededDifferentData(t *testing.T) {
	t.Parallel()

	t.Run("seeded L2 with different data under each key - fresher entry wins", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		idKey := `{"__typename":"User","key":{"id":"1234"}}`
		usernameKey := `{"__typename":"User","key":{"username":"Me"}}`

		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: idKey, Value: []byte(`{"id":"1234","username":"FreshName"}`), TTL: 30 * time.Second},
		})
		require.NoError(t, err)
		err = defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: usernameKey, Value: []byte(`{"id":"1234","username":"StaleName"}`), TTL: 10 * time.Second},
		})
		require.NoError(t, err)

		setupLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 30 * time.Second}}},
			{Operation: "set", Items: []CacheLogItem{{Key: usernameKey, TTL: 10 * time.Second}}},
		}, setupLog)

		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`,
			queryVariables{"id": "1234", "username": "Me"}, t)

		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"FreshName"}}}`, string(resp),
			"desired behavior serves the freshest cached entry when both keys hit")
		assert.Equal(t, 0, tracker.GetCount(accountsHost),
			"Should skip subgraph fetch since the selected cached entry passes validation")

		idData, idExists := defaultCache.Peek(idKey)
		assert.True(t, idExists)
		assert.Equal(t, `{"id":"1234","username":"FreshName"}`, string(idData))
		usernameData, usernameExists := defaultCache.Peek(usernameKey)
		assert.True(t, usernameExists)
		assert.Equal(t, `{"id":"1234","username":"StaleName"}`, string(usernameData))

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: true},
			}},
		}), sortCacheLogEntries(logAfterQuery))
	})
}

// TestRootFieldCachingWithArgs_ComplementaryPartialData verifies that two partial cache entries
// under different entity key mappings are merged into a complete hit, skipping the subgraph.
func TestRootFieldCachingWithArgs_ComplementaryPartialData(t *testing.T) {
	t.Parallel()

	t.Run("complementary partial data merges into a complete cache hit", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		idKey := `{"__typename":"User","key":{"id":"1234"}}`
		usernameKey := `{"__typename":"User","key":{"username":"Me"}}`

		err := defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: idKey, Value: []byte(`{"id":"1234","username":"Me"}`), TTL: 20 * time.Second},
		})
		require.NoError(t, err)
		err = defaultCache.Set(ctx, []*resolve.CacheEntry{
			{Key: usernameKey, Value: []byte(`{"id":"1234","nickname":"nick-Me"}`), TTL: 30 * time.Second},
		})
		require.NoError(t, err)

		setupLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 20 * time.Second}}},
			{Operation: "set", Items: []CacheLogItem{{Key: usernameKey, TTL: 30 * time.Second}}},
		}, setupLog)

		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username nickname } }`,
			queryVariables{"id": "1234", "username": "Me"}, t)

		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me","nickname":"nick-Me"}}}`, string(resp))
		assert.Equal(t, 0, tracker.GetCount(accountsHost),
			"desired behavior merges complementary cache hits and skips the subgraph fetch")

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: idKey, Hit: true},
				{Key: usernameKey, Hit: true},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: idKey, TTL: 30 * time.Second},
				{Key: usernameKey, TTL: 30 * time.Second},
			}},
		}), sortCacheLogEntries(logAfterQuery))

		idData, idExists := defaultCache.Peek(idKey)
		assert.True(t, idExists)
		assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(idData))
		usernameData, usernameExists := defaultCache.Peek(usernameKey)
		assert.True(t, usernameExists)
		assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(usernameData))
	})
}

// TestRootFieldCachingWithArgs_KeyPopulationAndBackfill verifies that a full-args query
// populates all entity key mappings, and subsequent single-arg queries hit the correct key.
func TestRootFieldCachingWithArgs_KeyPopulationAndBackfill(t *testing.T) {
	t.Parallel()

	t.Run("5a - full arg query populates both keys verified via Peek", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "userByIdAndName",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`,
			queryVariables{"id": "1234", "username": "Me"}, t)
		assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should fetch from subgraph")

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}), sortCacheLogEntries(logAfterQuery))

		idData, idExists := defaultCache.Peek(`{"__typename":"User","key":{"id":"1234"}}`)
		assert.True(t, idExists, "id key should exist after full-arg query")
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(idData))

		usernameData, usernameExists := defaultCache.Peek(`{"__typename":"User","key":{"username":"Me"}}`)
		assert.True(t, usernameExists, "username key should exist after full-arg query")
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(usernameData))
	})

	t.Run("5b - partial arg query backfills username key from response", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "user",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "id", ArgumentPath: []string{"id"}},
								}},
								{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "username", ArgumentPath: []string{"username"}},
								}},
							},
						},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query($id: ID!) { user(id: $id) { id username } }`,
			queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should fetch from subgraph")

		logAfterQuery := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
			}},
		}), sortCacheLogEntries(logAfterQuery))

		idData, idExists := defaultCache.Peek(`{"__typename":"User","key":{"id":"1234"}}`)
		assert.True(t, idExists, "id key should exist")
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(idData))
		usernameData, usernameExists := defaultCache.Peek(`{"__typename":"User","key":{"username":"Me"}}`)
		assert.True(t, usernameExists, "username key should be backfilled from the fetched response")
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(usernameData))
	})
}

// TestRootFieldCachingWithArgs_BackfillAfterPartialHit verifies that a cache hit on one
// entity key mapping backfills the missing sibling key when the cached entity has the data.
func TestRootFieldCachingWithArgs_BackfillAfterPartialHit(t *testing.T) {
	t.Parallel()

	// Scenario: the root field asks for id + username keys, only the id key is in
	// L2, and that cached entity already contains username. The request should be
	// served from cache, the missing username key should be backfilled, and the
	// existing id key should not be rewritten.
	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphCallTracker(http.DefaultTransport)

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(&http.Client{Transport: tracker}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:  "Query",
						FieldName: "userByIdAndName",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "username", ArgumentPath: []string{"username"}},
							}},
						},
					},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	idKey := `{"__typename":"User","key":{"id":"1234"}}`
	usernameKey := `{"__typename":"User","key":{"username":"Me"}}`

	// Seed only the id key with an entity that already proves username.
	err := defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: idKey, Value: []byte(`{"id":"1234","username":"Me"}`), TTL: 20 * time.Second},
	})
	require.NoError(t, err)

	setupLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 20 * time.Second}}},
	}, setupLog)

	defaultCache.ClearLog()
	tracker.Reset()
	// Make the root-field request that asks for both id and username mappings.
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`,
		queryVariables{"id": "1234", "username": "Me"}, t)

	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(accountsHost))

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only id.
	// 2. L2 writes only the missing username key.
	logAfterQuery := defaultCache.GetLog()
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: idKey, Hit: true},
			{Key: usernameKey, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{{Key: usernameKey, TTL: 30 * time.Second}}},
	}), sortCacheLogEntries(logAfterQuery))

	// Assert the pre-existing id entry is unchanged and the username key now points
	// at the same entity payload.
	idData, idExists := defaultCache.Peek(idKey)
	assert.True(t, idExists)
	assert.Equal(t, `{"id":"1234","username":"Me"}`, string(idData))
	usernameData, usernameExists := defaultCache.Peek(usernameKey)
	assert.True(t, usernameExists, "cache-hit serve should backfill the missing sibling key")
	assert.Equal(t, `{"id":"1234","username":"Me"}`, string(usernameData))
}

// TestRootFieldCachingWithArgs_BackfillRequiresFieldProof verifies that a missing sibling key
// is NOT backfilled when the cached entity lacks the field needed for that key mapping.
func TestRootFieldCachingWithArgs_BackfillRequiresFieldProof(t *testing.T) {
	t.Parallel()

	// Scenario: the root field asks for id + username keys, only the id key is in
	// L2, and the cached entity does not contain username. The request can still be
	// served from cache because it asks for id only, but the missing username key
	// must not be backfilled from request args alone.
	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphCallTracker(http.DefaultTransport)

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(&http.Client{Transport: tracker}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:  "Query",
						FieldName: "userByIdAndName",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "username", ArgumentPath: []string{"username"}},
							}},
						},
					},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	idKey := `{"__typename":"User","key":{"id":"1234"}}`
	usernameKey := `{"__typename":"User","key":{"username":"Me"}}`

	// Seed only the id key and deliberately omit username from the cached entity.
	err := defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: idKey, Value: []byte(`{"id":"1234"}`), TTL: 20 * time.Second},
	})
	require.NoError(t, err)

	setupLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 20 * time.Second}}},
	}, setupLog)

	defaultCache.ClearLog()
	tracker.Reset()
	// Make a request that only needs id in the response, so the cache-only path is still valid.
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id } }`,
		queryVariables{"id": "1234", "username": "Me"}, t)

	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234"}}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(accountsHost))

	// Assert the exact cache story:
	// 1. L2 reads both requested keys and finds only id.
	// 2. No write happens because the cached entity never proves username.
	logAfterQuery := defaultCache.GetLog()
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: idKey, Hit: true},
			{Key: usernameKey, Hit: false},
		}},
	}), sortCacheLogEntries(logAfterQuery))

	// Assert the id entry remains as seeded and the username key stays absent.
	idData, idExists := defaultCache.Peek(idKey)
	assert.True(t, idExists)
	assert.Equal(t, `{"id":"1234"}`, string(idData))
	_, usernameExists := defaultCache.Peek(usernameKey)
	assert.False(t, usernameExists, "missing sibling key must not be backfilled from request args alone")
}

// TestRootFieldCachingWithArgs_DerivedKeyExpansionAfterFetch verifies that after a subgraph fetch,
// all entity key mappings are populated including derived keys not in the request arguments.
func TestRootFieldCachingWithArgs_DerivedKeyExpansionAfterFetch(t *testing.T) {
	t.Parallel()

	// Scenario: the root field asks for id + username keys, but the cache config
	// also has a third nickname mapping. Only id is seeded, so the fetch runs. The
	// fetched entity should refresh id, backfill username, and add the extra
	// nickname key derived from final entity data.
	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphCallTracker(http.DefaultTransport)

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(&http.Client{Transport: tracker}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:  "Query",
						FieldName: "userByIdAndName",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "username", ArgumentPath: []string{"username"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "nickname", ArgumentPath: []string{"nickname"}},
							}},
						},
					},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	idKey := `{"__typename":"User","key":{"id":"1234"}}`
	usernameKey := `{"__typename":"User","key":{"username":"Me"}}`
	nicknameKey := `{"__typename":"User","key":{"nickname":"nick-Me"}}`

	// Seed only the id key so the request has one cache hit and one requested miss.
	err := defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: idKey, Value: []byte(`{"id":"1234"}`), TTL: 20 * time.Second},
	})
	require.NoError(t, err)

	setupLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 20 * time.Second}}},
	}, setupLog)

	defaultCache.ClearLog()
	tracker.Reset()
	// Make the root-field request. The response returns id, username, and nickname.
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username nickname } }`,
		queryVariables{"id": "1234", "username": "Me"}, t)

	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me","nickname":"nick-Me"}}}`, string(resp))
	assert.Equal(t, 1, tracker.GetCount(accountsHost))

	// Assert the exact cache story:
	// 1. L2 reads the requested id + username keys and finds only id.
	// 2. The fetch writes id refresh + username backfill + nickname derived key.
	logAfterQuery := defaultCache.GetLog()
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: idKey, Hit: true},
			{Key: usernameKey, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: idKey, TTL: 30 * time.Second},
			{Key: usernameKey, TTL: 30 * time.Second},
			{Key: nicknameKey, TTL: 30 * time.Second},
		}},
	}), sortCacheLogEntries(logAfterQuery))

	// Assert all three keys now point at the same final entity payload.
	idData, idExists := defaultCache.Peek(idKey)
	assert.True(t, idExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(idData))
	usernameData, usernameExists := defaultCache.Peek(usernameKey)
	assert.True(t, usernameExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(usernameData))
	nicknameData, nicknameExists := defaultCache.Peek(nicknameKey)
	assert.True(t, nicknameExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(nicknameData))
}

// TestRootFieldCachingWithArgs_FallbackAfterPartialSelection verifies that when multiple
// cached entries exist but disagree, the system falls back to a subgraph fetch.
func TestRootFieldCachingWithArgs_FallbackAfterPartialSelection(t *testing.T) {
	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphCallTracker(http.DefaultTransport)

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(&http.Client{Transport: tracker}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:  "Query",
						FieldName: "userByIdAndName",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "username", ArgumentPath: []string{"username"}},
							}},
						},
					},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host

	err := defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: `{"__typename":"User","key":{"id":"1234"}}`, Value: []byte(`{"id":"1234","username":"Me","nickname":"nick-Me"}`), TTL: 10 * time.Second},
	})
	require.NoError(t, err)
	err = defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: `{"__typename":"User","key":{"username":"Me"}}`, Value: []byte(`{"id":"1234"}`), TTL: 30 * time.Second},
	})
	require.NoError(t, err)

	setupLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 10 * time.Second}}},
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second}}},
	}, setupLog)

	defaultCache.ClearLog()
	tracker.Reset()
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username nickname } }`,
		queryVariables{"id": "1234", "username": "Me"}, t)

	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me","nickname":"nick-Me"}}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(accountsHost), "desired behavior resolves fresh-incomplete vs stale-complete from cache without a fetch")

	logAfterQuery := defaultCache.GetLog()
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true},
			{Key: `{"__typename":"User","key":{"username":"Me"}}`, Hit: true},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second},
			{Key: `{"__typename":"User","key":{"username":"Me"}}`, TTL: 30 * time.Second},
		}},
	}), sortCacheLogEntries(logAfterQuery))

	idData, idExists := defaultCache.Peek(`{"__typename":"User","key":{"id":"1234"}}`)
	assert.True(t, idExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(idData))
	usernameData, usernameExists := defaultCache.Peek(`{"__typename":"User","key":{"username":"Me"}}`)
	assert.True(t, usernameExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(usernameData))
}

// TestRootFieldCachingWithArgs_MergeConflictWholeEntrySelection verifies that when the merge
// selects the whole entry (rather than individual fields), the result is consistent.
func TestRootFieldCachingWithArgs_MergeConflictWholeEntrySelection(t *testing.T) {
	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphCallTracker(http.DefaultTransport)

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
		withHTTPClient(&http.Client{Transport: tracker}),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				RootFieldCaching: plan.RootFieldCacheConfigurations{
					{
						TypeName:  "Query",
						FieldName: "userByIdAndName",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							}},
							{EntityTypeName: "User", FieldMappings: []plan.FieldMapping{
								{EntityKeyField: "username", ArgumentPath: []string{"username"}},
							}},
						},
					},
				},
			},
		}),
	))
	t.Cleanup(setup.Close)
	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	idKey := `{"__typename":"User","key":{"id":"1234"}}`
	usernameKey := `{"__typename":"User","key":{"username":"Me"}}`

	err := defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: idKey, Value: []byte(`{"id":"1234","username":"OldName"}`), TTL: 20 * time.Second},
	})
	require.NoError(t, err)
	err = defaultCache.Set(ctx, []*resolve.CacheEntry{
		{Key: usernameKey, Value: []byte(`{"id":"1234","username":"Me","nickname":"nick-Me"}`), TTL: 30 * time.Second},
	})
	require.NoError(t, err)

	setupLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "set", Items: []CacheLogItem{{Key: idKey, TTL: 20 * time.Second}}},
		{Operation: "set", Items: []CacheLogItem{{Key: usernameKey, TTL: 30 * time.Second}}},
	}, setupLog)

	defaultCache.ClearLog()
	tracker.Reset()
	resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
		`query($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username nickname } }`,
		queryVariables{"id": "1234", "username": "Me"}, t)

	// This fixture is intentionally black-box: the desired observable outcome is that the
	// fresher overlapping username value wins and the complementary nickname is retained.
	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me","nickname":"nick-Me"}}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(accountsHost))

	logAfterQuery := defaultCache.GetLog()
	assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: idKey, Hit: true},
			{Key: usernameKey, Hit: true},
		}},
	}), sortCacheLogEntries(logAfterQuery))

	idData, idExists := defaultCache.Peek(idKey)
	assert.True(t, idExists)
	assert.Equal(t, `{"id":"1234","username":"OldName"}`, string(idData))
	usernameData, usernameExists := defaultCache.Peek(usernameKey)
	assert.True(t, usernameExists)
	assert.Equal(t, `{"id":"1234","username":"Me","nickname":"nick-Me"}`, string(usernameData))
}

// TestRootFieldEntityCacheMerge verifies that when a query crosses two subgraphs
// (accounts via root field with entity key mapping, reviews via entity resolution),
// both subgraphs write entity cache entries on the first request, and the second
// request hits the cache for both without making any subgraph calls.
// This tests that root field entity writes merge with existing entity data rather
// than clobbering it.
func TestRootFieldEntityCacheMerge(t *testing.T) {
	t.Parallel()
	defaultCache := NewFakeLoaderCache()
	caches := map[string]resolve.LoaderCache{
		"default": defaultCache,
	}

	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{Transport: tracker}

	// Configure accounts with root field entity key mapping AND entity caching,
	// and reviews with entity caching for User type.
	// Both share entity type User with cache name "default".
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
			SubgraphName: "reviews",
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

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	accountsHost := accountsURLParsed.Host
	reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
	reviewsHost := reviewsURLParsed.Host

	// First request: query that crosses both subgraphs → cache MISS for both → both write entity entries
	// user(id) root field fetches from accounts, reviews field triggers entity resolution from reviews
	defaultCache.ClearLog()
	tracker.Reset()
	resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_with_reviews.query"), queryVariables{"id": "1234"}, t)
	assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control."},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits."}]}}}`, string(resp))

	assert.Equal(t, 1, tracker.GetCount(accountsHost), "First request should call accounts subgraph once")
	assert.Equal(t, 1, tracker.GetCount(reviewsHost), "First request should call reviews subgraph once")

	logAfterFirst := defaultCache.GetLog()
	wantLogFirst := []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
	}
	assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First request should miss root field cache, set it, then entity fetch should merge")

	// Second request: same query → cache HIT for both subgraphs (entity data merged, not clobbered)
	defaultCache.ClearLog()
	tracker.Reset()
	resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_with_reviews.query"), queryVariables{"id": "1234"}, t)
	assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control."},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits."}]}}}`, string(resp))

	assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second request should skip accounts subgraph (cache hit)")
	assert.Equal(t, 0, tracker.GetCount(reviewsHost), "Second request should skip reviews subgraph (cache hit)")

	logAfterSecond := defaultCache.GetLog()
	wantLogSecond := []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
	}
	assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second request should hit cache for both root field and entity resolution")
}

// TestRootFieldCachingCompositeKeyInputObject verifies that root field caching works
// with composite entity keys mapped via multiple argument paths (simulating @is directive
// mapping with input object arguments). The cache key includes both "id" and "username"
// fields, so different argument combinations produce different cache entries.
func TestRootFieldCachingCompositeKeyInputObject(t *testing.T) {
	t.Parallel()
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

	// First request: cache miss → subgraph called → entity key written
	defaultCache.ClearLog()
	tracker.Reset()
	resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))

	assert.Equal(t, 1, tracker.GetCount(accountsHost), "First request should call accounts subgraph once")

	logAfterFirst := defaultCache.GetLog()
	wantLogFirst := []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234","username":"Me"}}`, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234","username":"Me"}}`, TTL: 30 * time.Second}}},
	}
	assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst), "First request should miss cache and set entity key with composite key")

	// Second request: same args → cache hit → subgraph NOT called
	defaultCache.ClearLog()
	tracker.Reset()
	resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Me"}, t)
	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Me"}}}`, string(resp))

	assert.Equal(t, 0, tracker.GetCount(accountsHost), "Second request should skip accounts subgraph (cache hit)")

	logAfterSecond := defaultCache.GetLog()
	wantLogSecond := []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234","username":"Me"}}`, Hit: true}}},
	}
	assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond), "Second request should hit cache for composite key")

	// Third request: different args → cache miss → subgraph called
	defaultCache.ClearLog()
	tracker.Reset()
	resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/user_by_id_and_name.query"), queryVariables{"id": "1234", "username": "Other"}, t)
	assert.Equal(t, `{"data":{"userByIdAndName":{"id":"1234","username":"Other"}}}`, string(resp))

	assert.Equal(t, 1, tracker.GetCount(accountsHost), "Third request with different args should call accounts subgraph")

	logAfterThird := defaultCache.GetLog()
	wantLogThird := []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234","username":"Other"}}`, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234","username":"Other"}}`, TTL: 30 * time.Second}}},
	}
	assert.Equal(t, sortCacheLogEntries(wantLogThird), sortCacheLogEntries(logAfterThird), "Third request should miss cache due to different username in composite key")
}
