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

// Known issue: writes happen as each fetch merges, before resolution checks
// the data, so a response it rejects is still cached.
func TestResponseCacheStoresUnresolvedResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "a resolved response", body: `{"data":{"me":"ok"}}`},
		{name: "a value of the wrong type", body: `{"data":{"me":5}}`},
		{name: "a null for a non-null field", body: `{"data":{"me":null}}`},
		{name: "an object for a string", body: `{"data":{"me":{"a":1}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, arena := range []bool{false, true} {
				store := newTestCache()
				ctx := NewContext(context.Background())
				ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
				resolver := newTestResolver(t, baseResolverOpts())
				var err error
				if arena {
					_, err = resolver.ArenaResolveGraphQLResponse(ctx, newResolutionResponse(resolutionDataSource{body: tc.body}, nil), &bytes.Buffer{})
				} else {
					_, err = resolver.ResolveGraphQLResponse(ctx, newResolutionResponse(resolutionDataSource{body: tc.body}, nil), nil, &bytes.Buffer{})
				}
				require.NoError(t, err)
				require.Len(t, store.items, 1, "arena=%v", arena)
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

// Writes happen as the fetch merges, so a slow client doesn't age the entry.
func TestResponseCacheWriteKeepsTheOriginLifetime(t *testing.T) {
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
			require.Len(t, store.items, 1, "written before the slow client write")
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
