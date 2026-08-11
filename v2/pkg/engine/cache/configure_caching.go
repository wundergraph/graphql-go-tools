// Package cache is the ONE common home for all cache logic: the plan-side
// postprocess passes (key building, fetch config assembly, L1 narrowing) and
// the runtime request-cache controller. The resolve package holds only the
// contract types and never imports this package; plan/postprocess hold only
// thin shims calling into it.
package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Configurator orchestrates the caching postprocess passes over finished fetch
// trees. It is constructed once per postprocess.Processor and holds the SINGLE
// planner no-op gate: with no caching configuration, ConfigureCaching returns
// immediately and no fetch is ever touched.
type Configurator struct {
	caching      *cacheconfig.CachingConfiguration
	configurator *fetchCacheConfigurator
	l1           *optimizeL1Cache
}

// NewConfigurator builds the caching pass pipeline from the caching
// configuration (nil = caching off); its per-subgraph overrides are keyed by
// datasource ID. Nothing else is needed: entity cache keys derive from the
// fetches' own representation nodes, so no federation metadata and no schema
// reach the caching passes.
func NewConfigurator(caching *cacheconfig.CachingConfiguration) *Configurator {
	return &Configurator{
		caching:      caching,
		configurator: &fetchCacheConfigurator{caching: caching},
		l1:           &optimizeL1Cache{},
	}
}

// ConfigureCaching runs the caching passes over one response's fetch trees:
// fetchCacheConfigurator sets the per-fetch *resolve.FetchCacheConfig, then
// optimizeL1Cache narrows cfg.L1 across all trees. It must run AFTER
// createConcreteSingleFetchTypes (the concrete fetch types carry the config)
// and, in the defer pipeline, AFTER buildDeferTree. treeParents gives the
// narrowing pass the defer-group ancestry — per tree, the index of the tree
// whose group ENCLOSES it, -1 for a root; nil means only root-before-defers
// ordering is assumed.
func (c *Configurator) ConfigureCaching(response *resolve.GraphQLResponse, treeParents []int, trees ...*resolve.FetchTreeNode) {
	if c.caching == nil {
		// The single planner no-op gate: no caching configured, nothing runs.
		return
	}
	for _, tree := range trees {
		c.configurator.configureTree(response, tree)
	}
	c.l1.optimize(trees, treeParents)
}
