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
// providers, ConfigureCaching leaves the fetch tree byte-identical and stamps
// no config; with providers configured, the task-03 skeletons are wired but
// inert, so the tree is still untouched.
func TestConfigureCachingNoOpGate(t *testing.T) {
	t.Run("no providers configured", func(t *testing.T) {
		c := NewConfigurator(nil, nil, nil)
		tree := fetchTreeFixture()
		response := &resolve.GraphQLResponse{Fetches: tree}
		c.ConfigureCaching(response, tree)
		assert.Equal(t, fetchTreeFixture(), tree)
		assert.Nil(t, tree.ChildNodes[0].Item.Fetch.CacheConfig())
		assert.Nil(t, tree.ChildNodes[1].Item.Fetch.CacheConfig())
	})

	t.Run("providers configured, skeleton passes are inert", func(t *testing.T) {
		providers := map[string]cacheconfig.CacheConfigProvider{
			"products": &cacheconfig.CachingConfiguration{},
		}
		c := NewConfigurator(providers, nil, nil)
		tree := fetchTreeFixture()
		response := &resolve.GraphQLResponse{Fetches: tree}
		c.ConfigureCaching(response, tree)
		assert.Equal(t, fetchTreeFixture(), tree)
		assert.Nil(t, tree.ChildNodes[0].Item.Fetch.CacheConfig())
		assert.Nil(t, tree.ChildNodes[1].Item.Fetch.CacheConfig())
	})
}
