package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"
)

// newTestResolverWithCaches constructs a minimal Resolver for handleTriggerEntityCache tests.
// It avoids New() which spawns the event-loop goroutine.
func newTestResolverWithCaches(caches map[string]LoaderCache) *Resolver {
	return &Resolver{
		ctx: context.Background(),
		options: ResolverOptions{
			Caches: caches,
		},
		resolveArenaPool:            arena.NewArenaPool(),
		subgraphRequestSingleFlight: NewSingleFlight(1),
	}
}

// productCacheKeyTemplate builds an EntityQueryCacheKeyTemplate that uses
// __typename + id as the cache key, matching the standard Product entity.
func productCacheKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{
					Name: []byte("__typename"),
					Value: &String{
						Path: []string{"__typename"},
					},
				},
				{
					Name: []byte("id"),
					Value: &String{
						Path: []string{"id"},
					},
				},
			},
		}),
	}
}

// TestHandleTriggerEntityCache verifies subscription-driven entity cache operations:
// populate (set), invalidate (delete), typename injection, and filtering.
// Without this, subscription events could corrupt or fail to update the L2 cache.
func TestHandleTriggerEntityCache(t *testing.T) {
	t.Run("populate single entity", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModePopulate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "default",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "updateProduct",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		data := []byte(`{"data":{"updateProduct":{"id":"prod-1","name":"Widget","price":9.99}}}`)

		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		// Expect exactly 1 set with 1 key
		// Verify single set with correct key and TTL
		require.Equal(t, 1, len(log))
		assert.Equal(t, CacheLogEntry{
			Operation: "set",
			Keys:      []string{`{"__typename":"Product","key":{"id":"prod-1"}}`},
			Hits:      nil,
			TTL:       30 * time.Second,
		}, log[0])

		// Verify stored data includes injected __typename
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0])
		assert.Equal(t, `{"id":"prod-1","name":"Widget","price":9.99,"__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("populate array of entities", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModePopulate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "default",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "updateProducts",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		data := []byte(`{"data":{"updateProducts":[{"id":"prod-1","name":"Widget"},{"id":"prod-2","name":"Gadget"}]}}`)

		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		// Verify single set with both entity keys
		require.Equal(t, 1, len(log))
		assert.Equal(t, "set", log[0].Operation)
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		}, log[0].Keys)
	})

	t.Run("typename filtering skips non-matching entities", func(t *testing.T) {
		// Regression test for the items[:0] backing array reuse bug (fixed in cc9b20aa).
		// Before the fix, using items[:0] to filter in-place corrupted the parsed JSON
		// array because GetArray() returns a slice over the parser's internal buffer.
		cache := NewFakeLoaderCache()
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModePopulate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "default",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "entityUpdates",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		// Mixed types: Product, Review, Product — only Products should be cached
		data := []byte(`{"data":{"entityUpdates":[{"__typename":"Product","id":"prod-1","name":"Widget"},{"__typename":"Review","id":"rev-1","body":"Great"},{"__typename":"Product","id":"prod-2","name":"Gadget"}]}}`)

		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		// Only Products cached, not the Review
		require.Equal(t, 1, len(log))
		assert.Equal(t, "set", log[0].Operation)
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		}, log[0].Keys)

		// Verify stored data integrity (the items[:0] bug would corrupt values)
		entries, err := cache.Get(context.Background(), []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(entries))
		require.NotNil(t, entries[0])
		require.NotNil(t, entries[1])
		assert.Equal(t, `{"__typename":"Product","id":"prod-1","name":"Widget"}`, string(entries[0].Value))
		assert.Equal(t, `{"__typename":"Product","id":"prod-2","name":"Gadget"}`, string(entries[1].Value))
	})

	t.Run("missing typename gets injected", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModePopulate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "default",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "updateProduct",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		// Entity without __typename — should be injected from EntityTypeName
		data := []byte(`{"data":{"updateProduct":{"id":"prod-1","name":"Widget"}}}`)

		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		// Cache key should include injected "Product" typename
		require.Equal(t, 1, len(log))
		assert.Equal(t, "set", log[0].Operation)
		assert.Equal(t, []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}, log[0].Keys)

		// Verify stored data includes injected __typename
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.NotNil(t, entries[0])
		assert.Equal(t, `{"id":"prod-1","name":"Widget","__typename":"Product"}`, string(entries[0].Value))
	})

	t.Run("invalidate mode deletes cache entry", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		// Pre-populate cache with an entity
		err := cache.Set(context.Background(), []*CacheEntry{
			{Key: `{"__typename":"Product","key":{"id":"prod-1"}}`, Value: []byte(`{"__typename":"Product","id":"prod-1","name":"Old"}`)},
		}, 30*time.Second)
		require.NoError(t, err)
		cache.ClearLog()

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModeInvalidate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "default",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "deleteProduct",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		data := []byte(`{"data":{"deleteProduct":{"id":"prod-1"}}}`)

		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		// Verify delete operation
		require.Equal(t, 1, len(log))
		assert.Equal(t, CacheLogEntry{
			Operation: "delete",
			Keys:      []string{`{"__typename":"Product","key":{"id":"prod-1"}}`},
			Hits:      nil,
		}, log[0])

		// Verify the entry is gone
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		assert.Nil(t, entries[0])
	})

	t.Run("missing cache name returns early", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		// Resolver has "default" cache, but config references "nonexistent"
		r := newTestResolverWithCaches(map[string]LoaderCache{"default": cache})

		resolveCtx := NewContext(context.Background())
		resolveCtx.ExecutionOptions.Caching.EnableL2Cache = true

		config := &triggerEntityCacheConfig{
			pop: &SubscriptionEntityCachePopulation{
				Mode:                  SubscriptionCacheModePopulate,
				CacheKeyTemplate:      productCacheKeyTemplate(),
				CacheName:             "nonexistent",
				TTL:                   30 * time.Second,
				SubscriptionFieldName: "updateProduct",
				EntityTypeName:        "Product",
			},
			resolveCtx: resolveCtx,
			postProcess: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		}

		data := []byte(`{"data":{"updateProduct":{"id":"prod-1","name":"Widget"}}}`)

		// Should not panic and should not perform any cache operations
		r.handleTriggerEntityCache(config, data)

		log := cache.GetLog()
		assert.Equal(t, 0, len(log))
	})
}
