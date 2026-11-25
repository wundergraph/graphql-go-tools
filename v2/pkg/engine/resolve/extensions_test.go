package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// failOnTokenWriter fails only on the selected token, allowing subsequent writes to succeed.
type failOnTokenWriter struct {
	token string
	err   error
}

func (w failOnTokenWriter) Write(p []byte) (int, error) {
	if string(p) == w.token {
		return 0, w.err
	}
	return len(p), nil
}

func TestResolve_ResponseExtensionErrors(t *testing.T) {
	t.Parallel()

	fieldErr := errors.New("field renderer failed")
	extensionErr := errors.New("authorization extension failed")
	writeErr := errors.New("writing data failed")
	const authExtension = `{"scope":"test"}`

	cases := []struct {
		name       string
		skipLoader bool
		writer     io.Writer
		want       error
	}{
		{name: "query plan preserves field error", writer: io.Discard, want: fieldErr},
		{name: "extension error takes precedence", writer: failOnTokenWriter{token: authExtension, err: extensionErr}, want: extensionErr},
		{name: "query plan preserves write error with loader skipped", skipLoader: true, writer: failOnTokenWriter{token: "null", err: writeErr}, want: writeErr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			ctx.ExecutionOptions.SkipLoader = tc.skipLoader
			ctx.ExecutionOptions.IncludeQueryPlanInResponse = true
			ctx.SetAuthorizer(&testAuthorizer{hasResponseExtensionData: true, responseExtension: []byte(authExtension)})
			ctx.SetFieldValueRenderer(createTestFieldValueRenderer(func(*Context, FieldValue, io.Writer) error {
				return fieldErr
			}))
			res := NewResolvable(nil, ResolvableOptions{})
			require.NoError(t, res.Init(ctx, []byte(`{"value":"hello"}`), ast.OperationTypeQuery))
			data := &Object{Fields: []*Field{{
				Name:  []byte("value"),
				Value: &String{Path: []string{"value"}},
				Info:  &FieldInfo{Name: "value", NamedType: "String", ExactParentTypeName: "Query"},
			}}}
			assert.ErrorIs(t, res.Resolve(t.Context(), data, nil, tc.writer), tc.want)
		})
	}
}

func TestExtensions(t *testing.T) {
	t.Run("authorization", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return &AuthorizationDeny{Reason: "test"}, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		resolveCtx := NewContext(context.Background())
		resolveCtx.authorizer = authorizer
		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test.","extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null}}`,
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

		resolveCtx := NewContext(context.Background())
		resolveCtx.authorizer = authorizer
		resolveCtx.rateLimiter = limiter
		resolveCtx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}
		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test.","extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0}}}`,
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

		resolveCtx := NewContext(context.Background())
		resolveCtx.authorizer = authorizer
		resolveCtx.rateLimiter = limiter
		resolveCtx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}
		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test.","extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0}}}`,
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

		resolveCtx := NewContext(context.Background())
		resolveCtx.authorizer = authorizer
		resolveCtx.rateLimiter = limiter
		resolveCtx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}
		return res, resolveCtx,
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

		ctx = NewContext(context.Background())
		ctx.authorizer = authorizer
		ctx.rateLimiter = limiter
		ctx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}
		ctx.TracingOptions = TraceOptions{Enable: true, IncludeTraceOutputInResponseExtensions: true, EnablePredictableDebugTimings: true, Debug: true}
		ctx.ctx = SetTraceStart(ctx.ctx, true)

		return res, ctx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: test.","extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"authorization":{"missingScopes":[["read:users"]]},"rateLimit":{"Policy":"policy","Allowed":0,"Used":0},"trace":{"version":"1","info":{"trace_start_time":"","trace_start_unix":0,"parse_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"normalize_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"validate_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""},"planner_stats":{"duration_nanoseconds":0,"duration_pretty":"","duration_since_start_nanoseconds":0,"duration_since_start_pretty":""}},"fetches":{"kind":"Sequence","children":[{"kind":"Single","fetch":{"kind":"Single","path":"query","source_id":"users","source_name":"users","trace":{"raw_input_data":{},"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}},{"kind":"Single","fetch":{"kind":"Single","path":"query.me","source_id":"reviews","source_name":"reviews","trace":{"raw_input_data":null,"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}},{"kind":"Single","fetch":{"kind":"Single","path":"query.me.reviews.@.product","source_id":"products","source_name":"products","trace":{"raw_input_data":null,"single_flight_used":false,"single_flight_shared_response":false,"load_skipped":false}}}]}}}}`,
			func(t *testing.T) {}
	}))
	t.Run("inline arguments", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		res := generateTestFederationGraphQLResponse(t, ctrl)

		resolveCtx := NewContext(context.Background())
		resolveCtx.InlineArguments = []string{"user.filter", "@include.if"}
		return res, resolveCtx,
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}},"extensions":{"inlineArguments":{"count":2,"arguments":["user.filter","@include.if"]}}}`,
			func(t *testing.T) {}
	}))
	t.Run("rate limit deny & inline arguments", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

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

		resolveCtx := NewContext(context.Background())
		resolveCtx.authorizer = authorizer
		resolveCtx.rateLimiter = limiter
		resolveCtx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}
		resolveCtx.InlineArguments = []string{"user.filter", "@include.if"}
		return res, resolveCtx,
			`{"errors":[{"message":"Rate limit exceeded for Subgraph 'users' at Path 'query', Reason: rate limit exceeded."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"Failed to render Fetch Input","path":["me","reviews","@","product"]}]}}],"data":{"me":null},"extensions":{"rateLimit":{"Policy":"policy","Allowed":0,"Used":1},"inlineArguments":{"count":2,"arguments":["user.filter","@include.if"]}}}`,
			func(t *testing.T) {}
	}))
}
