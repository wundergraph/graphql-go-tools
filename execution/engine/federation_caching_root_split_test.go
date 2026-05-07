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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestRootFieldSplitByDatasource verifies that when multiple root fields are split across
// different datasource fetches, each fetch gets its own cache entry and key.
func TestRootFieldSplitByDatasource(t *testing.T) {
	t.Parallel()

	// Verifies two cached root fields on the same subgraph are isolated into
	// separate L2 entries; a warm request should skip both subgraph fetches.
	t.Run("two cached root fields on same subgraph use independent cache entries", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		// Configure two Query root fields on accounts with the same cache and TTL.
		// They share a subgraph but must not share cache keys or write entries.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
						{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// COLD path: cache is empty, so both root fields miss L2 and are written
		// back under independent Query-field keys.
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Response proves both isolated fetches still merge into the original shape.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// One bulk Get covers both root keys; one bulk Set writes both independent keys.
		assert.Equal(t, 2, len(logAfterFirst), "Should have 2 cache operations (1 bulk get, 1 bulk set)")

		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, Hit: false},
				{Key: `{"__typename":"Query","field":"me"}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, TTL: 30 * time.Second},
				{Key: `{"__typename":"Query","field":"me"}`, TTL: 30 * time.Second},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))

		// Both fields miss, so accounts is called once per isolated root fetch.
		assert.Equal(t, 2, tracker.GetCount(accountsHost), "Should call accounts subgraph twice (once per root field)")

		// WARM path: both root field entries exist, so the same query should be
		// served entirely from L2 with no accounts call.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Same response proves cached values preserve the composed response shape.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Both keys hit in one bulk Get; no Set is needed on a complete hit.
		assert.Equal(t, 1, len(logAfterSecond), "Should have 1 bulk cache get operation (both hits)")

		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, Hit: true},
				{Key: `{"__typename":"Query","field":"me"}`, Hit: true},
			}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// Complete L2 hit means both accounts root fetches are skipped.
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Should not call accounts subgraph (both cache hits)")
	})

	// Verifies isolated root fields keep their own TTL values when written to
	// the same named cache.
	t.Run("root fields with different TTLs write separate TTLs", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		// Same setup pattern as above, but me gets 10s and cat gets 60s to prove
		// TTL is attached per root-field configuration, not per cache name.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 10 * time.Second},
						{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		gqlClient := NewGraphqlClient(http.DefaultClient)

		// COLD path: both fields miss and write entries with their configured TTLs.
		defaultCache.ClearLog()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Response is the control; the contract under test is the TTL in Set logs.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, Hit: false},
				{Key: `{"__typename":"Query","field":"me"}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, TTL: 60 * time.Second},
				{Key: `{"__typename":"Query","field":"me"}`, TTL: 10 * time.Second},
			}},
		}
		// Exact Set TTLs prove isolated fetches preserve per-field TTL config.
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))
	})

	// Verifies one cached root field does not accidentally cache its uncached
	// sibling; only the cached field should hit on the warm request.
	t.Run("cached root field hits while uncached sibling still fetches", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		// Only Query.me is cacheable. Query.cat remains uncached even though it
		// shares the same accounts subgraph and query document.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// COLD path: me misses and writes; cat is fetched but never appears in
		// the cache log because it has no root-field cache config.
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Both fields are fetched from accounts and merged despite only me caching.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Only me has get/set operations; cat is intentionally absent.
		assert.Equal(t, 2, len(logAfterFirst), "Should have 2 cache operations (get+set for me only)")

		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"me"}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"me"}`, TTL: 30 * time.Second}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))

		// Both root fields fetch on cold path: me to populate cache, cat because
		// it is uncached.
		assert.Equal(t, 2, tracker.GetCount(accountsHost), "Should call accounts subgraph twice (once per isolated root field)")

		// WARM path: me is served from L2, cat still calls accounts because it
		// was never cached.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Same response proves cached and live root-field results compose.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Only me is looked up and hits; cat remains absent from cache operations.
		assert.Equal(t, 1, len(logAfterSecond), "Should have 1 cache get (me hit)")

		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"me"}`, Hit: true}}},
		}
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// The one remaining accounts call is cat only; me is served from cache.
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should call accounts subgraph once (cat only, me from cache)")
	})

	// Verifies root-field cache isolation still composes correctly with entity
	// caching across other subgraphs in the same operation.
	t.Run("cached root split composes with entity caching", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		// Configure accounts root fields plus User entity caching, products root
		// caching, and reviews Product entity caching to exercise mixed cache layers.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
						{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 30 * time.Second},
					},
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
					},
				},
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
			}),
		))
		t.Cleanup(setup.Close)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// This query combines accounts root-field split (me/cat), products root
		// caching (topProducts), and reviews/accounts entity resolution.
		query := `{
			me { id username }
			cat { name }
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides { username }
				}
			}
		}`

		// COLD path: every configured root/entity cache is empty, so all involved
		// subgraphs must be called and then populated.
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, query, nil, t)
		// Response proves root-field split and entity resolution compose.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"},"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Query","field":"cat"}`, Hit: false},
					{Key: `{"__typename":"Query","field":"me"}`, Hit: false},
					{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: false},
				},
			},
			{
				Operation: "set",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Query","field":"cat"}`, TTL: 30 * time.Second},
					{Key: `{"__typename":"Query","field":"me"}`, TTL: 30 * time.Second},
					{Key: `{"__typename":"Query","field":"topProducts"}`, TTL: 30 * time.Second},
				},
			},
			{
				Operation: "get",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
					{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
				},
			},
			{
				Operation: "set",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
					{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
				},
			},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}
		// Cold path misses and writes all configured root/entity cache entries.
		assert.Equal(t, sortCacheLogEntries(wantLogFirst), sortCacheLogEntries(logAfterFirst))

		// accounts: me root, cat root, and User entity resolution all miss cold.
		assert.Equal(t, 3, tracker.GetCount(accountsHost), "accounts: once for me, once for cat, once for User entity")
		// products and reviews each miss once for their configured cache layer.
		assert.Equal(t, 1, tracker.GetCount(productsHost), "products: once for topProducts")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "reviews: once for Product entity")

		// WARM path: all root/entity entries exist, so no subgraph should be called.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, query, nil, t)
		// Same response proves all pieces can be served from their cache entries.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"},"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Query","field":"cat"}`, Hit: true},
					{Key: `{"__typename":"Query","field":"me"}`, Hit: true},
					{Key: `{"__typename":"Query","field":"topProducts"}`, Hit: true},
				},
			},
			{
				Operation: "get",
				Items: []CacheLogItem{
					{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
					{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
				},
			},
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}
		// Warm path hits every configured root/entity cache entry and writes nothing.
		assert.Equal(t, sortCacheLogEntries(wantLogSecond), sortCacheLogEntries(logAfterSecond))

		// Zero calls on every subgraph proves root-field and entity caches all hit.
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "accounts: all from cache")
		assert.Equal(t, 0, tracker.GetCount(productsHost), "products: root field from cache")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "reviews: entity from cache")
	})

	// Verifies deleting one isolated root-field key does not evict or poison the
	// sibling root-field entry stored in the same named cache.
	t.Run("deleting one root field key leaves sibling cache entry intact", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		// Same two-root-field cache setup as the first subtest; this one manually
		// deletes only Query.me after both entries have been populated.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
						{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// COLD path: populate both me and cat root-field entries.
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Control response before manual invalidation.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		// Invalidate only Query.me; Query.cat should remain present and hit.
		err := defaultCache.Delete(ctx, []string{`{"__typename":"Query","field":"me"}`})
		require.NoError(t, err)

		// MIXED path: cat should hit from L2, me should miss and be re-written.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, `{ me { id username } cat { name } }`, nil, t)
		// Response stays identical even though one field is refetched and one is cached.
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"},"cat":{"name":"Pepper"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		wantLog := []CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{
				{Key: `{"__typename":"Query","field":"cat"}`, Hit: true},
				{Key: `{"__typename":"Query","field":"me"}`, Hit: false},
			}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Query","field":"me"}`, TTL: 30 * time.Second}}},
		}
		// Bulk Get proves cat survived deletion while me missed; Set proves me
		// is re-cached after the refetch.
		assert.Equal(t, sortCacheLogEntries(wantLog), sortCacheLogEntries(logAfterSecond))

		// Only the invalidated me root field needs a new accounts call.
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Should call accounts once (me re-fetch only)")
	})
}
