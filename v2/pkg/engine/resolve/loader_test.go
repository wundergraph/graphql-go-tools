package resolve

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

func TestLoader_LoadGraphQLResponseData(t *testing.T) {
	ctrl := gomock.NewController(t)
	productsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`,
		`{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}`)

	reviewsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
		`{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}`)

	stockService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
		`{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`)

	usersService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`,
		`{"_entities":[{"name":"user-1"},{"name":"user-2"}]}`)
	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			Parallel(
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: reviewsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: stockService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
			),
			Single(&BatchEntityFetch{
				Input: BatchInput{
					Header: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Items: []InputTemplate{
						{
							Segments: []TemplateSegment{
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
							},
						},
					},
					Separator: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`,`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Footer: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
				},
				DataSource: usersService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data", "_entities"},
				},
			}, ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
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
	ctx := &Context{
		ctx: context.Background(),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)
	ctrl.Finish()
	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.NoError(t, err)
	expected := `{"errors":[],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}}`
	assert.Equal(t, expected, out)
}

func TestLoader_MergeErrorDifferingTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	names := mockedDS(t, ctrl,
		`{}`,
		`{"data":{"users":[{"name":"user-1"},{"name":"user-2"}]}}`)

	secondNames := mockedDS(t, ctrl,
		`{}`,
		`{"data":{"users":[{"name":"user-3"},{"name":123}]}}`)

	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: names,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceName: "names",
				},
			}),
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: secondNames,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceName: "secondNames",
				},
			}),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path: []string{"users"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	ctx := &Context{
		ctx: context.Background(),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.Error(t, err)
	assert.Equal(t, "unable to merge results from subgraph secondNames: differing types", err.Error())
}

func TestLoader_MergeErrorDifferingArrayLength(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	names := mockedDS(t, ctrl,
		`{}`,
		`{"data":{"users":[{"name":"user-1"},{"name":"user-2"}]}}`)

	ages := mockedDS(t, ctrl,
		`{}`,
		`{"data":{"users":[{"age":30},{"age":40},{"age":50}]}}`)

	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: names,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceName: "names",
				},
			}),
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: ages,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceName: "ages",
				},
			}),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path: []string{"users"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("age"),
									Value: &Integer{
										Path: []string{"age"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	ctx := &Context{
		ctx: context.Background(),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.Error(t, err)
	assert.Equal(t, "unable to merge results from subgraph ages: differing array lengths", err.Error())
}

func TestLoader_LoadGraphQLResponseDataWithExtensions(t *testing.T) {
	ctrl := gomock.NewController(t)
	productsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}","extensions":{"foo":"bar"}}}`,
		`{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}`)

	reviewsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]},"extensions":{"foo":"bar"}}}`,
		`{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}`)

	stockService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]},"extensions":{"foo":"bar"}}}`,
		`{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`)

	usersService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]},"extensions":{"foo":"bar"}}}`,
		`{"_entities":[{"name":"user-1"},{"name":"user-2"}]}`)
	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			Parallel(
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: reviewsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: stockService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
			),
			Single(&BatchEntityFetch{
				Input: BatchInput{
					Header: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Items: []InputTemplate{
						{
							Segments: []TemplateSegment{
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
							},
						},
					},
					Separator: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`,`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Footer: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
				},
				DataSource: usersService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data", "_entities"},
				},
			}, ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
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
	ctx := &Context{
		ctx:        context.Background(),
		Extensions: []byte(`{"foo":"bar"}`),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)
	ctrl.Finish()
	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.NoError(t, err)
	expected := `{"errors":[],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}}`
	assert.Equal(t, expected, out)
}

func BenchmarkLoader_LoadGraphQLResponseData(b *testing.B) {

	productsService := FakeDataSource(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`)
	stockService := FakeDataSource(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	usersService := FakeDataSource(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`)

	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			Parallel(
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: reviewsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: stockService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
			),
			Single(&BatchEntityFetch{
				Input: BatchInput{
					Header: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Items: []InputTemplate{
						{
							Segments: []TemplateSegment{
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
							},
						},
					},
					Separator: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`,`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Footer: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
				},
				DataSource: usersService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data", "_entities"},
				},
			}, ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
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
	ctx := &Context{
		ctx: context.Background(),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	expected := `{"errors":[],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}}`
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader.Free()
		resolvable.Reset()
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		if err != nil {
			b.Fatal(err)
		}
		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		if err != nil {
			b.Fatal(err)
		}
		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		if expected != out {
			b.Fatalf("expected %s, got %s", expected, out)
		}
	}
}

func TestLoader_RedactHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	productsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://products","header":{"Authorization":"value"},"body":{"query":"query{topProducts{name __typename upc}}"},"__trace__":true}`,
		`{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}`)

	response := &GraphQLResponse{
		Fetches: Single(&SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://products","header":{"Authorization":"`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       HeaderVariableKind,
						VariableSourcePath: []string{"Authorization"},
					},
					{
						Data:        []byte(`"},"body":{"query":"query{topProducts{name __typename upc}}"},"__trace__":true}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: productsService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
		}),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
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
					},
				},
			},
		},
	}

	ctx := &Context{
		ctx: context.Background(),
		Request: Request{
			Header: http.Header{"Authorization": []string{"value"}},
		},
		TracingOptions: TraceOptions{
			Enable: true,
		},
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)

	var input struct {
		Header map[string][]string
	}

	fetch := response.Fetches.Item.Fetch
	switch f := fetch.(type) {
	case *SingleFetch:
		_ = json.Unmarshal(f.Trace.Input, &input)
		authHeader := input.Header["Authorization"]
		assert.Equal(t, []string{"****"}, authHeader)
	default:
		t.Errorf("Incorrect fetch type")
	}
}

func TestLoader_InvalidBatchItemCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	productsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`,
		`{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}`)

	reviewsService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
		`{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}`)

	stockService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
		`{"_entities":[{"stock":8},{"stock":2}]}`) // 3 items expected, 2 returned

	usersService := mockedDS(t, ctrl,
		`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`,
		`{"_entities":[{"name":"user-1"},{"name":"user-2"},{"name":"user-3"}]}`) // 2 items expected, 3 returned
	response := &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			Parallel(
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: reviewsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
				Single(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Items: []InputTemplate{
							{
								Segments: []TemplateSegment{
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
													Name: []byte("upc"),
													Value: &String{
														Path: []string{"upc"},
													},
												},
											},
										}),
									},
								},
							},
						},
						Separator: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`,`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						Footer: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					},
					DataSource: stockService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, ArrayPath("topProducts")),
			),
			Single(&BatchEntityFetch{
				Input: BatchInput{
					Header: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Items: []InputTemplate{
						{
							Segments: []TemplateSegment{
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
							},
						},
					},
					Separator: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`,`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					Footer: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
				},
				DataSource: usersService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data", "_entities"},
				},
			}, ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
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
	ctx := &Context{
		ctx: context.Background(),
	}
	resolvable := NewResolvable(ResolvableOptions{})
	loader := &Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)
	ctrl.Finish()
	out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
	assert.NoError(t, err)
	// 2 errors expected in the response.
	expected := `{"errors":[{"message":"Failed to fetch from Subgraph, Reason: returned entities count does not match the count of representation variables in the entities request. Expected 3, got 2."},{"message":"Failed to fetch from Subgraph, Reason: returned entities count does not match the count of representation variables in the entities request. Expected 2, got 3."}],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`
	assert.Equal(t, expected, out)
}

func TestRewriteErrorPaths(t *testing.T) {
	mp := astjson.MustParse
	testCases := []struct {
		name           string
		fetchPath      []string
		inputErrors    []*astjson.Value
		expectedErrors []*astjson.Value
		description    string
	}{
		{
			name:      "rewrite _entities path with simple field",
			fetchPath: []string{"products"},
			inputErrors: []*astjson.Value{
				mp(`{"message": "simple", "path": ["_entities", 0, "name"]}`),
			},
			expectedErrors: []*astjson.Value{
				mp(`{"message": "simple", "path": ["products", "name"]}`),
			},
		},
		{
			name:      "rewrite _entities path with nested field",
			fetchPath: []string{"user", "profile"},
			inputErrors: []*astjson.Value{
				mp(`{"message": "nested", "path": ["_entities", 0, "address", "street"]}`),
				mp(`{"message": "index", "path": ["_entities", 0, "reviews", 1, "body"]}`),
			},
			expectedErrors: []*astjson.Value{
				mp(`{"message": "nested", "path": ["user", "profile", "address", "street"]}`),
				mp(`{"message": "index", "path": ["user", "profile", "reviews", "1", "body"]}`),
			},
		},
		{
			name:      "handle null, empty, no-entities, etc",
			fetchPath: []string{"products"},
			inputErrors: []*astjson.Value{
				mp(`{"message": "without path", "path": null}`),
				mp(`{"message": "with empty path", "path": []}`),
				mp(`{"message": "non-entities", "path": ["query", "products", "name"]}`),
				mp(`{"message": "with boolean", "path": ["_entities", 0, "field", true, "subfield"]}`),
				mp(`{"message": "_entities is last", "path": ["a", "_entities"]}`),
				mp(`{"message": "index is last", "path": ["a", "_entities", 2]}`),
			},
			expectedErrors: []*astjson.Value{
				mp(`{"message": "without path", "path": null}`),
				mp(`{"message": "with empty path", "path": []}`),
				mp(`{"message": "non-entities", "path": ["query", "products", "name"]}`),
				mp(`{"message": "with boolean", "path": ["products", "field", "subfield"]}`),
				mp(`{"message": "_entities is last", "path": ["products"]}`),
				mp(`{"message": "index is last", "path": ["products"]}`),
			},
		},
		{
			name:      "handle path with trailing @ in response path elements",
			fetchPath: []string{"products", "@"},
			inputErrors: []*astjson.Value{
				mp(`{"message": "@ at end", "path": ["_entities", 0, "name"]}`),
			},
			expectedErrors: []*astjson.Value{
				mp(`{"message": "@ at end", "path": ["products", "name"]}`),
			},
		},
		{
			name:      "handle path with non-trailing @ in response path elements",
			fetchPath: []string{"products", "@", "lead"},
			inputErrors: []*astjson.Value{
				mp(`{"message": "middle @", "path": ["_entities", 0, "name"]}`),
			},
			expectedErrors: []*astjson.Value{
				mp(`{"message": "middle @", "path": ["products", "@", "lead", "name"]}`),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Create FetchItem with the test response path elements
			fetchItem := &FetchItem{
				ResponsePathElements: tc.fetchPath,
			}

			// Make copies of input errors to avoid modifying the originals
			values := make([]*astjson.Value, len(tc.inputErrors))
			for i, inputError := range tc.inputErrors {
				// Create a copy by marshaling and parsing again
				data := inputError.MarshalTo(nil)
				value, err := astjson.ParseBytesWithoutCache(data)
				assert.NoError(t, err, "Failed to copy input error")
				values[i] = value
			}

			// Call the function under test
			rewriteErrorPaths(fetchItem, values)

			// Compare the results
			assert.Equal(t, len(tc.expectedErrors), len(values),
				"Number of errors should match")
			for i, expectedError := range tc.expectedErrors {
				expectedData := expectedError.MarshalTo(nil)
				actualData := values[i].MarshalTo(nil)
				assert.JSONEq(t, string(expectedData), string(actualData),
					"Error %d should match expected", i)
			}
		})
	}
}
