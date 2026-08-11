package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func newEntityConfigurator(caching *cacheconfig.CachingConfiguration) *fetchCacheConfigurator {
	return &fetchCacheConfigurator{caching: caching}
}

func productCaching(subgraph cacheconfig.SubgraphCacheConfig) *cacheconfig.CachingConfiguration {
	return &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{"products": subgraph},
	}
}

func ptr[T any](value T) *T {
	return &value
}

// upcRepresentation is the expected frozen representation for a Product fetch
// keyed by "upc".
func upcRepresentation() *resolve.Object {
	return &resolve.Object{
		Nullable: true,
		Fields: []*resolve.Field{
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Product")},
			},
			{
				Name:                []byte("upc"),
				Value:               &resolve.String{Path: []string{"upc"}},
				OnTypeNames:         [][]byte{[]byte("Product")},
				CacheEntityKeyField: true,
			},
		},
	}
}

func TestFetchCacheConfiguratorEntityArm(t *testing.T) {
	fullCaching := &cacheconfig.CachingConfiguration{
		Global: cacheconfig.GlobalCacheConfig{HashAnalyticsKeys: true},
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"products": {
				DefaultTTL:             ptr(time.Minute),
				NegativeCacheTTL:       ptr(5 * time.Second),
				EnablePartialCacheLoad: ptr(true),
				ShadowMode:             ptr(true),
				IncludeSubgraphHeaders: ptr(true),
			},
		},
	}

	t.Run("entity fetch receives the full config", func(t *testing.T) {
		configurator := newEntityConfigurator(fullCaching)
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
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		fetch.Info = info
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
			L1:                     true,
			L2:                     true,
			SubgraphName:           "products",
			TTL:                    time.Minute,
			NegativeCacheTTL:       5 * time.Second,
			IncludeSubgraphHeaders: true,
			EnablePartialCacheLoad: true,
			ShadowMode:             true,
			HashAnalyticsKeys:      true,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
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
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Minute),
		}))
		fetch := batchEntityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			TTL:          time.Minute,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	t.Run("a private subgraph marks its entity fetches private", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Minute),
			Scope:      ptr(cacheconfig.CacheScopePrivate),
		}))
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			TTL:          time.Minute,
			Private:      true,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	t.Run("the global DefaultTTL alone caches a subgraph without an entry", func(t *testing.T) {
		configurator := newEntityConfigurator(&cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: 30 * time.Second},
		})
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			TTL:          30 * time.Second,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	t.Run("the negative cache TTL alone caches the subgraph", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			NegativeCacheTTL: ptr(5 * time.Second),
		}))
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:               true,
			L2:               true,
			SubgraphName:     "products",
			NegativeCacheTTL: 5 * time.Second,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	nilRows := []struct {
		name    string
		fetch   func(t *testing.T) resolve.Fetch
		caching *cacheconfig.CachingConfiguration
	}{
		{
			// A single fetch takes the root-field arm, which needs a coordinate
			// entry; the subgraph's entity settings do not reach it.
			name:    "single fetch without a root field entry",
			fetch:   func(*testing.T) resolve.Fetch { return &resolve.SingleFetch{Info: productEntityInfo()} },
			caching: fullCaching,
		},
		{
			name: "a subgraph without any resolved TTL",
			fetch: func(t *testing.T) resolve.Fetch {
				return entityFetchWith(productRepresentation(t, "upc"))
			},
			caching: &cacheconfig.CachingConfiguration{},
		},
		{
			name: "a shadow-only subgraph does not cache entities",
			fetch: func(t *testing.T) resolve.Fetch {
				return entityFetchWith(productRepresentation(t, "upc"))
			},
			caching: productCaching(cacheconfig.SubgraphCacheConfig{ShadowMode: ptr(true)}),
		},
		{
			name: "the subgraph veto beats a positive TTL",
			fetch: func(t *testing.T) resolve.Fetch {
				return entityFetchWith(productRepresentation(t, "upc"))
			},
			caching: productCaching(cacheconfig.SubgraphCacheConfig{
				Enabled:    ptr(false),
				DefaultTTL: ptr(time.Minute),
			}),
		},
		{
			// No representation on the fetch means no key material at all.
			name:    "no usable key",
			fetch:   func(*testing.T) resolve.Fetch { return &resolve.EntityFetch{Info: productEntityInfo()} },
			caching: fullCaching,
		},
		{
			name:    "nil fetch info",
			fetch:   func(*testing.T) resolve.Fetch { return &resolve.EntityFetch{} },
			caching: fullCaching,
		},
		{
			// An abstract-path entity fetch can collect one root coordinate
			// per enclosing concrete type; the key spec derives from
			// RootFields[0].TypeName, so mixed types decline entirely.
			name: "mixed entity types decline caching",
			fetch: func(t *testing.T) resolve.Fetch {
				fetch := entityFetchWith(productRepresentation(t, "upc"))
				fetch.Info = &resolve.FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Product", FieldName: "name"},
						{TypeName: "User", FieldName: "username"},
					},
				}
				return fetch
			},
			caching: fullCaching,
		},
	}
	for _, row := range nilRows {
		t.Run(row.name, func(t *testing.T) {
			fetch := row.fetch(t)
			tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
			newEntityConfigurator(row.caching).configureTree(&resolve.GraphQLResponse{}, tree)
			assert.Nil(t, fetch.CacheConfig())
		})
	}
}
