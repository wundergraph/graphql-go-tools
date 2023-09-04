package postprocess

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestDataSourceInput_Process(t *testing.T) {
	pre := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					Input:      `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username __typename}}"}}`,
					DataSource: nil,
					Variables: []resolve.Variable{
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								Input: `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[$$0$$]}}}`,
								Variables: resolve.NewVariables(
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path:     []string{"__typename"},
														Nullable: false,
													},
												},
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: false,
													},
												},
											},
										}),
									},
								),
								DataSource: nil,
								PostProcessing: resolve.PostProcessingConfiguration{
									SelectResponseDataPath:   []string{"data", "_entities"},
									SelectResponseErrorsPath: []string{"errors"},
								},
								SetTemplateOutputToNullOnVariableNull: true,
							},
							Path:     []string{"me"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("username"),
									Value: &resolve.String{
										Path: []string{"username"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &resolve.Array{
										Path:     []string{"reviews"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("body"),
													Value: &resolve.String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("product"),
													Value: &resolve.Object{
														Path: []string{"product"},
														Fetch: &resolve.SingleFetch{
															Input:      `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource: nil,
															Variables: resolve.NewVariables(
																&resolve.ResolvableObjectVariable{
																	Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																		Nullable: false,
																		Fields: []*resolve.Field{
																			{
																				Name: []byte("__typename"),
																				Value: &resolve.String{
																					Path:     []string{"__typename"},
																					Nullable: false,
																				},
																			},
																			{
																				Name: []byte("upc"),
																				Value: &resolve.String{
																					Path:     []string{"upc"},
																					Nullable: false,
																				},
																			},
																		},
																	}),
																},
															),
															PostProcessing: resolve.PostProcessingConfiguration{
																SelectResponseDataPath:   []string{"data", "_entities"},
																SelectResponseErrorsPath: []string{"errors"},
															},
															SetTemplateOutputToNullOnVariableNull: true,
														},
														Fields: []*resolve.Field{
															{
																Name: []byte("name"),
																Value: &resolve.String{
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

	expected := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					InputTemplate: resolve.InputTemplate{
						Segments: []resolve.TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001/`),
								SegmentType: resolve.StaticSegmentType,
							},
							{
								SegmentType:        resolve.VariableSegmentType,
								VariableKind:       resolve.HeaderVariableKind,
								VariableSourcePath: []string{"Authorization"},
							},
							{
								Data:        []byte(`","body":{"query":"{me {id username __typename}}"}}`),
								SegmentType: resolve.StaticSegmentType,
							},
						},
					},
					DataSource: nil,
					PostProcessing: resolve.PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
											SegmentType: resolve.StaticSegmentType,
										},

										{
											SegmentType:  resolve.VariableSegmentType,
											VariableKind: resolve.ResolvableObjectVariableKind,
											Renderer: &resolve.GraphQLVariableResolveRenderer{
												Kind: resolve.VariableRendererKindGraphqlResolve,
												Node: &resolve.Object{
													Nullable: false,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path:     []string{"__typename"},
																Nullable: false,
															},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path:     []string{"id"},
																Nullable: false,
															},
														},
													},
												},
											},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`]}}}`),
										},
									},
									SetTemplateOutputToNullOnVariableNull: true,
								},
								DataSource: nil,
								PostProcessing: resolve.PostProcessingConfiguration{
									SelectResponseDataPath:   []string{"data", "_entities"},
									SelectResponseErrorsPath: []string{"errors"},
								},
							},
							Path:     []string{"me"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("username"),
									Value: &resolve.String{
										Path: []string{"username"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &resolve.Array{
										Path:     []string{"reviews"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("body"),
													Value: &resolve.String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("product"),
													Value: &resolve.Object{
														Path: []string{"product"},
														Fetch: &resolve.SingleFetch{
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[`),
																		SegmentType: resolve.StaticSegmentType,
																	},

																	{
																		SegmentType:  resolve.VariableSegmentType,
																		VariableKind: resolve.ResolvableObjectVariableKind,
																		Renderer: &resolve.GraphQLVariableResolveRenderer{
																			Kind: resolve.VariableRendererKindGraphqlResolve,
																			Node: &resolve.Object{
																				Nullable: false,
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:     []string{"__typename"},
																							Nullable: false,
																						},
																					},
																					{
																						Name: []byte("upc"),
																						Value: &resolve.String{
																							Path:     []string{"upc"},
																							Nullable: false,
																						},
																					},
																				},
																			},
																		},
																	},
																	{
																		SegmentType: resolve.StaticSegmentType,
																		Data:        []byte(`]}}}`),
																	},
																},
																SetTemplateOutputToNullOnVariableNull: true,
															},
															PostProcessing: resolve.PostProcessingConfiguration{
																SelectResponseDataPath:   []string{"data", "_entities"},
																SelectResponseErrorsPath: []string{"errors"},
															},
														},
														Fields: []*resolve.Field{
															{
																Name: []byte("name"),
																Value: &resolve.String{
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

	processor := &ProcessDataSource{}
	actual := processor.Process(pre)

	if !assert.Equal(t, expected, actual) {
		actualBytes, _ := json.MarshalIndent(actual, "", "  ")
		expectedBytes, _ := json.MarshalIndent(expected, "", "  ")

		if string(expectedBytes) != string(actualBytes) {
			assert.Equal(t, string(expectedBytes), string(actualBytes))
			t.Error(cmp.Diff(string(expectedBytes), string(actualBytes)))
		}
	}
}

func TestDataSourceInput_ProcessSerialFetch(t *testing.T) {
	pre := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SerialFetch{
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{Input: `a`, SerialID: 0},
						&resolve.SingleFetch{Input: `b`, SerialID: 2},
						&resolve.SingleFetch{Input: `c`, SerialID: 5},
					},
				},
			},
		},
	}

	expected := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SerialFetch{
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{
							SerialID: 5,
							InputTemplate: resolve.InputTemplate{
								Segments: []resolve.TemplateSegment{
									{
										Data:        []byte(`c`),
										SegmentType: resolve.StaticSegmentType,
									},
								},
							},
						},
						&resolve.SingleFetch{
							SerialID: 2,
							InputTemplate: resolve.InputTemplate{
								Segments: []resolve.TemplateSegment{
									{
										Data:        []byte(`b`),
										SegmentType: resolve.StaticSegmentType,
									},
								},
							},
						},
						&resolve.SingleFetch{
							SerialID: 0,
							InputTemplate: resolve.InputTemplate{
								Segments: []resolve.TemplateSegment{
									{
										Data:        []byte(`a`),
										SegmentType: resolve.StaticSegmentType,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processor := &ProcessDataSource{}
	actual := processor.Process(pre)

	if !assert.Equal(t, expected, actual) {
		actualBytes, _ := json.MarshalIndent(actual, "", "  ")
		expectedBytes, _ := json.MarshalIndent(expected, "", "  ")

		if string(expectedBytes) != string(actualBytes) {
			assert.Equal(t, string(expectedBytes), string(actualBytes))
			t.Error(cmp.Diff(string(expectedBytes), string(actualBytes)))
		}
	}
}

func TestDataSourceInput_Subscription_Process(t *testing.T) {

	pre := &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`),
				Variables: []resolve.Variable{
					&resolve.HeaderVariable{
						Path: []string{"Authorization"},
					},
				},
			},
			Response: &resolve.GraphQLResponse{},
		},
	}

	expected := &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001/`),
							SegmentType: resolve.StaticSegmentType,
						},
						{
							SegmentType:        resolve.VariableSegmentType,
							VariableKind:       resolve.HeaderVariableKind,
							VariableSourcePath: []string{"Authorization"},
						},
						{
							Data:        []byte(`","body":{"query":"{me {id username}}"}}`),
							SegmentType: resolve.StaticSegmentType,
						},
					},
				},
			},
			Response: &resolve.GraphQLResponse{},
		},
	}

	processor := &ProcessDataSource{}
	actual := processor.Process(pre)

	assert.Equal(t, expected, actual)
}
