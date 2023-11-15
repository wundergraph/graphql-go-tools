package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astjson"
)

func TestV2Loader_LoadGraphQLResponseData(t *testing.T) {
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
		Data: &Object{
			Fetch: &SingleFetch{
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
			},
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fetch: &ParallelFetch{
								Fetches: []Fetch{
									&BatchEntityFetch{
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
									},
									&BatchEntityFetch{
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
									},
								},
							},
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
														Fetch: &BatchEntityFetch{
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
														},
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
	resolvable := &Resolvable{
		storage: &astjson.JSON{},
	}
	loader := &V2Loader{}
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)
	ctrl.Finish()
	out := &bytes.Buffer{}
	err = resolvable.storage.PrintNode(resolvable.storage.Nodes[resolvable.storage.RootNode], out)
	assert.NoError(t, err)
	expected := `{"errors":[],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}}`
	assert.Equal(t, expected, out.String())
}

func BenchmarkV2Loader_LoadGraphQLResponseData(b *testing.B) {

	productsService := FakeDataSource(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`)
	stockService := FakeDataSource(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	usersService := FakeDataSource(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`)

	response := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
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
			},
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fetch: &ParallelFetch{
								Fetches: []Fetch{
									&BatchEntityFetch{
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
									},
									&BatchEntityFetch{
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
									},
								},
							},
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
														Fetch: &BatchEntityFetch{
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
														},
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
	resolvable := &Resolvable{
		storage: &astjson.JSON{},
	}
	loader := &V2Loader{}
	expected := []byte(`{"errors":[],"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}}`)
	out := &bytes.Buffer{}
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
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
		err = resolvable.storage.PrintNode(resolvable.storage.Nodes[resolvable.storage.RootNode], out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

var (
	DefaultPostProcessingConfiguration = PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	EntitiesPostProcessingConfiguration = PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data", "_entities"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	SingleEntityPostProcessingConfiguration = PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data", "_entities", "[0]"},
		SelectResponseErrorsPath: []string{"errors"},
	}
)
