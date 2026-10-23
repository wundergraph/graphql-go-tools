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

// resolutionDataSource answers every fetch with a cacheable body.
type resolutionDataSource struct{ body string }

func (d resolutionDataSource) Load(ctx context.Context, _ http.Header, _ []byte) ([]byte, error) {
	if rc := httpclient.GetResponseContext(ctx); rc != nil {
		rc.StatusCode = http.StatusOK
		rc.Response = &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{"public, max-age=60"}}}
	}
	return []byte(d.body), nil
}

func (d resolutionDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func TestResponseCacheStoresOnlyResolvedResponses(t *testing.T) {
	newResponse := func(body string) *GraphQLResponse {
		input := `{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: resolutionDataSource{body: body},
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
			Data: &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
	}

	for _, tc := range []struct {
		name   string
		body   string
		stored bool
	}{
		{name: "a resolved response is stored", body: `{"data":{"me":"ok"}}`, stored: true},
		{name: "a value of the wrong type is not stored", body: `{"data":{"me":5}}`},
		{name: "a null for a non-null field is stored", body: `{"data":{"me":null}}`, stored: true},
		{name: "an object for a string is not stored", body: `{"data":{"me":{"a":1}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, arena := range []bool{false, true} {
				store := newTestCache()
				ctx := NewContext(context.Background())
				ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
				resolver := newTestResolver(t, baseResolverOpts())
				var err error
				if arena {
					_, err = resolver.ArenaResolveGraphQLResponse(ctx, newResponse(tc.body), &bytes.Buffer{})
				} else {
					_, err = resolver.ResolveGraphQLResponse(ctx, newResponse(tc.body), nil, &bytes.Buffer{})
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
