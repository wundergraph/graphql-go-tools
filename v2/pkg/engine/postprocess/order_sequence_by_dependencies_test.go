package postprocess

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func sequenceToDeps(seq *resolve.FetchTreeNode) []resolve.FetchDependencies {
	result := make([]resolve.FetchDependencies, len(seq.ChildNodes))
	for i, node := range seq.ChildNodes {
		result[i] = node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies
	}
	return result
}

func depsToSequence(deps []resolve.FetchDependencies) *resolve.FetchTreeNode {
	result := &resolve.FetchTreeNode{
		ChildNodes: make([]*resolve.FetchTreeNode, len(deps)),
	}
	for i, dep := range deps {
		result.ChildNodes[i] = &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSingle,
			Item: &resolve.FetchItem{
				Fetch: &resolve.SingleFetch{FetchDependencies: dep},
			},
		}
	}
	return result
}

func prettyPrint(input any) string {
	out, _ := json.MarshalIndent(input, "", "  ")
	return string(out)
}

func TestOrderSquenceByDependencies_ProcessFetchTree(t *testing.T) {
	processor := &orderSequenceByDependencies{}
	t.Run("no dependencies", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 2},
			{FetchID: 0},
			{FetchID: 1},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0},
			{FetchID: 1},
			{FetchID: 2},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
	t.Run("serial dependencies", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 0},
			{FetchID: 2, DependsOnFetchIDs: []int{1}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{1}},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
	t.Run("more dependencies", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 4, DependsOnFetchIDs: []int{3}},
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 2, DependsOnFetchIDs: []int{1}},
			{FetchID: 3, DependsOnFetchIDs: []int{5, 1}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 5, DependsOnFetchIDs: []int{0}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 5, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{1}},
			{FetchID: 3, DependsOnFetchIDs: []int{5, 1}},
			{FetchID: 4, DependsOnFetchIDs: []int{3}},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
	t.Run("double dependencies", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{0, 5}},
			{FetchID: 3, DependsOnFetchIDs: []int{0, 1}},
			{FetchID: 4, DependsOnFetchIDs: []int{2}},
			{FetchID: 5, DependsOnFetchIDs: []int{0}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 5, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{0, 5}},
			{FetchID: 3, DependsOnFetchIDs: []int{0, 1}},
			{FetchID: 4, DependsOnFetchIDs: []int{2}},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
	t.Run("double dependencies variant", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 2, DependsOnFetchIDs: []int{0, 1}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 3, DependsOnFetchIDs: []int{2}},
			{FetchID: 5, DependsOnFetchIDs: []int{4}},
			{FetchID: 4, DependsOnFetchIDs: []int{2, 3}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{0, 1}},
			{FetchID: 3, DependsOnFetchIDs: []int{2}},
			{FetchID: 4, DependsOnFetchIDs: []int{2, 3}},
			{FetchID: 5, DependsOnFetchIDs: []int{4}},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
	t.Run("nested requires", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 3, DependsOnFetchIDs: []int{0, 2}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{0}},
			{FetchID: 4, DependsOnFetchIDs: []int{0, 1}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0, DependsOnFetchIDs: []int{}},
			{FetchID: 1, DependsOnFetchIDs: []int{0}},
			{FetchID: 2, DependsOnFetchIDs: []int{0}},
			{FetchID: 3, DependsOnFetchIDs: []int{0, 2}},
			{FetchID: 4, DependsOnFetchIDs: []int{0, 1}},
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		require.Equal(t, prettyPrint(expected), prettyPrint(sequenceToDeps(seq)))
	})
}
