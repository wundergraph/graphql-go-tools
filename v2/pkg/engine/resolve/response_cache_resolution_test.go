package resolve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// resolutionDataSource answers every fetch with a cacheable body, for a
// minute unless cacheControl says otherwise.
type resolutionDataSource struct {
	body         string
	cacheControl string
}

func (d resolutionDataSource) Load(ctx context.Context, _ http.Header, _ []byte) ([]byte, error) {
	cacheControl := d.cacheControl
	if cacheControl == "" {
		cacheControl = "public, max-age=60"
	}
	if rc := httpclient.GetResponseContext(ctx); rc != nil {
		rc.StatusCode = http.StatusOK
		rc.Response = &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{cacheControl}}}
	}
	return []byte(d.body), nil
}

func (d resolutionDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func newResolutionResponse(ds resolutionDataSource, data *Object) *GraphQLResponse {
	if data == nil {
		data = &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}}
	}
	input := `{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: ds,
				Input:      input,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(input), SegmentType: StaticSegmentType}}},
			DataSourceIdentifier: graphqlDataSourceIdentifier,
			Info:                 &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "accounts", DataSourceName: "accounts"},
		}),
		Data: data,
	}
}

func TestResponseCacheStoresOnlyResolvedResponses(t *testing.T) {
	// must is non-null and walked first, so its null stops the walk before bad.
	mustThenBad := &Object{Fields: []*Field{
		{Name: []byte("must"), Value: &String{Path: []string{"must"}}},
		{Name: []byte("bad"), Value: &String{Path: []string{"bad"}, Nullable: true}},
	}}

	for _, tc := range []struct {
		name   string
		body   string
		data   *Object
		stored bool
	}{
		{name: "a resolved response is stored", body: `{"data":{"me":"ok"}}`, stored: true},
		{name: "a value of the wrong type is not stored", body: `{"data":{"me":5}}`},
		{name: "a null for a non-null field is stored", body: `{"data":{"me":null}}`, stored: true},
		{name: "an object for a string is not stored", body: `{"data":{"me":{"a":1}}}`},
		{name: "a null that stops the walk before a value of the wrong type is not stored", body: `{"data":{"must":null,"bad":42}}`, data: mustThenBad},
		{name: "a null that stops the walk before valid values is not stored", body: `{"data":{"must":null,"bad":"ok"}}`, data: mustThenBad},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, arena := range []bool{false, true} {
				store := newTestCache()
				ctx := NewContext(context.Background())
				ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
				resolver := newTestResolver(t, baseResolverOpts())
				var err error
				if arena {
					_, err = resolver.ArenaResolveGraphQLResponse(ctx, newResolutionResponse(resolutionDataSource{body: tc.body}, tc.data), &bytes.Buffer{})
				} else {
					_, err = resolver.ResolveGraphQLResponse(ctx, newResolutionResponse(resolutionDataSource{body: tc.body}, tc.data), nil, &bytes.Buffer{})
				}
				require.NoError(t, err)
				if tc.stored {
					require.Len(t, store.items, 1, "arena=%v", arena)
				} else {
					require.Empty(t, store.items, "arena=%v", arena)
				}
			}
		})
	}
}

// slowWriter takes delay over its first write, as a slow client would.
type slowWriter struct {
	bytes.Buffer

	delay time.Duration
}

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	w.delay = 0
	return w.Buffer.Write(p)
}

func TestResponseCacheCommitKeepsTheOriginLifetime(t *testing.T) {
	t.Parallel()
	for _, arena := range []bool{false, true} {
		t.Run(fmt.Sprintf("arena=%v", arena), func(t *testing.T) {
			t.Parallel()
			store := newTestCache()
			ctx := NewContext(context.Background())
			ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
			response := newResolutionResponse(resolutionDataSource{body: `{"data":{"me":"ok"}}`, cacheControl: "public, max-age=1"}, nil)
			out := &slowWriter{delay: 1100 * time.Millisecond}
			resolver := newTestResolver(t, baseResolverOpts())
			var err error
			if arena {
				_, err = resolver.ArenaResolveGraphQLResponse(ctx, response, out)
			} else {
				_, err = resolver.ResolveGraphQLResponse(ctx, response, nil, out)
			}
			require.NoError(t, err)
			require.Empty(t, store.items, "the write waited out the origin's max-age")
		})
	}
}

func TestRemainingLifetimes(t *testing.T) {
	items := []caching.Item{{Key: "a", TTL: time.Minute, ClientTTL: 40 * time.Second}, {Key: "b", TTL: time.Second}}
	require.Equal(t, items, remainingLifetimes(items, 0))
	require.Equal(t, []caching.Item{{Key: "a", TTL: 30 * time.Second, ClientTTL: 10 * time.Second}}, remainingLifetimes(items, 30*time.Second))
	require.Equal(t, []caching.Item{{Key: "a", TTL: 15 * time.Second}}, remainingLifetimes(items, 45*time.Second), "ClientTTL stops at zero")
	require.Equal(t, time.Minute, items[0].TTL, "the originals are kept")
	require.Empty(t, remainingLifetimes(items, time.Minute))
}

// A deferred field's null stops the walk before the pass through fields.
func TestResolveDeferBatchFailsOnUnwalkedPassThrough(t *testing.T) {
	data := &Object{Fields: []*Field{
		{Name: []byte("nested"), Value: &Object{Path: []string{"nested"}, Nullable: true, Fields: []*Field{
			{Name: []byte("bad"), Value: &String{Path: []string{"bad"}, Nullable: true}, Defer: &DeferField{DeferID: 1}},
		}}},
		{Name: []byte("must"), Value: &String{Path: []string{"must"}}, Defer: &DeferField{DeferID: 1}},
	}}
	res := NewResolvable(nil, ResolvableOptions{})
	require.NoError(t, res.Init(NewContext(context.Background()), []byte(`{"nested":{"bad":42},"must":null}`), ast.OperationTypeQuery))
	res.currentDefer = &DeferDescriptor{ID: 1}
	var outstanding int64 = 1
	_, _ = res.ResolveDeferBatch(data, &bytes.Buffer{}, &outstanding)
	require.True(t, res.walkFailed)
}
