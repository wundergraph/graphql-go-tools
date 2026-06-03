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

const (
	productKeyTop1 = `{"__typename":"Product","key":{"upc":"top-1"}}`
	productKeyTop2 = `{"__typename":"Product","key":{"upc":"top-2"}}`
	productKeyTop3 = `{"__typename":"Product","key":{"upc":"top-3"}}`

	productValueTop1 = `{"upc":"top-1","name":"Trilby","price":11}`
	productValueTop2 = `{"upc":"top-2","name":"Fedora","price":22}`
	productValueTop3 = `{"upc":"top-3","name":"Boater","price":33}`
)

func expectedBatchProductCache(upcs ...string) map[string]string {
	expected := make(map[string]string, len(upcs))
	for _, upc := range upcs {
		switch upc {
		case "top-1":
			expected[productKeyTop1] = productValueTop1
		case "top-2":
			expected[productKeyTop2] = productValueTop2
		case "top-3":
			expected[productKeyTop3] = productValueTop3
		}
	}
	return expected
}

func assertFakeLoaderCacheContents(t *testing.T, cache *FakeLoaderCache, want map[string]string) {
	t.Helper()

	cache.mu.RLock()
	got := make(map[string]string, len(cache.storage))
	for key, entry := range cache.storage {
		got[key] = string(entry.data)
	}
	cache.mu.RUnlock()

	assert.Equal(t, want, got)
}

// TestBatchEntityCacheLookup_FullFetch_AllMiss tests batch entity cache with all cache misses.
// Query products(upcs: ["top-1","top-2","top-3"]) with ArgumentIsEntityKey=true.
// All entities are fetched from the subgraph and cached individually.
func TestBatchEntityCacheLookup_FullFetch_AllMiss(t *testing.T) {
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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: all cache misses → subgraph called
	defaultCache.ClearLog()
	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, string(resp))
	t.Logf("Request 1 tracker: %v", tracker.GetCounts())
	assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph once")

	// Verify cache log: 1 get (batch miss) + 1 set (batch write)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: false},
			{Key: productKeyTop2, Hit: false},
			{Key: productKeyTop3, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
			{Key: productKeyTop3, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
}

// TestBatchEntityCacheLookup_FullFetch_AllHit tests that a second identical batch request
// serves all entities from cache without calling the subgraph.
func TestBatchEntityCacheLookup_FullFetch_AllHit(t *testing.T) {
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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: populate cache
	resp1, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: false},
			{Key: productKeyTop2, Hit: false},
			{Key: productKeyTop3, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
			{Key: productKeyTop3, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
	defaultCache.ClearLog()

	// Request 2: should hit cache — no subgraph call
	tracker.Reset()
	resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, string(resp1), string(resp2), "both requests should return identical responses")
	assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should NOT call products subgraph (all cache hits)")

	// Exact cache log: single GET with all 3 hits, no SET (served from cache)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: true},
			{Key: productKeyTop2, Hit: true},
			{Key: productKeyTop3, Hit: true},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
}

// TestBatchEntityCacheLookup_FullFetch_PartialMiss_FetchesAll tests that in full fetch mode,
// even when some entities are cached, the resolver is called with the full argument list.
func TestBatchEntityCacheLookup_FullFetch_PartialMiss_FetchesAll(t *testing.T) {
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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: warm cache with just top-1
	gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1"]) { upc name price } }`, nil, t)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop1, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop1, TTL: 30 * time.Second}}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1"))

	// Request 2: top-1 cached, top-2 not → full fetch mode fetches all
	defaultCache.ClearLog()
	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22}]}}`, string(resp))
	assert.Equal(t, 1, tracker.GetCount(productsHost), "full fetch mode should call products subgraph with the complete list")
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: true},
			{Key: productKeyTop2, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2"))
}

// TestBatchEntityCacheLookup_FullFetch_EmptyList tests that an empty list argument
// returns an empty array without calling the resolver.
func TestBatchEntityCacheLookup_FullFetch_EmptyList(t *testing.T) {
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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	defaultCache.ClearLog()
	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: []) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[]}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(productsHost), "empty list should not call products subgraph")

	// No cache operations should have occurred
	assert.Equal(t, []CacheLogEntry{}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache())
}

// TestBatchEntityCacheLookup_CacheKeySharing_ScalarAndBatch tests that scalar and batch
// lookups produce the same cache key format, enabling cache sharing.
func TestBatchEntityCacheLookup_CacheKeySharing_ScalarAndBatch(t *testing.T) {
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
					{
						TypeName:  "Query",
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: scalar product(upc: "top-1") populates cache
	gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { product(upc: "top-1") { upc name price } }`, nil, t)
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop1, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop1, TTL: 30 * time.Second}}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1"))

	// Request 2: batch products(upcs: ["top-1", "top-2"]) — top-1 hits cache (from scalar),
	// top-2 misses. Full fetch mode still calls subgraph with full list.
	defaultCache.ClearLog()
	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22}]}}`, string(resp))
	// In full fetch mode, partial miss means subgraph is called
	assert.Equal(t, 1, tracker.GetCount(productsHost), "full fetch mode with partial miss should call products subgraph")
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: true},
			{Key: productKeyTop2, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2"))
}

// TestBatchEntityCacheLookup_FullFetch_SingleElement tests that a single-element batch
// behaves identically to scalar lookup — same cache key format.
func TestBatchEntityCacheLookup_FullFetch_SingleElement(t *testing.T) {
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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: single-element batch
	resp1, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1"]) { upc name price } }`, nil, t)
	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11}]}}`, string(resp1))
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop1, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop1, TTL: 30 * time.Second}}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1"))

	// Request 2: should hit cache
	defaultCache.ClearLog()
	tracker.Reset()
	resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1"]) { upc name price } }`, nil, t)
	assert.Equal(t, string(resp1), string(resp2))
	assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should hit cache")
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop1, Hit: true}}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1"))
}

func TestBatchEntityCacheLookup_PartialFetch_SomeCached(t *testing.T) {
	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphRequestTracker(http.DefaultTransport)

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
						TypeName:         "Query",
						FieldName:        "products",
						CacheName:        "default",
						TTL:              30 * time.Second,
						PartialBatchLoad: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1"]) { upc name price } }`, nil, t)

	warmLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop1, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop1, TTL: 30 * time.Second}}},
	}, warmLog)
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1"))
	defaultCache.ClearLog()

	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, string(resp))

	productsRequests := tracker.GetRequests(productsHost)
	require.Equal(t, 1, len(productsRequests))
	assert.Equal(t, `{"query":"query($a: [String!]!){products(upcs: $a){upc name price}}","variables":{"a":["top-2","top-3"]}}`, productsRequests[0])

	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: true},
			{Key: productKeyTop2, Hit: false},
			{Key: productKeyTop3, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop2, TTL: 30 * time.Second},
			{Key: productKeyTop3, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
}

func TestBatchEntityCacheLookup_PartialFetch_AllHit(t *testing.T) {
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
						TypeName:         "Query",
						FieldName:        "products",
						CacheName:        "default",
						TTL:              30 * time.Second,
						PartialBatchLoad: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	warmLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: false},
			{Key: productKeyTop2, Hit: false},
			{Key: productKeyTop3, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
			{Key: productKeyTop3, TTL: 30 * time.Second},
		}},
	}, warmLog)
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
	defaultCache.ClearLog()

	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, string(resp))
	assert.Equal(t, 0, tracker.GetCount(productsHost))
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: true},
			{Key: productKeyTop2, Hit: true},
			{Key: productKeyTop3, Hit: true},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
}

func TestBatchEntityCacheLookup_PartialFetch_AllMiss(t *testing.T) {
	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphRequestTracker(http.DefaultTransport)

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
						TypeName:         "Query",
						FieldName:        "products",
						CacheName:        "default",
						TTL:              30 * time.Second,
						PartialBatchLoad: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	tracker.Reset()
	defaultCache.ClearLog()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, string(resp))

	// Verify subgraph was called with full argument list (all miss)
	assert.Equal(t, 1, tracker.GetRequestCount(productsHost))

	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop1, Hit: false},
			{Key: productKeyTop2, Hit: false},
			{Key: productKeyTop3, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{
			{Key: productKeyTop1, TTL: 30 * time.Second},
			{Key: productKeyTop2, TTL: 30 * time.Second},
			{Key: productKeyTop3, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-2", "top-3"))
}

func TestBatchEntityCacheLookup_PartialFetch_OrderPreservation(t *testing.T) {
	t.Parallel()

	defaultCache := NewFakeLoaderCache()
	tracker := newSubgraphRequestTracker(http.DefaultTransport)

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
						TypeName:         "Query",
						FieldName:        "products",
						CacheName:        "default",
						TTL:              30 * time.Second,
						PartialBatchLoad: true,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-3"]) { upc name price } }`, nil, t)

	warmLog := defaultCache.GetLog()
	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{{Key: productKeyTop3, Hit: false}}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop3, TTL: 30 * time.Second}}},
	}, warmLog)
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-3"))
	defaultCache.ClearLog()

	tracker.Reset()
	resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-3", "top-1"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-3","name":"Boater","price":33},{"upc":"top-1","name":"Trilby","price":11}]}}`, string(resp))

	productsRequests := tracker.GetRequests(productsHost)
	require.Equal(t, 1, len(productsRequests))
	assert.Equal(t, `{"query":"query($a: [String!]!){products(upcs: $a){upc name price}}","variables":{"a":["top-1"]}}`, productsRequests[0])

	assert.Equal(t, []CacheLogEntry{
		{Operation: "get", Items: []CacheLogItem{
			{Key: productKeyTop3, Hit: true},
			{Key: productKeyTop1, Hit: false},
		}},
		{Operation: "set", Items: []CacheLogItem{{Key: productKeyTop1, TTL: 30 * time.Second}}},
	}, defaultCache.GetLog())
	assertFakeLoaderCacheContents(t, defaultCache, expectedBatchProductCache("top-1", "top-3"))
}

// TestBatchEntityKeyCachingWithArgumentIsEntityKey tests that ArgumentIsEntityKey=true
// produces per-element cache keys (not a single batch key), enabling individual entity
// cache hits on a second identical request with zero subgraph calls.
func TestBatchEntityKeyCachingWithArgumentIsEntityKey(t *testing.T) {
	t.Parallel()
	productKeyTop1 := `{"__typename":"Product","key":{"upc":"top-1"}}`
	productKeyTop2 := `{"__typename":"Product","key":{"upc":"top-2"}}`
	productKeyTop3 := `{"__typename":"Product","key":{"upc":"top-3"}}`

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
						FieldName: "products",
						CacheName: "default",
						TTL:       30 * time.Second,
						EntityKeyMappings: []plan.EntityKeyMapping{
							{
								EntityTypeName: "Product",
								FieldMappings: []plan.FieldMapping{
									{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
								},
							},
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

	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	productsHost := productsURLParsed.Host

	// Request 1: all cache misses — subgraph called, 3 per-element keys written
	tracker.Reset()
	resp1, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, `{"data":{"products":[{"upc":"top-1","name":"Trilby","price":11},{"upc":"top-2","name":"Fedora","price":22},{"upc":"top-3","name":"Boater","price":33}]}}`, string(resp1))
	assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph once")

	// Verify per-element cache contents were written
	assertFakeLoaderCacheContents(t, defaultCache, map[string]string{
		productKeyTop1: `{"upc":"top-1","name":"Trilby","price":11}`,
		productKeyTop2: `{"upc":"top-2","name":"Fedora","price":22}`,
		productKeyTop3: `{"upc":"top-3","name":"Boater","price":33}`,
	})

	// Verify cache log: 1 get (batch miss) + 1 set (batch write)
	assert.Equal(t, []CacheLogEntry{
		{Operation: CacheOperationGet, Items: []CacheLogItem{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: false},
			{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: false},
			{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: false},
		}},
		{Operation: CacheOperationSet, Items: []CacheLogItem{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second},
			{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, TTL: 30 * time.Second},
			{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, TTL: 30 * time.Second},
		}},
	}, defaultCache.GetLog())

	// Request 2: all cache hits — zero subgraph calls
	defaultCache.ClearLog()
	tracker.Reset()
	resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
		`query { products(upcs: ["top-1", "top-2", "top-3"]) { upc name price } }`, nil, t)

	assert.Equal(t, string(resp1), string(resp2), "both requests should return identical responses")
	assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should NOT call products subgraph (all cache hits)")

	// Verify cache log: 1 get (all hits) — no SET needed
	assert.Equal(t, []CacheLogEntry{
		{Operation: CacheOperationGet, Items: []CacheLogItem{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Hit: true},
			{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Hit: true},
			{Key: `{"__typename":"Product","key":{"upc":"top-3"}}`, Hit: true},
		}},
	}, defaultCache.GetLog())
}
