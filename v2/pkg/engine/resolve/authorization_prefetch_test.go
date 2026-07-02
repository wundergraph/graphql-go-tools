package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type batchTestAuthorizer struct {
	preFetchCalls    atomic.Int64
	objectFieldCalls atomic.Int64
	batchCalls       atomic.Int64

	decisions map[GraphCoordinate]AuthorizationDecision
	seen      [][]GraphCoordinate
}

func (a *batchTestAuthorizer) AuthorizePreFetch(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	a.preFetchCalls.Add(1)
	return nil, nil
}

func (a *batchTestAuthorizer) AuthorizeObjectField(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	a.objectFieldCalls.Add(1)
	return nil, nil
}

func (a *batchTestAuthorizer) HasResponseExtensionData(ctx *Context) bool {
	return false
}

func (a *batchTestAuthorizer) RenderResponseExtension(ctx *Context, out io.Writer) error {
	return nil
}

func (a *batchTestAuthorizer) AuthorizeFields(ctx *Context, coordinates []GraphCoordinate) (decisions []AuthorizationDecision, err error) {
	a.batchCalls.Add(1)
	a.seen = append(a.seen, append([]GraphCoordinate(nil), coordinates...))
	decisions = make([]AuthorizationDecision, len(coordinates))
	for i := range coordinates {
		decision, ok := a.decisions[GraphCoordinate{
			TypeName:  coordinates[i].TypeName,
			FieldName: coordinates[i].FieldName,
		}]
		if ok {
			decisions[i] = decision
			continue
		}
		decisions[i] = AuthorizationDecision{Allowed: true}
	}
	return decisions, nil
}

func TestPreFetchFieldAuthorization(t *testing.T) {
	t.Run("enabled denied query root skips dedicated fetch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		response := singleFieldResponse(service, "account", &String{
			Path:     []string{"account"},
			Nullable: true,
		}, GraphCoordinate{
			TypeName:             "Query",
			FieldName:            "account",
			HasAuthorizationRule: true,
		})
		response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
			{DataSourceID: "accounts", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "account"}},
		}

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Query", FieldName: "account"}: {Allowed: false, Reason: "missing scope 'account:read'"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.account', Reason: missing scope 'account:read'.","path":["account"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"account":null}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("enabled no protected coordinates skips batch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"name":"Ada"}}`), nil).
			Times(1)

		response := singleFieldResponse(service, "name", &String{
			Path: []string{"name"},
		}, GraphCoordinate{
			TypeName:  "Query",
			FieldName: "name",
		})
		authorizer := &batchTestAuthorizer{}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"data":{"name":"Ada"}}`, buf.String())
		assert.Equal(t, int64(0), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("enabled denied query root sharing fetch keeps authorized sibling", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"public":"visible","secret":"hidden"}}`), nil).
			Times(1)

		response := sharedRootFieldResponse(service)
		response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
			{DataSourceID: "accounts", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "secret"}},
		}

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Query", FieldName: "secret"}: {Allowed: false, Reason: "missing scope 'secret:read'"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.secret', Reason: missing scope 'secret:read'.","path":["secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"public":"visible","secret":null}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("enabled nested protected field under empty list emits indexless path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"products":[]}}`), nil).
			Times(1)

		response := productsSecretResponse(service)
		response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
			{DataSourceID: "products", Coordinate: GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
		}

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Product", FieldName: "secret"}: {Allowed: false, Reason: "missing scope 'read:secret'"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing scope 'read:secret'.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"products":[]}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("enabled batch called once across fetch and array fan out", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		response := generateTestFederationGraphQLResponse(t, ctrl)
		CollectAuthorizationCoordinates(response)

		authorizer := &batchTestAuthorizer{}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
		assert.Equal(t, [][]GraphCoordinate{{
			{TypeName: "Product", FieldName: "name"},
			{TypeName: "Review", FieldName: "body"},
			{TypeName: "Review", FieldName: "product"},
			{TypeName: "User", FieldName: "reviews"},
			{TypeName: "Query", FieldName: "me"},
		}}, authorizer.seen)
	})

	t.Run("enabled denied interface field uses static parent coordinate with runtime typename", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"data":{"profile":{"__typename":"User","secret":"hidden"}}}`), nil).
			Times(1)

		response := interfaceSecretResponse(service)
		CollectAuthorizationCoordinates(response)

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Profile", FieldName: "secret"}: {Allowed: false, Reason: "missing scope 'profile:secret'"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Query.profile.secret', Reason: missing scope 'profile:secret'.","path":["profile","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"profile":{"secret":null}}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
		assert.Equal(t, [][]GraphCoordinate{{
			{TypeName: "Profile", FieldName: "secret"},
		}}, authorizer.seen)
	})

	t.Run("enabled denied non-null mutation root emits a single field error and nulls data", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		response := singleFieldResponse(service, "updateAccount", &String{
			Path:     []string{"updateAccount"},
			Nullable: false,
		}, GraphCoordinate{
			TypeName:             "Mutation",
			FieldName:            "updateAccount",
			HasAuthorizationRule: true,
		})
		response.Info.OperationType = ast.OperationTypeMutation
		response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
			{DataSourceID: "accounts", Coordinate: GraphCoordinate{TypeName: "Mutation", FieldName: "updateAccount"}},
		}
		response.Fetches.Item.Fetch.FetchInfo().OperationType = ast.OperationTypeMutation
		response.Data.Fields[0].Info.ExactParentTypeName = "Mutation"

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Mutation", FieldName: "updateAccount"}: {Allowed: false, Reason: "missing scope 'account:write'"},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		// A denied mutation skips the origin request and reports exactly one field-level error,
		// in the same shape as the query field errors — no extra subgraph-level error.
		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Mutation.updateAccount', Reason: missing scope 'account:write'.","path":["updateAccount"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":null}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

	t.Run("enabled denied nullable mutation root emits a single field error and nulls only the field", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service := NewMockDataSource(ctrl)
		service.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// A nullable mutation root field: per the GraphQL spec, a field error on a nullable field
		// nulls only that field (no data-root propagation). Nullability is respected identically to
		// query fields; mutations are not special-cased. This variant also covers the no-reason
		// error message format.
		response := singleFieldResponse(service, "updateAccount", &String{
			Path:     []string{"updateAccount"},
			Nullable: true,
		}, GraphCoordinate{
			TypeName:             "Mutation",
			FieldName:            "updateAccount",
			HasAuthorizationRule: true,
		})
		response.Info.OperationType = ast.OperationTypeMutation
		response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
			{DataSourceID: "accounts", Coordinate: GraphCoordinate{TypeName: "Mutation", FieldName: "updateAccount"}},
		}
		response.Fetches.Item.Fetch.FetchInfo().OperationType = ast.OperationTypeMutation
		response.Data.Fields[0].Info.ExactParentTypeName = "Mutation"

		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Mutation", FieldName: "updateAccount"}: {Allowed: false},
			},
		}
		resolveCtx := NewContext(context.Background())
		resolveCtx.SetPreFetchFieldAuthorizer(authorizer)

		var buf bytes.Buffer
		resolver := newResolver(context.Background())
		_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
		require.NoError(t, err)

		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Mutation.updateAccount'.","path":["updateAccount"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"updateAccount":null}}`, buf.String())
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
		assert.Equal(t, int64(0), authorizer.preFetchCalls.Load())
		assert.Equal(t, int64(0), authorizer.objectFieldCalls.Load())
	})

}

func TestCollectAuthorizationCoordinates(t *testing.T) {
	response := productsSecretResponse(nil)
	response.Info.AuthorizationCoordinates = nil
	response.Fetches = Sequence(
		SingleWithPath(&SingleFetch{
			Info: &FetchInfo{
				DataSourceID: "catalog",
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "products", HasAuthorizationRule: true},
					{TypeName: "Query", FieldName: "public"},
				},
			},
		}, "query"),
	)

	CollectAuthorizationCoordinates(response)

	assert.Equal(t, []AuthorizationCoordinate{
		{DataSourceID: "catalog", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
		{DataSourceID: "products", Coordinate: GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
	}, response.Info.AuthorizationCoordinates)
}

// In normal engine execution the post-processor builds the Fetches tree from RawFetches only after
// planning, so at collection time fetch root-field coordinates live in RawFetches, not the tree.
func TestCollectAuthorizationCoordinatesFromRawFetches(t *testing.T) {
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		RawFetches: []*FetchItem{
			{Fetch: &SingleFetch{
				Info: &FetchInfo{
					DataSourceID: "catalog",
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "products", HasAuthorizationRule: true},
						{TypeName: "Query", FieldName: "public"},
					},
				},
			}},
		},
	}

	CollectAuthorizationCoordinates(response)

	assert.Equal(t, []AuthorizationCoordinate{
		{DataSourceID: "catalog", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
	}, response.Info.AuthorizationCoordinates)
}

// When the loader is skipped (e.g. query-plan-only responses) no origin fetch runs, so the batch
// authorizer must not be invoked.
func TestPreFetchFieldAuthorizationSkipLoader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := NewMockDataSource(ctrl)
	service.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	response := singleFieldResponse(service, "account", &String{
		Path:     []string{"account"},
		Nullable: true,
	}, GraphCoordinate{
		TypeName:             "Query",
		FieldName:            "account",
		HasAuthorizationRule: true,
	})
	response.Info.AuthorizationCoordinates = []AuthorizationCoordinate{
		{DataSourceID: "accounts", Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "account"}},
	}

	authorizer := &batchTestAuthorizer{
		decisions: map[GraphCoordinate]AuthorizationDecision{
			{TypeName: "Query", FieldName: "account"}: {Allowed: false, Reason: "missing scope 'account:read'"},
		},
	}
	resolveCtx := NewContext(context.Background())
	resolveCtx.SetPreFetchFieldAuthorizer(authorizer)
	resolveCtx.ExecutionOptions.SkipLoader = true

	var buf bytes.Buffer
	resolver := newResolver(context.Background())
	_, err := resolver.ResolveGraphQLResponse(resolveCtx, response, nil, &buf)
	require.NoError(t, err)

	assert.Equal(t, `{"data":null}`, buf.String())
	assert.Equal(t, int64(0), authorizer.batchCalls.Load())
}

// A subscription's protected root field must be authorized before the trigger starts, so an
// unauthorized subscription never opens an upstream subscription.
func TestAuthorizeSubscriptionPreFetch(t *testing.T) {
	newSubResponse := func() *GraphQLResponse {
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeSubscription},
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("messageAdded"),
						Info: &FieldInfo{
							Name:                 "messageAdded",
							ExactParentTypeName:  "Subscription",
							Source:               TypeFieldSource{IDs: []string{"chat"}, Names: []string{"chat"}},
							HasAuthorizationRule: true,
						},
						Value: &String{Path: []string{"messageAdded"}, Nullable: true},
					},
				},
			},
		}
	}

	t.Run("denied root field returns error body", func(t *testing.T) {
		authorizer := &batchTestAuthorizer{
			decisions: map[GraphCoordinate]AuthorizationDecision{
				{TypeName: "Subscription", FieldName: "messageAdded"}: {Allowed: false, Reason: "missing scope 'chat:read'"},
			},
		}
		ctx := NewContext(context.Background())
		ctx.SetPreFetchFieldAuthorizer(authorizer)
		resolver := newResolver(context.Background())

		body, denied, err := resolver.authorizeSubscriptionPreFetch(ctx, newSubResponse())
		require.NoError(t, err)
		assert.True(t, denied)
		assert.Equal(t, `{"errors":[{"message":"Unauthorized to load field 'Subscription.messageAdded', Reason: missing scope 'chat:read'.","extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":null}`, string(body))
		assert.Equal(t, int64(1), authorizer.batchCalls.Load())
	})

	t.Run("allowed root field proceeds", func(t *testing.T) {
		authorizer := &batchTestAuthorizer{}
		ctx := NewContext(context.Background())
		ctx.SetPreFetchFieldAuthorizer(authorizer)
		resolver := newResolver(context.Background())

		body, denied, err := resolver.authorizeSubscriptionPreFetch(ctx, newSubResponse())
		require.NoError(t, err)
		assert.False(t, denied)
		assert.Nil(t, body)
	})

	t.Run("mode disabled is a no-op", func(t *testing.T) {
		ctx := NewContext(context.Background())
		resolver := newResolver(context.Background())

		body, denied, err := resolver.authorizeSubscriptionPreFetch(ctx, newSubResponse())
		require.NoError(t, err)
		assert.False(t, denied)
		assert.Nil(t, body)
	})

	t.Run("wrong decision count fails closed", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.SetPreFetchFieldAuthorizer(miscountBatchAuthorizer{})
		resolver := newResolver(context.Background())

		body, denied, err := resolver.authorizeSubscriptionPreFetch(ctx, newSubResponse())
		require.Error(t, err)
		assert.False(t, denied)
		assert.Nil(t, body)
	})
}

// miscountBatchAuthorizer returns the wrong number of decisions to exercise the fail-closed path.
type miscountBatchAuthorizer struct{}

func (miscountBatchAuthorizer) AuthorizeFields(_ *Context, _ []GraphCoordinate) ([]AuthorizationDecision, error) {
	return nil, nil
}

func singleFieldResponse(service DataSource, fieldName string, value Node, rootField GraphCoordinate) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
				DataSourceID:   "accounts",
				DataSourceName: "accounts",
				RootFields:     []GraphCoordinate{rootField},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte(fieldName),
					Info: &FieldInfo{
						Name:                fieldName,
						ExactParentTypeName: "Query",
						Source: TypeFieldSource{
							IDs:   []string{"accounts"},
							Names: []string{"accounts"},
						},
						HasAuthorizationRule: rootField.HasAuthorizationRule,
					},
					Value: value,
				},
			},
		},
	}
}

func sharedRootFieldResponse(service DataSource) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
				DataSourceID:   "accounts",
				DataSourceName: "accounts",
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "public"},
					{TypeName: "Query", FieldName: "secret", HasAuthorizationRule: true},
				},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("public"),
					Info: &FieldInfo{
						Name:                "public",
						ExactParentTypeName: "Query",
						Source: TypeFieldSource{
							IDs:   []string{"accounts"},
							Names: []string{"accounts"},
						},
					},
					Value: &String{Path: []string{"public"}},
				},
				{
					Name: []byte("secret"),
					Info: &FieldInfo{
						Name:                "secret",
						ExactParentTypeName: "Query",
						Source: TypeFieldSource{
							IDs:   []string{"accounts"},
							Names: []string{"accounts"},
						},
						HasAuthorizationRule: true,
					},
					Value: &String{
						Path:     []string{"secret"},
						Nullable: true,
					},
				},
			},
		},
	}
}

func productsSecretResponse(service DataSource) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
				DataSourceID:   "products",
				DataSourceName: "products",
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "products"},
				},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Info: &FieldInfo{
						Name:                "products",
						ExactParentTypeName: "Query",
						Source: TypeFieldSource{
							IDs:   []string{"products"},
							Names: []string{"products"},
						},
					},
					Value: &Array{
						Path:     []string{"products"},
						Nullable: true,
						Item: &Object{
							Nullable: true,
							TypeName: "Product",
							Fields: []*Field{
								{
									Name: []byte("secret"),
									Info: &FieldInfo{
										Name:                "secret",
										ExactParentTypeName: "Product",
										Source: TypeFieldSource{
											IDs:   []string{"products"},
											Names: []string{"products"},
										},
										HasAuthorizationRule: true,
									},
									Value: &String{
										Path:     []string{"secret"},
										Nullable: true,
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

func interfaceSecretResponse(service DataSource) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
				DataSourceID:   "profiles",
				DataSourceName: "profiles",
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "profile"},
				},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("profile"),
					Info: &FieldInfo{
						Name:                "profile",
						ExactParentTypeName: "Query",
						Source: TypeFieldSource{
							IDs:   []string{"profiles"},
							Names: []string{"profiles"},
						},
					},
					Value: &Object{
						Path:          []string{"profile"},
						Nullable:      true,
						TypeName:      "Profile",
						PossibleTypes: map[string]struct{}{"User": {}},
						Fields: []*Field{
							{
								Name: []byte("secret"),
								Info: &FieldInfo{
									Name:                "secret",
									ExactParentTypeName: "Profile",
									ParentTypeNames:     []string{"Profile", "User"},
									Source: TypeFieldSource{
										IDs:   []string{"profiles"},
										Names: []string{"profiles"},
									},
									HasAuthorizationRule: true,
								},
								Value: &String{
									Path:     []string{"secret"},
									Nullable: true,
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestPreFetchFieldAuthorizationContextFreeResetsAuthorizer(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.SetPreFetchFieldAuthorizer(&batchTestAuthorizer{})
	ctx.Free()

	assert.Nil(t, ctx.preFetchFieldAuthorizer)
}
