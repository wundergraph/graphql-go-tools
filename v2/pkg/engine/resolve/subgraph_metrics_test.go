package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheAnalyticsSnapshot_SubgraphFetches(t *testing.T) {
	t.Run("returns nil when no subgraph fetches", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 10, Source: FieldSourceL1}, // L1 cache hit
				{DataSource: "accounts", DurationMs: 5, Source: FieldSourceL2},  // L2 cache hit
			},
		}
		assert.Equal(t, []SubgraphFetchMetrics(nil), snap.SubgraphFetches())
	})

	t.Run("returns one entry per subgraph fetch", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", EntityType: "User", DurationMs: 42, Source: FieldSourceSubgraph, ResponseBytes: 256, HTTPStatusCode: 200, IsEntityFetch: true},
				{DataSource: "products", EntityType: "", DurationMs: 80, Source: FieldSourceSubgraph, ResponseBytes: 500, HTTPStatusCode: 200, IsEntityFetch: false},
				{DataSource: "accounts", EntityType: "User", DurationMs: 15, Source: FieldSourceSubgraph, ResponseBytes: 90, HTTPStatusCode: 200, IsEntityFetch: true},
			},
		}
		result := snap.SubgraphFetches()
		assert.Equal(t, []SubgraphFetchMetrics{
			{SubgraphName: "accounts", EntityType: "User", DurationMs: 42, HTTPStatusCode: 200, ResponseBytes: 256, IsEntityFetch: true},
			{SubgraphName: "products", EntityType: "", DurationMs: 80, HTTPStatusCode: 200, ResponseBytes: 500, IsEntityFetch: false},
			{SubgraphName: "accounts", EntityType: "User", DurationMs: 15, HTTPStatusCode: 200, ResponseBytes: 90, IsEntityFetch: true},
		}, result)
	})

	t.Run("excludes cache hits", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 0, Source: FieldSourceL1},                                                                  // L1 cache hit
				{DataSource: "accounts", DurationMs: 5, Source: FieldSourceL2},                                                                  // L2 cache hit
				{DataSource: "accounts", EntityType: "User", DurationMs: 30, Source: FieldSourceSubgraph, HTTPStatusCode: 200, ResponseBytes: 128}, // actual fetch
			},
		}
		result := snap.SubgraphFetches()
		assert.Equal(t, []SubgraphFetchMetrics{
			{SubgraphName: "accounts", EntityType: "User", DurationMs: 30, HTTPStatusCode: 200, ResponseBytes: 128},
		}, result)
	})

	t.Run("empty snapshot returns nil", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, []SubgraphFetchMetrics(nil), snap.SubgraphFetches())
	})

	t.Run("preserves error status codes", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 100, Source: FieldSourceSubgraph, HTTPStatusCode: 500, ResponseBytes: 50},
				{DataSource: "accounts", DurationMs: 20, Source: FieldSourceSubgraph, HTTPStatusCode: 200, ResponseBytes: 256},
			},
		}
		result := snap.SubgraphFetches()
		assert.Equal(t, 500, result[0].HTTPStatusCode)
		assert.Equal(t, 200, result[1].HTTPStatusCode)
	})
}

func TestFetchTimingEvent_NewFields(t *testing.T) {
	t.Run("subgraph fetch carries HTTP status and response size", func(t *testing.T) {
		event := FetchTimingEvent{
			DataSource:     "accounts",
			DurationMs:     42,
			Source:         FieldSourceSubgraph,
			HTTPStatusCode: 200,
			ResponseBytes:  1024,
			TTFBMs:         0, // not yet instrumented
		}
		assert.Equal(t, 200, event.HTTPStatusCode)
		assert.Equal(t, 1024, event.ResponseBytes)
		assert.Equal(t, int64(0), event.TTFBMs)
	})

	t.Run("cache hit has zero values for HTTP fields", func(t *testing.T) {
		event := FetchTimingEvent{
			DataSource: "accounts",
			DurationMs: 1,
			Source:     FieldSourceL1,
		}
		assert.Equal(t, 0, event.HTTPStatusCode, "cache hits should have zero status code")
		assert.Equal(t, 0, event.ResponseBytes, "cache hits should have zero response bytes")
		assert.Equal(t, int64(0), event.TTFBMs, "cache hits should have zero TTFB")
	})
}
