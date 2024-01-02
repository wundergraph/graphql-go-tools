package postprocess

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCreateMultiFetchTypes_Process(t *testing.T) {
	type TestCase struct {
		name     string
		pre      plan.Plan
		expected plan.Plan
	}

	cases := []TestCase{
		{
			name: "simple serial fetch",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchConfiguration: resolve.FetchConfiguration{Input: `c`}, FetchID: 5, DependsOnFetchIDs: []int{2}},
								{FetchConfiguration: resolve.FetchConfiguration{Input: `a`}, FetchID: 1, DependsOnFetchIDs: []int{0}},
								{FetchConfiguration: resolve.FetchConfiguration{Input: `b`}, FetchID: 2, DependsOnFetchIDs: []int{1}},
							},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SerialFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									FetchID:            1,
									DependsOnFetchIDs:  []int{0},
									FetchConfiguration: resolve.FetchConfiguration{Input: `a`},
								},
								&resolve.SingleFetch{
									FetchID:            2,
									DependsOnFetchIDs:  []int{1},
									FetchConfiguration: resolve.FetchConfiguration{Input: `b`},
								},
								&resolve.SingleFetch{
									FetchID:            5,
									DependsOnFetchIDs:  []int{2},
									FetchConfiguration: resolve.FetchConfiguration{Input: `c`},
								},
							},
						},
					},
				},
			},
		},
		{
			/*
				   parent ID_0            DEPTH: 0
				   /  |     \   \
				ID_1  ID_2   \     ID6    DEPTH: 1
				  \ /         \  /
				   ID_4	      ID_3        DEPTH: 2
				               \
				               ID_5       DEPTH: 3
			*/
			name: "complex dependency tree with a single parent fetch",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchID: 1, DependsOnFetchIDs: []int{0}},
								{FetchID: 2, DependsOnFetchIDs: []int{0}},
								{FetchID: 3, DependsOnFetchIDs: []int{0, 6}},
								{FetchID: 4, DependsOnFetchIDs: []int{1, 2}},
								{FetchID: 5, DependsOnFetchIDs: []int{3}},
								{FetchID: 6, DependsOnFetchIDs: []int{0}},
							},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SerialFetch{
							Fetches: []resolve.Fetch{
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchID: 1, DependsOnFetchIDs: []int{0}},
										&resolve.SingleFetch{FetchID: 2, DependsOnFetchIDs: []int{0}},
										&resolve.SingleFetch{FetchID: 6, DependsOnFetchIDs: []int{0}},
									},
								},
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchID: 3, DependsOnFetchIDs: []int{0, 6}},
										&resolve.SingleFetch{FetchID: 4, DependsOnFetchIDs: []int{1, 2}},
									},
								},
								&resolve.SingleFetch{FetchID: 5, DependsOnFetchIDs: []int{3}},
							},
						},
					},
				},
			},
		},
	}

	processor := &CreateMultiFetchTypes{}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := processor.Process(c.pre)

			if !assert.Equal(t, c.expected, actual) {
				actualBytes, _ := json.MarshalIndent(actual, "", "  ")
				expectedBytes, _ := json.MarshalIndent(c.expected, "", "  ")

				if string(expectedBytes) != string(actualBytes) {
					assert.Equal(t, string(expectedBytes), string(actualBytes))
					t.Error(cmp.Diff(string(expectedBytes), string(actualBytes)))
				}
			}
		})
	}
}
