package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoaderBuildCacheTrace_PredictableDebugTimingsNormalizeZeroDurationOperations(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.TracingOptions = TraceOptions{
		Enable:                        true,
		EnablePredictableDebugTimings: true,
	}
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	loader := &Loader{ctx: ctx}
	res := &result{
		cache:                    NewFakeLoaderCache(),
		cacheTraceL2GetAttempted: true,
		cacheTraceL2SetAttempted: true,
		cacheTraceL2Misses:       1,
		cacheTraceL2SetError:     "write failed",
		l2CacheKeys: []*CacheKey{
			{Keys: []string{"key-1"}},
		},
	}

	trace := loader.buildCacheTrace(res, FetchCacheConfiguration{
		Enabled:          true,
		CacheName:        "default",
		TTL:              30 * time.Second,
		CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
	})

	assert.Equal(t, &CacheTrace{
		L2Enabled:           true,
		CacheName:           "default",
		TTLSeconds:          30,
		L2Miss:              1,
		L2GetDurationNano:   1,
		L2GetDurationPretty: "1ns",
		L2SetDurationNano:   1,
		L2SetDurationPretty: "1ns",
		Keys:                []string{"key-1"},
		L2SetError:          "write failed",
	}, trace)
}
