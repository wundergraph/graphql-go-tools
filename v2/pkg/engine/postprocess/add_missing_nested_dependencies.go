package postprocess

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type addMissingNestedDependencies struct {
	disable bool
}

func (a *addMissingNestedDependencies) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if a.disable {
		return
	}
	for i, node := range root.ChildNodes {
		if len(node.Item.ResponsePath) == 0 {
			continue
		}
		if len(node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs) != 0 {
			continue
		}
		for j, otherNode := range root.ChildNodes {
			if i == j {
				continue
			}
			if strings.HasPrefix(node.Item.ResponsePath, a.providedPathByNode(otherNode)) {
				node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs = append(node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs, otherNode.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.FetchID)
			}
		}
	}
}

func (a *addMissingNestedDependencies) providedPathByNode(node *resolve.FetchTreeNode) string {
	mergePath := strings.Join(node.Item.Fetch.(*resolve.SingleFetch).PostProcessing.MergePath, ".")
	if node.Item.ResponsePath != "" {
		return node.Item.ResponsePath + "." + mergePath
	}
	return mergePath
}
