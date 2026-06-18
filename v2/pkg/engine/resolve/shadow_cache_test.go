package resolve

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShadowCacheHitStillLoadsSourceAndServesFresh(t *testing.T) {
	root := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Grace"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	assert.NoError(t, cache.Set(nil, []*CacheEntry{
		{
			Key:   `{"__typename":"User","key":{"id":"1"}}`,
			Value: []byte(`{"__typename":"User","id":"1","name":"Ada"}`),
		},
	}))
	cache.ClearSetOps()

	response := cacheTestBatchEntityResponse(root, entities, shadowUserNameCacheConfig())
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out, snapshot := resolveShadowCacheTestGraphQLResponse(t, response, options)
	snapshot = normalizeShadowFetchTimings(snapshot)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Grace"}]}}`, out)
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, CacheAnalyticsSnapshot{
		L2Reads: []CacheKeyEvent{
			{
				Key:        `{"__typename":"User","key":{"id":"1"}}`,
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Read,
				Hit:        true,
				Shadow:     true,
				Bytes:      43,
			},
		},
		L2Writes: []CacheWriteEvent{
			{
				Key:        `{"__typename":"User","key":{"id":"1"}}`,
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Write,
				Bytes:      45,
				TTL:        time.Minute,
				Reason:     CacheWriteReasonRefresh,
			},
		},
		FetchTimings: []FetchTimingEvent{
			{
				SubgraphName: "users",
				CacheName:    "default",
				Operation:    "l2_get",
				Bytes:        43,
			},
			{
				SubgraphName: "users",
				CacheName:    "default",
				Operation:    "l2_set",
				Bytes:        45,
			},
		},
		ShadowComparisons: []ShadowComparisonEvent{
			{
				Key:        `{"__typename":"User","key":{"id":"1"}}`,
				EntityType: "User",
				Matched:    false,
				CachedHash: 10621478458753992337,
				FreshHash:  10414171129586596008,
				CachedSize: 43,
				FreshSize:  45,
			},
		},
	}, snapshot)
	assert.Equal(t, [][]*CacheEntry{
		{
			{
				Key:         `{"__typename":"User","key":{"id":"1"}}`,
				Value:       []byte(`{"__typename":"User","id":"1","name":"Grace"}`),
				TTL:         time.Minute,
				WriteReason: CacheWriteReasonRefresh,
			},
		},
	}, cache.SetOps())
	assert.Equal(t, map[string]string{
		`{"__typename":"User","key":{"id":"1"}}`: `{"__typename":"User","id":"1","name":"Grace"}`,
	}, cache.Snapshot())
}

func TestShadowCacheComparisonsRecordMatchedAndMismatchedHashes(t *testing.T) {
	root := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Lin"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	assert.NoError(t, cache.Set(nil, []*CacheEntry{
		{
			Key:   `{"__typename":"User","key":{"id":"1"}}`,
			Value: []byte(`{"__typename":"User","id":"1","name":"Ada"}`),
		},
		{
			Key:   `{"__typename":"User","key":{"id":"2"}}`,
			Value: []byte(`{"__typename":"User","id":"2","name":"Grace"}`),
		},
	}))
	cache.ClearSetOps()

	response := cacheTestBatchEntityResponse(root, entities, shadowUserNameCacheConfig())
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out, snapshot := resolveShadowCacheTestGraphQLResponse(t, response, options)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"},{"id":"2","name":"Lin"}]}}`, out)
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, []ShadowComparisonEvent{
		{
			Key:        `{"__typename":"User","key":{"id":"1"}}`,
			EntityType: "User",
			Matched:    true,
			CachedHash: 10621478458753992337,
			FreshHash:  10621478458753992337,
			CachedSize: 43,
			FreshSize:  43,
		},
		{
			Key:        `{"__typename":"User","key":{"id":"2"}}`,
			EntityType: "User",
			Matched:    false,
			CachedHash: 9999264088279515,
			FreshHash:  10734176697705671705,
			CachedSize: 45,
			FreshSize:  43,
		},
	}, snapshot.ShadowComparisons)
	assert.Equal(t, 0.5, snapshot.ShadowFreshnessRate())
	assert.Equal(t, [][]*CacheEntry{
		{
			{
				Key:         `{"__typename":"User","key":{"id":"1"}}`,
				Value:       []byte(`{"__typename":"User","id":"1","name":"Ada"}`),
				TTL:         time.Minute,
				WriteReason: CacheWriteReasonRefresh,
			},
		},
		{
			{
				Key:         `{"__typename":"User","key":{"id":"2"}}`,
				Value:       []byte(`{"__typename":"User","id":"2","name":"Lin"}`),
				TTL:         time.Minute,
				WriteReason: CacheWriteReasonRefresh,
			},
		},
	}, cache.SetOps())
}

func TestShadowFreshnessRateForKnownMix(t *testing.T) {
	snapshot := CacheAnalyticsSnapshot{
		ShadowComparisons: []ShadowComparisonEvent{
			{
				Key:     "entity:User:1",
				Matched: true,
			},
			{
				Key:     "entity:User:2",
				Matched: false,
			},
			{
				Key:     "entity:User:3",
				Matched: true,
			},
			{
				Key:     "entity:User:4",
				Matched: false,
			},
		},
	}

	assert.Equal(t, 0.5, snapshot.ShadowFreshnessRate())
	assert.Equal(t, 0.0, CacheAnalyticsSnapshot{}.ShadowFreshnessRate())
}

func shadowUserNameCacheConfig() *FetchCacheConfiguration {
	config := batchUserNameCacheConfig(false)
	config.ShadowMode = true
	return config
}

func normalizeShadowFetchTimings(snapshot CacheAnalyticsSnapshot) CacheAnalyticsSnapshot {
	for i := range snapshot.FetchTimings {
		snapshot.FetchTimings[i].Duration = 0
	}
	return snapshot
}

func resolveShadowCacheTestGraphQLResponse(t *testing.T, response *GraphQLResponse, options ResolverOptions) (string, CacheAnalyticsSnapshot) {
	t.Helper()

	resolverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := New(resolverCtx, options)
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

	var out bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, &out)
	assert.NoError(t, err)
	return out.String(), ctx.GetCacheStats()
}
