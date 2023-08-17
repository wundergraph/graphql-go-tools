package postprocess

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

func TestProcessModifyHeader_Process(t *testing.T) {
	headerModifier := func(header http.Header) {
		for headerKey := range header {
			for i := range header[headerKey] {
				header[headerKey][i] = fmt.Sprintf("%s modified", header[headerKey][i])
			}
		}
	}

	t.Run("should modify header of a synchronous plan", func(t *testing.T) {
		runTest := func(f resolve.Fetch, expectedFetch resolve.Fetch) func(t *testing.T) {
			return func(t *testing.T) {
				p := &plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Array{
							Item: &resolve.Object{
								Fetch: f,
							},
						},
					},
				}

				modifyHeaderPostProcessor := NewProcessModifyHeader(headerModifier)
				modifyHeaderPostProcessor.Process(p)
				assert.Equal(t, expectedFetch, p.Response.Data.(*resolve.Array).Item.(*resolve.Object).Fetch)
			}
		}

		fetchInput := `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"},"header":{"x-my-header":["my-value"]}}`
		expectedFetchInput := `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"},"header":{"x-my-header":["my-value modified"]}}`
		t.Run("should modify a single fetch", runTest(
			&resolve.SingleFetch{
				Input: fetchInput,
			},
			&resolve.SingleFetch{
				Input: expectedFetchInput,
			},
		))

		t.Run("should modify a batch fetch", runTest(
			&resolve.BatchFetch{
				Fetch: &resolve.SingleFetch{
					Input: fetchInput,
				},
			},
			&resolve.BatchFetch{
				Fetch: &resolve.SingleFetch{
					Input: expectedFetchInput,
				},
			},
		))

		t.Run("should modify a parallel fetch", runTest(
			&resolve.ParallelFetch{
				Fetches: resolve.Fetches{
					&resolve.SingleFetch{
						Input: fetchInput,
					},
					&resolve.SingleFetch{
						Input: fetchInput,
					},
				},
			},
			&resolve.ParallelFetch{
				Fetches: resolve.Fetches{
					&resolve.SingleFetch{
						Input: expectedFetchInput,
					},
					&resolve.SingleFetch{
						Input: expectedFetchInput,
					},
				},
			},
		))
	})
}
