package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func printErrorsForTest(ctx *Context, errors *astjson.Value, sources responseErrorSources) string {
	var output bytes.Buffer
	r := &Resolvable{ctx: ctx, errors: errors, errorSources: sources, out: &output}
	r.printErrorsArray()
	return output.String()
}

func TestResponseErrorHandlerSubgraphSources(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mode SubgraphErrorPropagationMode
	}{
		{name: "wrapped", mode: SubgraphErrorPropagationModeWrapped},
		{name: "pass-through", mode: SubgraphErrorPropagationModePassThrough},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			var downstreamTypes []string
			ctx.ResponseErrorHandler = func(_ context.Context, raw []byte, source *SubgraphError) ([]byte, error) {
				assert.Contains(t, string(raw), "secret")
				require.NotNil(t, source)
				assert.Equal(t, "products", source.DataSourceInfo.Name)
				assert.Equal(t, "query", source.Path)
				assert.Equal(t, 503, source.ResponseCode)
				for _, downstream := range source.DownstreamErrors {
					assert.Nil(t, downstream.Extensions.Get("code"), "the handler must see the original errors, before code defaulting")
					downstreamTypes = append(downstreamTypes, string(downstream.Extensions.GetStringBytes("errorType")))
				}
				return []byte(`{"message":"redacted"}`), nil
			}
			loader := &Loader{
				ctx: ctx, subgraphErrorPropagationMode: tc.mode, propagateSubgraphErrors: true,
				allowedSubgraphErrorFields: map[string]struct{}{"message": {}, "extensions": {}, "path": {}},
				defaultErrorExtensionCode:  "DEFAULT", allowedErrorExtensionFields: map[string]struct{}{"code": {}},
			}
			res := &result{ds: DataSourceInfo{Name: "products"}, statusCode: 503}
			require.NoError(t, loader.mergeErrors(res, &FetchItem{ResponsePath: "query"}, astjson.MustParseBytes([]byte(`[
				{"message":"secret1","extensions":{"errorType":"FIRST"}},
				{"message":"secret2","extensions":{"errorType":"SECOND"}}
			]`))))
			before := loader.errors.String()

			output := printErrorsForTest(ctx, loader.errors, loader.errorSources)

			if tc.mode == SubgraphErrorPropagationModeWrapped {
				// One wrapping error, whose source holds every downstream error.
				assert.JSONEq(t, `[{"message":"redacted"}]`, output)
			} else {
				// One source per error, each holding only its own downstream error.
				assert.JSONEq(t, `[{"message":"redacted"},{"message":"redacted"}]`, output)
			}
			assert.Equal(t, []string{"FIRST", "SECOND"}, downstreamTypes)
			assert.Equal(t, before, loader.errors.String(), "rewrites must not modify engine errors")

			recorded, ok := errors.AsType[*SubgraphError](res.subgraphError)
			require.True(t, ok)
			require.NotEmpty(t, recorded.DownstreamErrors)
			assert.Equal(t, "DEFAULT", string(recorded.DownstreamErrors[0].Extensions.GetStringBytes("code")), "telemetry keeps the processed errors")
		})
	}
}

func TestResponseErrorHandlerFetchFailureSources(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result result
		reason string
	}{
		{name: "transport failure", result: result{err: errors.New("dial failed")}},
		{name: "HTTP status fallback", result: result{statusCode: http.StatusServiceUnavailable, out: []byte("unavailable")}, reason: "503: Service Unavailable"},
		{name: "rate limit", result: result{rateLimitRejected: true, rateLimitRejectedReason: "too many requests"}},
		{name: "authorization", result: result{authorizationRejected: true, authorizationRejectedReasons: []string{"forbidden"}}, reason: "forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			var subgraphs []string
			ctx.ResponseErrorHandler = func(_ context.Context, _ []byte, source *SubgraphError) ([]byte, error) {
				require.NotNil(t, source)
				subgraphs = append(subgraphs, source.DataSourceInfo.Name)
				if source.DataSourceInfo.Name == "products" {
					assert.Equal(t, "query.products", source.Path)
					assert.Equal(t, tc.result.statusCode, source.ResponseCode)
					assert.Equal(t, tc.reason, source.Reason)
					assert.Empty(t, source.DownstreamErrors)
				}
				return []byte(`{"message":"redacted"}`), nil
			}
			loader := &Loader{ctx: ctx}
			// An earlier fetch's errors keep their own source.
			require.NoError(t, loader.mergeResult(&FetchItem{ResponsePath: "query.accounts"}, &result{ds: DataSourceInfo{Name: "accounts"}, err: errors.New("dial failed")}, nil))
			res := tc.result
			res.ds = DataSourceInfo{Name: "products"}
			require.NoError(t, loader.mergeResult(&FetchItem{ResponsePath: "query.products"}, &res, nil))

			assert.JSONEq(t, `[{"message":"redacted"},{"message":"redacted"}]`, printErrorsForTest(ctx, loader.errors, loader.errorSources))
			assert.Equal(t, []string{"accounts", "products"}, subgraphs)
		})
	}
}

func TestResponseErrorHandlerLocalErrorsHaveNoSource(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, raw []byte, source *SubgraphError) ([]byte, error) {
		assert.Nil(t, source)
		assert.JSONEq(t, `{"message":"Cannot return null","path":["field"]}`, string(raw))
		return []byte(`{"message":"redacted"}`), nil
	}
	clone := ctx.clone(t.Context())
	var output bytes.Buffer
	r := &Resolvable{ctx: clone, out: &output}
	r.addError("Cannot return null", []string{"field"})
	r.printErrorsArray()
	assert.JSONEq(t, `[{"message":"redacted"}]`, output.String())

	clone.Free()
	assert.Nil(t, clone.ResponseErrorHandler)
}

func TestResponseErrorHandlerResults(t *testing.T) {
	t.Parallel()

	const original = `{"message":"original","path":["field",0],"extensions":{"code":"ORIGINAL"},"custom":9007199254740993}`
	cases := []struct {
		name    string
		rewrite func(raw []byte) ([]byte, error)
		want    string
	}{
		{name: "nil keeps the error", rewrite: func([]byte) ([]byte, error) { return nil, nil }, want: original},
		{name: "replacement", rewrite: func([]byte) ([]byte, error) {
			return []byte(`{"message":"redacted","extensions":{"code":"CUSTOM"}}`), nil
		}, want: `{"message":"redacted","extensions":{"code":"CUSTOM"}}`},
		{name: "handler error", rewrite: func([]byte) ([]byte, error) { return nil, errors.New("boom") }, want: `{"message":"Internal server error"}`},
		{name: "invalid JSON", rewrite: func([]byte) ([]byte, error) { return []byte(`{"message":`), nil }, want: `{"message":"Internal server error"}`},
		{name: "not an object", rewrite: func([]byte) ([]byte, error) { return []byte(`"redacted"`), nil }, want: `{"message":"Internal server error"}`},
		{name: "empty", rewrite: func([]byte) ([]byte, error) { return []byte{}, nil }, want: `{"message":"Internal server error"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			ctx.ResponseErrorHandler = func(_ context.Context, raw []byte, _ *SubgraphError) ([]byte, error) {
				return tc.rewrite(raw)
			}
			errorsValue := astjson.MustParse(`[` + original + `]`)

			output := printErrorsForTest(ctx, errorsValue, nil)

			assert.JSONEq(t, "["+tc.want+"]", output)
			assert.Equal(t, "["+original+"]", errorsValue.String())
		})
	}
}

func TestResponseErrorHandlerSubscriptionSources(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	var sources []string
	ctx.ResponseErrorHandler = func(_ context.Context, raw []byte, source *SubgraphError) ([]byte, error) {
		if source == nil {
			sources = append(sources, "local")
			return nil, nil
		}
		sources = append(sources, source.DataSourceInfo.Name)
		if source.DataSourceInfo.Name == "events" {
			require.Len(t, source.DownstreamErrors, 1)
			assert.Contains(t, string(raw), source.DownstreamErrors[0].Message)
		}
		return fmt.Appendf(nil, `{"message":%q}`, source.DataSourceInfo.Name), nil
	}
	r := NewResolvable(nil, ResolvableOptions{})
	postProcessing := PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	require.NoError(t, r.InitSubscription(ctx, []byte(`{"data":{},"errors":[{"message":"event1"},{"message":"event2"}]}`), postProcessing))
	r.setSubscriptionErrorSources(GraphQLSubscriptionTrigger{SourceName: "events"})

	loader := &Loader{ctx: ctx}
	require.NoError(t, loader.mergeResult(&FetchItem{ResponsePath: "subscription.product"}, &result{ds: DataSourceInfo{Name: "products"}, err: errors.New("dial failed")}, nil))
	r.errors.AppendArrayItems(nil, loader.errors)
	r.errorSources = r.errorSources.merge(loader.errorSources)
	r.addError("Cannot return null", []string{"field"})

	var output bytes.Buffer
	r.out = &output
	r.printErrorsArray()
	assert.Equal(t, []string{"events", "events", "products", "local"}, sources)
	assert.JSONEq(t, `[{"message":"events"},{"message":"events"},{"message":"products"},{"message":"Cannot return null","path":["field"]}]`, output.String())

	// The next event must not keep errors or sources from the previous one.
	require.NoError(t, r.InitSubscription(ctx, []byte(`{"data":{}}`), postProcessing))
	r.setSubscriptionErrorSources(GraphQLSubscriptionTrigger{SourceName: "events"})
	assert.False(t, r.hasErrors())
	assert.Nil(t, r.errorSources)
}

func TestResponseErrorHandlerNotSet(t *testing.T) {
	// AllocsPerRun cannot run in parallel with other tests.

	ctx := NewContext(t.Context())
	loader := &Loader{
		ctx: ctx, propagateSubgraphErrors: true,
		allowedSubgraphErrorFields: map[string]struct{}{"message": {}, "extensions": {}},
	}
	res := &result{ds: DataSourceInfo{Name: "products"}, statusCode: 503}
	require.NoError(t, loader.mergeErrors(res, &FetchItem{ResponsePath: "query"}, astjson.MustParse(`[{"message":"secret"}]`)))
	require.NoError(t, loader.mergeResult(&FetchItem{ResponsePath: "query"}, &result{ds: res.ds, err: errors.New("dial failed")}, nil))
	assert.Nil(t, loader.errorSources, "sources are only recorded for a handler")

	r := &Resolvable{ctx: ctx, out: io.Discard, errors: loader.errors, marshalBuf: make([]byte, 0, 1024)}
	assert.Zero(t, testing.AllocsPerRun(100, r.printErrorsArray))
	var output bytes.Buffer
	r.out = &output
	r.printErrorsArray()
	assert.Equal(t, loader.errors.String(), output.String())
}

// TestResponseErrorHandlerDeduplication checks that requests with a handler are
// still deduplicated: the handler runs once and followers share its result.
func TestResponseErrorHandlerDeduplication(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	ds := NewMockDataSource(gomock.NewController(t))
	ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, http.Header, []byte) ([]byte, error) {
			close(entered)
			<-release
			return []byte(`{"errors":[{"message":"secret"}],"data":{"name":null}}`), nil
		}).Times(1)
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: SingleWithPath(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: ds,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Info: &FetchInfo{DataSourceID: "users", DataSourceName: "users"},
		}, "query"),
		Data: &Object{Fields: []*Field{{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}}}},
	}
	r := newResolver(t.Context())
	var calls atomic.Int32
	resolve := func() (string, *GraphQLResolveInfo) {
		ctx := NewContext(t.Context())
		ctx.Request.ID = 42
		ctx.ResponseErrorHandler = func(_ context.Context, _ []byte, source *SubgraphError) ([]byte, error) {
			calls.Add(1)
			return fmt.Appendf(nil, `{"message":"redacted error from %s"}`, source.DataSourceInfo.Name), nil
		}
		var out bytes.Buffer
		info, err := r.ArenaResolveGraphQLResponse(ctx, response, &out)
		require.NoError(t, err)
		return out.String(), info
	}

	leaderOut := make(chan string, 1)
	go func() {
		out, _ := resolve()
		leaderOut <- out
	}()
	<-entered

	const followers = 2
	followerOut := make([]string, followers)
	var wg sync.WaitGroup
	for i := range followers {
		wg.Go(func() {
			out, info := resolve()
			assert.True(t, info.ResolveDeduplicated)
			followerOut[i] = out
		})
	}
	require.Eventually(t, func() bool {
		var joined int32
		for i := range r.inboundRequestSingleFlight.shards {
			r.inboundRequestSingleFlight.shards[i].m.Range(func(_, v any) bool {
				joined += v.(*InflightRequest).followerCount.Load()
				return true
			})
		}
		return joined == followers
	}, 3*time.Second, 5*time.Millisecond)
	close(release)
	wg.Wait()

	const expected = `{"errors":[{"message":"redacted error from users"}],"data":{"name":null}}`
	assert.Equal(t, expected, <-leaderOut)
	assert.Equal(t, []string{expected, expected}, followerOut)
	assert.Equal(t, int32(1), calls.Load(), "followers share the leader's rewritten errors")
}
