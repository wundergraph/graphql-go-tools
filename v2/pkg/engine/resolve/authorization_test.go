package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type preFetchAuthFunc func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)
type objectFieldAuthFunc func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)

type testAuthorizer struct {
	preFetchCalls            atomic.Int64
	objectFieldCalls         atomic.Int64
	authorizePreFetch        preFetchAuthFunc
	authorizeObjectField     objectFieldAuthFunc
	hasResponseExtensionData bool
	responseExtension        []byte
}

func (t *testAuthorizer) HasResponseExtensionData(ctx *Context) bool {
	return t.hasResponseExtensionData
}

func (t *testAuthorizer) RenderResponseExtension(ctx *Context, out io.Writer) error {
	_, err := out.Write(t.responseExtension)
	return err
}

func (t *testAuthorizer) AuthorizePreFetch(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	t.preFetchCalls.Add(1)
	return t.authorizePreFetch(ctx, dataSourceID, input, coordinate)
}

func (t *testAuthorizer) AuthorizeObjectField(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	t.objectFieldCalls.Add(1)
	return t.authorizeObjectField(ctx, dataSourceID, object, coordinate)
}

func createTestAuthorizer(authorizePreFetch preFetchAuthFunc, authorizeObjectField objectFieldAuthFunc) Authorizer {
	return &testAuthorizer{
		authorizePreFetch:    authorizePreFetch,
		authorizeObjectField: authorizeObjectField,
	}
}

func TestAuthorization(t *testing.T) {
	t.Run("allow all", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				require.Nil(t, resolveCtx.subgraphErrors)
			}
	}))
	t.Run("validate authorizer args", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {
		assertions := atomic.Int64{}
		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "Query" && coordinate.FieldName == "me" {
				assert.Equal(t, `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`, string(input))
				assertions.Add(1)
			}
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				assert.Equal(t, `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}}`, string(input))
				assertions.Add(1)
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "reviews" && coordinate.TypeName == "User" && coordinate.FieldName == "reviews" {
				assert.Equal(t, `{"id":"1234","username":"Me","__typename":"User"}`, string(object))
				assertions.Add(1)
			}
			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				assert.Equal(t, `{"body":"A highly effective form of birth control."}`, string(object))
				assertions.Add(1)
			}
			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "product" {
				assert.Equal(t, `{"body":"A highly effective form of birth control."}`, string(object))
				assertions.Add(1)
			}
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				assert.Equal(t, `{"upc":"top-1","__typename":"Product"}`, string(object))
				assertions.Add(1)
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())
				assert.Equal(t, int64(6), assertions.Load())
			}
	}))
	t.Run("disallow field without policy", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "User" && coordinate.FieldName == "id" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch id on User",
				}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				require.Nil(t, resolveCtx.subgraphErrors)
			}
	}))
	t.Run("disallow field with extension", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "User" && coordinate.FieldName == "id" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch id on User",
				}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		authorizer.(*testAuthorizer).hasResponseExtensionData = true
		authorizer.(*testAuthorizer).responseExtension = []byte(`{"missingScopes":["id"]}`)

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}},"extensions":{"authorization":{"missingScopes":["id"]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				require.Nil(t, resolveCtx.subgraphErrors)
			}
	}))
	t.Run("no authorization rules/checks", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponseWithoutAuthorizationRules(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(0), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(0), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				require.Nil(t, resolveCtx.subgraphErrors)
			}
	}))
	t.Run("disallow root fetch", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "Query" && coordinate.FieldName == "me" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from users Subgraph",
				}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query', Reason: Not allowed to fetch from users Subgraph."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"could not render fetch input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"could not render fetch input","path":["me","reviews","@","product"]}]}}],"data":{"me":null}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(1), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(0), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "users", subgraphError.SubgraphName)
				require.Equal(t, "query", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch from users Subgraph", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))

	t.Run("disallow root fetch without reason", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "Query" && coordinate.FieldName == "me" {
				return &AuthorizationDeny{}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at Path 'query'."},{"message":"Failed to fetch from Subgraph 'reviews' at Path 'query.me'.","extensions":{"errors":[{"message":"could not render fetch input","path":["me"]}]}},{"message":"Failed to fetch from Subgraph 'products' at Path 'query.me.reviews.@.product'.","extensions":{"errors":[{"message":"could not render fetch input","path":["me","reviews","@","product"]}]}}],"data":{"me":null}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(1), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(0), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "users", subgraphError.SubgraphName)
				require.Equal(t, "query", subgraphError.Path)
				require.Equal(t, "", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow child fetch", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from products Subgraph",
				}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		resolveCtx := Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, &resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'products' at Path 'query.me.reviews.@.product', Reason: Not allowed to fetch from products Subgraph."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				require.NotEmpty(t, resolveCtx.subgraphErrors)
				require.EqualError(t, resolveCtx.subgraphErrors, "Failed to fetch from Subgraph 'products' at Path: 'query.me.reviews.@.product', Reason: Not allowed to fetch from products Subgraph.")

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "products", subgraphError.SubgraphName)
				require.Equal(t, "query.me.reviews.@.product", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch from products Subgraph", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow child field", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch name on Product",
				}, nil
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "products", subgraphError.SubgraphName)
				require.Equal(t, "Query.me.reviews.product.data.name", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch name on Product", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow nested child fetch", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from products Subgraph",
				}, nil
			}

			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'products' at Path 'query.me.reviews.@.product', Reason: Not allowed to fetch from products Subgraph."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "products", subgraphError.SubgraphName)
				require.Equal(t, "query.me.reviews.@.product", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch from products Subgraph", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("error from authorizer should return", testFnWithError(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, errors.New("some error")
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			``
	}))
	t.Run("disallow nullable field", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch body on Review",
				}, nil
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.body', Reason: Not allowed to fetch body on Review.","path":["me","reviews",0,"body"]},{"message":"Unauthorized to load field 'Query.me.reviews.body', Reason: Not allowed to fetch body on Review.","path":["me","reviews",1,"body"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":null,"product":{"upc":"top-1","name":"Trilby"}},{"body":null,"product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "reviews", subgraphError.SubgraphName)
				require.Equal(t, "Query.me.reviews.body", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch body on Review", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow nullable field without a reason", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{}, nil
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.body'.","path":["me","reviews",0,"body"]},{"message":"Unauthorized to load field 'Query.me.reviews.body'.","path":["me","reviews",1,"body"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":null,"product":{"upc":"top-1","name":"Trilby"}},{"body":null,"product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "reviews", subgraphError.SubgraphName)
				require.Equal(t, "Query.me.reviews.body", subgraphError.Path)
				require.Equal(t, "", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow non-nullable field (fetch)", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch name on Product",
				}, nil
			}
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized request to Subgraph 'products' at Path 'query.me.reviews.@.product', Reason: Not allowed to fetch name on Product."}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "products", subgraphError.SubgraphName)
				require.Equal(t, "query.me.reviews.@.product", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch name on Product", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("disallow non-nullable field (field)", testFnWithPostEvaluation(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch name on Product",
				}, nil
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)
		resolveCtx := &Context{ctx: context.Background(), Variables: nil, authorizer: authorizer}

		return res, resolveCtx,
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).preFetchCalls.Load())
				assert.Equal(t, int64(4), authorizer.(*testAuthorizer).objectFieldCalls.Load())

				var subgraphError *SubgraphError
				require.ErrorAs(t, resolveCtx.subgraphErrors, &subgraphError)
				require.Equal(t, "products", subgraphError.SubgraphName)
				require.Equal(t, "Query.me.reviews.product.data.name", subgraphError.Path)
				require.Equal(t, "Not allowed to fetch name on Product", subgraphError.Reason)
				require.Equal(t, 0, subgraphError.ResponseCode)
				require.Len(t, subgraphError.DownstreamErrors, 0)
			}
	}))
	t.Run("reject during the resolvable process", testFnWithError(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		}, func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch body on Review",
				}, errors.New("some error")
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name', Reason: Not allowed to fetch name on Product.","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`
	}))
}

func generateTestFederationGraphQLResponse(t *testing.T, ctrl *gomock.Controller) *GraphQLResponse {
	userService := NewMockDataSource(ctrl)
	userService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename": "User"}}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	reviewsService := NewMockDataSource(ctrl)
	reviewsService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"_entities": [{"__typename":"User","reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	productService := NewMockDataSource(ctrl)
	productService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"_entities": [{"name": "Trilby"},{"name": "Fedora"}]}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	return &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: userService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
						SelectResponseDataPath:   []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceID: "users",
					RootFields: []GraphCoordinate{
						{
							TypeName:             "Query",
							FieldName:            "me",
							HasAuthorizationRule: true,
						},
					},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("me"),
					Value: &Object{
						Fetch: &SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
										SegmentType: StaticSegmentType,
									},
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{
													Name: []byte("__typename"),
													Value: &String{
														Path: []string{"__typename"},
													},
												},
												{
													Name: []byte("id"),
													Value: &String{
														Path: []string{"id"},
													},
												},
											},
										}),
									},
									{
										Data:        []byte(`]}}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							Info: &FetchInfo{
								DataSourceID: "reviews",
								RootFields: []GraphCoordinate{
									{
										TypeName:  "User",
										FieldName: "reviews",
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: reviewsService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseErrorsPath: []string{"errors"},
									SelectResponseDataPath:   []string{"data", "_entities", "[0]"},
								},
							},
						},
						Path:     []string{"me"},
						Nullable: true,
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
								Info: &FieldInfo{
									Name:                "id",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"users"},
									},
								},
							},
							{
								Name: []byte("username"),
								Value: &String{
									Path: []string{"username"},
								},
								Info: &FieldInfo{
									Name:                "username",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"users"},
									},
								},
							},
							{
								Name: []byte("reviews"),
								Info: &FieldInfo{
									Name:                "reviews",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"reviews"},
									},
									HasAuthorizationRule: true,
								},
								Value: &Array{
									Path:     []string{"reviews"},
									Nullable: true,
									Item: &Object{
										Nullable: true,
										Fields: []*Field{
											{
												Name: []byte("body"),
												Value: &String{
													Path:     []string{"body"},
													Nullable: true,
												},
												Info: &FieldInfo{
													Name:                "body",
													ExactParentTypeName: "Review",
													Source: TypeFieldSource{
														IDs: []string{"reviews"},
													},
													HasAuthorizationRule: true,
												},
											},
											{
												Name: []byte("product"),
												Info: &FieldInfo{
													Name:                "product",
													ExactParentTypeName: "Review",
													Source: TypeFieldSource{
														IDs: []string{"reviews"},
													},
													HasAuthorizationRule: true,
												},
												Value: &Object{
													Path: []string{"product"},
													Fetch: &SingleFetch{
														InputTemplate: InputTemplate{
															Segments: []TemplateSegment{
																{
																	Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																	SegmentType: StaticSegmentType,
																},
																{
																	SegmentType:  VariableSegmentType,
																	VariableKind: ResolvableObjectVariableKind,
																	Renderer: NewGraphQLVariableResolveRenderer(&Array{
																		Item: &Object{
																			Fields: []*Field{
																				{
																					Name: []byte("__typename"),
																					Value: &String{
																						Path: []string{"__typename"},
																					},
																				},
																				{
																					Name: []byte("upc"),
																					Value: &String{
																						Path: []string{"upc"},
																					},
																				},
																			},
																		},
																	}),
																},
																{
																	Data:        []byte(`}}}`),
																	SegmentType: StaticSegmentType,
																},
															},
														},
														Info: &FetchInfo{
															DataSourceID: "products",
															RootFields: []GraphCoordinate{
																{
																	TypeName:             "Product",
																	FieldName:            "name",
																	HasAuthorizationRule: true,
																},
															},
														},
														FetchConfiguration: FetchConfiguration{
															DataSource: productService,
															PostProcessing: PostProcessingConfiguration{
																SelectResponseErrorsPath: []string{"errors"},
																SelectResponseDataPath:   []string{"data", "_entities"},
																MergePath:                []string{"data"},
															},
														},
													},
													Fields: []*Field{
														{
															Name: []byte("upc"),
															Value: &String{
																Path: []string{"upc"},
															},
															Info: &FieldInfo{
																Name:                "upc",
																ExactParentTypeName: "Product",
																Source: TypeFieldSource{
																	IDs: []string{"products"},
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &String{
																Path: []string{"data", "name"},
															},
															Info: &FieldInfo{
																Name:                "name",
																ExactParentTypeName: "Product",
																Source: TypeFieldSource{
																	IDs: []string{"products"},
																},
																HasAuthorizationRule: true,
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
					},
				},
			},
		},
	}
}

func generateTestFederationGraphQLResponseWithoutAuthorizationRules(t *testing.T, ctrl *gomock.Controller) *GraphQLResponse {
	userService := NewMockDataSource(ctrl)
	userService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename": "User"}}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	reviewsService := NewMockDataSource(ctrl)
	reviewsService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"_entities": [{"__typename":"User","reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	productService := NewMockDataSource(ctrl)
	productService.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
			actual := string(input)
			expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}}`
			assert.Equal(t, expected, actual)
			pair := NewBufPair()
			pair.Data.WriteString(`{"_entities": [{"name": "Trilby"},{"name": "Fedora"}]}`)
			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()

	return &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: userService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceID: "users",
					RootFields: []GraphCoordinate{
						{
							TypeName:  "Query",
							FieldName: "me",
						},
					},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("me"),
					Value: &Object{
						Fetch: &SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
										SegmentType: StaticSegmentType,
									},
									{
										SegmentType:  VariableSegmentType,
										VariableKind: ResolvableObjectVariableKind,
										Renderer: NewGraphQLVariableResolveRenderer(&Object{
											Fields: []*Field{
												{
													Name: []byte("__typename"),
													Value: &String{
														Path: []string{"__typename"},
													},
												},
												{
													Name: []byte("id"),
													Value: &String{
														Path: []string{"id"},
													},
												},
											},
										}),
									},
									{
										Data:        []byte(`]}}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							Info: &FetchInfo{
								DataSourceID: "reviews",
								RootFields: []GraphCoordinate{
									{
										TypeName:  "User",
										FieldName: "reviews",
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: reviewsService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities", "[0]"},
								},
							},
						},
						Path:     []string{"me"},
						Nullable: true,
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
								Info: &FieldInfo{
									Name:                "id",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"users"},
									},
								},
							},
							{
								Name: []byte("username"),
								Value: &String{
									Path: []string{"username"},
								},
								Info: &FieldInfo{
									Name:                "username",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"users"},
									},
								},
							},
							{
								Name: []byte("reviews"),
								Info: &FieldInfo{
									Name:                "reviews",
									ExactParentTypeName: "User",
									Source: TypeFieldSource{
										IDs: []string{"reviews"},
									},
								},
								Value: &Array{
									Path:     []string{"reviews"},
									Nullable: true,
									Item: &Object{
										Nullable: true,
										Fields: []*Field{
											{
												Name: []byte("body"),
												Value: &String{
													Path:     []string{"body"},
													Nullable: true,
												},
												Info: &FieldInfo{
													Name:                "body",
													ExactParentTypeName: "Review",
													Source: TypeFieldSource{
														IDs: []string{"reviews"},
													},
												},
											},
											{
												Name: []byte("product"),
												Info: &FieldInfo{
													Name:                "product",
													ExactParentTypeName: "Review",
													Source: TypeFieldSource{
														IDs: []string{"reviews"},
													},
												},
												Value: &Object{
													Path: []string{"product"},
													Fetch: &SingleFetch{
														InputTemplate: InputTemplate{
															Segments: []TemplateSegment{
																{
																	Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																	SegmentType: StaticSegmentType,
																},
																{
																	SegmentType:  VariableSegmentType,
																	VariableKind: ResolvableObjectVariableKind,
																	Renderer: NewGraphQLVariableResolveRenderer(&Array{
																		Item: &Object{
																			Fields: []*Field{
																				{
																					Name: []byte("__typename"),
																					Value: &String{
																						Path: []string{"__typename"},
																					},
																				},
																				{
																					Name: []byte("upc"),
																					Value: &String{
																						Path: []string{"upc"},
																					},
																				},
																			},
																		},
																	}),
																},
																{
																	Data:        []byte(`}}}`),
																	SegmentType: StaticSegmentType,
																},
															},
														},
														Info: &FetchInfo{
															DataSourceID: "products",
															RootFields: []GraphCoordinate{
																{
																	TypeName:  "Product",
																	FieldName: "name",
																},
															},
														},
														FetchConfiguration: FetchConfiguration{
															DataSource: productService,
															PostProcessing: PostProcessingConfiguration{
																SelectResponseDataPath: []string{"data", "_entities"},
																MergePath:              []string{"data"},
															},
														},
													},
													Fields: []*Field{
														{
															Name: []byte("upc"),
															Value: &String{
																Path: []string{"upc"},
															},
															Info: &FieldInfo{
																Name:                "upc",
																ExactParentTypeName: "Product",
																Source: TypeFieldSource{
																	IDs: []string{"products"},
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &String{
																Path: []string{"data", "name"},
															},
															Info: &FieldInfo{
																Name:                "name",
																ExactParentTypeName: "Product",
																Source: TypeFieldSource{
																	IDs: []string{"products"},
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
						},
					},
				},
			},
		},
	}
}
