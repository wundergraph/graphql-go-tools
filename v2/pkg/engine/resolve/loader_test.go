package resolve

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestExtractEntityIndex(t *testing.T) {
	tests := []struct {
		name           string
		responseJSON   string
		pathElements   []interface{} // Can be strings or numbers
		expectedEntity string        // JSON string of expected entity, or "nil" for nil
		expectedIndex  int
	}{
		{
			name:           "complex federation-like structure",
			responseJSON:   `[{"__typename": "User", "id": "1", "name": "John"}, {"__typename": "User", "id": "2", "name": null}]`,
			pathElements:   []interface{}{1},
			expectedEntity: `{"__typename": "User", "id": "2", "name": null}`,
			expectedIndex:  1,
		},
		{
			name:           "mixed path with number then string",
			responseJSON:   `[{"user": {"name": "John"}}, {"user": {"name": "Jane"}}]`,
			pathElements:   []interface{}{1, "user"},
			expectedEntity: `{"name": "Jane"}`,
			expectedIndex:  1,
		},
		{
			name:           "multiple numbers in path",
			responseJSON:   `[[{"name": "A"}, {"name": "B"}], [{"name": "C"}, {"name": "D"}]]`,
			pathElements:   []interface{}{1, 0},
			expectedEntity: `{"name": "C"}`,
			expectedIndex:  1,
		},
		{
			name:           "path leads to non-existent key",
			responseJSON:   `[{"user": {"name": "John"}}]`,
			pathElements:   []interface{}{0, "user", "nonexistent"},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "negative index is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{-2},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "out of bound index is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{9},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "empty path is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := astjson.ParseBytesWithoutCache([]byte(tt.responseJSON))
			assert.NoError(t, err, "Failed to parse response JSON")

			// Convert path elements to astjson.Value slice
			path := make([]*astjson.Value, len(tt.pathElements))
			for i, elem := range tt.pathElements {
				switch v := elem.(type) {
				case string:
					path[i] = astjson.MustParse(`"` + v + `"`)
				case int:
					path[i] = astjson.MustParse(fmt.Sprintf("%d", v))
				default:
					t.Fatalf("Unsupported path element type: %T", v)
				}
			}

			entity, index := extractEntityIndex(response, path)

			assert.Equal(t, tt.expectedIndex, index, "Index mismatch")

			if tt.expectedEntity == "nil" {
				assert.Nil(t, entity, "Expected nil entity")
			} else {
				assert.NotNil(t, entity, "Expected non-nil entity")
				expectedEntity, err := astjson.ParseBytesWithoutCache([]byte(tt.expectedEntity))
				assert.NoError(t, err, "Failed to parse expected entity JSON")

				// Compare JSON representations
				actualJSON := entity.MarshalTo(nil)
				expectedJSON := expectedEntity.MarshalTo(nil)
				assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Entity content mismatch")
			}
		})
	}
}

func TestGetTaintedIndices(t *testing.T) {
	tests := []struct {
		name            string
		fetchReasons    []FetchReason
		responseJSON    string
		errorsJSON      string
		expectedIndices []int
	}{
		{
			name: "single entity with requires dependency failure",
			fetchReasons: []FetchReason{
				{TypeName: "User", FieldName: "email", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "User", "id": "1", "email": null},
				{"__typename": "User", "id": "2", "email": "user2@example.com"}
			]`,
			errorsJSON: `[
				{
					"message": "Cannot resolve field email",
					"path": ["_entities", 0, "email"]
				}
			]`,
			expectedIndices: []int{0},
		},
		{
			name: "multiple entities with requires dependency failures",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "rating", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": null, "rating": 4.5},
				{"__typename": "Product", "upc": "2", "reviews": [], "rating": null},
				{"__typename": "Product", "upc": "3", "reviews": [], "rating": 3.8}
			]`,
			errorsJSON: `[
				{
					"message": "Cannot resolve field reviews",
					"path": ["_entities", 0, "reviews"]
				},
				{
					"message": "Cannot resolve field rating", 
					"path": ["_entities", 1, "rating"]
				}
			]`,
			expectedIndices: []int{1, 0},
		},
		{
			name: "error in non-required field should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "description", IsKey: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": [], "description": null}
			]`,
			errorsJSON: `[
				{
					"message": "Description not available",
					"path": ["_entities", 0, "description"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "error in non-nullable field should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "description", IsRequires: true, Nullable: false},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": [], "description": null}
			]`,
			errorsJSON: `[
				{
					"message": "Description not available",
					"path": ["_entities", 0, "description"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "error path without _entities should be ignored",
			fetchReasons: []FetchReason{
				{TypeName: "User", FieldName: "email", IsRequires: true, Nullable: true},
			},
			responseJSON: `{
				"users": [
					{"__typename": "User", "id": "1", "email": null}
				]
			}`,
			errorsJSON: `[
				{
					"message": "Email service down",
					"path": ["users", 0, "email"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "entity field is not null - should not be tainted",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": []}
			]`,
			errorsJSON: `[
				{
					"message": "Some error occurred",
					"path": ["_entities", 0, "reviews"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "missing __typename should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"upc": "1", "reviews": null}
			]`,
			errorsJSON: `[
				{
					"message": "Reviews failed",
					"path": ["_entities", 0, "reviews"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "deeply nested entity path",
			fetchReasons: []FetchReason{
				{TypeName: "Review", FieldName: "sentiment", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{
					"__typename": "Product",
					"reviews": [
						{"__typename": "Review", "id": "1", "sentiment": "cool"}
					]
				},
				{
					"__typename": "Product",
					"reviews": [
						{"__typename": "Review", "id": "2", "sentiment": null}
					]
				}
			]`,
			errorsJSON: `[
				{
					"message": "Sentiment analysis failed",
					"path": ["_entities", 1, "reviews", 0, "sentiment"]
				}
			]`,
			expectedIndices: []int{1},
		},
		{
			name: "error path too short",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "name", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "name": null}
			]`,
			errorsJSON: `[
				{
					"message": "General error",
					"path": ["_entities", 0]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "invalid error path format",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": null}
			]`,
			errorsJSON: `[
				{
					"message": "Invalid path",
					"path": "not_an_array"
				}
			]`,
			expectedIndices: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock fetch with FetchInfo
			loader := &Loader{}
			fetchInfo := &FetchInfo{
				FetchReasons: tt.fetchReasons,
			}
			mockFetch := &mockFetchWithInfo{info: fetchInfo}

			response, err := astjson.ParseBytesWithoutCache([]byte(tt.responseJSON))
			assert.NoError(t, err, "Failed to parse response JSON")

			errors, err := astjson.ParseBytesWithoutCache([]byte(tt.errorsJSON))
			assert.NoError(t, err, "Failed to parse errors JSON")

			indices := loader.getTaintedIndicesAndCleanErrors(mockFetch, response, errors)

			assert.Equal(t, tt.expectedIndices, indices, "Tainted indices mismatch: %s")
		})
	}
}

// Mock fetch implementation for testing
type mockFetchWithInfo struct {
	info *FetchInfo
}

func (m *mockFetchWithInfo) FetchInfo() *FetchInfo {
	return m.info
}

func (m *mockFetchWithInfo) FetchKind() FetchKind {
	return FetchKindSingle
}

func (m *mockFetchWithInfo) Dependencies() *FetchDependencies {
	return nil
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
