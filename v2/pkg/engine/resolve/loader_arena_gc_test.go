package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// _errorReturningDataSource implements DataSource and returns a configurable error from Load.
type _errorReturningDataSource struct {
	err error
}

func (d *_errorReturningDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	return nil, d.err
}

func (d *_errorReturningDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return nil, d.err
}

// gcTestResponse builds a minimal GraphQLResponse with a single fetch using the given DataSource.
// Callers can override FetchConfiguration and Info fields on the returned SingleFetch.
func gcTestResponse(ds DataSource) (*GraphQLResponse, *SingleFetch) {
	fetch := &SingleFetch{
		FetchConfiguration: FetchConfiguration{
			DataSource: ds,
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data"},
				SelectResponseErrorsPath: []string{"errors"},
			},
		},
		Info: &FetchInfo{
			DataSourceID:   "test-ds",
			DataSourceName: "testService",
			RootFields: []GraphCoordinate{
				{
					TypeName:  "Query",
					FieldName: "field",
				},
			},
		},
	}
	return &GraphQLResponse{
		Fetches: SingleWithPath(fetch, "query"),
		Data: &Object{
			Nullable: true,
			Fields: []*Field{
				{
					Name:  []byte("field"),
					Value: &String{Path: []string{"field"}, Nullable: true},
				},
			},
		},
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
	}, fetch
}

// Benchmark_ArenaGCSafety exercises all error codepaths that produce arena-allocated
// JSON values via parseStringOnArena / stringValueOnArena. Each sub-benchmark resolves
// a GraphQLResponse through ArenaResolveGraphQLResponse with runtime.GC() calls between
// iterations to maximize GC pressure on any dangling pointers.
//
// If the GC safety fix (copying string bytes onto the arena before parsing) were reverted,
// these benchmarks would SIGSEGV.
//
// Codepaths NOT directly covered (require HTTP status codes injected by loadByContext):
//   - renderErrorsStatusFallback (status code fallback)
//   - addApolloRouterCompatibilityError (Apollo compat error)
//   - setSubgraphStatusCode (subgraph status code propagation)
//
// These use the same parseStringOnArena helper and are covered transitively.
func Benchmark_ArenaGCSafety(b *testing.B) {
	type testCase struct {
		name         string
		resolverOpts func() ResolverOptions
		setupCtx     func() *Context
		setupResp    func() *GraphQLResponse
	}

	baseResolverOpts := func() ResolverOptions {
		return ResolverOptions{
			MaxConcurrency:               1024,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
		}
	}

	cases := []testCase{
		{
			// Codepath 1: DataSource.Load() returns error → renderErrorsFailedToFetch
			name:         "fetchError",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(&_errorReturningDataSource{err: errors.New("connection refused")})
				return resp
			},
		},
		{
			// Codepath 4: DataSource.Load() returns empty response → renderErrorsFailedToFetch(emptyGraphQLResponse)
			name:         "emptyResponse",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(""))
				return resp
			},
		},
		{
			// Codepath 5: DataSource.Load() returns invalid JSON → renderErrorsFailedToFetch(invalidGraphQLResponse)
			name:         "invalidJSON",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource("{invalid"))
				return resp
			},
		},
		{
			// Codepath 6: Response has no data/errors key → renderErrorsFailedToFetch(invalidGraphQLResponseShape)
			name:         "invalidShape",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"something":"else"}`))
				return resp
			},
		},
		{
			// Codepath 3: Subgraph returns errors with wrap mode (default) → mergeErrors wrap path → parseStringOnArena
			name:         "subgraphErrorsWrapMode",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
				return resp
			},
		},
		{
			// Codepath 2: Subgraph returns errors with passthrough mode → mergeErrors passthrough path
			name: "subgraphErrorsPassthroughMode",
			resolverOpts: func() ResolverOptions {
				opts := baseResolverOpts()
				opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
				return opts
			},
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
				return resp
			},
		},
		{
			// Codepath 12: defaultErrorExtensionCode set + subgraph errors → stringValueOnArena
			name: "subgraphErrorsWithExtensionCode",
			resolverOpts: func() ResolverOptions {
				opts := baseResolverOpts()
				opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
				opts.DefaultErrorExtensionCode = "DOWNSTREAM_SERVICE_ERROR"
				return opts
			},
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
				return resp
			},
		},
		{
			// Codepath 13: attachServiceNameToErrorExtension set → stringValueOnArena
			name: "subgraphErrorsWithServiceName",
			resolverOpts: func() ResolverOptions {
				opts := baseResolverOpts()
				opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
				opts.AttachServiceNameToErrorExtensions = true
				return opts
			},
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
				return resp
			},
		},
		{
			// Codepath 12+13 combined: both extension code and service name
			name: "subgraphErrorsWithExtensionCodeAndServiceName",
			resolverOpts: func() ResolverOptions {
				opts := baseResolverOpts()
				opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
				opts.DefaultErrorExtensionCode = "DOWNSTREAM_SERVICE_ERROR"
				opts.AttachServiceNameToErrorExtensions = true
				return opts
			},
			setupCtx: func() *Context {
				return NewContext(context.Background())
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
				return resp
			},
		},
		{
			// Codepath 9: Authorization rejected → renderAuthorizationRejectedErrors → parseStringOnArena
			name:         "authorizationRejected",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				ctx := NewContext(context.Background())
				ctx.SetAuthorizer(createTestAuthorizer(
					func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
						return &AuthorizationDeny{Reason: "not allowed"}, nil
					},
					func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
						return nil, nil
					},
				))
				return ctx
			},
			setupResp: func() *GraphQLResponse {
				resp, fetch := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
				fetch.Info.RootFields[0].HasAuthorizationRule = true
				return resp
			},
		},
		{
			// Codepath 10: Rate limit rejected → renderRateLimitRejectedErrors → parseStringOnArena
			name:         "rateLimitRejected",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				ctx := NewContext(context.Background())
				ctx.SetRateLimiter(&testRateLimiter{
					allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
						return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
					},
				})
				ctx.RateLimitOptions = RateLimitOptions{Enable: true}
				return ctx
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
				return resp
			},
		},
		{
			// Codepath 14: Rate limit with extension code → extra parseStringOnArena for extension
			name:         "rateLimitWithExtensionCode",
			resolverOpts: baseResolverOpts,
			setupCtx: func() *Context {
				ctx := NewContext(context.Background())
				ctx.SetRateLimiter(&testRateLimiter{
					allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
						return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
					},
				})
				ctx.RateLimitOptions = RateLimitOptions{
					Enable:             true,
					ErrorExtensionCode: RateLimitErrorExtensionCode{Enabled: true, Code: "RATE_LIMIT_EXCEEDED"},
				}
				return ctx
			},
			setupResp: func() *GraphQLResponse {
				resp, _ := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
				return resp
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			rCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := New(rCtx, tc.resolverOpts())
			buf := &bytes.Buffer{}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				response := tc.setupResp()
				resolveCtx := tc.setupCtx()

				// Force GC between iterations to maximize pressure on any
				// dangling pointers in arena-allocated Values. If the GC safety
				// fix were reverted, this would cause SIGSEGV.
				runtime.GC()

				buf.Reset()
				_, err := resolver.ArenaResolveGraphQLResponse(resolveCtx, response, buf)
				if err != nil {
					b.Fatal(err)
				}
				if buf.Len() == 0 {
					b.Fatal("empty output")
				}
			}
		})
	}
}
