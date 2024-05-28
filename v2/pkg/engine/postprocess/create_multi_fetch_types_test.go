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
		pre      *plan.SynchronousResponsePlan
		expected *plan.SynchronousResponsePlan
	}

	cases := []TestCase{
		{
			name: "parallel fetch without parent dependencies",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
							},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.ParallelFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
							},
						},
					},
				},
			},
		},
		{
			name: "serial fetch with the same level dependency",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{3}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
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
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{3}}},
							},
						},
					},
				},
			},
		},
		{
			name: "simple serial fetch",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{2}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}},
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
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{2}}},
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
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{0, 6}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 2}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{3}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{0}}},
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
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{0}}},
									},
								},
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{0, 6}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4, DependsOnFetchIDs: []int{1, 2}}},
									},
								},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{3}}},
							},
						},
					},
				},
			},
		},
		{
			/*
				   ID_0     ID_4         DEPTH: 0
				   /  |     \   \
				ID_1  ID_2   \  ID5      DEPTH: 1
				  \ /         \  /
				   ID_3	      ID_6       DEPTH: 2
				               \
				               ID_7      DEPTH: 3
			*/
			name: "complex dependency tree with 2 parent fetches",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{4}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{4, 5}}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 7, DependsOnFetchIDs: []int{6}}},
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
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{4}}},
									},
								},
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1, 2}}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 6, DependsOnFetchIDs: []int{4, 5}}},
									},
								},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 7, DependsOnFetchIDs: []int{6}}},
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
			processor.Process(c.pre.Response.Data)

			if !assert.Equal(t, c.expected, c.pre) {
				actualBytes, _ := json.MarshalIndent(c.pre, "", "  ")
				expectedBytes, _ := json.MarshalIndent(c.expected, "", "  ")

				if string(expectedBytes) != string(actualBytes) {
					assert.Equal(t, string(expectedBytes), string(actualBytes))
					t.Error(cmp.Diff(string(expectedBytes), string(actualBytes)))
				}
			}
		})
	}
}
