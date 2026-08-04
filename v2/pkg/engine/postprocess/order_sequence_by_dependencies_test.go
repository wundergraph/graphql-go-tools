package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			sf(2, deps(1)),
			sf(1, deps(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(1)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("serial + requires dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(1, deps(0, 2)),
			sf(2, deps(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(2, deps(0)),
			sf(1, deps(0, 2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("more dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(4, deps(3)),
			sf(0),
			sf(2, deps(1)),
			sf(3, deps(5, 1)),
			sf(1, deps(0)),
			sf(5, deps(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, deps(0)),
			sf(5, deps(0)),
			sf(2, deps(1)),
			sf(3, deps(5, 1)),
			sf(4, deps(3)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("double dependencies", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0, 5)),
			sf(3, deps(0, 1)),
			sf(4, deps(2)),
			sf(5, deps(0)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, deps(0)),
			sf(5, deps(0)),
			sf(2, deps(0, 5)),
			sf(3, deps(0, 1)),
			sf(4, deps(2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("double dependencies variant", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(2, deps(0, 1)),
			sf(1, deps(0)),
			sf(3, deps(2)),
			sf(5, deps(4)),
			sf(4, deps(2, 3)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0, 1)),
			sf(3, deps(2)),
			sf(4, deps(2, 3)),
			sf(5, deps(4)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("nested requires", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0),
			sf(3, deps(0, 2)),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(4, deps(0, 1)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(0, 2)),
			sf(4, deps(0, 1)),
		)
		require.Equal(t, expected, input)
	})

	t.Run("dependent with fetch ID 0 must come after its dependency", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(0, deps(3)),
			sf(3, deps(1, 2)),
			sf(1, deps(5)),
			sf(2, deps(5)),
			sf(5),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(5),
			sf(1, deps(5)),
			sf(2, deps(5)),
			sf(3, deps(1, 2)),
			sf(0, deps(3)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("equal transitive dependencies tie-break by fetch ID (diamond)", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(7, deps(4, 5)),
			sf(6, deps(3, 4, 5)),
			sf(3),
			sf(4, deps(3)),
			sf(5, deps(3)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(3),
			sf(4, deps(3)),
			sf(5, deps(3)),
			sf(6, deps(3, 4, 5)),
			sf(7, deps(4, 5)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("duplicate direct dependency IDs tie-break by fetch ID", func(t *testing.T) {
		processor := &orderSequenceByDependencies{}
		input := seq(
			sf(3, deps(1)),
			sf(2, deps(1, 1)),
			sf(1),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(1),
			sf(2, deps(1, 1)),
			sf(3, deps(1)),
		)
		require.Equal(t, expected, input)
	})
}

// func TestOrderSequenceByDependencies_DependencyCycle(t *testing.T) {
// 	processor := &orderSequenceByDependencies{}
// 	processor.ProcessFetchTree(seq(
// 		sf(1, deps(2)),
// 		sf(2, deps(1)),
// 	))
// }
//
// func TestOrderSequenceByDependencies_SelfDependency(t *testing.T) {
// 	processor := &orderSequenceByDependencies{}
// 	processor.ProcessFetchTree(seq(
// 		sf(0),
// 		sf(1, deps(1)),
// 	))
// }
