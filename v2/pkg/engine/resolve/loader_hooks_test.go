package resolve

import (
	"bytes"
	"context"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"io"
	"sync"
	"sync/atomic"
	"testing"
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

func (f *TestLoaderHooks) OnLoad(ctx context.Context, dataSourceID string) context.Context {
	f.preFetchCalls.Add(1)

	return ctx
}

func (f *TestLoaderHooks) OnFinished(ctx context.Context, statusCode int, dataSourceID string, err error) {
	f.postFetchCalls.Add(1)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.errors = append(f.errors, err)
}

func TestLoaderHooks_FetchPipeline(t *testing.T) {

	t.Run("simple fetch with simple subgraph error", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := Context{
			ctx:         context.Background(),
			LoaderHooks: NewTestLoaderHooks(),
		}
		return &GraphQLResponse{
				Data: &Object{
					Nullable: false,
					Fetch: &SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: mockDataSource,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						Info: &FetchInfo{
							DataSourceID: "Users",
						},
					},
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
			}, &resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.SubgraphName)
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
			Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := &Context{
			ctx:         context.Background(),
			LoaderHooks: NewTestLoaderHooks(),
		}
		resp := &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: mockDataSource,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					Info: &FetchInfo{
						DataSourceID: "Users",
					},
				},
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
		err := r.ResolveGraphQLResponse(resolveCtx, resp, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'."}],"data":{"name":null}}`, buf.String())
		ctrl.Finish()

		loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

		assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
		assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

		var subgraphError *SubgraphError
		assert.Len(t, loaderHooks.errors, 1)
		assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
		assert.Equal(t, "Users", subgraphError.SubgraphName)
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
			Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := &Context{
			ctx:         context.Background(),
			LoaderHooks: NewTestLoaderHooks(),
		}
		return &GraphQLResponse{
				Data: &Object{
					Nullable: false,
					Fetch: &ParallelFetch{
						Fetches: []Fetch{
							&SingleFetch{
								FetchConfiguration: FetchConfiguration{
									DataSource: mockDataSource,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseErrorsPath: []string{"errors"},
									},
								},
								Info: &FetchInfo{
									DataSourceID: "Users",
								},
							},
						},
					},
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
				assert.Equal(t, "Users", subgraphError.SubgraphName)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

	t.Run("parallel list item fetch with simple subgraph error", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := Context{
			ctx:         context.Background(),
			LoaderHooks: NewTestLoaderHooks(),
		}
		return &GraphQLResponse{
				Data: &Object{
					Nullable: false,
					Fetch: &ParallelListItemFetch{
						Fetch: &SingleFetch{
							FetchConfiguration: FetchConfiguration{
								DataSource: mockDataSource,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseErrorsPath: []string{"errors"},
								},
							},
							Info: &FetchInfo{
								DataSourceID: "Users",
							},
						},
					},
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
			}, &resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.SubgraphName)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

	t.Run("fetch with subgraph error and custom extension code", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, []byte("{\"code\":\"GRAPHQL_VALIDATION_FAILED\"}"))
				pair.WriteErr([]byte("errorMessage2"), nil, nil, []byte("{\"code\":\"BAD_USER_INPUT\"}"))
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := Context{
			ctx:         context.Background(),
			LoaderHooks: NewTestLoaderHooks(),
		}
		return &GraphQLResponse{
				Data: &Object{
					Nullable: false,
					Fetch: &SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: mockDataSource,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						Info: &FetchInfo{
							DataSourceID: "Users",
						},
					},
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
			}, &resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"errorMessage2","extensions":{"code":"BAD_USER_INPUT"}}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				loaderHooks := resolveCtx.LoaderHooks.(*TestLoaderHooks)

				assert.Equal(t, int64(1), loaderHooks.preFetchCalls.Load())
				assert.Equal(t, int64(1), loaderHooks.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, loaderHooks.errors, 1)
				assert.ErrorAs(t, loaderHooks.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.SubgraphName)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 2)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Equal(t, "GRAPHQL_VALIDATION_FAILED", subgraphError.DownstreamErrors[0].Extensions["code"])
				assert.Equal(t, "errorMessage2", subgraphError.DownstreamErrors[1].Message)
				assert.Equal(t, "BAD_USER_INPUT", subgraphError.DownstreamErrors[1].Extensions["code"])

				assert.NotNil(t, resolveCtx.SubgraphErrors())
			}
	}))

}
