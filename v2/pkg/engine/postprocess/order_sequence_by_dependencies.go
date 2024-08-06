package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type orderSquenceByDependencies struct {
}

func (o *orderSquenceByDependencies) ProcessFetchTree(root *resolve.FetchTreeNode) {
	slices.SortFunc(root.ChildNodes, func(_a, _b *resolve.FetchTreeNode) int {
		a, b := o.nodeFetchID(_a), o.nodeFetchID(_b)
		aDeps, bDeps := o.nodeDependsOn(_a, root), o.nodeDependsOn(_b, root)
		if slices.Equal(aDeps, bDeps) {
			return a - b
		}
		if slices.Contains(bDeps, a) {
			return -1
		}
		if slices.Contains(aDeps, b) {
			return 1
		}
		if len(aDeps) == len(bDeps) {
			return b - a
		}
		return len(aDeps) - len(bDeps)
	})
}

func (o *orderSquenceByDependencies) nodeByFetchID(id int, root *resolve.FetchTreeNode) *resolve.FetchTreeNode {
	for _, node := range root.ChildNodes {
		if o.nodeFetchID(node) == id {
			return node
		}
	}
	return nil
}

func (o *orderSquenceByDependencies) nodeDependsOn(node, root *resolve.FetchTreeNode) []int {
	dependencies := node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs
	result := make([]int, 0, len(dependencies))
	for _, dep := range dependencies {
		result = append(result, dep)
		if child := o.nodeByFetchID(dep, root); child != nil {
			result = append(result, o.nodeDependsOn(child, root)...)
		}
	}
	index := make(map[int]struct{}, len(result))
	for _, id := range result {
		index[id] = struct{}{}
	}
	result = make([]int, 0, len(index))
	for id := range index {
		result = append(result, id)
	}
	slices.Sort(result)
	return result
}

func (o *orderSquenceByDependencies) nodeFetchID(node *resolve.FetchTreeNode) int {
	return node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.FetchID
}
