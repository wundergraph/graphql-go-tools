package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type buildDeferTree struct {
	disable bool
}

func (b *buildDeferTree) Process(response *resolve.GraphQLDeferResponse) {
	if b.disable || len(response.Defers) == 0 {
		return
	}

	// group DeferFetchGroups by their parent's DeferID
	childrenOf := make(map[int][]*resolve.DeferFetchGroup)
	for _, g := range response.Defers {
		desc, ok := response.DeferDescriptors[g.DeferID]
		if !ok {
			continue
		}
		childrenOf[desc.ParentID] = append(childrenOf[desc.ParentID], g)
	}

	// sort each sibling list by DeferID for deterministic tree shape
	for k := range childrenOf {
		slices.SortFunc(childrenOf[k], func(a, b *resolve.DeferFetchGroup) int {
			return a.DeferID - b.DeferID
		})
	}

	roots := childrenOf[0]
	if len(roots) == 0 {
		return
	}

	if len(roots) == 1 {
		response.DeferTree = b.buildChain(roots[0], childrenOf)
		return
	}

	branches := make([]*resolve.DeferTreeNode, len(roots))
	for i, root := range roots {
		branches[i] = b.buildChain(root, childrenOf)
	}
	response.DeferTree = resolve.DeferParallel(branches...)
}

// buildChain returns Single for a leaf, or Sequence(Single, subtree) when
// the group has children.
func (b *buildDeferTree) buildChain(
	group *resolve.DeferFetchGroup,
	childrenOf map[int][]*resolve.DeferFetchGroup,
) *resolve.DeferTreeNode {
	single := resolve.DeferSingle(group)

	children := childrenOf[group.DeferID]
	if len(children) == 0 {
		return single
	}

	childNodes := make([]*resolve.DeferTreeNode, len(children))
	for i, child := range children {
		childNodes[i] = b.buildChain(child, childrenOf)
	}

	var subtree *resolve.DeferTreeNode
	if len(childNodes) == 1 {
		subtree = childNodes[0]
	} else {
		subtree = resolve.DeferParallel(childNodes...)
	}

	return resolve.DeferSequence(single, subtree)
}
