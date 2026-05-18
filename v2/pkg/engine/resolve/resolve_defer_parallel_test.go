package resolve

import (
	"bytes"
	"context"
	"fmt"
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
