package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type mergeSameSourceFetches struct {
	disable bool
}

func (m *mergeSameSourceFetches) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if m.disable {
		return
	}

	fetchGroups := make(map[string][]*resolve.FetchTreeNode)

	for _, node := range root.ChildNodes {
		if node.Kind == resolve.FetchTreeNodeKindSingle {
			info := node.Item.Fetch.DataSourceInfo()
			key := info.ID // + string(node.Item.Fetch.(*resolve.SingleFetch).InputTemplate...)
			fetchGroups[key] = append(fetchGroups[key], node)
		}
	}

	var mergedNodes []*resolve.FetchTreeNode
	for _, group := range fetchGroups {
		if len(group) > 1 {
			mergedNode := mergeFetchNodes(group)
			mergedNodes = append(mergedNodes, mergedNode)
		} else {
			mergedNodes = append(mergedNodes, group[0])
		}
	}

	root.ChildNodes = mergedNodes
}

func mergeFetchNodes(nodes []*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	mergedNode := nodes[0]
	for _, node := range nodes[1:] {
		mergedNode.ChildNodes = append(mergedNode.ChildNodes, node.ChildNodes...)
	}
	return mergedNode
}
