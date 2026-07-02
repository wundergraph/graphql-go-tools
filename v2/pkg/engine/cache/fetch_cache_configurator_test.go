package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func newEntityConfigurator(t *testing.T, providers map[string]cacheconfig.CacheConfigProvider) *fetchCacheConfigurator {
	t.Helper()
	return &fetchCacheConfigurator{
		providers:  providers,
		keyBuilder: newKeyBuilder(t, parseKeyBuilderDefinition(t), newKeyBuilderFederation(t, "upc")),
	}
}

func productProviders(policy cacheconfig.EntityCachePolicy) map[string]cacheconfig.CacheConfigProvider {
	return map[string]cacheconfig.CacheConfigProvider{
		"products": &cacheconfig.CachingConfiguration{
			Entities: []cacheconfig.EntityCachePolicy{policy},
		},
	}
}

// upcCandidate is the single expected candidate for the "upc" key.
func upcCandidate() []resolve.CacheKeyCandidate {
	return []resolve.CacheKeyCandidate{
		{
			Representation: &resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name:        []byte("__typename"),
						Value:       &resolve.String{Path: []string{"__typename"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:        []byte("upc"),
						Value:       &resolve.String{Path: []string{"upc"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
				},
			},
		},
	}
}

func TestFetchCacheConfiguratorEntityArm(t *testing.T) {
	fullPolicy := cacheconfig.EntityCachePolicy{
		TypeName:                    "Product",
		CacheName:                   "products",
		TTL:                         time.Minute,
		NegativeCacheTTL:            5 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		HashAnalyticsKeys:           true,
		ShadowMode:                  true,
	}

	t.Run("entity fetch receives the full config", func(t *testing.T) {
		configurator := newEntityConfigurator(t, productProviders(fullPolicy))
		info := productEntityInfo()
		providesData := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:         []byte("productName"),
					OriginalName: []byte("name"),
					Value:        &resolve.Scalar{Nullable: false, Path: []string{"productName"}},
					OnTypeNames:  [][]byte{[]byte("Product")},
				},
			},
		}
		fetch := &resolve.EntityFetch{Info: info}
		response := &resolve.GraphQLResponse{}
		response.SetCacheProvidesData(map[*resolve.FetchInfo]*resolve.Object{info: providesData})
		tree := &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSequence,
			ChildNodes: []*resolve.FetchTreeNode{
				{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}},
			},
		}
		configurator.configureTree(response, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:                          true,
			L2:                          true,
			CacheName:                   "products",
			TTL:                         time.Minute,
			NegativeCacheTTL:            5 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnablePartialCacheLoad:      true,
			ShadowMode:                  true,
			HashAnalyticsKeys:           true,
			KeySpec: resolve.CacheKeySpec{
				Scope:      resolve.CacheScopeEntity,
				TypeName:   "Product",
				Candidates: upcCandidate(),
			},
			ProvidesData: &resolve.Object{
				HasAliases: true, // ComputeHasAliases folded in (OriginalName present)
				Fields: []*resolve.Field{
					{
						Name:         []byte("productName"),
						OriginalName: []byte("name"),
						Value:        &resolve.Scalar{Nullable: false, Path: []string{"productName"}},
						OnTypeNames:  [][]byte{[]byte("Product")},
					},
				},
			},
		}, fetch.Cache)
		// The ProvidesData is the side-table's tree itself, not a copy.
		assert.Same(t, providesData, fetch.Cache.ProvidesData)
	})

	t.Run("batch entity fetch receives config through the interface", func(t *testing.T) {
		configurator := newEntityConfigurator(t, productProviders(cacheconfig.EntityCachePolicy{
			TypeName:  "Product",
			CacheName: "products",
			TTL:       time.Minute,
		}))
		fetch := &resolve.BatchEntityFetch{Info: productEntityInfo()}
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:        true,
			L2:        true,
			CacheName: "products",
			TTL:       time.Minute,
			KeySpec: resolve.CacheKeySpec{
				Scope:      resolve.CacheScopeEntity,
				TypeName:   "Product",
				Candidates: upcCandidate(),
			},
		}, fetch.Cache)
	})

	t.Run("zero TTLs keep L1 eligibility with L2 off", func(t *testing.T) {
		configurator := newEntityConfigurator(t, productProviders(cacheconfig.EntityCachePolicy{
			TypeName:  "Product",
			CacheName: "products",
		}))
		fetch := &resolve.EntityFetch{Info: productEntityInfo()}
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.True(t, fetch.Cache.L1)
		assert.False(t, fetch.Cache.L2)
	})

	nilRows := []struct {
		name  string
		fetch resolve.Fetch
		cfg   func(t *testing.T) *fetchCacheConfigurator
	}{
		{
			name:  "single fetch stays uncached until task 13",
			fetch: &resolve.SingleFetch{Info: productEntityInfo()},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return newEntityConfigurator(t, productProviders(fullPolicy))
			},
		},
		{
			name:  "no provider for the datasource",
			fetch: &resolve.EntityFetch{Info: productEntityInfo()},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return newEntityConfigurator(t, map[string]cacheconfig.CacheConfigProvider{})
			},
		},
		{
			name:  "no entity policy for the type",
			fetch: &resolve.EntityFetch{Info: productEntityInfo()},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return newEntityConfigurator(t, map[string]cacheconfig.CacheConfigProvider{
					"products": &cacheconfig.CachingConfiguration{},
				})
			},
		},
		{
			name:  "no usable key",
			fetch: &resolve.EntityFetch{Info: productEntityInfo()},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return &fetchCacheConfigurator{
					providers:  productProviders(fullPolicy),
					keyBuilder: newKeyBuilder(t, parseKeyBuilderDefinition(t), newKeyBuilderFederation(t)),
				}
			},
		},
		{
			name:  "nil fetch info",
			fetch: &resolve.EntityFetch{},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return newEntityConfigurator(t, productProviders(fullPolicy))
			},
		},
		{
			// An abstract-path entity fetch can collect one root coordinate
			// per enclosing concrete type; policy and key spec both derive
			// from RootFields[0].TypeName, so mixed types decline entirely.
			name: "mixed entity types decline caching",
			fetch: &resolve.EntityFetch{Info: &resolve.FetchInfo{
				DataSourceID: "products",
				RootFields: []resolve.GraphCoordinate{
					{TypeName: "Product", FieldName: "name"},
					{TypeName: "User", FieldName: "username"},
				},
			}},
			cfg: func(t *testing.T) *fetchCacheConfigurator {
				return newEntityConfigurator(t, productProviders(fullPolicy))
			},
		},
	}
	for _, row := range nilRows {
		t.Run(row.name, func(t *testing.T) {
			tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: row.fetch}}
			row.cfg(t).configureTree(&resolve.GraphQLResponse{}, tree)
			assert.Nil(t, row.fetch.CacheConfig())
		})
	}
}
