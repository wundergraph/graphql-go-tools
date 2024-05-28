package postprocess

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestDeduplicateMultiFetch_Process(t *testing.T) {
	type TestCase struct {
		name     string
		pre      *plan.SynchronousResponsePlan
		expected *plan.SynchronousResponsePlan
	}

	cases := []TestCase{
		{
			name: "parallel fetch without duplicates",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.ParallelFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}, FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}, FetchConfiguration: resolve.FetchConfiguration{Input: "b"}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}, FetchConfiguration: resolve.FetchConfiguration{Input: "c"}},
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
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}, FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}, FetchConfiguration: resolve.FetchConfiguration{Input: "b"}},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}, FetchConfiguration: resolve.FetchConfiguration{Input: "c"}},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple parallel fetches with duplicates",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SerialFetch{
							Fetches: []resolve.Fetch{
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}, FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}, FetchConfiguration: resolve.FetchConfiguration{Input: "b"}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 6}, FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
									},
								},
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}, FetchConfiguration: resolve.FetchConfiguration{Input: "c"}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 4}, FetchConfiguration: resolve.FetchConfiguration{Input: "c"}},
									},
								},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{3}}},
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
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}, FetchConfiguration: resolve.FetchConfiguration{Input: "a"}},
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}, FetchConfiguration: resolve.FetchConfiguration{Input: "b"}},
									},
								},
								&resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}, FetchConfiguration: resolve.FetchConfiguration{Input: "c"}},
									},
								},
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 5, DependsOnFetchIDs: []int{3}}},
							},
						},
					},
				},
			},
		},
	}

	processor := &DeduplicateMultiFetch{}

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
