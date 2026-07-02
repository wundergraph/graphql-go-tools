// Package cache is the ONE common home for all cache logic (PLAN.md D5): the
// plan-side postprocess passes (key building, fetch config assembly, L1
// narrowing) and, in later tasks, the runtime controller modules. The resolve
// package holds only the contract types and never imports this package;
// plan/postprocess hold only thin shims calling into it.
package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Configurator orchestrates the caching postprocess passes over finished fetch
// trees. It is constructed once per postprocess.Processor and holds the SINGLE
// planner no-op gate: with no providers configured, ConfigureCaching returns
// immediately and no fetch is ever touched.
type Configurator struct {
	providers    map[string]cacheconfig.CacheConfigProvider
	configurator *fetchCacheConfigurator
	l1           *optimizeL1Cache
}

// NewConfigurator builds the caching pass pipeline. providers and federation
// are keyed by datasource ID; definition is the composed schema the key
// builder resolves @key selection sets against.
func NewConfigurator(providers map[string]cacheconfig.CacheConfigProvider, federation map[string]plan.FederationMetaData, definition *ast.Document) *Configurator {
	keyBuilder := &cacheKeyBuilder{
		federation: federation,
		definition: definition,
	}
	return &Configurator{
		providers: providers,
		configurator: &fetchCacheConfigurator{
			providers:  providers,
			keyBuilder: keyBuilder,
		},
		l1: &optimizeL1Cache{},
	}
}

// ConfigureCaching runs the caching passes over the given fetch trees of one
// response: fetchCacheConfigurator assembles and sets the per-fetch
// *resolve.FetchCacheConfig, then optimizeL1Cache narrows cfg.L1 across all
// trees. It must run AFTER createConcreteSingleFetchTypes (the concrete fetch
// types carry the config); the passes walk flat AND organized trees alike,
// and the defer pipeline calls it AFTER buildDeferTree so the group trees and
// their ancestry come from the authoritative DeferTree.
//
// treeParents gives the narrowing pass the defer-group ancestry: one entry per
// tree, the index of the tree whose group ENCLOSES it (-1 for a root). The
// resolver resolves a parent group fully before its children, so an ancestor
// tree's fetches execute before every descendant tree's. Pass nil when there
// is only the root tree (or when ancestry is unknown — the pass then assumes
// only root-before-defers).
func (c *Configurator) ConfigureCaching(response *resolve.GraphQLResponse, treeParents []int, trees ...*resolve.FetchTreeNode) {
	if len(c.providers) == 0 {
		// The single planner no-op gate: no caching configured, nothing runs.
		return
	}
	for _, tree := range trees {
		c.configurator.configureTree(response, tree)
	}
	c.l1.optimize(trees, treeParents)
}
