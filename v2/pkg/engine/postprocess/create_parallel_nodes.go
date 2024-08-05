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
	for i := 0; i < len(root.SerialNodes); i++ {
		providedFetchIDs := resolveProvidedFetchIDs(root.SerialNodes[:i])
		parallel := resolve.Parallel(root.SerialNodes[i])
		for j := i + 1; j < len(root.SerialNodes); j++ {
			if c.dependenciesCanBeProvided(root.SerialNodes[j], providedFetchIDs) {
				parallel.ParallelNodes = append(parallel.ParallelNodes, root.SerialNodes[j])
				root.SerialNodes = append(root.SerialNodes[:j], root.SerialNodes[j+1:]...)
				j--
			}
		}
		if len(parallel.ParallelNodes) > 1 {
			root.SerialNodes[i] = parallel
		}
	}
}

func (c *createParallelNodes) dependenciesCanBeProvided(node *resolve.FetchTreeNode, providedFetchIDs []int) bool {
	dependencies := node.Item.Fetch.(*resolve.SingleFetch).DependsOnFetchIDs
	for _, dep := range dependencies {
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
			provided = append(provided, node.Item.Fetch.(*resolve.SingleFetch).FetchID)
		case resolve.FetchTreeNodeKindParallel:
			provided = append(provided, resolveProvidedFetchIDs(node.ParallelNodes)...)
		case resolve.FetchTreeNodeKindSequence:
			provided = append(provided, resolveProvidedFetchIDs(node.SerialNodes)...)
		}
	}
	return provided
}
