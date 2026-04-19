package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TestLoader_PopulateFromCache verifies that populateFromCache correctly assigns
// cache hits to FromCache, tracks freshness ordering across multi-key entities,
// and records missing keys for partial hits. Without this, stale or wrong candidates
// could be served from L2 cache.
func TestLoader_PopulateFromCache(t *testing.T) {
	t.Parallel()

	t.Run("single key single entry sets FromCache", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`},
			},
		}
		entries := []*CacheEntry{
			{
				Key:          `{"__typename":"User","key":{"id":"1234"}}`,
				Value:        []byte(`{"id":"1234","username":"Me"}`),
				RemainingTTL: 15 * time.Second,
			},
		}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		require.NotNil(t, cacheKeys[0].FromCache)
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(cacheKeys[0].FromCache.MarshalTo(nil)))
		assert.Equal(t, 15*time.Second, cacheKeys[0].fromCacheRemainingTTL)
		assert.Equal(t, []fromCacheCandidate{
			{
				value:        []byte(`{"id":"1234","username":"Me"}`),
				remainingTTL: 15 * time.Second,
			},
		}, cacheKeys[0].fromCacheCandidates)
		assert.Nil(t, cacheKeys[0].missingKeys)
		assert.False(t, cacheKeys[0].fromCacheNeedsWriteback)
	})

	t.Run("two keys both hit uses freshest candidate and retains stale fallback", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		entries := []*CacheEntry{
			{
				Key:          `{"__typename":"User","key":{"id":"1234"}}`,
				Value:        []byte(`{"id":"1234","username":"FreshName"}`),
				RemainingTTL: 30 * time.Second,
			},
			{
				Key:          `{"__typename":"User","key":{"username":"Me"}}`,
				Value:        []byte(`{"id":"1234","username":"StaleName"}`),
				RemainingTTL: 10 * time.Second,
			},
		}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		require.NotNil(t, cacheKeys[0].FromCache)
		assert.Equal(t, `{"id":"1234","username":"FreshName"}`, string(cacheKeys[0].FromCache.MarshalTo(nil)))
		assert.Equal(t, 30*time.Second, cacheKeys[0].fromCacheRemainingTTL)
		assert.Equal(t, []fromCacheCandidate{
			{
				value:        []byte(`{"id":"1234","username":"FreshName"}`),
				remainingTTL: 30 * time.Second,
			},
			{
				value:        []byte(`{"id":"1234","username":"StaleName"}`),
				remainingTTL: 10 * time.Second,
			},
		}, cacheKeys[0].fromCacheCandidates)
		assert.Nil(t, cacheKeys[0].missingKeys)
	})

	t.Run("known freshness outranks unknown freshness", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		entries := []*CacheEntry{
			{
				Key:          `{"__typename":"User","key":{"id":"1234"}}`,
				Value:        []byte(`{"id":"1234","username":"FreshName"}`),
				RemainingTTL: 20 * time.Second,
			},
			{
				Key:   `{"__typename":"User","key":{"username":"Me"}}`,
				Value: []byte(`{"id":"1234","username":"UnknownFreshness"}`),
			},
		}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		require.NotNil(t, cacheKeys[0].FromCache)
		assert.Equal(t, `{"id":"1234","username":"FreshName"}`, string(cacheKeys[0].FromCache.MarshalTo(nil)))
		assert.Equal(t, []fromCacheCandidate{
			{
				value:        []byte(`{"id":"1234","username":"FreshName"}`),
				remainingTTL: 20 * time.Second,
			},
			{
				value:        []byte(`{"id":"1234","username":"UnknownFreshness"}`),
				remainingTTL: 0,
			},
		}, cacheKeys[0].fromCacheCandidates)
		assert.Nil(t, cacheKeys[0].missingKeys)
	})

	t.Run("equal freshness preserves cache.Get order", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		entries := []*CacheEntry{
			{
				Key:          `{"__typename":"User","key":{"id":"1234"}}`,
				Value:        []byte(`{"id":"1234","username":"First"}`),
				RemainingTTL: 25 * time.Second,
			},
			{
				Key:          `{"__typename":"User","key":{"username":"Me"}}`,
				Value:        []byte(`{"id":"1234","username":"Second"}`),
				RemainingTTL: 25 * time.Second,
			},
		}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		require.NotNil(t, cacheKeys[0].FromCache)
		assert.Equal(t, `{"id":"1234","username":"First"}`, string(cacheKeys[0].FromCache.MarshalTo(nil)))
		assert.Equal(t, []fromCacheCandidate{
			{
				value:        []byte(`{"id":"1234","username":"First"}`),
				remainingTTL: 25 * time.Second,
			},
			{
				value:        []byte(`{"id":"1234","username":"Second"}`),
				remainingTTL: 25 * time.Second,
			},
		}, cacheKeys[0].fromCacheCandidates)
		assert.Nil(t, cacheKeys[0].missingKeys)
	})

	t.Run("partial hit records exactly which requested keys were missing", func(t *testing.T) {
		t.Parallel()

		// Scenario: one CacheKey asks for three concrete L2 keys, but the cache only
		// returns a value for the id key. populateFromCache should preserve the hit as
		// FromCache and record the exact missing requested keys in order.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"email":"me@example.com"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		entries := []*CacheEntry{
			{
				Key:          `{"__typename":"User","key":{"id":"1234"}}`,
				Value:        []byte(`{"id":"1234","username":"Me"}`),
				RemainingTTL: 20 * time.Second,
			},
		}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		// Assert the hit candidate becomes FromCache and missingKeys keeps only the
		// two requested keys that did not come back from cache.Get.
		require.NotNil(t, cacheKeys[0].FromCache)
		assert.Equal(t, `{"id":"1234","username":"Me"}`, string(cacheKeys[0].FromCache.MarshalTo(nil)))
		assert.Equal(t, []string{
			`{"__typename":"User","key":{"email":"me@example.com"}}`,
			`{"__typename":"User","key":{"username":"Me"}}`,
		}, cacheKeys[0].missingKeys)
	})

	t.Run("no keys hit leaves FromCache nil", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{}

		cacheKeys := []*CacheKey{
			{
				Item: astjson.MustParse(`{}`),
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"username":"Me"}}`,
				},
			},
		}
		entries := []*CacheEntry{nil, nil}

		err := l.populateFromCache(ar, cacheKeys, entries)
		require.NoError(t, err)
		assert.Nil(t, cacheKeys[0].FromCache)
		assert.Zero(t, cacheKeys[0].fromCacheRemainingTTL)
		assert.Nil(t, cacheKeys[0].fromCacheCandidates)
		assert.Equal(t, []string{
			`{"__typename":"User","key":{"id":"1234"}}`,
			`{"__typename":"User","key":{"username":"Me"}}`,
		}, cacheKeys[0].missingKeys)
		assert.False(t, cacheKeys[0].fromCacheNeedsWriteback)
	})
}

// TestLoaderBuildCacheTrace_PredictableDebugTimingsNormalizeZeroDurationOperations
// verifies that predictable debug timings normalize zero-duration L2 operations to 1ns.
// Without this, flaky timing values would make trace output non-deterministic in tests.
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
		cacheTraceEntityCount:    1,
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
		DurationSinceStartNano:   1,  // predictable debug timing
		DurationSinceStartPretty: "1ns",
		DurationNano:             1,
		DurationPretty:           "1ns",
		L2Enabled:                true,
		CacheName:                "default",
		TTLSeconds:               30,
		EntityCount:              1, // 1 cache key
		L2Miss:                   1,
		L2GetDurationNano:        1,
		L2GetDurationPretty:      "1ns",
		L2SetDurationNano:        1,
		L2SetDurationPretty:      "1ns",
		Keys:                     []string{"key-1"},
		L2SetError:               "write failed",
	}, trace)
}
