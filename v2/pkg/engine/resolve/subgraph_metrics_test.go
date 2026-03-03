package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheAnalyticsSnapshot_SubgraphMetrics(t *testing.T) {
	t.Run("returns nil when no subgraph fetches or errors", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 10, Source: FieldSourceL1}, // cache hit, not subgraph
				{DataSource: "accounts", DurationMs: 5, Source: FieldSourceL2},  // cache hit, not subgraph
			},
		}
		assert.Equal(t, []SubgraphRequestMetrics(nil), snap.SubgraphMetrics())
	})

	t.Run("single subgraph with one fetch", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 42, Source: FieldSourceSubgraph, ResponseBytes: 256, HTTPStatusCode: 200},
			},
		}
		result := snap.SubgraphMetrics()
		assert.Equal(t, 1, len(result), "should have exactly 1 subgraph")
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName:       "accounts",
			RequestCount:       1,
			ErrorCount:         0,
			TotalDurationMs:    42,
			MaxDurationMs:      42,
			TotalResponseBytes: 256,
		}, result[0])
	})

	t.Run("single subgraph with multiple fetches picks max duration", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 10, Source: FieldSourceSubgraph, ResponseBytes: 100, HTTPStatusCode: 200},
				{DataSource: "accounts", DurationMs: 50, Source: FieldSourceSubgraph, ResponseBytes: 200, HTTPStatusCode: 200},
				{DataSource: "accounts", DurationMs: 30, Source: FieldSourceSubgraph, ResponseBytes: 150, HTTPStatusCode: 200},
			},
		}
		result := snap.SubgraphMetrics()
		assert.Equal(t, 1, len(result), "should have exactly 1 subgraph")
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName:       "accounts",
			RequestCount:       3,
			ErrorCount:         0,
			TotalDurationMs:    90,
			MaxDurationMs:      50,
			TotalResponseBytes: 450,
		}, result[0])
	})

	t.Run("multiple subgraphs with mixed success and errors", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 20, Source: FieldSourceSubgraph, ResponseBytes: 100},
				{DataSource: "products", DurationMs: 80, Source: FieldSourceSubgraph, ResponseBytes: 500},
				{DataSource: "accounts", DurationMs: 15, Source: FieldSourceSubgraph, ResponseBytes: 90},
				{DataSource: "products", DurationMs: 120, Source: FieldSourceSubgraph, ResponseBytes: 600},
			},
			ErrorEvents: []SubgraphErrorEvent{
				{DataSource: "products", EntityType: "Product", Message: "timeout", Code: "TIMEOUT"},
				{DataSource: "reviews", EntityType: "Review", Message: "not found", Code: "NOT_FOUND"},
			},
		}
		result := snap.SubgraphMetrics()
		assert.Equal(t, 3, len(result), "should have exactly 3 subgraphs")

		// accounts: 2 fetches, 0 errors
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName:       "accounts",
			RequestCount:       2,
			ErrorCount:         0,
			TotalDurationMs:    35,
			MaxDurationMs:      20,
			TotalResponseBytes: 190,
		}, result[0])

		// products: 2 fetches, 1 error
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName:       "products",
			RequestCount:       2,
			ErrorCount:         1,
			TotalDurationMs:    200,
			MaxDurationMs:      120,
			TotalResponseBytes: 1100,
		}, result[1])

		// reviews: 0 fetches, 1 error (error-only subgraph)
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName:    "reviews",
			RequestCount:    0,
			ErrorCount:      1,
			TotalDurationMs: 0,
			MaxDurationMs:   0,
		}, result[2])
	})

	t.Run("cache hits are excluded from subgraph metrics", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			FetchTimings: []FetchTimingEvent{
				{DataSource: "accounts", DurationMs: 0, Source: FieldSourceL1},        // L1 cache hit
				{DataSource: "accounts", DurationMs: 5, Source: FieldSourceL2},        // L2 cache hit
				{DataSource: "accounts", DurationMs: 30, Source: FieldSourceSubgraph}, // actual fetch
			},
		}
		result := snap.SubgraphMetrics()
		assert.Equal(t, 1, len(result), "should have exactly 1 subgraph")
		assert.Equal(t, 1, result[0].RequestCount, "should count only the subgraph fetch")
		assert.Equal(t, int64(30), result[0].TotalDurationMs, "should only sum subgraph fetch duration")
	})

	t.Run("empty snapshot returns nil", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, []SubgraphRequestMetrics(nil), snap.SubgraphMetrics())
	})

	t.Run("errors-only subgraph has zero request count", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{
			ErrorEvents: []SubgraphErrorEvent{
				{DataSource: "accounts", Message: "connection refused"},
				{DataSource: "accounts", Message: "connection refused"},
			},
		}
		result := snap.SubgraphMetrics()
		assert.Equal(t, 1, len(result), "should have exactly 1 subgraph")
		assert.Equal(t, SubgraphRequestMetrics{
			SubgraphName: "accounts",
			RequestCount: 0,
			ErrorCount:   2,
		}, result[0])
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
