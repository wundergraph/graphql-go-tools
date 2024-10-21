package resolve

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
)

func TestExtensions(t *testing.T) {
	t.Run("authorization", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null}}`,
			func(t *testing.T) {}
	}))
	t.Run("authorization deny & rate limit deny", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		})

		authorizer.(*testAuthorizer).hasResponseExtensionData = true
		authorizer.(*testAuthorizer).responseExtension = []byte(`{"missingScopes":[["read:users"]]}`)

		limiter := &testRateLimiter{
			policy:  "policy",
			allowed: 0,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}},
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0}}}`,
			func(t *testing.T) {}
	}))
	t.Run("authorization deny & rate limit", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		})

		authorizer.(*testAuthorizer).hasResponseExtensionData = true
		authorizer.(*testAuthorizer).responseExtension = []byte(`{"missingScopes":[["read:users"]]}`)

		limiter := &testRateLimiter{
			policy:  "policy",
			allowed: 0,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return nil, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}},
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0}}}`,
			func(t *testing.T) {}
	}))
	t.Run("authorization & rate limit deny", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		limiter := &testRateLimiter{
			policy:  "policy",
			allowed: 0,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}},
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'users' at Path 'query', Reason: rate limit exceeded."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"rateLimit":{"Policy":"policy","Allowed":0,"Used":1}}}`,
			func(t *testing.T) {}
	}))
	t.Run("authorization & rate limit & trace", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		})

		authorizer.(*testAuthorizer).hasResponseExtensionData = true
		authorizer.(*testAuthorizer).responseExtension = []byte(`{"missingScopes":[["read:users"]]}`)

		limiter := &testRateLimiter{
			policy:  "policy",
			allowed: 0,
			allowFn: func(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
				return &RateLimitDeny{Reason: "rate limit exceeded"}, nil
			},
		}

		res := generateTestFederationGraphQLResponse(t, ctrl)

		ctx = &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer, rateLimiter: limiter, RateLimitOptions: RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}, TracingOptions: TraceOptions{Enable: true, IncludeTraceOutputInResponseExtensions: true, EnablePredictableDebugTimings: true, Debug: true}}
		ctx.ctx = SetTraceStart(ctx.ctx, true)

		return res, ctx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0},"trace":{"version":"1","info":{"trace_start_time":"","trace_start_unix":0,"parse_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"normalize_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"validate_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"planner_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""}},"fetches":{"kind":"Sequence","children":[{"kind":"Single","fetch":{"kind":"Single","path":"query","source_id":"users","source_name":"users","trace":{"raw_input_data":{},"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}},{"kind":"Single","fetch":{"kind":"Single","path":"query.me","source_id":"reviews","source_name":"reviews","trace":{"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}},{"kind":"Single","fetch":{"kind":"Single","path":"query.me.reviews.@.product","source_id":"products","source_name":"products","trace":{"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}}]}}}}`,
			func(t *testing.T) {}
	}))
}
