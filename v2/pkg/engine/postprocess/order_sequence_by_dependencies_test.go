package postprocess

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestOrderSequenceByDependencies_ProcessFetchTree(t *testing.T) {
	t.Run("no dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(2),
			sf(0),
			sf(1),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1),
			sf(2),
		)
		require.Equal(t, expected, input)
	})
	t.Run("serial dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(2, dependsOn(1)),
			sf(1, dependsOn(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(2, dependsOn(1)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("serial + requires dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(1, dependsOn(0, 2)),
			sf(2, dependsOn(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(2, dependsOn(0)),
			sf(1, dependsOn(0, 2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("more dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(4, dependsOn(3)),
			sf(0),
			sf(2, dependsOn(1)),
			sf(3, dependsOn(5, 1)),
			sf(1, dependsOn(0)),
			sf(5, dependsOn(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(5, dependsOn(0)),
			sf(2, dependsOn(1)),
			sf(3, dependsOn(5, 1)),
			sf(4, dependsOn(3)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("double dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(2, dependsOn(0, 5)),
			sf(3, dependsOn(0, 1)),
			sf(4, dependsOn(2)),
			sf(5, dependsOn(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(5, dependsOn(0)),
			sf(2, dependsOn(0, 5)),
			sf(3, dependsOn(0, 1)),
			sf(4, dependsOn(2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("double dependencies variant", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(2, dependsOn(0, 1)),
			sf(1, dependsOn(0)),
			sf(3, dependsOn(2)),
			sf(5, dependsOn(4)),
			sf(4, dependsOn(2, 3)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(2, dependsOn(0, 1)),
			sf(3, dependsOn(2)),
			sf(4, dependsOn(2, 3)),
			sf(5, dependsOn(4)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("nested requires", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(3, dependsOn(0, 2)),
			sf(1, dependsOn(0)),
			sf(2, dependsOn(0)),
			sf(4, dependsOn(0, 1)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, dependsOn(0)),
			sf(2, dependsOn(0)),
			sf(3, dependsOn(0, 2)),
			sf(4, dependsOn(0, 1)),
		)
		require.Equal(t, expected, input)
	})

	t.Run("dependent with fetch ID 0 must come after its dependency", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0, dependsOn(3)),
			sf(3, dependsOn(1, 2)),
			sf(1, dependsOn(5)),
			sf(2, dependsOn(5)),
			sf(5),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(5),
			sf(1, dependsOn(5)),
			sf(2, dependsOn(5)),
			sf(3, dependsOn(1, 2)),
			sf(0, dependsOn(3)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("equal transitive dependencies tie-break by fetch ID (diamond)", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(7, dependsOn(4, 5)),
			sf(6, dependsOn(3, 4, 5)),
			sf(3),
			sf(4, dependsOn(3)),
			sf(5, dependsOn(3)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(3),
			sf(4, dependsOn(3)),
			sf(5, dependsOn(3)),
			sf(6, dependsOn(3, 4, 5)),
			sf(7, dependsOn(4, 5)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("duplicate direct dependency IDs tie-break by fetch ID", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(3, dependsOn(1)),
			sf(2, dependsOn(1, 1)),
			sf(1),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(1),
			sf(2, dependsOn(1, 1)),
			sf(3, dependsOn(1)),
		)
		require.Equal(t, expected, input)
	})

	t.Run("dense fully-connected chain (exponential regression)", func(t *testing.T) {
		// This happens on mutations that have many fetches.
		// Node i depends on every node j > i,
		// so the correct order is the reverse of the ascending input.
		const n = 255
		tree := seq(denseChain(n)...)
		processor := &orderSequenceByDependencies{}
		processor.ProcessFetchTree(tree)
		require.Len(t, tree.ChildNodes, n)
		for i := range n {
			require.Equal(t, n-1-i, tree.ChildNodes[i].FetchID(), "node at position %d should be fetchID %d", i, n-1-i)
		}
	})
}

// denseChain returns n fetches where fetch i depends on every fetch j > i.
func denseChain(n int) []*resolve.FetchTreeNode {
	input := make([]*resolve.FetchTreeNode, 0, n)
	for i := range n {
		deps := make([]int, 0, n-i-1)
		for j := i + 1; j < n; j++ {
			deps = append(deps, j)
		}
		input = append(input, sf(i, dependsOn(deps...)))
	}
	return input
}

func BenchmarkOrderSequenceByDependencies_Dense(b *testing.B) {
	for _, n := range []int{50, 100, 255} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			processor := &orderSequenceByDependencies{}
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				tree := seq(denseChain(n)...)
				b.StartTimer()
				processor.ProcessFetchTree(tree)
			}
		})
	}
}
