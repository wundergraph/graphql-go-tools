package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"sync/atomic"
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
	// newPreFetchResolvable enables pre-fetch mode on the context; subtests seed decisions directly.
	newPreFetchResolvable := func() *Resolvable {
		ctx := NewContext(context.Background())
		ctx.SetPreFetchFieldAuthorizer(&batchTestAuthorizer{})
		return newResolvableForAuthorizationTest(ctx)
	}

	// walkUnreached runs the pre-render walk with the synthetic authorization descent armed,
	// exactly as Resolve does for the initial walk in pre-fetch mode.
	walkUnreached := func(t *testing.T, resolvable *Resolvable, root *Object, data *astjson.Value) {
		t.Helper()
		resolvable.unreachedAuthWalk = true
		resolvable.walkObject(root, data)
		resolvable.unreachedAuthWalk = false
	}

	t.Run("empty list emits nested denied field at the list wildcard", func(t *testing.T) {
		resolvable := newPreFetchResolvable()
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		root := &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("secret"),
									Info: &FieldInfo{
										Name:                 "secret",
										ExactParentTypeName:  "Product",
										HasAuthorizationRule: true,
										Source: TypeFieldSource{
											IDs:   []string{"products"},
											Names: []string{"products"},
										},
									},
									Value: &String{Path: []string{"secret"}},
								},
							},
						},
					},
				},
			},
		}
		data := mustParseAuthorizationValue(t, `{"products":[]}`)

		walkUnreached(t, resolvable, root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","@","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("null nullable parent emits denied nested field", func(t *testing.T) {
		resolvable := newPreFetchResolvable()
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Account", FieldName: "secret"}, "missing account scope")
		root := &Object{
			Fields: []*Field{
				{
					Name: []byte("account"),
					Value: &Object{
						Path:     []string{"account"},
						Nullable: true,
						Fields: []*Field{
							{
								Name: []byte("secret"),
								Info: &FieldInfo{
									Name:                 "secret",
									ExactParentTypeName:  "Account",
									HasAuthorizationRule: true,
									Source: TypeFieldSource{
										IDs:   []string{"accounts"},
										Names: []string{"Accounts"},
									},
								},
								Value: &String{Path: []string{"secret"}, Nullable: true},
							},
						},
					},
				},
			},
		}
		data := mustParseAuthorizationValue(t, `{"account":null}`)

		walkUnreached(t, resolvable, root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret', Reason: missing account scope.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("reached child is denied by the walk itself, not the synthetic descent", func(t *testing.T) {
		resolvable := newPreFetchResolvable()
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Account", FieldName: "secret"}, "missing account scope")
		root := &Object{
			Fields: []*Field{
				{
					Name: []byte("account"),
					Value: &Object{
						Path:     []string{"account"},
						Nullable: true,
						Fields: []*Field{
							{
								Name: []byte("secret"),
								Info: &FieldInfo{
									Name:                 "secret",
									ExactParentTypeName:  "Account",
									HasAuthorizationRule: true,
									Source: TypeFieldSource{
										IDs:   []string{"accounts"},
										Names: []string{"Accounts"},
									},
								},
								Value: &String{Path: []string{"secret"}, Nullable: true},
							},
						},
					},
				},
			},
		}
		data := mustParseAuthorizationValue(t, `{"account":{"secret":"hidden"}}`)

		walkUnreached(t, resolvable, root, data)

		// exactly one error: the data walk owns reached fields, the synthetic descent stays out
		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.secret', Reason: missing account scope.","path":["account","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("denied frontier field stops the synthetic descent into its subtree", func(t *testing.T) {
		resolvable := newPreFetchResolvable()
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Account", FieldName: "vault"}, "missing vault scope")
		resolvable.authorization.seedDeny("accounts", GraphCoordinate{TypeName: "Vault", FieldName: "secret"}, "missing secret scope")
		root := &Object{
			Fields: []*Field{
				{
					Name: []byte("account"),
					Value: &Object{
						Path:     []string{"account"},
						Nullable: true,
						Fields: []*Field{
							{
								Name: []byte("vault"),
								Info: &FieldInfo{
									Name:                 "vault",
									ExactParentTypeName:  "Account",
									HasAuthorizationRule: true,
									Source: TypeFieldSource{
										IDs:   []string{"accounts"},
										Names: []string{"Accounts"},
									},
								},
								Value: &Object{
									Path:     []string{"vault"},
									Nullable: true,
									Fields: []*Field{
										{
											Name: []byte("secret"),
											Info: &FieldInfo{
												Name:                 "secret",
												ExactParentTypeName:  "Vault",
												HasAuthorizationRule: true,
												Source: TypeFieldSource{
													IDs:   []string{"accounts"},
													Names: []string{"Accounts"},
												},
											},
											Value: &String{Path: []string{"secret"}, Nullable: true},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		data := mustParseAuthorizationValue(t, `{"account":null}`)

		walkUnreached(t, resolvable, root, data)

		// the denied vault covers its subtree: no Vault.secret error
		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.account.vault', Reason: missing vault scope.","path":["account","vault"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})

	t.Run("nested empty arrays recurse with a wildcard per list level", func(t *testing.T) {
		resolvable := newPreFetchResolvable()
		resolvable.authorization.seedDeny("products", GraphCoordinate{TypeName: "Product", FieldName: "secret"}, "missing product scope")
		root := &Object{
			Fields: []*Field{
				{
					Name: []byte("edges"),
					Value: &Array{
						Path:     []string{"edges"},
						Nullable: true,
						Item: &Array{
							Path:     []string{"nodes"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("secret"),
										Info: &FieldInfo{
											Name:                 "secret",
											ExactParentTypeName:  "Product",
											HasAuthorizationRule: true,
											Source: TypeFieldSource{
												IDs:   []string{"products"},
												Names: []string{"products"},
											},
										},
										Value: &String{Path: []string{"secret"}, Nullable: true},
									},
								},
							},
						},
					},
				},
			},
		}
		data := mustParseAuthorizationValue(t, `{"edges":[]}`)

		walkUnreached(t, resolvable, root, data)

		assert.Equal(t, `[{"message":"Unauthorized to load field 'Query.edges.nodes.secret', Reason: missing product scope.","path":["edges","@","nodes","@","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]`, resolvable.errors.String())
	})
}

type countingAuthorizationDataSource struct {
	loads *atomic.Int64
	data  []byte
}

func (d countingAuthorizationDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	d.loads.Add(1)
	return d.data, nil
}

func (d countingAuthorizationDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	d.loads.Add(1)
	return d.data, nil
}

// deepOrdersResponse models: query { orders { total items { product { secret pricing { internal } } } } }
// with every field served by the "shop" data source. Protected coordinates: Order.total,
// Product.secret, Pricing.internal — plus Query.orders itself when rootProtected is true.
func deepOrdersResponse(service DataSource, rootProtected bool) *GraphQLResponse {
	ordersInfo := &FieldInfo{
		Name:                "orders",
		ExactParentTypeName: "Query",
		Source:              TypeFieldSource{IDs: []string{"shop"}, Names: []string{"shop"}},
	}
	coordinates := []AuthorizationCoordinate{
		{DataSourceID: "shop", Coordinate: GraphCoordinate{TypeName: "Order", FieldName: "total"}},
		{DataSourceID: "shop", Coordinate: GraphCoordinate{TypeName: "Pricing", FieldName: "internal"}},
		{DataSourceID: "shop", Coordinate: GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
	}
	rootField := GraphCoordinate{TypeName: "Query", FieldName: "orders"}
	if rootProtected {
		ordersInfo.HasAuthorizationRule = true
		rootField.HasAuthorizationRule = true
		coordinates = append(coordinates, AuthorizationCoordinate{DataSourceID: "shop", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "orders"}})
	}
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType:            ast.OperationTypeQuery,
			AuthorizationCoordinates: coordinates,
		},
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: service,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{SegmentType: StaticSegmentType, Data: []byte(`{}`)},
				},
			},
			Info: &FetchInfo{
				DataSourceID:   "shop",
				DataSourceName: "shop",
				RootFields:     []GraphCoordinate{rootField},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("orders"),
					Info: ordersInfo,
					Value: &Array{
						Path:     []string{"orders"},
						Nullable: true,
						Item: &Object{
							Nullable: true,
							TypeName: "Order",
							Fields: []*Field{
								{
									Name: []byte("total"),
									Info: &FieldInfo{
										Name:                 "total",
										ExactParentTypeName:  "Order",
										HasAuthorizationRule: true,
										Source: TypeFieldSource{
											IDs:   []string{"shop"},
											Names: []string{"shop"},
										},
									},
									Value: &String{Path: []string{"total"}},
								},
								{
									Name: []byte("items"),
									Value: &Array{
										Path: []string{"items"},
										Item: &Object{
											TypeName: "Item",
											Fields: []*Field{
												{
													Name: []byte("product"),
													Value: &Object{
														Path:     []string{"product"},
														TypeName: "Product",
														Fields: []*Field{
															{
																Name: []byte("secret"),
																Info: &FieldInfo{
																	Name:                 "secret",
																	ExactParentTypeName:  "Product",
																	HasAuthorizationRule: true,
																	Source: TypeFieldSource{
																		IDs:   []string{"shop"},
																		Names: []string{"shop"},
																	},
																},
																Value: &String{Path: []string{"secret"}, Nullable: true},
															},
															{
																Name: []byte("pricing"),
																Value: &Object{
																	Path:     []string{"pricing"},
																	Nullable: true,
																	TypeName: "Pricing",
																	Fields: []*Field{
																		{
																			Name: []byte("internal"),
																			Info: &FieldInfo{
																				Name:                 "internal",
																				ExactParentTypeName:  "Pricing",
																				HasAuthorizationRule: true,
																				Source: TypeFieldSource{
																					IDs:   []string{"shop"},
																					Names: []string{"shop"},
																				},
																			},
																			Value: &String{Path: []string{"internal"}, Nullable: true},
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
			},
		},
	}
}

func TestResolvableAuthorizationEndToEnd(t *testing.T) {
	t.Run("empty list emits nested denied field", func(t *testing.T) {
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

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing product scope.","path":["products","@","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"products":[]}}`, buf.String())
	})

	// The two subtests below exercise the interplay between the synthetic (unreached-data)
	// descent and the auth walk on one deep response tree (orders -> items -> product -> pricing).

	t.Run("fetch runs, mixed reached and unreached branches", func(t *testing.T) {
		// orders[0] is reached: the walk denies+nulls secret; its null pricing hides
		// Pricing.internal, reported by the synthetic descent. orders[1].items is empty: both
		// denied fields are reported there at the "@" wildcard. Order.total is allowed: no error.
		loads := &atomic.Int64{}
		service := countingAuthorizationDataSource{
			loads: loads,
			data:  []byte(`{"data":{"orders":[{"total":"a","items":[{"product":{"secret":"classified","pricing":null}}]},{"total":"b","items":[]}]}}`),
		}
		response := deepOrdersResponse(service, false)
		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Product", FieldName: "secret"}:   {Allowed: false, Reason: "missing product scope"},
				{TypeName: "Pricing", FieldName: "internal"}: {Allowed: false, Reason: "missing pricing scope"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.orders.items.product.secret', Reason: missing product scope.","path":["orders",0,"items",0,"product","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Unauthorized to load field 'Query.orders.items.product.pricing.internal', Reason: missing pricing scope.","path":["orders",0,"items",0,"product","pricing","internal"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Unauthorized to load field 'Query.orders.items.product.secret', Reason: missing product scope.","path":["orders",1,"items","@","product","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Unauthorized to load field 'Query.orders.items.product.pricing.internal', Reason: missing pricing scope.","path":["orders",1,"items","@","product","pricing","internal"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"orders":[{"total":"a","items":[{"product":{"secret":null,"pricing":null}}]},{"total":"b","items":[]}]}}`, buf.String())
		assert.Equal(t, int64(1), loads.Load())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("denied root prevents fetch, nested denials still reported", func(t *testing.T) {
		// Query.orders is the fetch's only root field and is denied, so the fetch is skipped.
		// The walk still reaches orders itself (the root object always has data), emits the deny
		// and nulls it; its error covers the subtree, so Product.secret is not reported.
		loads := &atomic.Int64{}
		service := countingAuthorizationDataSource{
			loads: loads,
			data:  []byte(`{"data":{"orders":[{"total":"leak","items":[{"product":{"secret":"leak","pricing":{"internal":"leak"}}}]}]}}`),
		}
		response := deepOrdersResponse(service, true)
		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Query", FieldName: "orders"}:   {Allowed: false, Reason: "missing orders scope"},
				{TypeName: "Product", FieldName: "secret"}: {Allowed: false, Reason: "missing product scope"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.orders', Reason: missing orders scope.","path":["orders"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"orders":null}}`, buf.String())
		assert.Equal(t, int64(0), loads.Load())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})
}

func TestResolvableAuthorizationHelpers(t *testing.T) {
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
