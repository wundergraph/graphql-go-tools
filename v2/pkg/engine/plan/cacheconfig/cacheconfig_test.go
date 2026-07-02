package cacheconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testConfiguration() *CachingConfiguration {
	return &CachingConfiguration{
		Entities: []EntityCachePolicy{
			{
				TypeName:                    "Product",
				CacheName:                   "products",
				TTL:                         30 * time.Second,
				NegativeCacheTTL:            5 * time.Second,
				IncludeSubgraphHeaderPrefix: true,
				EnablePartialCacheLoad:      true,
				HashAnalyticsKeys:           true,
				ShadowMode:                  true,
			},
		},
		RootFields: []RootFieldCachePolicy{
			{
				TypeName:                    "Query",
				FieldName:                   "topProducts",
				CacheName:                   "top-products",
				TTL:                         time.Minute,
				IncludeSubgraphHeaderPrefix: true,
				ShadowMode:                  true,
				PartialBatchLoad:            true,
			},
		},
		Mutations: []MutationCachePolicy{
			{
				FieldName:   "updateProduct",
				Invalidate:  true,
				PopulateL2:  true,
				TTLOverride: 10 * time.Second,
			},
		},
		Subscriptions: []SubscriptionCachePolicy{
			{
				TypeName:                    "Subscription",
				FieldName:                   "productUpdated",
				CacheName:                   "product-updates",
				TTL:                         15 * time.Second,
				IncludeSubgraphHeaderPrefix: true,
				EnableInvalidationOnKeyOnly: true,
			},
		},
	}
}

// TestCachingConfigurationProvider pins the CacheConfigProvider lookup
// semantics of CachingConfiguration: exact-match hit with the full policy, and
// the (zero, false) miss that gates caching off per coordinate.
func TestCachingConfigurationProvider(t *testing.T) {
	cfg := testConfiguration()

	t.Run("entity policy found", func(t *testing.T) {
		policy, ok := cfg.EntityPolicy("Product")
		assert.True(t, ok)
		assert.Equal(t, EntityCachePolicy{
			TypeName:                    "Product",
			CacheName:                   "products",
			TTL:                         30 * time.Second,
			NegativeCacheTTL:            5 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnablePartialCacheLoad:      true,
			HashAnalyticsKeys:           true,
			ShadowMode:                  true,
		}, policy)
	})

	t.Run("entity policy not found", func(t *testing.T) {
		policy, ok := cfg.EntityPolicy("Review")
		assert.False(t, ok)
		assert.Equal(t, EntityCachePolicy{}, policy)
	})

	t.Run("root field policy found", func(t *testing.T) {
		policy, ok := cfg.RootFieldPolicy("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCachePolicy{
			TypeName:                    "Query",
			FieldName:                   "topProducts",
			CacheName:                   "top-products",
			TTL:                         time.Minute,
			IncludeSubgraphHeaderPrefix: true,
			ShadowMode:                  true,
			PartialBatchLoad:            true,
		}, policy)
	})

	t.Run("root field policy requires both coordinates", func(t *testing.T) {
		policy, ok := cfg.RootFieldPolicy("Query", "topReviews")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCachePolicy{}, policy)

		policy, ok = cfg.RootFieldPolicy("Mutation", "topProducts")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCachePolicy{}, policy)
	})

	t.Run("mutation policy found", func(t *testing.T) {
		policy, ok := cfg.MutationPolicy("updateProduct")
		assert.True(t, ok)
		assert.Equal(t, MutationCachePolicy{
			FieldName:   "updateProduct",
			Invalidate:  true,
			PopulateL2:  true,
			TTLOverride: 10 * time.Second,
		}, policy)
	})

	t.Run("mutation policy not found", func(t *testing.T) {
		policy, ok := cfg.MutationPolicy("deleteProduct")
		assert.False(t, ok)
		assert.Equal(t, MutationCachePolicy{}, policy)
	})

	t.Run("subscription policy found", func(t *testing.T) {
		policy, ok := cfg.SubscriptionPolicy("Subscription", "productUpdated")
		assert.True(t, ok)
		assert.Equal(t, SubscriptionCachePolicy{
			TypeName:                    "Subscription",
			FieldName:                   "productUpdated",
			CacheName:                   "product-updates",
			TTL:                         15 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnableInvalidationOnKeyOnly: true,
		}, policy)
	})

	t.Run("subscription policy not found", func(t *testing.T) {
		policy, ok := cfg.SubscriptionPolicy("Subscription", "reviewAdded")
		assert.False(t, ok)
		assert.Equal(t, SubscriptionCachePolicy{}, policy)
	})

	t.Run("empty configuration misses everywhere", func(t *testing.T) {
		empty := &CachingConfiguration{}
		_, ok := empty.EntityPolicy("Product")
		assert.False(t, ok)
		_, ok = empty.RootFieldPolicy("Query", "topProducts")
		assert.False(t, ok)
		_, ok = empty.MutationPolicy("updateProduct")
		assert.False(t, ok)
		_, ok = empty.SubscriptionPolicy("Subscription", "productUpdated")
		assert.False(t, ok)
	})
}
