package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
