package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

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

// _statusCodeDataSource implements DataSource and injects an HTTP status code into the response context.
type _statusCodeDataSource struct {
	data       string
	statusCode int
}

func (d *_statusCodeDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	if rc := httpclient.GetResponseContext(ctx); rc != nil {
		rc.StatusCode = d.statusCode
	}
	return []byte(d.data), nil
}

func (d *_statusCodeDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

// _testCustomResolve implements CustomResolve by returning the input unchanged.
type _testCustomResolve struct{}

func (r *_testCustomResolve) Resolve(ctx *Context, value []byte) ([]byte, error) {
	return value, nil
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

// Benchmark_ArenaGCSafety exercises error codepaths that produce arena-allocated
// JSON values. Each sub-benchmark resolves a GraphQLResponse through
// ArenaResolveGraphQLResponse with runtime.GC() calls between iterations to
// maximize GC pressure on any dangling pointers.
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
			// Codepath 3: Subgraph returns errors with wrap mode (default) → mergeErrors wrap path → astjson.ParseWithArena
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
			// Codepath 12: defaultErrorExtensionCode set + subgraph errors → astjson.StringValue
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
			// Codepath 13: attachServiceNameToErrorExtension set → astjson.StringValue
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
			// Codepath 9: Authorization rejected → renderAuthorizationRejectedErrors → astjson.ParseWithArena
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
			// Codepath 10: Rate limit rejected → renderRateLimitRejectedErrors → astjson.ParseWithArena
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
			// Codepath 14: Rate limit with extension code → extra astjson.ParseWithArena for extension
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
		{
			// Codepath: L1 cache population — entity fetch with UseL1Cache stores
			// arena-allocated *astjson.Value pointers in Loader.l1Cache (sync.Map).
			// After ArenaResolveGraphQLResponse releases the arena, those pointers
			// become dangling. runtime.GC() should detect them.
			name: "l1CacheDanglingPointers",
			resolverOpts: func() ResolverOptions {
				return ResolverOptions{
					MaxConcurrency: 1024,
				}
			},
			setupCtx: func() *Context {
				ctx := NewContext(context.Background())
				ctx.ExecutionOptions.Caching.EnableL1Cache = true
				ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
				return ctx
			},
			setupResp: func() *GraphQLResponse {
				productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
					Keys: NewResolvableObjectVariable(&Object{
						Fields: []*Field{
							{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						},
					}),
				}
				providesData := &Object{
					Fields: []*Field{
						{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
						{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
					},
				}
				return &GraphQLResponse{
					Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
					Fetches: Sequence(
						// Root fetch
						SingleWithPath(&SingleFetch{
							FetchConfiguration: FetchConfiguration{
								DataSource: FakeDataSource(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`),
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`), SegmentType: StaticSegmentType},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "query"),
						// Entity fetch — populates L1 cache with arena-allocated pointers
						SingleWithPath(&SingleFetch{
							FetchConfiguration: FetchConfiguration{
								DataSource: FakeDataSource(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`),
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities", "0"},
								},
								Caching: FetchCacheConfiguration{
									Enabled:          true,
									CacheName:        "default",
									TTL:              30 * time.Second,
									CacheKeyTemplate: productCacheKeyTemplate,
									UseL1Cache:       true,
								},
							},
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
									{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
										Fields: []*Field{
											{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
											{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
										},
									})},
									{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
								},
							},
							Info: &FetchInfo{
								DataSourceID:   "products",
								DataSourceName: "products",
								OperationType:  ast.OperationTypeQuery,
								ProvidesData:   providesData,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "query.product", ObjectPath("product")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("product"),
								Value: &Object{
									Path: []string{"product"},
									Fields: []*Field{
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
										{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
									},
								},
							},
						},
					},
				}
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

// TestL1CacheStalePointersAfterArenaReset deterministically proves that L1 cache
// entries become stale when the arena is reset and reused. This is the root cause
// of the CI crash "found pointer to free object": the Loader's l1Cache (sync.Map)
// holds *astjson.Value pointers into arena memory that becomes invalid after
// resolveArenaPool.Release() resets the arena.
func TestL1CacheStalePointersAfterArenaReset(t *testing.T) {
	// Shared entity fetch setup — same as l1_cache_test.go
	productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
	providesData := &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}}},
		},
	}

	// buildResponse creates a GraphQLResponse with a root fetch + entity fetch that populates L1 cache.
	buildResponse := func(rootDS, entityDS DataSource) *GraphQLResponse {
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							})},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					},
				},
			},
		}
	}

	t.Run("stale pointers after arena reset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil).Times(1)

		response := buildResponse(rootDS, entityDS)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		loader := &Loader{jsonArena: ar}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true

		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Verify L1 cache was populated with correct data
		var cacheCount int
		var originalBytes []byte
		loader.l1Cache.Range(func(key, value any) bool {
			cacheCount++
			originalBytes = value.(*astjson.Value).MarshalTo(nil)
			return true
		})
		require.Equal(t, 1, cacheCount, "entity fetch should populate exactly 1 L1 cache entry")
		assert.Contains(t, string(originalBytes), `Product One`)

		// Simulate arena reuse after resolveArenaPool.Release():
		// Reset zeroes the offset (same as Pool.Release → Arena.Reset)
		ar.Reset()
		// A subsequent request reuses the arena, overwriting old allocations
		_, _ = astjson.ParseBytesWithArena(ar, []byte(`{"__typename":"Product","id":"STALE","name":"CORRUPTED DATA"}`))

		// The l1Cache still holds pointers into the arena buffer.
		// Those pointers now reference the overwritten memory → stale data.
		var staleBytes []byte
		loader.l1Cache.Range(func(key, value any) bool {
			staleBytes = value.(*astjson.Value).MarshalTo(nil)
			return true
		})
		assert.NotEqual(t, string(originalBytes), string(staleBytes),
			"L1 cache entries should be stale after arena reset+reuse — "+
				"this proves the bug: l1Cache holds dangling pointers into reused arena memory")
	})

	t.Run("Free prevents stale pointer access", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil).Times(1)

		response := buildResponse(rootDS, entityDS)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		loader := &Loader{jsonArena: ar}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true

		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Verify L1 cache was populated
		var cacheCount int
		loader.l1Cache.Range(func(key, value any) bool {
			cacheCount++
			return true
		})
		require.Equal(t, 1, cacheCount, "entity fetch should populate exactly 1 L1 cache entry")

		// The fix: Free() nils l1Cache before arena release
		loader.Free()
		assert.Nil(t, loader.l1Cache,
			"Free() must nil l1Cache to sever all references to arena-allocated values — "+
				"this prevents the GC crash when the arena is released and reused")
	})
}
