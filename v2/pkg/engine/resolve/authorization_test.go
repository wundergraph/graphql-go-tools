package resolve

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type testAuthorizer struct {
	calls     atomic.Int64
	authorize func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)
}

func (t *testAuthorizer) Authorize(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	t.calls.Add(1)
	return t.authorize(ctx, dataSourceID, coordinate)
}

func createTestAuthorizer(authorize func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)) Authorizer {
	return &testAuthorizer{authorize: authorize}
}

func TestAuthorization(t *testing.T) {
	t.Run("allow all", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow field without policy", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			if dataSourceID == "users" && coordinate.TypeName == "User" && coordinate.FieldName == "id" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch id on User",
				}, nil
			}
			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("no authorization rules/checks", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, nil
		})

		res := generateTestFederationGraphQLResponseWithoutAuthorizationRules(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(0), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow root fetch", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "users" && coordinate.TypeName == "Query" && coordinate.FieldName == "me" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from users Subgraph.",
				}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized request to Subgraph 'users' at path 'query'. Reason: Not allowed to fetch from users Subgraph."}],"data":null}`,
			func(t *testing.T) {
				assert.Equal(t, int64(1), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow child fetch", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "reviews" && coordinate.TypeName == "User" && coordinate.FieldName == "reviews" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from reviews Subgraph.",
				}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews'. Reason: Not allowed to fetch from reviews Subgraph.","path":["me","reviews"]}],"data":{"me":{"id":"1234","username":"Me","reviews":null}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(2), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow nested child fetch", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch from products Subgraph.",
				}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch from products Subgraph.","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch from products Subgraph.","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("error from authorizer should return", testFnWithError(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
			return nil, errors.New("some error")
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized request to Subgraph 'products' at path 'query.me.reviews.@.product'. Reason: Not allowed to fetch from products Subgraph."},{"message":"Cannot return null for non-nullable field 'Query.me.reviews.product.data.name'.","path":["me","reviews",0,"product","data","name"]},{"message":"Cannot return null for non-nullable field 'Query.me.reviews.product.data.name'.","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`
	}))
	t.Run("disallow nullable field", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch body on Review",
				}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.body'. Reason: Not allowed to fetch body on Review","path":["me","reviews",0,"body"]},{"message":"Unauthorized to load field 'Query.me.reviews.body'. Reason: Not allowed to fetch body on Review","path":["me","reviews",1,"body"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":null,"product":{"upc":"top-1","name":"Trilby"}},{"body":null,"product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow nullable field without a reason", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.body'.","path":["me","reviews",0,"body"]},{"message":"Unauthorized to load field 'Query.me.reviews.body'.","path":["me","reviews",1,"body"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":null,"product":{"upc":"top-1","name":"Trilby"}},{"body":null,"product":{"upc":"top-2","name":"Fedora"}}]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("disallow non-nullable field", testFnWithPostEvaluation(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string, postEvaluation func(t *testing.T)) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "products" && coordinate.TypeName == "Product" && coordinate.FieldName == "name" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch name on Product",
				}, nil
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch name on Product","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch name on Product","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`,
			func(t *testing.T) {
				assert.Equal(t, int64(5), authorizer.(*testAuthorizer).calls.Load())
			}
	}))
	t.Run("reject during the resolvable process", testFnWithError(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		authorizer := createTestAuthorizer(func(ctx *Context, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {

			if dataSourceID == "reviews" && coordinate.TypeName == "Review" && coordinate.FieldName == "body" {
				return &AuthorizationDeny{
					Reason: "Not allowed to fetch body on Review",
				}, errors.New("some error")
			}

			return nil, nil
		})

		res := generateTestFederationGraphQLResponse(t, ctrl)

		return res, Context{ctx: context.Background(), Variables: nil, authorizer: authorizer},
			`{"errors":[{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch name on Product","path":["me","reviews",0,"product","data","name"]},{"message":"Unauthorized to load field 'Query.me.reviews.product.data.name'. Reason: Not allowed to fetch name on Product","path":["me","reviews",1,"product","data","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`
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
						SelectResponseDataPath: []string{"data"},
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
