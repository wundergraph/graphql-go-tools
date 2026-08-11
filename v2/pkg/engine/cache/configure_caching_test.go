package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// fetchTreeFixture builds a small flat fetch tree with one single fetch and
// one entity fetch, mirroring what the caching passes receive from postprocess.
func fetchTreeFixture() *resolve.FetchTreeNode {
	return &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindSequence,
		ChildNodes: []*resolve.FetchTreeNode{
			{
				Kind: resolve.FetchTreeNodeKindSingle,
				Item: &resolve.FetchItem{
					Fetch: &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 1},
						Info: &resolve.FetchInfo{
							DataSourceID:   "products",
							DataSourceName: "products",
						},
					},
				},
			},
			{
				Kind: resolve.FetchTreeNodeKindSingle,
				Item: &resolve.FetchItem{
					Fetch: &resolve.EntityFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}},
						Info: &resolve.FetchInfo{
							DataSourceID:   "reviews",
							DataSourceName: "reviews",
						},
					},
				},
			},
		},
	}
}

// TestConfigureCachingNoOpGate pins the single planner no-op gate: with no
// caching configuration, ConfigureCaching leaves the fetch tree
// byte-identical and stamps no config; a configuration that enables nothing
// leaves it untouched as well.
func TestConfigureCachingNoOpGate(t *testing.T) {
	t.Run("no caching configuration", func(t *testing.T) {
		c := NewConfigurator(nil)
		tree := fetchTreeFixture()
		response := &resolve.GraphQLResponse{Fetches: tree}
		c.ConfigureCaching(response, nil, tree)
		assert.Equal(t, fetchTreeFixture(), tree)
		assert.Nil(t, tree.ChildNodes[0].Item.Fetch.CacheConfig())
		assert.Nil(t, tree.ChildNodes[1].Item.Fetch.CacheConfig())
	})

	t.Run("a configuration enabling nothing stamps no config", func(t *testing.T) {
		c := NewConfigurator(&cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{"products": {}},
		})
		tree := fetchTreeFixture()
		response := &resolve.GraphQLResponse{Fetches: tree}
		c.ConfigureCaching(response, nil, tree)
		assert.Equal(t, fetchTreeFixture(), tree)
		assert.Nil(t, tree.ChildNodes[0].Item.Fetch.CacheConfig())
		assert.Nil(t, tree.ChildNodes[1].Item.Fetch.CacheConfig())
	})
}
