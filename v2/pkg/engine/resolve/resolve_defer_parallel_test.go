package resolve

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type testDeferWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	payloads []string
	complete bool
}

func (w *testDeferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testDeferWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, w.buf.String())
	w.buf.Reset()
	return nil
}

func (w *testDeferWriter) Complete() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.complete = true
}

// minimalDeferResponse builds a GraphQLDeferResponse with an empty initial
// response and one deferred field per group. Each group's fetch returns the
// provided JSON blob via FakeDataSource.
func minimalDeferResponse(groups []*DeferFetchGroup, descriptors map[int]DeferDescriptor) *GraphQLDeferResponse {
	fields := make([]*Field, len(groups))
	for i, g := range groups {
		fields[i] = &Field{
			Name:  []byte(fmt.Sprintf("f%d", g.DeferID)),
			Defer: &DeferField{DeferID: g.DeferID},
			Value: &String{
				Path:     []string{fmt.Sprintf("f%d", g.DeferID)},
				Nullable: true,
			},
		}
	}
	return &GraphQLDeferResponse{
		DeferDescriptors: descriptors,
		Response: &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Data: &Object{
				Nullable: true,
				Fields:   fields,
			},
		},
	}
}

// TestResolveDeferTree_TwoParallelSiblings: two root-level defers produce three
// flushed payloads (one initial response + two deferred incremental payloads)
// in any order for the deferred ones, with no data races.
func TestResolveDeferTree_TwoParallelSiblings(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"valueA"}`)},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f2":"valueB"}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	// 1 initial response (pending frame) + 2 deferred incremental payloads
	assert.Len(t, writer.payloads, 3)
	assert.True(t, writer.complete)
}

// TestResolveDeferTree_SequenceOrdering: child defer only executes after parent
// has flushed its payload.
func TestResolveDeferTree_SequenceOrdering(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"parent"}`)},
		}),
	}
	groupC := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f2":"child"}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupC}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 1},
	})
	response.DeferTree = DeferSequence(DeferSingle(groupA), DeferSingle(groupC))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	// 1 initial response (pending frame) + 2 deferred incremental payloads
	require.Len(t, writer.payloads, 3)
	assert.Contains(t, writer.payloads[1], "parent")
	assert.Contains(t, writer.payloads[2], "child")
}

// TestResolveDeferTree_SiblingFailureIsIndependent: an error in one sibling
// goroutine does not cancel the other sibling.
func TestResolveDeferTree_SiblingFailureIsIndependent(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"f1":"valueA"}`)},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{}`)},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	// 1 initial response (pending frame) + 2 deferred incremental payloads
	assert.Len(t, writer.payloads, 3)
}

func TestResolveDeferTree_ParallelSiblings_ErrorsAreIsolated(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pass-through mode so raw subgraph error messages appear verbatim in the
	// incremental frames, making them easy to assert on.
	r := New(rCtx, ResolverOptions{
		MaxConcurrency:               1024,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
	})

	// Each group's data source returns errors alongside data. PostProcessing
	// must select the "data" and "errors" paths — without SelectResponseErrorsPath
	// the loader never extracts the errors array (loader.go gates extraction on
	// res.postProcessing.SelectResponseErrorsPath != nil), and the errors would be
	// silently dropped regardless of the fix.
	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group A"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group B"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 3, "expected 1 initial + 2 incremental frames")

	// Each error must appear exactly once across all incremental payloads.
	// Before the fix both errors are discarded (errors=nil wipes them); this
	// assertion fails with count 0 for both.
	all := strings.Join(writer.payloads[1:], " ")
	require.Equal(t, 1, strings.Count(all, "error from group A"),
		"error from group A must appear in exactly one incremental frame")
	require.Equal(t, 1, strings.Count(all, "error from group B"),
		"error from group B must appear in exactly one incremental frame")

	// Isolation: no single incremental frame must contain both errors.
	for _, p := range writer.payloads[1:] {
		if strings.Contains(p, "error from group A") {
			require.NotContains(t, p, "error from group B",
				"group A frame must not contain group B error")
		}
	}
}

// TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext:
// defer-group subgraph errors must reach Context.SubgraphErrors(). Before the
// Context clone is removed they are recorded on the discarded per-group clone
// and SubgraphErrors() returns nil.
func TestResolveDeferTree_ParallelSiblings_SubgraphErrorsAggregateIntoContext(t *testing.T) {
	t.Parallel()

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newResolver(rCtx)

	groupA := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group A"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Info: &FetchInfo{DataSourceID: "subgraph-A", DataSourceName: "subgraph-A"},
		}),
	}
	groupB := &DeferFetchGroup{
		DeferID: 2,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{},"errors":[{"message":"error from group B"}]}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Info: &FetchInfo{DataSourceID: "subgraph-B", DataSourceName: "subgraph-B"},
		}),
	}

	response := minimalDeferResponse([]*DeferFetchGroup{groupA, groupB}, map[int]DeferDescriptor{
		1: {ID: 1, ParentID: 0},
		2: {ID: 2, ParentID: 0},
	})
	response.DeferTree = DeferParallel(DeferSingle(groupA), DeferSingle(groupB))

	writer := &testDeferWriter{}
	ctx := NewContext(context.Background())

	_, err := r.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)

	joined := ctx.SubgraphErrors()
	require.Error(t, joined, "defer-group subgraph errors must aggregate into the Context")
	assert.Contains(t, joined.Error(), "subgraph-A")
	assert.Contains(t, joined.Error(), "subgraph-B")
}
