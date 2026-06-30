package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cachingPlanner struct {
	providers map[string]cacheconfig.CacheConfigProvider
	freezer   *cacheKeySpecFreezer
	stamper   *cacheConfigStamper
	l1        *optimizeL1Cache
}

func (c *cachingPlanner) Annotate(resp *resolve.GraphQLResponse, trees ...*resolve.FetchTreeNode) {
	if c == nil || len(c.providers) == 0 || resp == nil {
		return
	}
	pd := resp.CacheProvidesData()
	for _, tree := range trees {
		c.stamper.process(tree, pd)
	}
	c.l1.processTrees(trees...)
}

func deferTrees(d *plan.DeferResponsePlan) []*resolve.FetchTreeNode {
	trees := make([]*resolve.FetchTreeNode, 0, 1+len(d.Response.Defers))
	trees = append(trees, d.Response.Response.Fetches)
	for _, g := range d.Response.Defers {
		trees = append(trees, g.Fetches)
	}
	return trees
}
