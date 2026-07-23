package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateParallelNodes_ProcessFetchTree(t *testing.T) {
	t.Run("root with 2 dependent children and one 3rd child", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(1)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			sf(3, deps(1)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("root with 2 dependent children and one 3rd child variant", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(2)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			sf(3, deps(2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("root with 2 dependent children and one 3rd child variant 2", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(1, 2)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			sf(3, deps(1, 2)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels depending on each other", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(1, 2)),
			sf(4, deps(1, 2)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			par(
				sf(3, deps(1, 2)),
				sf(4, deps(1, 2)),
			),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels depending on each other mixed dependencies", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(1)),
			sf(4, deps(2)),
			sf(5, deps(4)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			par(
				sf(3, deps(1)),
				sf(4, deps(2)),
			),
			sf(5, deps(4)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels with single in the middle", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1, deps(0)),
			sf(2, deps(0)),
			sf(3, deps(1, 2)),
			sf(4, deps(1, 3)),
			sf(5, deps(2, 3)),
			sf(6, deps(4, 5)),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0),
			par(
				sf(1, deps(0)),
				sf(2, deps(0)),
			),
			sf(3, deps(1, 2)),
			par(
				sf(4, deps(1, 3)),
				sf(5, deps(2, 3)),
			),
			sf(6, deps(4, 5)),
		)
		require.Equal(t, expected, input)
	})
	t.Run("3 fetches in parallel without dependencies", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := seq(
			sf(0),
			sf(1),
			sf(2),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			par(
				sf(0),
				sf(1),
				sf(2),
			),
		)
		require.Equal(t, expected, input)
	})
}
