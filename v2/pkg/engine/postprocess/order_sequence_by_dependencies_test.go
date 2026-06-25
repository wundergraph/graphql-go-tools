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
	t.Run("serial + requires dependencies", func(t *testing.T) {
		input := []resolve.FetchDependencies{
			{FetchID: 0},
			{FetchID: 1, DependsOnFetchIDs: []int{0, 2}},
			{FetchID: 2, DependsOnFetchIDs: []int{0}},
		}
		expected := []resolve.FetchDependencies{
			{FetchID: 0},
			{FetchID: 2, DependsOnFetchIDs: []int{0}},
			{FetchID: 1, DependsOnFetchIDs: []int{0, 2}},
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
	// Regression for the O(2^N) blowup: a densely-connected fetch tree where node
	// i depends on every earlier node [0..i-1] — the shape produced by mutations
	// with many aliased root fields (e.g. 28-31 aliased delete_webhook). The old
	// unmemoized recursive nodeDependsOn re-derived each node's transitive set on
	// every comparison, so a tree this size would never finish. With memoization
	// the result is computed once per ID and the test returns effectively instantly.
	// Reaching the assertion at all proves the exponential is gone; we also assert
	// the ordering is the expected ascending-by-fetchID sequence (0..N-1).
	t.Run("dense fully-connected chain (exponential regression)", func(t *testing.T) {
		const n = 31
		// Shuffle the input order so the sort has real work to do rather than
		// receiving an already-sorted slice.
		input := make([]resolve.FetchDependencies, 0, n)
		for i := n - 1; i >= 0; i-- {
			dependsOn := make([]int, 0, i)
			for j := 0; j < i; j++ {
				dependsOn = append(dependsOn, j)
			}
			input = append(input, resolve.FetchDependencies{FetchID: i, DependsOnFetchIDs: dependsOn})
		}
		seq := depsToSequence(input)
		processor.ProcessFetchTree(seq)
		got := sequenceToDeps(seq)
		require.Len(t, got, n)
		for i := range n {
			require.Equal(t, i, got[i].FetchID, "node at position %d should be fetchID %d", i, i)
		}
	})
}
