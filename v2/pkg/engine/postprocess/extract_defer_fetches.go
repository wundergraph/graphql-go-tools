package postprocess

import (
	"maps"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractDeferFetches struct {
	disable bool
}

func (d *extractDeferFetches) Process(deferPlan *plan.DeferResponsePlan) {
	if d.disable {
		return
	}

	root, fetchGroups := d.fetchGroups(deferPlan)

	deferPlan.Response.Fetches = &resolve.FetchTreeNode{
		Kind:       resolve.FetchTreeNodeKindSequence,
		ChildNodes: root,
	}

	deferIds := slices.Sorted(maps.Keys(fetchGroups))

	for _, deferID := range deferIds {
		fetches := fetchGroups[deferID]
		deferResponse := &resolve.DeferGraphQLResponse{
			DeferID: deferID,

			Fetches: &resolve.FetchTreeNode{
				Kind:       resolve.FetchTreeNodeKindSequence,
				ChildNodes: fetches,
			},
		}
		deferPlan.Defers = append(deferPlan.Defers, deferResponse)
	}
}

func (d *extractDeferFetches) fetchGroups(deferPlan *plan.DeferResponsePlan) (root []*resolve.FetchTreeNode, deffered map[string][]*resolve.FetchTreeNode) {
	fetchGroups := make(map[string][]*resolve.FetchTreeNode)

	for _, fetch := range deferPlan.Response.Fetches.ChildNodes {
		deferID := fetch.Item.Fetch.Dependencies().DeferID
		if deferID == "" {
			root = append(root, fetch)
			continue
		}

		fetchGroups[deferID] = append(fetchGroups[deferID], fetch)
	}

	return root, fetchGroups
}
