package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
)

func TestLoaderCacheCopyInvariantCacheSkipFetchMergeCopiesOut(t *testing.T) {
	loader, release := newLoaderCacheTransformTestLoader()
	defer release()
	loader.ctx = NewContext(context.Background())

	item := parseLoaderCacheTransformTestValue(t, loader, `{}`)
	cached := parseLoaderCacheTransformTestValue(t, loader, `{"profile":{"name":"Ada"}}`)
	res := &result{
		cacheSkipFetch: true,
		cacheKeys: []*CacheKey{
			{
				FromCache: cached,
			},
		},
	}

	err := loader.mergeResult(&FetchItem{}, res, []*astjson.Value{item})
	assert.NoError(t, err)

	item.Get("profile").Set(loader.jsonArena, "name", astjson.StringValue(loader.jsonArena, "Grace"))

	assert.Equal(t, `{"profile":{"name":"Ada"}}`, string(cached.MarshalTo(nil)))
	assert.Equal(t, `{"profile":{"name":"Grace"}}`, string(item.MarshalTo(nil)))
}

func TestLoaderCacheCopyInvariantPopulateL1UsesWorkingCopyAndSwap(t *testing.T) {
	loader, release := newLoaderCacheTransformTestLoader()
	defer release()
	loader.ctx = NewContext(context.Background())
	loader.ctx.ExecutionOptions.Caching.EnableL1Cache = true
	loader.l1Cache = map[string]*astjson.Value{
		`{"__typename":"User","key":{"id":"1"}}`: parseLoaderCacheTransformTestValue(t, loader, `{"id":"1","profile":{"name":"Ada"}}`),
	}
	responseValue := parseLoaderCacheTransformTestValue(t, loader, `{"id":"1","profile":{"social":{"handle":"ada"}}}`)
	res := &result{
		cacheKeys: []*CacheKey{
			{
				Keys: []string{
					`{"__typename":"User","key":{"id":"1"}}`,
				},
			},
		},
	}

	loader.populateL1Cache(&FetchCacheConfiguration{
		UseL1Cache:  true,
		KeyTemplate: cacheTestUserKeyTemplate(),
	}, res, responseValue)

	responseValue.Get("profile", "social").Set(loader.jsonArena, "handle", astjson.StringValue(loader.jsonArena, "grace"))

	assert.Equal(t, `{"id":"1","profile":{"name":"Ada","social":{"handle":"ada"}}}`, string(loader.l1Cache[`{"__typename":"User","key":{"id":"1"}}`].MarshalTo(nil)))
	assert.Equal(t, `{"id":"1","profile":{"social":{"handle":"grace"}}}`, string(responseValue.MarshalTo(nil)))
}

func TestLoaderCacheCopyInvariantBatchCacheHitCopiesOut(t *testing.T) {
	loader, release := newLoaderCacheTransformTestLoader()
	defer release()
	loader.ctx = NewContext(context.Background())

	item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"1"}`)
	cached := parseLoaderCacheTransformTestValue(t, loader, `{"id":"1","profile":{"name":"Ada"}}`)
	res := &result{
		cacheSkipFetch: true,
		batchStats: [][]*astjson.Value{
			{
				item,
			},
		},
		cacheKeys: []*CacheKey{
			{
				FromCache: cached,
			},
		},
	}

	err := loader.mergeResult(&FetchItem{
		Fetch: &BatchEntityFetch{
			Cache: &FetchCacheConfiguration{
				KeyTemplate: cacheTestUserKeyTemplate(),
			},
		},
	}, res, []*astjson.Value{item})
	assert.NoError(t, err)

	item.Get("profile").Set(loader.jsonArena, "name", astjson.StringValue(loader.jsonArena, "Grace"))

	assert.Equal(t, `{"id":"1","profile":{"name":"Ada"}}`, string(cached.MarshalTo(nil)))
	assert.Equal(t, `{"id":"1","profile":{"name":"Grace"}}`, string(item.MarshalTo(nil)))
}

func TestLoaderCacheCopyInvariantBatchPartialResponseKeepsCachedHitsIntact(t *testing.T) {
	loader, release := newLoaderCacheTransformTestLoader()
	defer release()
	loader.ctx = NewContext(context.Background())

	cachedItem := parseLoaderCacheTransformTestValue(t, loader, `{"id":"1"}`)
	fetchedItem := parseLoaderCacheTransformTestValue(t, loader, `{"id":"2"}`)
	cached := parseLoaderCacheTransformTestValue(t, loader, `{"id":"1","profile":{"name":"Ada"}}`)
	res := &result{
		batchStats: [][]*astjson.Value{
			{
				cachedItem,
			},
			{
				fetchedItem,
			},
		},
		cacheKeys: []*CacheKey{
			{
				FromCache: cached,
			},
			{},
		},
	}

	err := loader.mergeBatchCacheHits(&FetchItem{}, res)
	assert.NoError(t, err)

	fetched := parseLoaderCacheTransformTestValue(t, loader, `{"id":"2","profile":{"name":"Grace"}}`)
	err = loader.mergeBatchFetchedValue(&FetchItem{}, res, 1, fetched)
	assert.NoError(t, err)

	fetchedItem.Get("profile").Set(loader.jsonArena, "name", astjson.StringValue(loader.jsonArena, "Lin"))

	assert.Equal(t, `{"id":"1","profile":{"name":"Ada"}}`, string(cached.MarshalTo(nil)))
	assert.Equal(t, `{"id":"1","profile":{"name":"Ada"}}`, string(cachedItem.MarshalTo(nil)))
	assert.Equal(t, `{"id":"2","profile":{"name":"Lin"}}`, string(fetchedItem.MarshalTo(nil)))
}
