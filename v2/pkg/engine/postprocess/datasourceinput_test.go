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
					BufferId:   0,
					Input:      `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username __typename}}"}}`,
					DataSource: nil,
					Variables: []resolve.Variable{
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								Input:    `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":$$0$$}}}`,
								Variables: resolve.NewVariables(
									&resolve.ListVariable{
										Variables: resolve.NewVariables(
											&resolve.ResolvableObjectVariable{
												Path: []string{"me"},
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
									},
								),
								DataSource: nil,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
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
									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
															BufferId:   2,
															Input:      `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":$$0$$}}}`,
															DataSource: nil,
															Variables: resolve.NewVariables(
																&resolve.ListVariable{
																	Variables: resolve.NewVariables(
																		&resolve.ResolvableObjectVariable{
																			Path: []string{"product"},
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
																},
															),

															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
															SetTemplateOutputToNullOnVariableNull: true,
														},
														Fields: []*resolve.Field{
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
					BufferId: 0,
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
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType: resolve.ListSegmentType,
											Segments: []resolve.TemplateSegment{
												{
													SegmentType: resolve.StaticSegmentType,
													Data:        []byte(`[`),
												},
												{
													SegmentType:        resolve.VariableSegmentType,
													VariableKind:       resolve.ResolvableObjectVariableKind,
													VariableSourcePath: []string{"me"},
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
													Data:        []byte(`]`),
												},
											},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`}}}`),
										},
									},
									SetTemplateOutputToNullOnVariableNull: true,
								},
								DataSource: nil,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
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
									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
															BufferId: 2,
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType: resolve.ListSegmentType,
																		Segments: []resolve.TemplateSegment{

																			{
																				SegmentType: resolve.StaticSegmentType,
																				Data:        []byte(`[`),
																			},
																			{
																				SegmentType:        resolve.VariableSegmentType,
																				VariableKind:       resolve.ResolvableObjectVariableKind,
																				VariableSourcePath: []string{"product"},
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
																				Data:        []byte(`]`),
																			},
																		},
																	},
																	{
																		SegmentType: resolve.StaticSegmentType,
																		Data:        []byte(`}}}`),
																	},
																},
																SetTemplateOutputToNullOnVariableNull: true,
															},
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
														},
														Fields: []*resolve.Field{
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
