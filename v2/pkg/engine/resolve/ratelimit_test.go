package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type rateLimitStats struct {
	Policy  string
	Allowed int64
	Used    int64
}

type testRateLimiter struct {
	policy                 string
	allowed                int64
	allowFn                func(*Context, *FetchInfo, json.RawMessage) (*RateLimitDeny, error)
	rateLimitPreFetchCalls atomic.Int64
}

func (t *testRateLimiter) RenderResponseExtension(ctx *Context, out io.Writer) error {
	stats := rateLimitStats{
		Policy:  t.policy,
		Allowed: t.allowed,
		Used:    t.rateLimitPreFetchCalls.Load(),
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (t *testRateLimiter) RateLimitPreFetch(ctx *Context, info *FetchInfo, input json.RawMessage) (result *RateLimitDeny, err error) {
	t.rateLimitPreFetchCalls.Add(1)
	return t.allowFn(ctx, info, input)
}

func TestRateLimiter(t *testing.T) {
	t.Run("allow", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(3), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("allow with stats", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			policy:  "10 requests per second",
			allowed: 10,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}},"extensions":{"rateLimit":{"Policy":"10 requests per second","Allowed":10,"Used":3}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(3), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("deny all", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}},
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'users' at Path 'query', Reason: rate limit exceeded."}],"data":null}`,
			func(t *testing.T) {
				assert.Equal(t, int64(1), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("err all", testFnWithError(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return nil, errors.New("some error")
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}}, ""
	}))
	t.Run("deny nested", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				if info.DataSourceID == "products" {
					for _, coordinate := range info.RootFields {
						if coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
							return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
						}
					}
				}
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}},
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'products' at Path 'query.me.reviews.@.product', Reason: rate limit exceeded."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(3), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("deny nested with stats", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			policy:  "1 request per second",
			allowed: 1,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				if info.DataSourceID == "products" {
					for _, coordinate := range info.RootFields {
						if coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
							return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
						}
					}
				}
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}},
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'products' at Path 'query.me.reviews.@.product', Reason: rate limit exceeded."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}},"extensions":{"rateLimit":{"Policy":"1 request per second","Allowed":1,"Used":3}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(3), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("deny nested without reason", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				if info.DataSourceID == "products" {
					for _, coordinate := range info.RootFields {
						if coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
							return &RateLimitDeny{Reason: ""}, nil
						}
					}
				}
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}},
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'products' at Path 'query.me.reviews.@.product'."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(3), limiter.rateLimitPreFetchCalls.Load())
			}
	}))
	t.Run("err nested", testFnWithError(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		limiter := &testRateLimiter{
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				if info.DataSourceID == "products" {
					for _, coordinate := range info.RootFields {
						if coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
							return nil, errors.New("some error")
						}
					}
				}
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true}}, ""
	}))
}
