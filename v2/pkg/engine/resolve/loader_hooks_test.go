package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type TestLoaderHooks struct {
	preFetchCalls  atomic.Int64
	postFetchCalls atomic.Int64
	errors         []error
	mu             sync.Mutex
}

func NewTestLoaderHooks() LoaderHooks {
	return &TestLoaderHooks{
		preFetchCalls:  atomic.Int64{},
		postFetchCalls: atomic.Int64{},
		errors:         make([]error, 0),
		mu:             sync.Mutex{},
	}
}

func (f *TestLoaderHooks) OnLoad(ctx context.Context, ds DataSourceInfo) context.Context {
	f.preFetchCalls.Add(1)

	return ctx
}

func (f *TestLoaderHooks) OnFinished(ctx context.Context, ds DataSourceInfo, responseInfo *ResponseInfo) {
	f.postFetchCalls.Add(1)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.errors = append(f.errors, responseInfo.Err)
}

func TestLoaderHooks_FetchPipeline(t *testing.T) {

	t.Run("simple fetch with simple subgraph error", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage"}]}`), nil
			})
		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()
		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: mockDataSource,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "Users",
						DataSourceName: "Users",
					},
				}, "query"),
				Data: &Object{
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: true,
							},
						},
					},
				},
			}, resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.DataSourceInfo.Name)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

	t.Run("Subgraph errors are available on resolve context when error propagation is disabled", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      false,
			PropagateSubgraphStatusCodes: false,
		})

		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage"}]}`), nil
			})
		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()
		resp := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(resolveCtx, resp, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'."}],"data":{"name":null}}`, buf.String())
		ctrl.Finish()

		loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

		assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
		assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

		var subgraphError *SubgraphError
		assert.Len(t, loaderHooks.errors, 1)
		assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
		assert.Equal(t, "Users", subgraphError.DataSourceInfo.Name)
		assert.Equal(t, "query", subgraphError.Path)
		assert.Equal(t, "", subgraphError.Reason)
		assert.Equal(t, 0, subgraphError.ResponseCode)
		assert.Len(t, subgraphError.DownstreamErrors, 1)
		assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
		assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)

		assert.NotNil(t, resolveCtx.SubgraphErrors())
	})

	t.Run("parallel fetch with simple subgraph error", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage"}]}`), nil
			})
		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()
		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Parallel(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: mockDataSource,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Users",
							DataSourceName: "Users",
						},
					}, "query"),
				),
				Data: &Object{
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: true,
							},
						},
					},
				},
			}, resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.DataSourceInfo.Name)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

	t.Run("fetch with subgraph error and custom extension code. No extension fields are propagated by default", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}]}`), nil
			})
		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()
		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: mockDataSource,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "Users",
						DataSourceName: "Users",
					},
				}, "query"),
				Data: &Object{
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: true,
							},
						},
					},
				},
			}, resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"},{"message":"errorMessage2"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.DataSourceInfo.Name)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 2)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Empty(t, subgraphError.DownstreamErrors[0].Extensions["code"])
				assert.Equal(t, "errorMessage2", subgraphError.DownstreamErrors[1].Message)
				assert.Empty(t, subgraphError.DownstreamErrors[1].Extensions["code"])

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

	t.Run("Propagate only extension code field from subgraph errors", testFnSubgraphErrorsWithExtensionFieldCode(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED","foo":"bar"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}],"data":{"name":null}}`
	}))

	t.Run("Propagate all extension fields from subgraph errors when allow all option is enabled", testFnSubgraphErrorsWithAllowAllExtensionFields(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED","foo":"bar"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED","foo":"bar"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}],"data":{"name":null}}`
	}))

	t.Run("Include datasource name as serviceName extension field", testFnSubgraphErrorsWithExtensionFieldServiceName(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED","serviceName":"Users"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT","serviceName":"Users"}}],"data":{"name":null}}`
	}))

	t.Run("Include datasource name as serviceName when extensions is null", testFnSubgraphErrorsWithExtensionFieldServiceName(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":null},{"message":"errorMessage2","extensions":null}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR","serviceName":"Users"}},{"message":"errorMessage2","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR","serviceName":"Users"}}],"data":{"name":null}}`
	}))

	t.Run("Include datasource name as serviceName when extensions is an empty object", testFnSubgraphErrorsWithExtensionFieldServiceName(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{}},{"message":"errorMessage2","extensions":null}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR","serviceName":"Users"}},{"message":"errorMessage2","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR","serviceName":"Users"}}],"data":{"name":null}}`
	}))

	t.Run("Fallback to default extension code value when no code field was set", testFnSubgraphErrorsWithExtensionDefaultCode(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2"}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR"}}],"data":{"name":null}}`
	}))

	t.Run("Fallback to default extension code value when extensions is null", testFnSubgraphErrorsWithExtensionDefaultCode(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":null},{"message":"errorMessage2"}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR"}},{"message":"errorMessage2","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR"}}],"data":{"name":null}}`
	}))

	t.Run("skipped fetch does not call OnFinished with nil loaderHookContext", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		// First data source returns data where the "user" field is null
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":null}}`), nil
			})

		// Second data source should never be called — the fetch is skipped because parent data is null
		detailsService := NewMockDataSource(ctrl)
		detailsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()

		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Sequence(
					Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Users",
							DataSourceName: "Users",
						},
					}),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: detailsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Details",
							DataSourceName: "Details",
						},
					}, "query.user", ObjectPath("user")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Nullable: true,
								Path:     []string{"user"},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			}, resolveCtx, `{"data":{"user":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)
				// Only the first fetch should trigger OnLoad/OnFinished.
				// The second fetch is skipped (null parent), so OnFinished must NOT be called
				// (its loaderHookContext would be nil, which previously caused a panic in the router).
				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())
			}
	}))

	t.Run("Fallback to default extension code value when extensions is an empty object", testFnSubgraphErrorsWithExtensionDefaultCode(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"errorMessage","extensions":{}},{"message":"errorMessage2"}]}`), nil
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, *NewContext(context.Background()), `{"errors":[{"message":"errorMessage","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR"}},{"message":"errorMessage2","extensions":{"code":"DOWNSTREAM_SERVICE_ERROR"}}],"data":{"name":null}}`
	}))

	t.Run("parallel skipped fetch does not call OnFinished with nil loaderHookContext", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":null}}`), nil
			})

		detailsService := NewMockDataSource(ctrl)
		detailsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()

		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Sequence(
					Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Users",
							DataSourceName: "Users",
						},
					}),
					Parallel(
						SingleWithPath(&SingleFetch{
							FetchConfiguration: FetchConfiguration{
								DataSource: detailsService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
							Info: &FetchInfo{
								DataSourceID:   "Details",
								DataSourceName: "Details",
							},
						}, "query.user", ObjectPath("user")),
					),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Nullable: true,
								Path:     []string{"user"},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			}, resolveCtx, `{"data":{"user":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)
				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())
			}
	}))

	t.Run("skipped entity fetch does not call OnFinished with nil loaderHookContext", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"name":"Bill","info":null}}}`), nil
			})

		infoService := NewMockDataSource(ctrl)
		infoService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()

		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Sequence(
					Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Users",
							DataSourceName: "Users",
						},
					}),
					SingleWithPath(&EntityFetch{
						FetchDependencies: FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						Input: EntityInput{
							Header: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}","variables":{"representations":[`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							Item: InputTemplate{
								Segments: []TemplateSegment{
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{
													Name: []byte("id"),
													Value: &Integer{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Info")},
												},
												{
													Name: []byte("__typename"),
													Value: &String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Info")},
												},
											},
										}),
									},
								},
							},
							Footer: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`]}}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							SkipErrItem: true,
						},
						DataSource: infoService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Info: &FetchInfo{
							DataSourceID:   "Info",
							DataSourceName: "Info",
						},
					}, "user.info", ObjectPath("user"), ObjectPath("info")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path: []string{"user"},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("info"),
										Value: &Object{
											Nullable: true,
											Path:     []string{"info"},
											Fields: []*Field{
												{
													Name: []byte("age"),
													Value: &Integer{
														Path: []string{"age"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, resolveCtx, `{"data":{"user":{"name":"Bill","info":null}}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)
				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())
			}
	}))

	t.Run("skipped batch entity fetch does not call OnFinished with nil loaderHookContext", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return []byte(`{"data":{"user":{"name":"Bill","infoList":[{"id":1,"__typename":"Unknown"}]}}}`), nil
			})

		infoService := NewMockDataSource(ctrl)
		infoService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		resolveCtx := NewContext(context.Background())
		resolveCtx.LoaderHooks = NewTestLoaderHooks()

		return &GraphQLResponse{
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: Sequence(
					Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Info: &FetchInfo{
							DataSourceID:   "Users",
							DataSourceName: "Users",
						},
					}),
					SingleWithPath(&BatchEntityFetch{
						FetchDependencies: FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						Input: BatchInput{
							Header: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}","variables":{"representations":[`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							Items: []InputTemplate{
								{
									Segments: []TemplateSegment{
										{
											SegmentType:  VariableSegmentType,
											VariableKind: ResolvableObjectVariableKind,
											Renderer: NewGraphQLVariableResolveRenderer(&Object{
												Fields: []*Field{
													{
														Name: []byte("id"),
														Value: &Integer{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
													{
														Name: []byte("__typename"),
														Value: &String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											}),
										},
									},
								},
							},
							Separator: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`,`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							Footer: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`]}}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							SkipNullItems:        true,
							SkipEmptyObjectItems: true,
							SkipErrItems:         true,
						},
						DataSource: infoService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities"},
						},
						Info: &FetchInfo{
							DataSourceID:   "Info",
							DataSourceName: "Info",
						},
					}, "user.infoList", ObjectPath("user"), ArrayPath("infoList")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path: []string{"user"},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("infoList"),
										Value: &Array{
											Path: []string{"infoList"},
											Item: &Object{
												Fields: []*Field{
													{
														Name: []byte("age"),
														Value: &Integer{
															Path: []string{"age"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, resolveCtx, `{"data":{"user":{"name":"Bill","infoList":[{}]}}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)
				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())
			}
	}))

}
