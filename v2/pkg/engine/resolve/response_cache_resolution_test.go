package resolve

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
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
