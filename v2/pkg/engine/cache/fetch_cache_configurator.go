package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// fetchCacheConfigurator assembles the self-contained *resolve.FetchCacheConfig
// for each cache-eligible fetch (policy + frozen key spec + ProvidesData) and
// sets it on the concrete fetch via Fetch.SetCacheConfig — after
// createConcreteSingleFetchTypes, so the config lands on the final fetch types.
//
// Skeleton only — the entity arm lands with task 06, the root-field arm with
// task 13.
type fetchCacheConfigurator struct {
	providers  map[string]cacheconfig.CacheConfigProvider
	keyBuilder *cacheKeyBuilder
}

// configureTree walks one flat fetch tree and stamps per-fetch cache config.
// Inert until task 06; every fetch keeps a nil Cache, preserving the planner
// no-op gate.
func (c *fetchCacheConfigurator) configureTree(response *resolve.GraphQLResponse, tree *resolve.FetchTreeNode) {
}
