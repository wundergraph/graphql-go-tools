package postprocess

import (
	"cmp"
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
	// Index of nodes for collecting all deps.
	nodeByID := make(map[int]*resolve.FetchTreeNode, len(root.ChildNodes))
	for _, node := range root.ChildNodes {
		nodeByID[node.FetchID()] = node
	}

	// bitset is being used to prevent allocations in the findAllDeps.
	allDeps := make(map[int]bitset, len(root.ChildNodes))

	// findAllDeps builds a full set of dependencies (depsMap) for each fetch.
	// If a node depends on ID 1 and 2, and 2 depends on 3, the resulting map will be {1, 2, 3}.
	var findAllDeps func(id int) bitset
	findAllDeps = func(id int) bitset {
		if m, found := allDeps[id]; found {
			return m
		}
		node := nodeByID[id]
		if node == nil {
			return nil // A defer-tree node depends on a fetch in main tree.
		}
		allDeps[id] = nil // Mark as in progress. First lookup will return nil on a cycle.
		dependencies := node.Item.Fetch.Dependencies().DependsOnFetchIDs
		var seen bitset
		for _, dep := range dependencies {
			// dep should not be negative, it will trip the bitset.
			seen.set(dep)
			seen.union(findAllDeps(dep))
		}
		allDeps[id] = seen
		return seen
	}
	for _, node := range root.ChildNodes {
		findAllDeps(node.FetchID())
	}

	slices.SortFunc(root.ChildNodes, func(nodeA, nodeB *resolve.FetchTreeNode) int {
		a, b := nodeA.FetchID(), nodeB.FetchID()
		// Order by fewer total dependencies first, then by the lower fetch ID.
		// It is a topological order because a dependent has strictly more dependencies than its dependency.
		return cmp.Or(
			cmp.Compare(allDeps[a].count(), allDeps[b].count()),
			cmp.Compare(a, b))
	})
}
