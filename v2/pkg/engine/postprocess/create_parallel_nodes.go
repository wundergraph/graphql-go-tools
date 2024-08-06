package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type createParallelNodes struct {
	disable bool
}

func (c *createParallelNodes) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if c.disable {
		return
	}
	for i := 0; i < len(root.ChildNodes); i++ {
		providedFetchIDs := resolveProvidedFetchIDs(root.ChildNodes[:i])
		parallel := resolve.Parallel(root.ChildNodes[i])
		for j := i + 1; j < len(root.ChildNodes); j++ {
			if c.dependenciesCanBeProvided(root.ChildNodes[j], providedFetchIDs) {
				parallel.ChildNodes = append(parallel.ChildNodes, root.ChildNodes[j])
				root.ChildNodes = append(root.ChildNodes[:j], root.ChildNodes[j+1:]...)
				j--
			}
		}
		if len(parallel.ChildNodes) > 1 {
			root.ChildNodes[i] = parallel
		}
	}
}

func (c *createParallelNodes) dependenciesCanBeProvided(node *resolve.FetchTreeNode, providedFetchIDs []int) bool {
	deps := node.Item.Fetch.Dependencies()
	for _, dep := range deps.DependsOnFetchIDs {
		if !slices.Contains(providedFetchIDs, dep) {
			return false
		}
	}
	return true
}

func resolveProvidedFetchIDs(nodes []*resolve.FetchTreeNode) []int {
	provided := make([]int, 0, len(nodes))
	for _, node := range nodes {
		switch node.Kind {
		case resolve.FetchTreeNodeKindSingle:
			deps := node.Item.Fetch.Dependencies()
			provided = append(provided, deps.FetchID)
		case resolve.FetchTreeNodeKindParallel:
			provided = append(provided, resolveProvidedFetchIDs(node.ChildNodes)...)
		case resolve.FetchTreeNodeKindSequence:
			provided = append(provided, resolveProvidedFetchIDs(node.ChildNodes)...)
		}
	}
	return provided
}
