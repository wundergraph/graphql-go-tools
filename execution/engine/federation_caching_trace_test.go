package engine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func parseTraceFromResponse(t *testing.T, resp []byte) map[string]any {
	t.Helper()
	var response map[string]any
	require.NoError(t, json.Unmarshal(resp, &response))
	extensions, ok := response["extensions"].(map[string]any)
	if !ok {
		return nil
	}
	trace, ok := extensions["trace"].(map[string]any)
	if !ok {
		return nil
	}
	return trace
}

func collectCacheTraces(t *testing.T, trace map[string]any) []resolve.CacheTrace {
	t.Helper()
	var results []resolve.CacheTrace
	fetches, ok := trace["fetches"].(map[string]any)
	if !ok {
		return nil
	}
	walkFetchNode(t, fetches, &results)
	return results
}

func walkFetchNode(t *testing.T, node map[string]any, results *[]resolve.CacheTrace) {
	t.Helper()
	if fetch, ok := node["fetch"].(map[string]any); ok {
		if traceData, ok := fetch["trace"].(map[string]any); ok {
			if ctRaw, ok := traceData["cache_trace"].(map[string]any); ok {
				ctJSON, err := json.Marshal(ctRaw)
				require.NoError(t, err)
				var ct resolve.CacheTrace
				require.NoError(t, json.Unmarshal(ctJSON, &ct))
				*results = append(*results, ct)
			}
		}
		// Also check traces array (for batch/entity fetches with multiple traces)
		if traces, ok := fetch["traces"].([]any); ok {
			for _, traceItem := range traces {
				if traceMap, ok := traceItem.(map[string]any); ok {
					if ctRaw, ok := traceMap["cache_trace"].(map[string]any); ok {
						ctJSON, err := json.Marshal(ctRaw)
						require.NoError(t, err)
						var ct resolve.CacheTrace
						require.NoError(t, json.Unmarshal(ctJSON, &ct))
						*results = append(*results, ct)
					}
				}
			}
		}
	}
	if children, ok := node["children"].([]any); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]any); ok {
				walkFetchNode(t, childMap, results)
			}
		}
	}
}

func TestFederationCaching_CacheTraceInExtensions(t *testing.T) {
	t.Parallel()
	t.Run("L2 miss then hit shows cache_trace in extensions.trace", func(t *testing.T) {
		t.Parallel()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(true),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": NewFakeLoaderCache()}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{SubgraphName: "products", RootFieldCaching: plan.RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
				}},
				{SubgraphName: "reviews", EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
				}},
				{SubgraphName: "accounts", EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				}},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// --- Request 1: all L2 misses — cache is empty, all fetches go to subgraphs ---
		tracker.Reset()
		resp1, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body author: authorWithoutProvides { username } } } }`, nil, t)
		assert.Contains(t, string(resp1), `"topProducts"`)

		trace1 := parseTraceFromResponse(t, resp1)
		require.NotNil(t, trace1, "Response should contain extensions.trace")

		cacheTraces1 := collectCacheTraces(t, trace1)
		require.Equal(t, 3, len(cacheTraces1), "Should have 3 cache traces: products root field, reviews entities, accounts entities")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Miss:              1, // 1 root field miss: Query.topProducts
			L2GetDurationNano:   1, // predictable timing
			L2GetDurationPretty: "1ns",
			L2SetDurationNano:   1, // L2 Set happened after fetch
			L2SetDurationPretty: "1ns",
			Keys:                []string{`{"__typename":"Query","field":"topProducts"}`},
		}, cacheTraces1[0], "products root field: L2 miss, populated after fetch")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Miss:              2, // 2 Product entities missed
			L2GetDurationNano:   1,
			L2GetDurationPretty: "1ns",
			L2SetDurationNano:   1,
			L2SetDurationPretty: "1ns",
			Keys: []string{
				`{"__typename":"Product","key":{"upc":"top-1"}}`,
				`{"__typename":"Product","key":{"upc":"top-2"}}`,
			},
		}, cacheTraces1[1], "reviews entities: 2 Product entities missed")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Miss:              2, // 2 User entity lookups missed (same user for 2 reviews, deduplicated in batch but 2 cache keys)
			L2GetDurationNano:   1,
			L2GetDurationPretty: "1ns",
			L2SetDurationNano:   1,
			L2SetDurationPretty: "1ns",
			Keys: []string{
				`{"__typename":"User","key":{"id":"1234"}}`,
				`{"__typename":"User","key":{"id":"1234"}}`,
			},
		}, cacheTraces1[2], "accounts entities: User 1234 missed (2 lookups for 2 reviews)")

		// --- Request 2: all L2 hits — cache was populated by Request 1 ---
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body author: authorWithoutProvides { username } } } }`, nil, t)
		assert.Contains(t, string(resp2), `"topProducts"`)

		trace2 := parseTraceFromResponse(t, resp2)
		require.NotNil(t, trace2, "Response should contain extensions.trace on second request")

		cacheTraces2 := collectCacheTraces(t, trace2)
		require.Equal(t, 3, len(cacheTraces2), "Should have 3 cache traces on second request")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Hit:               1, // root field hit from L2
			L2GetDurationNano:   1,
			L2GetDurationPretty: "1ns",
			Entities: []resolve.CacheTraceEntity{
				{Key: `{"__typename":"Query","field":"topProducts"}`, Source: "l2", ByteSize: 127},
			},
			Keys: []string{`{"__typename":"Query","field":"topProducts"}`},
		}, cacheTraces2[0], "products root field: L2 hit, no Set")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Hit:               2, // both Product entities hit
			L2GetDurationNano:   1,
			L2GetDurationPretty: "1ns",
			Entities: []resolve.CacheTraceEntity{
				{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Source: "l2", ByteSize: 132},
				{Key: `{"__typename":"Product","key":{"upc":"top-2"}}`, Source: "l2", ByteSize: 188},
			},
			Keys: []string{
				`{"__typename":"Product","key":{"upc":"top-1"}}`,
				`{"__typename":"Product","key":{"upc":"top-2"}}`,
			},
		}, cacheTraces2[1], "reviews entities: both Products from L2")

		assert.Equal(t, resolve.CacheTrace{
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          30,
			L2Hit:               2, // both User lookups hit (same user, 2 cache key lookups)
			L2GetDurationNano:   1,
			L2GetDurationPretty: "1ns",
			Entities: []resolve.CacheTraceEntity{
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Source: "l2", ByteSize: 49},
				{Key: `{"__typename":"User","key":{"id":"1234"}}`, Source: "l2", ByteSize: 49},
			},
			Keys: []string{
				`{"__typename":"User","key":{"id":"1234"}}`,
				`{"__typename":"User","key":{"id":"1234"}}`,
			},
		}, cacheTraces2[2], "accounts entities: User 1234 from L2 (2 lookups)")

		// On full cache hit, no subgraph calls should be made
		assert.Equal(t, map[string]int{}, tracker.GetCounts(), "no subgraph calls expected on full cache hit")
	})
}
