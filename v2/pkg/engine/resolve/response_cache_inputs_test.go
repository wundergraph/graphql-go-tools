package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// An entity fetch whose input carries a client variable: an omitted variable
// and an explicit null reach the subgraph differently, so they must not share
// a cache entry.
func TestResponseCacheUndefinedVariableKey(t *testing.T) {
	entityDS := &recordingDataSource{
		response:        []byte(`{"data":{"_entities":[{"notes":"n"}]}}`),
		responseHeaders: cacheableHeaders(),
	}

	footer := InputTemplate{Segments: []TemplateSegment{
		{SegmentType: StaticSegmentType, Data: []byte(`],"first":`)},
		(&ContextVariable{Path: []string{"first"}, Renderer: NewJSONVariableRenderer()}).TemplateSegment(),
		{SegmentType: StaticSegmentType, Data: []byte(`}}}`)},
	}}
	tree := func() *GraphQLResponse {
		entity := &EntityFetch{
			FetchDependencies: FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}},
			Input: EntityInput{
				Header: multiStaticTemplate(`{"method":"POST","url":"http://products","body":{"query":"notes","variables":{"representations":[`),
				Item:   multiRepresentationsTemplate(),
				Footer: footer,
			},
			DataSource:     entityDS,
			PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
			Info:           &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "products-id"},
		}
		return &GraphQLResponse{Fetches: Sequence(
			Single(multiEntityRootFetch(&recordingDataSource{response: []byte(multiEntityRootResponse)})),
			SingleWithPath(entity, "employee", ObjectPath("employee")),
		)}
	}

	load := func(cache *testCache, variables string) string {
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.Variables = astjson.MustParse(variables)
		ctx.SetResponseCache(ResponseCacheOptions{Store: cache, DefaultTTL: time.Minute})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, tree()))
		return string(entityDS.lastInput)
	}

	// Cold caches: what each request sends.
	omitted := load(newTestCache(), `{}`)
	explicitNull := load(newTestCache(), `{"first":null}`)
	require.NotEqual(t, omitted, explicitNull, "the two requests differ on the wire")

	// One cache: the second must not be served the first's entry.
	entityDS.calls = 0
	shared := newTestCache()
	load(shared, `{}`)
	load(shared, `{"first":null}`)
	require.Equal(t, 2, entityDS.calls, "an explicit null was served the omitted variable's entry")
}

// A root query fetch whose subgraph request differs only in what is sent
// beside the rendered input: uploaded files, or forwarded client extensions.
func TestResponseCacheInputsBesideTheKey(t *testing.T) {
	run := func(t *testing.T, vary func(ctx *Context, second bool)) int {
		t.Helper()
		ds := &recordingDataSource{
			response:        []byte(`{"data":{"employee":{"id":1}}}`),
			responseHeaders: cacheableHeaders(),
		}
		fetch := &SingleFetch{
			InputTemplate: multiStaticTemplate(`{"method":"POST","url":"http://accounts","body":{"query":"{employee{id}}"}}`),
			FetchConfiguration: FetchConfiguration{
				DataSource:     ds,
				PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
			},
			DataSourceIdentifier: graphqlDataSourceIdentifier,
			Info: &FetchInfo{
				OperationType:  ast.OperationTypeQuery,
				DataSourceID:   "accounts-id",
				DataSourceName: "accounts",
				RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "employee"}},
			},
		}
		cache := newTestCache()
		for i := range 2 {
			ctx := NewContext(t.Context())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			ctx.SetResponseCache(ResponseCacheOptions{Store: cache, DefaultTTL: time.Minute})
			vary(ctx, i == 1)
			loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
			require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: Single(fetch)}))
		}
		return ds.calls
	}

	t.Run("different uploaded files", func(t *testing.T) {
		calls := run(t, func(ctx *Context, second bool) {
			name := "a.txt"
			if second {
				name = "b.txt"
			}
			ctx.Files = []*httpclient.FileUpload{httpclient.NewFileUpload("/tmp/"+name, name, "variables.file")}
		})
		require.Equal(t, 2, calls, "a different file was served the first file's answer")
	})

	t.Run("different forwarded extensions", func(t *testing.T) {
		calls := run(t, func(ctx *Context, second bool) {
			ctx.Extensions = []byte(`{"tenant":"a"}`)
			if second {
				ctx.Extensions = []byte(`{"tenant":"b"}`)
			}
		})
		require.Equal(t, 2, calls, "different extensions were served the first request's answer")
	})
}
