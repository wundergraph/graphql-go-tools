package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// forceGC runs multiple GC cycles to maximize pressure on dangling pointers.
// Arena buffers are marked noscan, so any heap pointer stored inside an arena
// allocation is invisible to the GC. If such a pointer is the only reference
// keeping an object alive, the GC will collect it and subsequent access will
// SIGSEGV or return corrupted data.
func forceGC() {
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
}

const gcIterations = 100

func baseResolverOpts() ResolverOptions {
	return ResolverOptions{
		MaxConcurrency:               1024,
		PropagateSubgraphErrors:      true,
		PropagateSubgraphStatusCodes: true,
	}
}

func newTestResolver(t *testing.T, opts ResolverOptions) *Resolver {
	t.Helper()
	rCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(rCtx, opts)
}

// resolveWithGCPressure runs ArenaResolveGraphQLResponse in a loop with GC pressure
// between iterations. Returns the last output for assertion.
func resolveWithGCPressure(t *testing.T, resolver *Resolver, setupCtx func() *Context, setupResp func() *GraphQLResponse) string {
	t.Helper()
	var lastOutput string
	for i := 0; i < gcIterations; i++ {
		response := setupResp()
		resolveCtx := setupCtx()
		forceGC()
		buf := &bytes.Buffer{}
		_, err := resolver.ArenaResolveGraphQLResponse(resolveCtx, response, buf)
		require.NoError(t, err)
		require.NotZero(t, buf.Len(), "empty output on iteration %d", i)
		lastOutput = buf.String()
	}
	return lastOutput
}

// --- Error codepath tests (via ArenaResolveGraphQLResponse) ---

func TestArenaGCSafety_FetchError(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(&_errorReturningDataSource{err: errors.New("connection refused")})
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `"data"`)
}

func TestArenaGCSafety_EmptyResponse(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(""))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_InvalidJSON(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource("{invalid"))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_InvalidShape(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"something":"else"}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_SubgraphErrorsWrapMode(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_SubgraphErrorsPassthrough(t *testing.T) {
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `downstream error`)
}

func TestArenaGCSafety_SubgraphErrorsWithExtensionCode(t *testing.T) {
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.DefaultErrorExtensionCode = "DOWNSTREAM_SERVICE_ERROR"
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
			return resp
		},
	)
	// The extension code is set on errors; verify the output is valid
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `downstream error`)
}

func TestArenaGCSafety_SubgraphErrorsWithServiceName(t *testing.T) {
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.AttachServiceNameToErrorExtensions = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `testService`)
}

func TestArenaGCSafety_SubgraphErrorsWithExtensionCodeAndServiceName(t *testing.T) {
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.DefaultErrorExtensionCode = "DOWNSTREAM_SERVICE_ERROR"
	opts.AttachServiceNameToErrorExtensions = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"downstream error"}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `testService`)
}

func TestArenaGCSafety_AuthorizationRejected(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context {
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
		func() *GraphQLResponse {
			resp, fetch := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
			fetch.Info.RootFields[0].HasAuthorizationRule = true
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `Unauthorized`)
}

func TestArenaGCSafety_RateLimitRejected(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context {
			ctx := NewContext(context.Background())
			ctx.SetRateLimiter(&testRateLimiter{
				allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
					return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
				},
			})
			ctx.RateLimitOptions = RateLimitOptions{Enable: true}
			return ctx
		},
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `Rate limit`)
}

func TestArenaGCSafety_RateLimitWithExtensionCode(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context {
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
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"data":{"field":"value"}}`))
			return resp
		},
	)
	assert.Contains(t, output, `RATE_LIMIT_EXCEEDED`)
}

// --- Successful data merge tests ---

func TestArenaGCSafety_MergeResult(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"data":{"field":"hello world"}}`))
			return resp
		},
	)
	assert.Contains(t, output, `hello world`)
	assert.NotContains(t, output, `"errors"`)
}

// --- Resolvable SetNull path tests ---

func TestArenaGCSafety_NullableFieldNull(t *testing.T) {
	// A nullable field that returns null should be set to null via SetNull
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"obj":null}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "obj"},
					},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("obj"),
							Value: &Object{
								Path:     []string{"obj"},
								Nullable: true,
								Fields: []*Field{
									{
										Name:  []byte("name"),
										Value: &String{Path: []string{"name"}, Nullable: true},
									},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `"obj":null`)
}

func TestArenaGCSafety_NonNullableFieldNull(t *testing.T) {
	// A non-nullable field returning null should bubble up and null the parent (if nullable)
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"wrapper":{"name":null}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "wrapper"},
					},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("wrapper"),
							Value: &Object{
								Path:     []string{"wrapper"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: false, // non-nullable
										},
									},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	// The non-nullable field being null should bubble up to null the wrapper object
	assert.Contains(t, output, `"wrapper":null`)
	assert.Contains(t, output, `"errors"`)
}

// --- Authorization skip errors (TrueValue) test ---

func TestArenaGCSafety_AuthRejectionNullableField(t *testing.T) {
	// Tests the TrueValue (formerly MustParse("true")) path:
	// when authorization rejects a nullable field, the field is set to null
	// and __skipErrors=true is set on the item via arena-allocated TrueValue
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context {
			ctx := NewContext(context.Background())
			ctx.SetAuthorizer(createTestAuthorizer(
				func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
					return nil, nil // allow pre-fetch
				},
				func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
					return &AuthorizationDeny{Reason: "forbidden field"}, nil
				},
			))
			return ctx
		},
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"user":{"name":"Alice","secret":"classified"}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "user"},
					},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*Field{
									{
										Name:  []byte("name"),
										Value: &String{Path: []string{"name"}, Nullable: true},
									},
									{
										Name:  []byte("secret"),
										Value: &String{Path: []string{"secret"}, Nullable: true},
										Info: &FieldInfo{
											HasAuthorizationRule: true,
										},
									},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `"name"`)
	assert.Contains(t, output, `"data"`)
}

// --- Nested fetch tree tests ---

func TestArenaGCSafety_SequenceWithErrorThenSuccess(t *testing.T) {
	// Sequence of two fetches: first one has an error, second succeeds.
	// Tests that arena-allocated error values from the first fetch survive GC
	// when the second fetch runs.
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)

	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			errorFetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"errors":[{"message":"first fetch failed"}],"data":{"field":null}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "ds-1",
					DataSourceName: "svc1",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "field"}},
				},
			}
			successFetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"other":"ok"}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "ds-2",
					DataSourceName: "svc2",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "other"}},
				},
			}

			return &GraphQLResponse{
				Fetches: Sequence(
					SingleWithPath(errorFetch, "query"),
					SingleWithPath(successFetch, "query"),
				),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name:  []byte("field"),
							Value: &String{Path: []string{"field"}, Nullable: true},
						},
						{
							Name:  []byte("other"),
							Value: &String{Path: []string{"other"}, Nullable: true},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `first fetch failed`)
	assert.Contains(t, output, `ok`)
}

func TestArenaGCSafety_ParallelFetches(t *testing.T) {
	// Two independent fetches resolved in parallel. Tests that concurrent arena usage
	// doesn't corrupt values when GC runs between iterations.
	resolver := newTestResolver(t, baseResolverOpts())

	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			userFetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"user":{"name":"Bob"}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "ds-users",
					DataSourceName: "users",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "user"}},
				},
			}
			productFetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"product":{"title":"Widget"}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "ds-products",
					DataSourceName: "products",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "product"}},
				},
			}

			return &GraphQLResponse{
				Fetches: Parallel(
					SingleWithPath(userFetch, "query"),
					SingleWithPath(productFetch, "query"),
				),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*Field{
									{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								},
							},
						},
						{
							Name: []byte("product"),
							Value: &Object{
								Path:     []string{"product"},
								Nullable: true,
								Fields: []*Field{
									{Name: []byte("title"), Value: &String{Path: []string{"title"}}},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `Bob`)
	assert.Contains(t, output, `Widget`)
}

// --- Array nullability tests (SetNull for arrays) ---

func TestArenaGCSafety_NullableArrayWithNullItem(t *testing.T) {
	// Tests SetNull path in walkArray: when an array item is null and the item type
	// is non-nullable, the nullable array should be set to null via SetNull.
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"items":[null]}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "items"},
					},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("items"),
							Value: &Array{
								Path:     []string{"items"},
								Nullable: true,
								Item: &String{
									Nullable: false, // non-nullable items
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	// Non-nullable item being null should propagate to null the nullable array
	assert.Contains(t, output, `"items":null`)
}

// --- Mixed success and error responses ---

func TestArenaGCSafety_PartialDataWithErrors(t *testing.T) {
	// Subgraph returns partial data and errors. Both must survive GC.
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(
				`{"errors":[{"message":"partial failure","path":["field"]}],"data":{"field":"partial value"}}`,
			))
			return resp
		},
	)
	assert.Contains(t, output, `partial value`)
	assert.Contains(t, output, `partial failure`)
}

// --- Large/stress tests ---

func TestArenaGCSafety_ManyErrors(t *testing.T) {
	// Subgraph returns many errors. All must survive GC.
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)

	// Build a response with 20 errors
	var errMsgs []string
	for i := 0; i < 20; i++ {
		errMsgs = append(errMsgs, `{"message":"error `+strings.Repeat("x", 100)+`"}`)
	}
	errorsJSON := "[" + strings.Join(errMsgs, ",") + "]"

	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":` + errorsJSON + `,"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
}

// --- Verify JSON validity ---

func TestArenaGCSafety_OutputIsValidJSON(t *testing.T) {
	// Run several different codepaths and verify the output is always valid JSON.
	cases := []struct {
		name string
		opts ResolverOptions
		data string
	}{
		{
			name: "success",
			opts: baseResolverOpts(),
			data: `{"data":{"field":"hello"}}`,
		},
		{
			name: "error_wrap",
			opts: baseResolverOpts(),
			data: `{"errors":[{"message":"fail"}],"data":null}`,
		},
		{
			name: "error_passthrough",
			opts: func() ResolverOptions {
				o := baseResolverOpts()
				o.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
				return o
			}(),
			data: `{"errors":[{"message":"fail"}],"data":null}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := newTestResolver(t, tc.opts)
			for i := 0; i < gcIterations; i++ {
				resp, _ := gcTestResponse(FakeDataSource(tc.data))
				ctx := NewContext(context.Background())
				forceGC()
				buf := &bytes.Buffer{}
				_, err := resolver.ArenaResolveGraphQLResponse(ctx, resp, buf)
				require.NoError(t, err)
				var parsed map[string]interface{}
				require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed), "invalid JSON on iteration %d: %s", i, buf.String())
			}
		})
	}
}

// gcTestResponseWithField builds a GraphQLResponse with a single fetch and a custom field value node.
func gcTestResponseWithField(ds DataSource, fieldName string, fieldValue Node) *GraphQLResponse {
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
				{TypeName: "Query", FieldName: fieldName},
			},
		},
	}
	return &GraphQLResponse{
		Fetches: SingleWithPath(fetch, "query"),
		Data: &Object{
			Nullable: true,
			Fields: []*Field{
				{
					Name:  []byte(fieldName),
					Value: fieldValue,
				},
			},
		},
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
	}
}

// --- Group A: Loader status-code codepaths ---

func TestArenaGCSafety_StatusCodeFallback(t *testing.T) {
	// Covers: renderErrorsStatusFallback, setSubgraphStatusCode
	// When data is null and status code is non-2XX, falls back to status-code error
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(&_statusCodeDataSource{
				data:       `{"data":null}`,
				statusCode: 503,
			})
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `503`)
}

func TestArenaGCSafety_ApolloRouterCompatError(t *testing.T) {
	// Covers: addApolloRouterCompatibilityError
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.ApolloRouterCompatibilitySubrequestHTTPError = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(&_statusCodeDataSource{
				data:       `{"errors":[{"message":"bad"}],"data":null}`,
				statusCode: 500,
			})
			return resp
		},
	)
	assert.Contains(t, output, `SUBREQUEST_HTTP_ERROR`)
}

func TestArenaGCSafety_SubgraphStatusCodeInExtensions(t *testing.T) {
	// Covers: setSubgraphStatusCode (adds statusCode to existing and new extensions)
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(&_statusCodeDataSource{
				data:       `{"errors":[{"message":"fail","extensions":{"code":"SOME_CODE"}}],"data":null}`,
				statusCode: 502,
			})
			return resp
		},
	)
	assert.Contains(t, output, `"statusCode"`)
	assert.Contains(t, output, `502`)
}

// --- Group B: Loader error filtering codepaths ---

func TestArenaGCSafety_OmitErrorExtensions(t *testing.T) {
	// Covers: optionallyOmitErrorExtensions
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.OmitSubgraphErrorExtensions = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"err","extensions":{"code":"X"}}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.NotContains(t, output, `"extensions"`)
}

func TestArenaGCSafety_OmitErrorLocations(t *testing.T) {
	// Covers: optionallyOmitErrorLocations (filters invalid locations, keeps valid ones)
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"err","locations":[{"line":1,"column":2},{"line":0,"column":0}]}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"locations"`)
}

func TestArenaGCSafety_OmitAllErrorLocations(t *testing.T) {
	// Covers: optionallyOmitErrorLocations (omit flag set)
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.OmitSubgraphErrorLocations = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"err","locations":[{"line":1,"column":2}]}],"data":null}`))
			return resp
		},
	)
	assert.NotContains(t, output, `"locations"`)
}

func TestArenaGCSafety_AllowedExtensionFields(t *testing.T) {
	// Covers: optionallyAllowCustomExtensionProperties
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	opts.AllowedErrorExtensionFields = []string{"code"}
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"err","extensions":{"code":"X","secret":"Y"}}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"code"`)
	assert.NotContains(t, output, `"secret"`)
}

func TestArenaGCSafety_WrapModeWithPropagation(t *testing.T) {
	// Covers: mergeErrors wrap mode with propagateSubgraphErrors → SetValue to nest errors under extensions.errors
	opts := baseResolverOpts()
	opts.PropagateSubgraphErrors = true
	// wrap mode is default
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			resp, _ := gcTestResponse(FakeDataSource(`{"errors":[{"message":"inner"}],"data":null}`))
			return resp
		},
	)
	assert.Contains(t, output, `"errors"`)
	assert.Contains(t, output, `inner`)
}

// --- Group C: Resolvable scalar walk functions ---

func TestArenaGCSafety_BooleanField(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"active":true}}`),
				"active",
				&Boolean{Path: []string{"active"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `true`)
	assert.NotContains(t, output, `"errors"`)
}

func TestArenaGCSafety_IntegerField(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"count":42}}`),
				"count",
				&Integer{Path: []string{"count"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `42`)
}

func TestArenaGCSafety_FloatField(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"price":9.99}}`),
				"price",
				&Float{Path: []string{"price"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `9.99`)
}

func TestArenaGCSafety_FloatTruncation(t *testing.T) {
	// Covers: walkFloat with ApolloCompatibilityTruncateFloatValues
	opts := baseResolverOpts()
	opts.ResolvableOptions.ApolloCompatibilityTruncateFloatValues = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"price":10.0}}`),
				"price",
				&Float{Path: []string{"price"}, Nullable: true},
			)
		},
	)
	// Whole-number float should be truncated to int representation
	assert.Contains(t, output, `"price":10`)
}

func TestArenaGCSafety_BigIntField(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"id":9007199254740993}}`),
				"id",
				&BigInt{Path: []string{"id"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `9007199254740993`)
}

func TestArenaGCSafety_ScalarField(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"meta":{"key":"value"}}}`),
				"meta",
				&Scalar{Path: []string{"meta"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"key"`)
}

func TestArenaGCSafety_EnumValid(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"status":"ACTIVE"}}`),
				"status",
				&Enum{Path: []string{"status"}, TypeName: "Status", Values: []string{"ACTIVE", "INACTIVE"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"ACTIVE"`)
}

func TestArenaGCSafety_EnumInvalid(t *testing.T) {
	// Covers: walkEnum invalid value → addErrorWithCode
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"status":"UNKNOWN"}}`),
				"status",
				&Enum{Path: []string{"status"}, TypeName: "Status", Values: []string{"ACTIVE", "INACTIVE"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_StringUnescapeResponseJson(t *testing.T) {
	// Covers: walkString with UnescapeResponseJson → renderScalarFieldBytes → ParseBytesWithArena
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"payload":"{\"nested\":\"value\"}"}}`),
				"payload",
				&String{Path: []string{"payload"}, UnescapeResponseJson: true, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `nested`)
}

func TestArenaGCSafety_CustomNode(t *testing.T) {
	// Covers: walkCustom → MarshalTo + renderScalarFieldBytes → ParseBytesWithArena
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"custom":"hello"}}`),
				"custom",
				&CustomNode{CustomResolve: &_testCustomResolve{}, Path: []string{"custom"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"hello"`)
}

func TestArenaGCSafety_ArrayObjectItemWalkFail(t *testing.T) {
	// Covers: walkArray → SetArrayItem(arena, i, NullValue) when nullable object item has non-nullable field null
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"items":[{"name":"ok"},{"name":null}]}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "items"}},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("items"),
							Value: &Array{
								Path:     []string{"items"},
								Nullable: true,
								Item: &Object{
									Nullable: true,
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path:     []string{"name"},
												Nullable: false, // non-nullable → will fail for null
											},
										},
									},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `"ok"`)
	assert.Contains(t, output, `null`)
}

func TestArenaGCSafety_ValueCompletion(t *testing.T) {
	// Covers: addValueCompletion, printValueCompletionExtension (Apollo compat)
	opts := baseResolverOpts()
	opts.ResolvableOptions.ApolloCompatibilityValueCompletionInExtensions = true
	resolver := newTestResolver(t, opts)
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			fetch := &SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"wrapper":{"required":null}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "test-ds",
					DataSourceName: "testService",
					RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "wrapper"}},
				},
			}
			return &GraphQLResponse{
				Fetches: SingleWithPath(fetch, "query"),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("wrapper"),
							Value: &Object{
								Path:     []string{"wrapper"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("required"),
										Value: &String{
											Path:     []string{"required"},
											Nullable: false, // non-nullable → null triggers value completion
										},
									},
								},
							},
						},
					},
				},
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			}
		},
	)
	assert.Contains(t, output, `"extensions"`)
	assert.Contains(t, output, `"valueCompletion"`)
}

// --- Group D: Type-mismatch error paths ---

func TestArenaGCSafety_BooleanTypeMismatch(t *testing.T) {
	// Covers: walkBoolean type error → addError
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"active":"not_a_bool"}}`),
				"active",
				&Boolean{Path: []string{"active"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_IntegerTypeMismatch(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"count":"not_a_number"}}`),
				"count",
				&Integer{Path: []string{"count"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_FloatTypeMismatch(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"price":"not_a_float"}}`),
				"price",
				&Float{Path: []string{"price"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"errors"`)
}

func TestArenaGCSafety_StringTypeMismatch(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	output := resolveWithGCPressure(t, resolver,
		func() *Context { return NewContext(context.Background()) },
		func() *GraphQLResponse {
			return gcTestResponseWithField(
				FakeDataSource(`{"data":{"name":123}}`),
				"name",
				&String{Path: []string{"name"}, Nullable: true},
			)
		},
	)
	assert.Contains(t, output, `"errors"`)
}
