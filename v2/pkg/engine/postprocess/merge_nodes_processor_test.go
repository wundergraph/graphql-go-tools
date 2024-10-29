package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMergeSameSourceFetches_ProcessFetchTree(t *testing.T) {
	t.Run("merge single fetches with same data source", func(t *testing.T) {
		processor := &mergeSameSourceFetches{}
		input := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies:    resolve.FetchDependencies{FetchID: 0},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				Info:                 &resolve.FetchInfo{DataSourceID: "1", DataSourceName: "a"},
			}),
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           1,
					DependsOnFetchIDs: []int{0},
				},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				Info:                 &resolve.FetchInfo{DataSourceID: "1", DataSourceName: "a"},
			}),
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           2,
					DependsOnFetchIDs: []int{0},
				},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				Info:                 &resolve.FetchInfo{DataSourceID: "1", DataSourceName: "a"},
			}),
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           3,
					DependsOnFetchIDs: []int{1},
				},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				Info:                 &resolve.FetchInfo{DataSourceID: "1", DataSourceName: "a"},
			}),
		)
		processor.ProcessFetchTree(input)
		expected := resolve.Sequence(
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies:    resolve.FetchDependencies{FetchID: 0},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
			}),
			resolve.Parallel(
				resolve.Single(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           1,
						DependsOnFetchIDs: []int{0},
					},
					DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				}),
				resolve.Single(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           2,
						DependsOnFetchIDs: []int{0},
					},
					DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
				}),
			),
			resolve.Single(&resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID:           3,
					DependsOnFetchIDs: []int{1},
				},
				DataSourceIdentifier: []byte(resolve.DataSourceInfo{ID: "1", Name: "a"}.ID),
			}),
		)
		require.Equal(t, expected, input)
	})
}
