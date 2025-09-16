package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCreateParallelNodes_ProcessFetchTree(t *testing.T) {
	t.Run("root with 2 dependent children and one 3rd child", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}}),
		)
		require.Equal(t, expected, input)
	})
	t.Run("root with 2 dependent children and one 3rd child variant", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{2}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{2}}}),
		)
		require.Equal(t, expected, input)
	})
	t.Run("root with 2 dependent children and one 3rd child variant 2", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels depending on each other", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 2}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 2}}}),
			),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels depending on each other mixed dependencies", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{2}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{4}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{2}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{4}}}),
		)
		require.Equal(t, expected, input)
	})
	t.Run("2 parallels with single in the middle", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 3}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{2, 3}}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{4, 5}}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 3}}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{2, 3}}}),
			),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{4, 5}}}),
		)
		require.Equal(t, expected, input)
	})
	t.Run("3 fetches in parallel without dependencies", func(t *testing.T) {
		processor := &createParallelNodes{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}),
			resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 0}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}),
				resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}}),
			),
		)
		require.Equal(t, expected, input)
	})
}
