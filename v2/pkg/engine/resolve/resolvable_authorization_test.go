package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type resolvableAuthorizationAuthorizer struct {
	authorizeObjectField func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error)
	objectFieldCalls     int
	seenCoordinates      []GraphCoordinate
}

type resolvableAuthorizationDataSource struct {
	data []byte
}

func (d resolvableAuthorizationDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	return d.data, nil
}

func (d resolvableAuthorizationDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.data, nil
}

func (a *resolvableAuthorizationAuthorizer) AuthorizePreFetch(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (a *resolvableAuthorizationAuthorizer) AuthorizeObjectField(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
	a.objectFieldCalls++
	a.seenCoordinates = append(a.seenCoordinates, coordinate)
	if a.authorizeObjectField == nil {
		return nil, nil
	}
	return a.authorizeObjectField(ctx, dataSourceID, object, coordinate)
}

func (a *resolvableAuthorizationAuthorizer) HasResponseExtensionData(ctx *Context) bool {
	return false
}

func (a *resolvableAuthorizationAuthorizer) RenderResponseExtension(ctx *Context, out io.Writer) error {
	return nil
}

func TestResolvableAuthorizationAuthorizeField(t *testing.T) {
	value := mustParseAuthorizationValue(t, `{"__typename":"ActualParent","name":"Ada"}`)
	baseField := func() *Field {
		return &Field{
			Name: []byte("secret"),
			Info: &FieldInfo{
				Name:                 "secret",
				ExactParentTypeName:  "DeclaredParent",
				HasAuthorizationRule: true,
				Source: TypeFieldSource{
					IDs:   []string{"accounts"},
					Names: []string{"Accounts"},
				},
			},
			Value: &String{Path: []string{"secret"}},
		}
	}

	t.Run("field info nil", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		assert.False(t, resolvable.authorizeField(value, &Field{}))
	})

	t.Run("field has no authorization rule", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		field := baseField()
		field.Info.HasAuthorizationRule = false
		assert.False(t, resolvable.authorizeField(value, field))
	})

	t.Run("no authorizers", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		assert.False(t, resolvable.authorizeField(value, baseField()))
	})

	t.Run("empty source IDs", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(&resolvableAuthorizationAuthorizer{})
		resolvable := newResolvableForAuthorizationTest(ctx)
		field := baseField()
		field.Info.Source.IDs = nil
		assert.False(t, resolvable.authorizeField(value, field))
	})

	t.Run("allowed", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		assert.False(t, resolvable.authorizeField(value, baseField()))
		require.Len(t, authorizer.seenCoordinates, 1)
		assert.Equal(t, GraphCoordinate{TypeName: "ActualParent", FieldName: "secret"}, authorizer.seenCoordinates[0])
	})

	t.Run("denied adds reject error", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{
			authorizeObjectField: func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
				return &AuthorizationDeny{Reason: "missing scope"}, nil
			},
		}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		assert.True(t, resolvable.authorizeField(value, baseField()))
		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.secret', Reason: missing scope.","path":["secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
		require.Error(t, ctx.subgraphErrors["Accounts"])
	})

	t.Run("pre-fetch authorizer uses exact parent type name", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		ctx.SetPreFetchFieldAuthorizer(&batchTestAuthorizer{})
		resolvable := newResolvableForAuthorizationTest(ctx)

		assert.False(t, resolvable.authorizeField(value, baseField()))
		require.Len(t, authorizer.seenCoordinates, 1)
		assert.Equal(t, GraphCoordinate{TypeName: "DeclaredParent", FieldName: "secret"}, authorizer.seenCoordinates[0])
	})

	t.Run("authorization error", func(t *testing.T) {
		authErr := stderrors.New("authorizer failed")
		authorizer := &resolvableAuthorizationAuthorizer{
			authorizeObjectField: func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
				return nil, authErr
			},
		}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		assert.True(t, resolvable.authorizeField(value, baseField()))
		assert.ErrorIs(t, resolvable.authorizationError, authErr)
	})
}

func TestResolvableAuthorizationAuthorize(t *testing.T) {
	value := mustParseAuthorizationValue(t, `{"__typename":"User","name":"Ada"}`)
	coordinate := GraphCoordinate{TypeName: "User", FieldName: "name"}

	t.Run("allow cache hit", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)
		resolvable.authorization.seedAllow("users", coordinate)

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, authorizer.objectFieldCalls)
	})

	t.Run("deny cache hit", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)
		resolvable.authorization.seedDeny("users", coordinate, "cached deny")

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "cached deny", result.Reason)
		assert.Equal(t, 0, authorizer.objectFieldCalls)
	})

	t.Run("nil authorizer miss allows", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("authorizer allows and seeds cache", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		require.NoError(t, err)
		assert.Nil(t, result)

		_, ok := resolvable.authorization.allow[authorizationDecisionID("users", coordinate)]
		assert.True(t, ok)
	})

	t.Run("authorizer denies and seeds cache", func(t *testing.T) {
		authorizer := &resolvableAuthorizationAuthorizer{
			authorizeObjectField: func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
				return &AuthorizationDeny{Reason: "denied"}, nil
			},
		}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "denied", result.Reason)

		reason, ok := resolvable.authorization.deny[authorizationDecisionID("users", coordinate)]
		assert.True(t, ok)
		assert.Equal(t, "denied", reason)
	})

	t.Run("authorizer error", func(t *testing.T) {
		authErr := stderrors.New("boom")
		authorizer := &resolvableAuthorizationAuthorizer{
			authorizeObjectField: func(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
				return nil, authErr
			},
		}
		ctx := NewContext(context.Background())
		ctx.SetAuthorizer(authorizer)
		resolvable := newResolvableForAuthorizationTest(ctx)

		result, err := resolvable.authorization.decide(value, "users", coordinate)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, authErr)
	})
}

func TestFieldAuthorizationDecisionSeeding(t *testing.T) {
	authorization := NewFieldAuthorization(NewContext(context.Background()))
	coordinate := GraphCoordinate{TypeName: "User", FieldName: "email"}

	assert.Equal(t, authorizationDecisionID("users", coordinate), authorizationDecisionID("users", coordinate))
	assert.NotEqual(t, authorizationDecisionID("users", coordinate), authorizationDecisionID("profiles", coordinate))

	authorization.seedAllow("users", coordinate)
	_, allowed := authorization.allow[authorizationDecisionID("users", coordinate)]
	assert.True(t, allowed)

	authorization.seedDeny("users", coordinate, "missing email scope")
	reason, denied := authorization.denyReason("users", coordinate)
	assert.True(t, denied)
	assert.Equal(t, "missing email scope", reason)

	reason, denied = authorization.denyReason("users", GraphCoordinate{TypeName: "User", FieldName: "name"})
	assert.False(t, denied)
	assert.Empty(t, reason)
}

func TestResolvableAuthorizationUnreachedData(t *testing.T) {
	t.Run("empty list emits nested denied field once", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		root := authorizationUnreachedRoot()
		data := mustParseAuthorizationValue(t, `{"products":[]}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("null nullable parent emits denied nested field", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Account", FieldName: "secret"}, "missing account scope")
		root := authorizationNestedRoot()
		data := mustParseAuthorizationValue(t, `{"account":null}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret', Reason: missing account scope.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("nested object with reached child emits nothing", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Account", FieldName: "secret"}, "missing account scope")
		root := authorizationNestedRoot()
		data := mustParseAuthorizationValue(t, `{"account":{"secret":"hidden"}}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Nil(t, resolvable.errors)
	})

	t.Run("array of non-objects emits denied nested field once via dedup map", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		root := authorizationUnreachedRoot()
		data := mustParseAuthorizationValue(t, `{"products":["not-object","also-not-object"]}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("non-array value for array field emits denied nested field", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		root := authorizationUnreachedRoot()
		data := mustParseAuthorizationValue(t, `{"products":"not-array"}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("non-object root walks subtree", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "")
		root := authorizationUnreachedRoot()
		data := mustParseAuthorizationValue(t, `null`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedData(root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.secret'.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("direct nested array path emits denied field", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		node := &Array{
			Path: []string{"edges"},
			Item: &Object{
				Fields: []*Field{authorizationProtectedField("products", "products", "Product", "secret", &String{Path: []string{"secret"}})},
			},
		}
		value := mustParseAuthorizationValue(t, `{"edges":[]}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedNode(node, value, []string{"products"}, map[string]struct{}{})

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.edges.secret', Reason: missing product scope.","path":["products","edges","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("direct nested array path with non-array child emits denied field", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		node := &Array{
			Path: []string{"edges"},
			Item: &Object{
				Fields: []*Field{authorizationProtectedField("products", "products", "Product", "secret", &String{Path: []string{"secret"}})},
			},
		}
		value := mustParseAuthorizationValue(t, `{"edges":"not-array"}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedNode(node, value, []string{"products"}, map[string]struct{}{})

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.edges.secret', Reason: missing product scope.","path":["products","edges","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("direct nested array path recurses into array items", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		node := &Array{
			Path: []string{"edges"},
			Item: &Array{
				Path: []string{"nodes"},
				Item: &Object{
					Fields: []*Field{authorizationProtectedField("products", "products", "Product", "secret", &String{Path: []string{"secret"}})},
				},
			},
		}
		value := mustParseAuthorizationValue(t, `{"edges":[{"nodes":[]}]}`)

		resolvable.appendUnauthorizedFieldErrorsForUnreachedNode(node, value, []string{"products"}, map[string]struct{}{})

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.edges.nodes.secret', Reason: missing product scope.","path":["products","edges","nodes","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})
}

func TestResolvableAuthorizationUnreachedDataEndToEnd(t *testing.T) {
	service := resolvableAuthorizationDataSource{data: []byte(`{"data":{"products":[]}}`)}
	response := productsSecretResponse(service)
	response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
		{DataSourceID: "products", Coordinate: GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
	}
	authorizer := &batchTestAuthorizer{
		decisions: map[GraphCoordinate]AuthorizationDecision{
			{TypeName: "Product", FieldName: "secret"}: {Allowed: false, Reason: "missing product scope"},
		},
	}
	resolveCtx := NewContext(context.Background())
	resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

	var buf bytes.Buffer
	resolver := newResolver(context.Background())
	_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
	require.NoError(t, err)

	assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"products":[]}}`, buf.String())
}

func TestResolvableAuthorizationHelpers(t *testing.T) {
	t.Run("append authorization path", func(t *testing.T) {
		assert.Equal(t, []string{"root", "child", "leaf"}, appendAuthorizationPath([]string{"root"}, []string{"child", "leaf"}))
	})

	t.Run("append authorization field path uses node path", func(t *testing.T) {
		field := &Field{Name: []byte("alias"), Value: &String{Path: []string{"node", "field"}}}
		assert.Equal(t, []string{"root", "node", "field"}, appendAuthorizationFieldPath([]string{"root"}, field))
	})

	t.Run("append authorization field path falls back to field name", func(t *testing.T) {
		field := &Field{Name: []byte("alias"), Value: &StaticString{Value: "constant"}}
		assert.Equal(t, []string{"root", "alias"}, appendAuthorizationFieldPath([]string{"root"}, field))
	})

	t.Run("first string", func(t *testing.T) {
		assert.Empty(t, firstString(nil))
		assert.Equal(t, "first", firstString([]string{"first", "second"}))
	})
}

func TestResolvableAuthorizationRejectErrors(t *testing.T) {
	field := &Field{Name: []byte("secret"), Value: &String{Path: []string{"account", "secret"}}}

	t.Run("field error with reason", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))

		resolvable.addRejectFieldError("missing scope", DataSourceInfo{ID: "accounts", Name: "Accounts"}, field)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret', Reason: missing scope.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
		require.Error(t, resolvable.ctx.subgraphErrors["Accounts"])
	})

	t.Run("field error without reason", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))

		resolvable.addRejectFieldError("", DataSourceInfo{ID: "accounts", Name: "Accounts"}, field)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret'.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("field path error with reason", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))

		resolvable.addRejectFieldPathError("missing scope", DataSourceInfo{ID: "accounts", Name: "Accounts"}, []string{"account", "secret"})

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret', Reason: missing scope.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
		require.Error(t, resolvable.ctx.subgraphErrors["Accounts"])
	})

	t.Run("field path error without reason", func(t *testing.T) {
		resolvable := newResolvableForAuthorizationTest(NewContext(context.Background()))

		resolvable.addRejectFieldPathError("", DataSourceInfo{ID: "accounts", Name: "Accounts"}, []string{"account", "secret"})

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret'.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})
}

func TestResolvableAuthorizationObjectFieldTypeName(t *testing.T) {
	resolvable := NewResolvable(nil, ResolvableOptions{})
	field := &Field{
		Info: &FieldInfo{ExactParentTypeName: "Fallback"},
	}

	assert.Equal(t, "Concrete", resolvable.objectFieldTypeName(mustParseAuthorizationValue(t, `{"__typename":"Concrete"}`), field))
	assert.Equal(t, "Fallback", resolvable.objectFieldTypeName(mustParseAuthorizationValue(t, `{}`), field))
}

func newResolvableForAuthorizationTest(ctx *Context) *Resolvable {
	resolvable := NewResolvable(nil, ResolvableOptions{})
	requireNoError := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	if requireNoError != nil {
		panic(requireNoError)
	}
	return resolvable
}

func mustParseAuthorizationValue(t *testing.T, data string) *astjson.Value {
	t.Helper()
	value, err := astjson.ParseBytes([]byte(data))
	require.NoError(t, err)
	return value
}

func authorizationUnreachedRoot() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte("products"),
				Value: &Array{
					Path: []string{"products"},
					Item: &Object{
						Fields: []*Field{
							authorizationProtectedField("products", "products", "Product", "secret", &String{Path: []string{"secret"}}),
						},
					},
				},
			},
		},
	}
}

func authorizationNestedRoot() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:  []byte("account"),
				Value: &Object{Path: []string{"account"}, Fields: []*Field{authorizationProtectedField("accounts", "Accounts", "Account", "secret", &String{Path: []string{"secret"}})}},
			},
		},
	}
}

func authorizationProtectedField(dataSourceID, dataSourceName, parentTypeName, fieldName string, value Node) *Field {
	return &Field{
		Name: []byte(fieldName),
		Info: &FieldInfo{
			Name:                 fieldName,
			ExactParentTypeName:  parentTypeName,
			HasAuthorizationRule: true,
			Source: TypeFieldSource{
				IDs:   []string{dataSourceID},
				Names: []string{dataSourceName},
			},
		},
		Value: value,
	}
}
