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
	t.Run("L2 miss then hit shows cache_trace in extensions.trace", func(t *testing.T) {
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

		// Request 1: all L2 misses — cache is empty, all fetches go to subgraphs
		tracker.Reset()
		resp1, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body author: authorWithoutProvides { username } } } }`, nil, t)
		assert.Contains(t, string(resp1), `"topProducts"`)

		trace1 := parseTraceFromResponse(t, resp1)
		require.NotNil(t, trace1, "Response should contain extensions.trace")

		cacheTraces1 := collectCacheTraces(t, trace1)
		require.True(t, len(cacheTraces1) > 0, "Should have at least one cache_trace entry on first request")

		for _, ct := range cacheTraces1 {
			assert.True(t, ct.L2Enabled, "L2 should be enabled for all cached fetches")
			assert.Equal(t, "default", ct.CacheName, "All fetches use the 'default' cache")
			assert.Equal(t, int64(30), ct.TTLSeconds, "TTL should be 30s as configured")
			assert.Equal(t, 0, ct.L2Hit, "No L2 hits on first request — cache is empty")
			assert.True(t, ct.L2Miss > 0 || ct.L1Miss > 0, "Should have at least one miss (L2 or L1)")
			if ct.L2Miss > 0 {
				assert.Equal(t, int64(1), ct.L2SetDurationNano, "Predictable debug timing: Set duration is 1ns") // predictable timing
				assert.Equal(t, int64(1), ct.L2GetDurationNano, "Predictable debug timing: Get duration is 1ns") // L2 Get always happens (miss returns quickly)
			}
		}

		// Request 2: all L2 hits — cache was populated by Request 1
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name reviews { body author: authorWithoutProvides { username } } } }`, nil, t)
		assert.Contains(t, string(resp2), `"topProducts"`)

		trace2 := parseTraceFromResponse(t, resp2)
		require.NotNil(t, trace2, "Response should contain extensions.trace on second request")

		cacheTraces2 := collectCacheTraces(t, trace2)
		require.True(t, len(cacheTraces2) > 0, "Should have at least one cache_trace entry on second request")

		for _, ct := range cacheTraces2 {
			assert.True(t, ct.L2Enabled, "L2 should be enabled for all cached fetches")
			assert.True(t, ct.L2Hit > 0, "Should have L2 hits on second request — populated by Request 1")
			assert.Equal(t, 0, ct.L2Miss, "No L2 misses on second request — all cached")
			assert.Equal(t, int64(1), ct.L2GetDurationNano, "Predictable debug timing: Get duration is 1ns")
			assert.Equal(t, int64(0), ct.L2SetDurationNano, "No L2 Set on cache hit — nothing to write")
		}

		// On full cache hit, no subgraph calls should be made
		counts := tracker.GetCounts()
		for host, count := range counts {
			assert.Equal(t, 0, count, "No subgraph calls expected on full cache hit, but got %d for %s", count, host)
		}
	})
}
