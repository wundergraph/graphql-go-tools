package resolve

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheTrace_JSON(t *testing.T) {
	t.Run("full cache trace serializes correctly", func(t *testing.T) {
		ct := &CacheTrace{
			L1Enabled:           true,
			L2Enabled:           true,
			CacheName:           "default",
			TTLSeconds:          60,
			L1Hit:               2,
			L1Miss:              1,
			L2Hit:               0,
			L2Miss:              3,
			L2GetDurationNano:   5000000,
			L2GetDurationPretty: "5ms",
			PartialCacheLoad:    true,
			Entities: []CacheTraceEntity{
				{Key: `{"__typename":"User","key":{"id":"1"}}`, Source: "l1", ByteSize: 42},
				{Key: `{"__typename":"User","key":{"id":"2"}}`, Source: "l1", ByteSize: 38},
				{Key: `{"__typename":"User","key":{"id":"3"}}`, Source: "subgraph"},
			},
		}

		data, err := json.Marshal(ct)
		require.NoError(t, err)

		var decoded CacheTrace
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, true, decoded.L1Enabled)
		assert.Equal(t, true, decoded.L2Enabled)
		assert.Equal(t, "default", decoded.CacheName)
		assert.Equal(t, int64(60), decoded.TTLSeconds)
		assert.Equal(t, 2, decoded.L1Hit)
		assert.Equal(t, 1, decoded.L1Miss)
		assert.Equal(t, 3, len(decoded.Entities))
		assert.Equal(t, "l1", decoded.Entities[0].Source)
		assert.Equal(t, "subgraph", decoded.Entities[2].Source)
	})

	t.Run("empty cache trace omits zero fields", func(t *testing.T) {
		ct := &CacheTrace{
			L1Enabled: false,
			L2Enabled: false,
		}

		data, err := json.Marshal(ct)
		require.NoError(t, err)

		var raw map[string]any
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)
		_, hasCacheName := raw["cache_name"]
		assert.False(t, hasCacheName, "cache_name should be omitted when empty")
		_, hasEntities := raw["entities"]
		assert.False(t, hasEntities, "entities should be omitted when empty")
		_, hasShadowMode := raw["shadow_mode"]
		assert.False(t, hasShadowMode, "shadow_mode should be omitted when false")
	})

	t.Run("shadow mode fields serialize", func(t *testing.T) {
		ct := &CacheTrace{
			L2Enabled:  true,
			ShadowMode: true,
			ShadowHit:  true,
			L2Hit:      1,
		}

		data, err := json.Marshal(ct)
		require.NoError(t, err)

		var decoded CacheTrace
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, true, decoded.ShadowMode)
		assert.Equal(t, true, decoded.ShadowHit)
	})
}

func TestBuildCacheTrace(t *testing.T) {
	t.Run("returns nil when tracing disabled", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: false}
		res := &result{}
		cfg := FetchCacheConfiguration{CacheKeyTemplate: &EntityQueryCacheKeyTemplate{}}
		ct := l.buildCacheTrace(res, cfg)
		assert.Nil(t, ct)
	})

	t.Run("returns nil when ExcludeCacheStats true", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: true, ExcludeCacheStats: true}
		res := &result{}
		cfg := FetchCacheConfiguration{CacheKeyTemplate: &EntityQueryCacheKeyTemplate{}}
		ct := l.buildCacheTrace(res, cfg)
		assert.Nil(t, ct)
	})

	t.Run("returns nil when no cache key template", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: true}
		res := &result{}
		cfg := FetchCacheConfiguration{}
		ct := l.buildCacheTrace(res, cfg)
		assert.Nil(t, ct)
	})

	t.Run("full L1 hit", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: true}
		l.ctx.ExecutionOptions.Caching = CachingOptions{EnableL1Cache: true, EnableL2Cache: true}
		res := &result{
			cacheSkipFetch:   true,
			cacheTraceL1Hits: 3,
			cache:            NewFakeLoaderCache(),
		}
		cfg := FetchCacheConfiguration{
			Enabled:          true,
			UseL1Cache:       true,
			CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
			CacheName:        "default",
			TTL:              60 * time.Second,
		}
		ct := l.buildCacheTrace(res, cfg)
		require.NotNil(t, ct)
		assert.Equal(t, true, ct.L1Enabled)
		assert.Equal(t, true, ct.L2Enabled)
		assert.Equal(t, 3, ct.L1Hit)
		assert.Equal(t, 0, ct.L1Miss)
		assert.Equal(t, "default", ct.CacheName)
		assert.Equal(t, int64(60), ct.TTLSeconds)
	})

	t.Run("shadow mode shows shadow_hit", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: true}
		l.ctx.ExecutionOptions.Caching = CachingOptions{EnableL2Cache: true}
		res := &result{
			cacheTraceL2Hits:    1,
			cacheTraceShadowHit: true,
			cache:               NewFakeLoaderCache(),
		}
		cfg := FetchCacheConfiguration{
			Enabled:          true,
			ShadowMode:       true,
			CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
		}
		ct := l.buildCacheTrace(res, cfg)
		require.NotNil(t, ct)
		assert.Equal(t, true, ct.ShadowMode)
		assert.Equal(t, true, ct.ShadowHit)
	})

	t.Run("predictable debug timings", func(t *testing.T) {
		l := &Loader{ctx: NewContext(context.Background())}
		l.ctx.TracingOptions = TraceOptions{Enable: true, EnablePredictableDebugTimings: true}
		l.ctx.ExecutionOptions.Caching = CachingOptions{EnableL2Cache: true}
		res := &result{
			cacheTraceL2GetDuration: 5 * time.Millisecond,
			cacheTraceL2SetDuration: 3 * time.Millisecond,
			cache:                   NewFakeLoaderCache(),
		}
		cfg := FetchCacheConfiguration{
			Enabled:          true,
			CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
		}
		ct := l.buildCacheTrace(res, cfg)
		require.NotNil(t, ct)
		assert.Equal(t, int64(1), ct.L2GetDurationNano)
		assert.Equal(t, "1ns", ct.L2GetDurationPretty)
		assert.Equal(t, int64(1), ct.L2SetDurationNano)
		assert.Equal(t, "1ns", ct.L2SetDurationPretty)
	})
}
