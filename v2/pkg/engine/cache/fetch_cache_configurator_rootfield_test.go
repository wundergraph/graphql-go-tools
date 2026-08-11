package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func rootFieldInfo(fields ...string) *resolve.FetchInfo {
	info := &resolve.FetchInfo{DataSourceID: "products", DataSourceName: "products"}
	for _, field := range fields {
		info.RootFields = append(info.RootFields, resolve.GraphCoordinate{TypeName: "Query", FieldName: field})
	}
	return info
}

func rootFieldCaching(entries ...cacheconfig.RootFieldCacheConfig) *cacheconfig.CachingConfiguration {
	return productCaching(cacheconfig.SubgraphCacheConfig{RootFields: entries})
}

func configureRootFieldFetch(t *testing.T, caching *cacheconfig.CachingConfiguration, info *resolve.FetchInfo) *resolve.SingleFetch {
	t.Helper()
	configurator := &fetchCacheConfigurator{caching: caching}
	fetch := &resolve.SingleFetch{Info: info}
	tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
	configurator.configureTree(&resolve.GraphQLResponse{}, tree)
	return fetch
}

// TestFetchCacheConfiguratorRootFieldArm covers the plan-side root-field rows:
// full config for a single cached root field, the subgraph-level fold, and the
// conservative declines.
func TestFetchCacheConfiguratorRootFieldArm(t *testing.T) {
	products := cacheconfig.RootFieldCacheConfig{
		TypeName:               "Query",
		FieldName:              "products",
		TTL:                    time.Minute,
		ShadowMode:             true,
		PartialBatchLoad:       true,
		IncludeSubgraphHeaders: true,
	}

	t.Run("single cached root field receives the full config", func(t *testing.T) {
		fetch := configureRootFieldFetch(t, rootFieldCaching(products), rootFieldInfo("products"))
		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:                     false,
			L2:                     true,
			SubgraphName:           "products",
			TTL:                    time.Minute,
			IncludeSubgraphHeaders: true,
			ShadowMode:             true,
			PartialBatchLoad:       true,
			KeySpec: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "products",
			},
		}, fetch.Cache)
	})

	t.Run("subgraph shadow mode and header inclusion reach the root field", func(t *testing.T) {
		caching := productCaching(cacheconfig.SubgraphCacheConfig{
			ShadowMode:             ptr(true),
			IncludeSubgraphHeaders: ptr(true),
			RootFields: []cacheconfig.RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "products", TTL: time.Minute},
			},
		})
		fetch := configureRootFieldFetch(t, caching, rootFieldInfo("products"))
		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:                     false,
			L2:                     true,
			SubgraphName:           "products",
			TTL:                    time.Minute,
			IncludeSubgraphHeaders: true,
			ShadowMode:             true,
			KeySpec: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "products",
			},
		}, fetch.Cache)
	})

	t.Run("a private coordinate marks its fetch private", func(t *testing.T) {
		private := cacheconfig.RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "me",
			TTL:       time.Minute,
			Scope:     cacheconfig.CacheScopePrivate,
		}
		fetch := configureRootFieldFetch(t, rootFieldCaching(private), rootFieldInfo("me"))
		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           false,
			L2:           true,
			SubgraphName: "products",
			TTL:          time.Minute,
			Private:      true,
			KeySpec: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "me",
			},
		}, fetch.Cache)
	})

	t.Run("a fetch merging a private and a public coordinate declines caching", func(t *testing.T) {
		private := cacheconfig.RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "me",
			TTL:       time.Minute,
			Scope:     cacheconfig.CacheScopePrivate,
		}
		public := cacheconfig.RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "products",
			TTL:       time.Minute,
		}
		// One fetch, one key, one scope: caching a mixed pair would have to pick
		// one of them, and either pick is wrong.
		fetch := configureRootFieldFetch(t, rootFieldCaching(private, public), rootFieldInfo("me", "products"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("the global HashAnalyticsKeys reaches the root field", func(t *testing.T) {
		caching := rootFieldCaching(products)
		caching.Global.HashAnalyticsKeys = true
		fetch := configureRootFieldFetch(t, caching, rootFieldInfo("products"))
		require.NotNil(t, fetch.Cache)
		assert.True(t, fetch.Cache.HashAnalyticsKeys)
	})

	t.Run("identical settings across a merged fetch keep the config", func(t *testing.T) {
		promotions := products
		promotions.FieldName = "promotions"
		fetch := configureRootFieldFetch(t, rootFieldCaching(products, promotions), rootFieldInfo("products", "promotions"))
		require.NotNil(t, fetch.Cache)
		// The key spec carries the FIRST coordinate; the cached value covers
		// all of the fetch's fields and coverage guards servability.
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeRootField,
			TypeName:  "Query",
			FieldName: "products",
		}, fetch.Cache.KeySpec)
	})

	t.Run("mixed settings decline caching", func(t *testing.T) {
		promotions := cacheconfig.RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "promotions",
			TTL:       time.Second,
		}
		fetch := configureRootFieldFetch(t, rootFieldCaching(products, promotions), rootFieldInfo("products", "promotions"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("cached + uncached merge declines caching", func(t *testing.T) {
		fetch := configureRootFieldFetch(t, rootFieldCaching(products), rootFieldInfo("products", "promotions"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("an entry enabling nothing yields nil config", func(t *testing.T) {
		inert := cacheconfig.RootFieldCacheConfig{TypeName: "Query", FieldName: "products"}
		fetch := configureRootFieldFetch(t, rootFieldCaching(inert), rootFieldInfo("products"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("the subgraph veto declines an otherwise cached coordinate", func(t *testing.T) {
		caching := productCaching(cacheconfig.SubgraphCacheConfig{
			Enabled:    ptr(false),
			RootFields: []cacheconfig.RootFieldCacheConfig{products},
		})
		fetch := configureRootFieldFetch(t, caching, rootFieldInfo("products"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("a subgraph TTL alone never caches a root field", func(t *testing.T) {
		caching := productCaching(cacheconfig.SubgraphCacheConfig{DefaultTTL: ptr(time.Minute)})
		fetch := configureRootFieldFetch(t, caching, rootFieldInfo("products"))
		assert.Nil(t, fetch.Cache)
	})
}
