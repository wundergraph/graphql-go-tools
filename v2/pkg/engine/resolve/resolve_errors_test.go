package resolve

import (
	"bytes"
	"context"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

type TestRequestHooks struct {
	preFetchCalls  atomic.Int64
	postFetchCalls atomic.Int64
	errors         []error
	mu             sync.Mutex
}

func NewRequestHooks() RequestHooks {
	return &TestRequestHooks{
		preFetchCalls:  atomic.Int64{},
		postFetchCalls: atomic.Int64{},
		errors:         make([]error, 0),
		mu:             sync.Mutex{},
	}
}

func (f *TestRequestHooks) OnRequest(ctx *Context, dataSourceID string) *Context {
	f.preFetchCalls.Add(1)

	return ctx
}

func (f *TestRequestHooks) OnResponse(ctx *Context, dataSourceID string, err error) *Context {
	f.postFetchCalls.Add(1)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.errors = append(f.errors, err)

	return ctx
}

func TestResolver_FetchPipeline(t *testing.T) {

	t.Run("fetch with simple subgraph error", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := Context{
			ctx:          context.Background(),
			RequestHooks: NewRequestHooks(),
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
			}, resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				fp := resolveCtx.RequestHooks.(*TestRequestHooks)

				assert.Equal(t, int64(1), fp.preFetchCalls.Load())
				assert.Equal(t, int64(1), fp.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, fp.errors, 1)
				assert.ErrorAs(t, fp.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.SubgraphName)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Nil(t, subgraphError.DownstreamErrors[0].Extensions)
			}
	}))

	t.Run("fetch with subgraph error and custom extension code", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, []byte("{\"code\":\"GRAPHQL_VALIDATION_FAILED\"}"))
				return writeGraphqlResponse(pair, w, false)
			})
		resolveCtx := Context{
			ctx:          context.Background(),
			RequestHooks: NewRequestHooks(),
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
			}, resolveCtx, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at path 'query'.","extensions":{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}}],"data":{"name":null}}`,
			func(t *testing.T) {
				fp := resolveCtx.RequestHooks.(*TestRequestHooks)

				assert.Equal(t, int64(1), fp.preFetchCalls.Load())
				assert.Equal(t, int64(1), fp.postFetchCalls.Load())

				var subgraphError *SubgraphError
				assert.Len(t, fp.errors, 1)
				assert.ErrorAs(t, fp.errors[0], &subgraphError)
				assert.Equal(t, "Users", subgraphError.SubgraphName)
				assert.Equal(t, "query", subgraphError.Path)
				assert.Equal(t, "", subgraphError.Reason)
				assert.Equal(t, 0, subgraphError.ResponseCode)
				assert.Len(t, subgraphError.DownstreamErrors, 1)
				assert.Equal(t, "errorMessage", subgraphError.DownstreamErrors[0].Message)
				assert.Equal(t, "GRAPHQL_VALIDATION_FAILED", subgraphError.DownstreamErrors[0].Extensions.Code)
			}
	}))

}
