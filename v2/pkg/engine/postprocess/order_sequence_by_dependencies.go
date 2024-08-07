package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// orderSequenceByDependencies is a postprocessor that orders the fetch tree nodes by their dependencies.
type orderSequenceByDependencies struct {
	disable bool
}

func (o *orderSequenceByDependencies) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if o.disable {
		return
	}
	slices.SortFunc(root.ChildNodes, func(_a, _b *resolve.FetchTreeNode) int {
		// get the fetch IDs of both nodes
		a, b := o.nodeFetchID(_a), o.nodeFetchID(_b)
		// for each node, recursively get the fetch IDs of all dependencies
		// this means that if a node depends on ID 1 and 2, and 2 depends on 3, the result will be [1, 2, 3]
		aDeps, bDeps := o.nodeDependsOn(_a, root), o.nodeDependsOn(_b, root)
		// if both nodes have the exact same dependencies, the node with the lower fetch ID should come first
		if slices.Equal(aDeps, bDeps) {
			return a - b
		}
		// if b's dependencies contain a, or in other words, b depends on a, a should come first
		if slices.Contains(bDeps, a) {
			return -1
		}
		// if a's dependencies contain b, or in other words, a depends on b, b should come first
		if slices.Contains(aDeps, b) {
			return 1
		}
		// both nodes have different dependencies, which might overlap, but they don't depend on each other
		// if both nodes have the same number of dependencies, the node with the lower fetch ID should come first
		if len(aDeps) == len(bDeps) {
			return a - b
		}
		// the node with fewer dependencies should come first
		return len(aDeps) - len(bDeps)
	})
}

func (o *orderSequenceByDependencies) nodeByFetchID(id int, root *resolve.FetchTreeNode) *resolve.FetchTreeNode {
	for _, node := range root.ChildNodes {
		if o.nodeFetchID(node) == id {
			return node
		}
	}
	return nil
}

func (o *orderSequenceByDependencies) nodeDependsOn(node, root *resolve.FetchTreeNode) []int {
	dependencies := node.Item.Fetch.Dependencies().DependsOnFetchIDs
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

func (o *orderSequenceByDependencies) nodeFetchID(node *resolve.FetchTreeNode) int {
	return node.Item.Fetch.Dependencies().FetchID
}
