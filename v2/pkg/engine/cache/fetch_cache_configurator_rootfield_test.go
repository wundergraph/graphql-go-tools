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
	info := &resolve.FetchInfo{DataSourceID: "products"}
	for _, field := range fields {
		info.RootFields = append(info.RootFields, resolve.GraphCoordinate{TypeName: "Query", FieldName: field})
	}
	return info
}

func rootFieldProviders(policies ...cacheconfig.RootFieldCachePolicy) map[string]cacheconfig.CacheConfigProvider {
	return map[string]cacheconfig.CacheConfigProvider{
		"products": &cacheconfig.CachingConfiguration{RootFields: policies},
	}
}

func configureRootFieldFetch(t *testing.T, providers map[string]cacheconfig.CacheConfigProvider, info *resolve.FetchInfo) *resolve.SingleFetch {
	t.Helper()
	configurator := &fetchCacheConfigurator{
		providers:  providers,
		keyBuilder: newKeyBuilder(t, parseKeyBuilderDefinition(t), newKeyBuilderFederation(t)),
	}
	fetch := &resolve.SingleFetch{Info: info}
	tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
	configurator.configureTree(&resolve.GraphQLResponse{}, tree)
	return fetch
}

// TestFetchCacheConfiguratorRootFieldArm covers the plan-side root-field rows:
// full config for a single cached root field, and the conservative declines.
func TestFetchCacheConfiguratorRootFieldArm(t *testing.T) {
	products := cacheconfig.RootFieldCachePolicy{
		TypeName:                    "Query",
		FieldName:                   "products",
		CacheName:                   "root-fields",
		TTL:                         time.Minute,
		IncludeSubgraphHeaderPrefix: true,
		ShadowMode:                  true,
		PartialBatchLoad:            true,
	}

	t.Run("single cached root field receives the full config", func(t *testing.T) {
		fetch := configureRootFieldFetch(t, rootFieldProviders(products), rootFieldInfo("products"))
		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:                          false,
			L2:                          true,
			CacheName:                   "root-fields",
			TTL:                         time.Minute,
			IncludeSubgraphHeaderPrefix: true,
			ShadowMode:                  true,
			PartialBatchLoad:            true,
			KeySpec: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "products",
			},
		}, fetch.Cache)
	})

	t.Run("identical policy VALUES across a merged fetch keep the config", func(t *testing.T) {
		promotions := products
		promotions.FieldName = "promotions"
		fetch := configureRootFieldFetch(t, rootFieldProviders(products, promotions), rootFieldInfo("products", "promotions"))
		require.NotNil(t, fetch.Cache)
		// The key spec carries the FIRST coordinate; the cached value covers
		// all of the fetch's fields and coverage guards servability.
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeRootField,
			TypeName:  "Query",
			FieldName: "products",
		}, fetch.Cache.KeySpec)
	})

	t.Run("mixed policies decline caching", func(t *testing.T) {
		promotions := cacheconfig.RootFieldCachePolicy{
			TypeName:  "Query",
			FieldName: "promotions",
			CacheName: "other-cache",
			TTL:       time.Second,
		}
		fetch := configureRootFieldFetch(t, rootFieldProviders(products, promotions), rootFieldInfo("products", "promotions"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("cached + uncached merge declines caching", func(t *testing.T) {
		fetch := configureRootFieldFetch(t, rootFieldProviders(products), rootFieldInfo("products", "promotions"))
		assert.Nil(t, fetch.Cache)
	})

	t.Run("all-flags-false policy yields nil config", func(t *testing.T) {
		inert := cacheconfig.RootFieldCachePolicy{TypeName: "Query", FieldName: "products", CacheName: "root-fields"}
		fetch := configureRootFieldFetch(t, rootFieldProviders(inert), rootFieldInfo("products"))
		assert.Nil(t, fetch.Cache)
	})
}
