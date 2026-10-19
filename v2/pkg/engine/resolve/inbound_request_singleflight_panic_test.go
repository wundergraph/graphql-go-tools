package resolve

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// panicHooks panics in OnFinished once release is closed.
type panicHooks struct {
	entered chan struct{}
	release chan struct{}
}

func (h *panicHooks) OnLoad(ctx context.Context, _ DataSourceInfo) context.Context {
	close(h.entered)
	return ctx
}

func (h *panicHooks) OnFinished(context.Context, DataSourceInfo, *ResponseInfo) {
	<-h.release
	panic("hook failed")
}

func TestInboundSingleFlight_LeaderPanicReleasesFlight(t *testing.T) {
	ctrl := gomock.NewController(t)
	ds := NewMockDataSource(ctrl)
	ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte(`{"data":{"name":"Jens"}}`), nil).AnyTimes()

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: SingleWithPath(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource:     ds,
				PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
			},
			Info: &FetchInfo{DataSourceID: "users", DataSourceName: "users"},
		}, "query"),
		Data: &Object{Fields: []*Field{{Name: []byte("name"), Value: &String{Path: []string{"name"}}}}},
	}

	r := newResolver(t.Context())
	newCtx := func(hooks LoaderHooks) *Context {
		ctx := NewContext(t.Context())
		ctx.Request.ID = 42
		ctx.LoaderHooks = hooks
		return ctx
	}

	hooks := &panicHooks{entered: make(chan struct{}), release: make(chan struct{})}
	leaderPanic := make(chan any, 1)
	go func() {
		defer func() { leaderPanic <- recover() }()
		_, _ = r.ArenaResolveGraphQLResponse(newCtx(hooks), response, &bytes.Buffer{})
	}()
	<-hooks.entered

	followerErr := make(chan error, 1)
	go func() {
		_, err := r.ArenaResolveGraphQLResponse(newCtx(nil), response, &bytes.Buffer{})
		followerErr <- err
	}()
	require.Eventually(t, func() bool {
		joined := false
		for i := range r.inboundRequestSingleFlight.shards {
			r.inboundRequestSingleFlight.shards[i].m.Range(func(_, v any) bool {
				joined = joined || v.(*InflightRequest).HasFollowers()
				return true
			})
		}
		return joined
	}, 3*time.Second, 5*time.Millisecond)

	close(hooks.release)
	require.Equal(t, "hook failed", <-leaderPanic)

	select {
	case err := <-followerErr:
		require.ErrorContains(t, err, "deduplicated request failed: hook failed")
	case <-time.After(3 * time.Second):
		t.Fatal("follower still waiting on the panicked leader")
	}

	// The flight is gone: the same request leads again and resolves.
	done := make(chan string, 1)
	go func() {
		buf := &bytes.Buffer{}
		_, err := r.ArenaResolveGraphQLResponse(newCtx(nil), response, buf)
		require.NoError(t, err)
		done <- buf.String()
	}()
	select {
	case out := <-done:
		require.Equal(t, `{"data":{"name":"Jens"}}`, out)
	case <-time.After(3 * time.Second):
		t.Fatal("request after the panic is wedged on the stale flight")
	}
}
