package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestAddMissingNestedDependencies_ProcessFetchTree(t *testing.T) {
	t.Run("add missing dependencies to nested fetches on same merge path", func(t *testing.T) {
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "a",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"a"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 0,
				},
			}),
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "b",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"b"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 1,
				},
			}),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "c",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 2,
				},
			}, "a", resolve.ObjectPath("a")),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "d",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 3,
				},
			}, "b.c", resolve.ObjectPath("b"), resolve.ObjectPath("c")),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "x",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           4,
					DependsOnFetchIDs: []int{0},
				},
			}, "a", resolve.ObjectPath("a")),
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "y",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"y"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 5,
				},
			}),
		)

		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "a",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"a"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 0,
				},
			}),
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "b",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"b"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 1,
				},
			}),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "c",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           2,
					DependsOnFetchIDs: []int{0},
				},
			}, "a", resolve.ObjectPath("a")),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "d",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           3,
					DependsOnFetchIDs: []int{1},
				},
			}, "b.c", resolve.ObjectPath("b"), resolve.ObjectPath("c")),
			resolve.SingleWithPath(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "x",
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           4,
					DependsOnFetchIDs: []int{0},
				},
			}, "a", resolve.ObjectPath("a")),
			resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input: "y",
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"y"},
					},
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 5,
				},
			}),
		)

		processor := &addMissingNestedDependencies{}
		processor.ProcessFetchTree(input)
		require.Equal(t, expected, input)
	})
}
