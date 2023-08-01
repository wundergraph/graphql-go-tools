package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

func TestDefaultProcessor_Process(t *testing.T) {

	userService := &fakeService{}
	postsService := &fakeService{}

	original := &plan.SynchronousResponsePlan{
		FlushInterval: 500,
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("users"),
						Stream: &resolve.StreamField{
							InitialBatchSize: 0,
						},
						Value: &resolve.Array{
							Item: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									DataSource: postsService,
									InputTemplate: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableKind:       resolve.ObjectVariableKind,
												VariableSourcePath: []string{"id"},
											},
										},
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.Integer{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path: []string{"name"},
										},
									},

									{

										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("posts"),
										Defer:     &resolve.DeferField{},
										Value: &resolve.Array{
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
														},
													},
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
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

	expected := &plan.StreamingResponsePlan{
		FlushInterval: 500,
		Response: &resolve.GraphQLStreamingResponse{
			FlushInterval: 500,
			InitialResponse: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSource: userService,
						BufferId:   0,
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("users"),
							Value: &resolve.Array{
								Stream: resolve.Stream{
									Enabled:          true,
									InitialBatchSize: 0,
									PatchIndex:       1,
								},
							},
						},
					},
				},
			},
			Patches: []*resolve.GraphQLResponsePatch{
				{
					Operation: literal.REPLACE,
					Fetch: &resolve.SingleFetch{
						DataSource: postsService,
						InputTemplate: resolve.InputTemplate{
							Segments: []resolve.TemplateSegment{
								{
									SegmentType:        resolve.VariableSegmentType,
									VariableKind:       resolve.ObjectVariableKind,
									VariableSourcePath: []string{"id"},
								},
							},
						},
					},
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("title"),
									Value: &resolve.String{
										Path: []string{"title"},
									},
								},
								{
									Name: []byte("body"),
									Value: &resolve.String{
										Path: []string{"body"},
									},
								},
							},
						},
					},
				},
				{
					Operation: literal.ADD,
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("id"),
								Value: &resolve.Integer{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
							},

							{
								Name: []byte("posts"),
								Value: &resolve.Null{
									Defer: resolve.Defer{
										Enabled:    true,
										PatchIndex: 0,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processor := DefaultProcessor()
	actual := processor.Process(original)

	assert.Equal(t, expected, actual)
}

func TestDefaultProcessor_Federation(t *testing.T) {
	pre := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					BufferId: 0,
					Input:    `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`,
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								Input:    `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}}}`,
								Variables: resolve.NewVariables(
									&resolve.ObjectVariable{
										Path: []string{"id"},
									},
								),
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
									Defer:     &resolve.DeferField{},
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
															Input:    `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}}}`,
															Variables: resolve.NewVariables(
																&resolve.ObjectVariable{
																	Path: []string{"upc"},
																},
															),
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

	expected := &plan.StreamingResponsePlan{
		Response: &resolve.GraphQLStreamingResponse{
			InitialResponse: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						InputTemplate: resolve.InputTemplate{
							Segments: []resolve.TemplateSegment{
								{
									SegmentType: resolve.StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("me"),
							Value: &resolve.Object{
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
										Value: &resolve.Null{
											Defer: resolve.Defer{
												Enabled:    true,
												PatchIndex: 0,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Patches: []*resolve.GraphQLResponsePatch{
				{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						InputTemplate: resolve.InputTemplate{
							Segments: []resolve.TemplateSegment{
								{
									SegmentType: resolve.StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
								},
								{
									SegmentType:        resolve.VariableSegmentType,
									VariableKind:       resolve.ObjectVariableKind,
									VariableSourcePath: []string{"id"},
								},
								{
									SegmentType: resolve.StaticSegmentType,
									Data:        []byte(`","__typename":"User"}]}}}`),
								},
							},
						},
						ProcessResponseConfig: resolve.ProcessResponseConfig{
							ExtractGraphqlResponse:    true,
							ExtractFederationEntities: true,
						},
					},
					Operation: literal.REPLACE,
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
														SegmentType: resolve.StaticSegmentType,
														Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
													},
													{
														SegmentType:        resolve.VariableSegmentType,
														VariableKind:       resolve.ObjectVariableKind,
														VariableSourcePath: []string{"upc"},
													},
													{
														SegmentType: resolve.StaticSegmentType,
														Data:        []byte(`","__typename":"Product"}]}}}`),
													},
												},
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
	}

	processor := DefaultProcessor()
	actual := processor.Process(pre)
	assert.Equal(t, expected, actual)
}
