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

	deferPlan.Response.Response.Fetches = &resolve.FetchTreeNode{
		Kind:       resolve.FetchTreeNodeKindSequence,
		ChildNodes: root,
	}

	// sort defer ids in direct natural order
	deferIds := slices.Sorted(maps.Keys(fetchGroups))

	for _, deferID := range deferIds {
		fetches := fetchGroups[deferID]
		deferResponse := &resolve.DeferFetchGroup{
			DeferID: deferID,

			Fetches: &resolve.FetchTreeNode{
				Kind:       resolve.FetchTreeNodeKindSequence,
				ChildNodes: fetches,
			},
		}
		deferPlan.Response.Defers = append(deferPlan.Response.Defers, deferResponse)
	}
}

func (d *extractDeferFetches) fetchGroups(deferPlan *plan.DeferResponsePlan) (root []*resolve.FetchTreeNode, deffered map[int][]*resolve.FetchTreeNode) {
	fetchGroups := make(map[int][]*resolve.FetchTreeNode)

	for _, fetch := range deferPlan.Response.Response.Fetches.ChildNodes {
		deferID := fetch.Item.Fetch.Dependencies().DeferID
		if deferID == 0 {
			root = append(root, fetch)
			continue
		}

		fetchGroups[deferID] = append(fetchGroups[deferID], fetch)
	}

	return root, fetchGroups
}
