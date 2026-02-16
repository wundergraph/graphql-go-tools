package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"
)

// newTestResolver constructs a minimal Resolver for handleTriggerEntityCache tests.
// It avoids New() which spawns the event-loop goroutine.
func newTestResolver(caches map[string]LoaderCache) *Resolver {
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

func TestHandleTriggerEntityCache(t *testing.T) {
	t.Run("populate single entity", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		require.Equal(t, 1, len(log), "should have exactly 1 cache operation")
		assert.Equal(t, CacheLogEntry{
			Operation: "set",
			Keys:      []string{`{"__typename":"Product","key":{"id":"prod-1"}}`},
			Hits:      nil,
		}, log[0], "should set the entity with correct cache key")

		// Verify stored data
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries), "should return exactly 1 entry")
		require.NotNil(t, entries[0], "entry should not be nil")
		assert.Equal(t, `{"id":"prod-1","name":"Widget","price":9.99,"__typename":"Product"}`, string(entries[0].Value), "stored data should match original entity with injected __typename")
	})

	t.Run("populate array of entities", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		// Expect exactly 1 set with 2 keys
		require.Equal(t, 1, len(log), "should have exactly 1 cache operation")
		assert.Equal(t, "set", log[0].Operation, "operation should be set")
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		}, log[0].Keys, "should set both entities with correct cache keys")
	})

	t.Run("typename filtering skips non-matching entities", func(t *testing.T) {
		// Regression test for the items[:0] backing array reuse bug (fixed in cc9b20aa).
		// Before the fix, using items[:0] to filter in-place corrupted the parsed JSON
		// array because GetArray() returns a slice over the parser's internal buffer.
		cache := NewFakeLoaderCache()
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		// Expect exactly 1 set with 2 keys (the 2 Products, not the Review)
		require.Equal(t, 1, len(log), "should have exactly 1 cache operation")
		assert.Equal(t, "set", log[0].Operation, "operation should be set")
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		}, log[0].Keys, "should only cache Product entities, not Review")

		// Verify stored data integrity — the items[:0] bug would corrupt values
		entries, err := cache.Get(context.Background(), []string{
			`{"__typename":"Product","key":{"id":"prod-1"}}`,
			`{"__typename":"Product","key":{"id":"prod-2"}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(entries), "should return exactly 2 entries")
		require.NotNil(t, entries[0], "first entry should not be nil")
		require.NotNil(t, entries[1], "second entry should not be nil")
		assert.Equal(t, `{"__typename":"Product","id":"prod-1","name":"Widget"}`, string(entries[0].Value), "first Product data should be intact")
		assert.Equal(t, `{"__typename":"Product","id":"prod-2","name":"Gadget"}`, string(entries[1].Value), "second Product data should be intact")
	})

	t.Run("missing typename gets injected", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		require.Equal(t, 1, len(log), "should have exactly 1 cache operation")
		assert.Equal(t, "set", log[0].Operation, "operation should be set")
		// Cache key should include "Product" typename even though it wasn't in the data
		assert.Equal(t, []string{`{"__typename":"Product","key":{"id":"prod-1"}}`}, log[0].Keys, "cache key should use injected typename")

		// Verify stored data includes injected __typename
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries), "should return exactly 1 entry")
		require.NotNil(t, entries[0], "entry should not be nil")
		assert.Equal(t, `{"id":"prod-1","name":"Widget","__typename":"Product"}`, string(entries[0].Value), "stored data should include injected __typename")
	})

	t.Run("invalidate mode deletes cache entry", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		// Expect exactly 1 delete with 1 key
		require.Equal(t, 1, len(log), "should have exactly 1 cache operation")
		assert.Equal(t, CacheLogEntry{
			Operation: "delete",
			Keys:      []string{`{"__typename":"Product","key":{"id":"prod-1"}}`},
			Hits:      nil,
		}, log[0], "should delete the correct cache key")

		// Verify the entry is gone
		entries, err := cache.Get(context.Background(), []string{`{"__typename":"Product","key":{"id":"prod-1"}}`})
		require.NoError(t, err)
		require.Equal(t, 1, len(entries), "should return exactly 1 result")
		assert.Nil(t, entries[0], "entry should be nil after deletion")
	})

	t.Run("missing cache name returns early", func(t *testing.T) {
		cache := NewFakeLoaderCache()
		// Resolver has "default" cache, but config references "nonexistent"
		r := newTestResolver(map[string]LoaderCache{"default": cache})

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
		assert.Equal(t, 0, len(log), "should not perform any cache operations when cache name is missing")
	})
}
