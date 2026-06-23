package resolve

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func newTestLoader() *Loader {
	return &Loader{ctx: NewContext(context.Background())}
}

func TestRecordSubgraphError_PerFetchIsolation(t *testing.T) {
	l := newTestLoader()
	resA := &result{ds: DataSourceInfo{Name: "Users"}}
	resB := &result{ds: DataSourceInfo{Name: "Users"}} // same subgraph, no error

	l.recordSubgraphError(resA, errors.New("boom"))

	// Per-fetch isolation: only resA carries the error.
	require.Error(t, resA.subgraphError)
	assert.NoError(t, resB.subgraphError)

	// newResponseInfo reads the per-result field.
	assert.Error(t, newResponseInfo(resA).Err)
	assert.NoError(t, newResponseInfo(resB).Err)
}

func TestRecordSubgraphError_Accumulates(t *testing.T) {
	l := newTestLoader()
	res := &result{ds: DataSourceInfo{Name: "Products"}}

	l.recordSubgraphError(res, errors.New("first"))
	l.recordSubgraphError(res, errors.New("second"))

	require.Error(t, res.subgraphError)
	msg := res.subgraphError.Error()
	assert.Contains(t, msg, "first")
	assert.Contains(t, msg, "second")
}

func TestFlushSubgraphErrors_AggregatesIntoContext(t *testing.T) {
	l := newTestLoader()
	resU := &result{ds: DataSourceInfo{Name: "Users"}}
	resP := &result{ds: DataSourceInfo{Name: "Products"}}

	l.recordSubgraphError(resU, errors.New("users-down"))
	l.recordSubgraphError(resP, errors.New("products-down"))

	// Before flush the Context is untouched.
	assert.NoError(t, l.ctx.SubgraphErrors())

	l.appendSubgraphErrorsToContext()

	joined := l.ctx.SubgraphErrors()
	require.Error(t, joined)
	assert.Contains(t, joined.Error(), "users-down")
	assert.Contains(t, joined.Error(), "products-down")
}

func TestRecordSubgraphError_NilIsNoOp(t *testing.T) {
	l := newTestLoader()
	res := &result{ds: DataSourceInfo{Name: "Users"}}

	l.recordSubgraphError(res, nil)

	assert.NoError(t, res.subgraphError)
	l.appendSubgraphErrorsToContext()
	assert.NoError(t, l.ctx.SubgraphErrors())
}

// orderedLoaderHooks records, in OnFinished call order, whether each fetch's
// ResponseInfo.Err was set.
type orderedLoaderHooks struct {
	pre   atomic.Int64
	mu    sync.Mutex
	calls []bool // info.Err != nil, per OnFinished, in order
}

func (h *orderedLoaderHooks) OnLoad(ctx context.Context, ds DataSourceInfo) context.Context {
	h.pre.Add(1)
	return ctx
}

func (h *orderedLoaderHooks) OnFinished(ctx context.Context, ds DataSourceInfo, info *ResponseInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, info.Err != nil)
}

func TestOnFinished_SameSubgraphName_NoErrorInheritance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rCtx := t.Context()
	r := New(rCtx, ResolverOptions{MaxConcurrency: 1024})

	// Fetch #1: returns subgraph errors.
	failing := NewMockDataSource(ctrl)
	failing.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
			return []byte(`{"errors":[{"message":"boom"}]}`), nil
		})

	// Fetch #2: clean response, no errors array.
	clean := NewMockDataSource(ctrl)
	clean.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
			return []byte(`{}`), nil
		})

	hooks := &orderedLoaderHooks{}
	resolveCtx := NewContext(context.Background())
	resolveCtx.LoaderHooks = hooks

	resp := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: failing,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{DataSourceID: "Users", DataSourceName: "Users"},
			}, "query"),
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: clean,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{DataSourceID: "Users", DataSourceName: "Users"},
			}, "query"),
		),
		Data: &Object{
			Nullable: false,
			Fields: []*Field{
				{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
			},
		},
	}

	buf := &bytes.Buffer{}
	_, err := r.ResolveGraphQLResponse(resolveCtx, resp, nil, buf)
	require.NoError(t, err)

	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	require.Equal(t, int64(2), hooks.pre.Load(), "both fetches should load")
	require.Len(t, hooks.calls, 2, "OnFinished should fire once per fetch")
	assert.True(t, hooks.calls[0], "failing fetch's OnFinished must report its own error")
	assert.False(t, hooks.calls[1], "clean fetch's OnFinished must NOT inherit the failing fetch's error")

	// The aggregate Context error still reflects the (single) failing fetch.
	assert.Error(t, resolveCtx.SubgraphErrors())
}
