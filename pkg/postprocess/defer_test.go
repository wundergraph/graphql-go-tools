package postprocess

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

func TestProcessDefer_Process(t *testing.T) {

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
						Value: &resolve.Array{
							Item: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									DataSource: postsService,
									InputTemplate: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableSource:     resolve.VariableSourceObject,
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
		Response: resolve.GraphQLStreamingResponse{
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
								Item: &resolve.Object{
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
									VariableSource:     resolve.VariableSourceObject,
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
			},
		},
	}

	proc := &ProcessDefer{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}

func TestProcessDefer_Process_Nested(t *testing.T) {

	userService := &fakeService{}
	postsService := &fakeService{}
	commentsService := &fakeService{}

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
						Value: &resolve.Array{
							Item: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									DataSource: postsService,
									InputTemplate: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableSource:     resolve.VariableSourceObject,
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
												Fetch: &resolve.SingleFetch{
													BufferId:   2,
													DataSource: commentsService,
													InputTemplate: resolve.InputTemplate{
														Segments: []resolve.TemplateSegment{
															{
																SegmentType:        resolve.VariableSegmentType,
																VariableSource:     resolve.VariableSourceObject,
																VariableSourcePath: []string{"id"},
															},
														},
													},
												},
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

													{

														HasBuffer: true,
														BufferID:  2,
														Name:      []byte("comments"),
														Defer:     &resolve.DeferField{},
														Value: &resolve.Array{
															Item: &resolve.Object{
																Fields: []*resolve.Field{
																	{
																		Name: []byte("user"),
																		Value: &resolve.String{
																			Path: []string{"user"},
																		},
																	},
																	{
																		Name: []byte("text"),
																		Value: &resolve.String{
																			Path: []string{"text"},
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

	expected := &plan.StreamingResponsePlan{
		FlushInterval: 500,
		Response: resolve.GraphQLStreamingResponse{
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
								Item: &resolve.Object{
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
									VariableSource:     resolve.VariableSourceObject,
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
								{

									Name: []byte("comments"),
									Value: &resolve.Null{
										Defer: resolve.Defer{
											Enabled:    true,
											PatchIndex: 1,
										},
									},
								},
							},
						},
					},
				},
				{
					Operation: literal.REPLACE,
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						DataSource: commentsService,
						InputTemplate: resolve.InputTemplate{
							Segments: []resolve.TemplateSegment{
								{
									SegmentType:        resolve.VariableSegmentType,
									VariableSource:     resolve.VariableSourceObject,
									VariableSourcePath: []string{"id"},
								},
							},
						},
					},
					Value: &resolve.Array{
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
									Value: &resolve.String{
										Path: []string{"user"},
									},
								},
								{
									Name: []byte("text"),
									Value: &resolve.String{
										Path: []string{"text"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	proc := &ProcessDefer{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}

func TestProcessDefer_Process_KeepFetchIfUsedUndeferred(t *testing.T) {

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
						Value: &resolve.Array{
							Item: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									DataSource: postsService,
									InputTemplate: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableSource:     resolve.VariableSourceObject,
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
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("no_defer_posts"),
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
		Response: resolve.GraphQLStreamingResponse{
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
								Item: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId:   1,
										DataSource: postsService,
										InputTemplate: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType:        resolve.VariableSegmentType,
													VariableSource:     resolve.VariableSourceObject,
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
											Value: &resolve.Null{
												Defer: resolve.Defer{
													Enabled:    true,
													PatchIndex: 0,
												},
											},
										},
										{
											HasBuffer: true,
											BufferID:  1,
											Name:      []byte("no_defer_posts"),
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
			Patches: []*resolve.GraphQLResponsePatch{
				{
					Operation: literal.REPLACE,
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
	}

	proc := &ProcessDefer{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}

func TestProcessDefer_Process_ParallelFetch(t *testing.T) {

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
						Value: &resolve.Array{
							Item: &resolve.Object{
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											BufferId:   1,
											DataSource: postsService,
											InputTemplate: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType:        resolve.VariableSegmentType,
														VariableSource:     resolve.VariableSourceObject,
														VariableSourcePath: []string{"id"},
													},
												},
											},
										},
										&resolve.SingleFetch{
											BufferId:   2,
											DataSource: postsService,
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
		Response: resolve.GraphQLStreamingResponse{
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
								Item: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId:   2,
										DataSource: postsService,
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
									VariableSource:     resolve.VariableSourceObject,
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
			},
		},
	}

	proc := &ProcessDefer{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}

func TestProcessDefer_Process_ShouldSkipWithoutDefer(t *testing.T) {

	userService := &fakeService{}
	postsService := &fakeService{}

	planFactory := func() plan.Plan {
		return &plan.SynchronousResponsePlan{
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
							Value: &resolve.Array{
								Item: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId:   1,
										DataSource: postsService,
										InputTemplate: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType:        resolve.VariableSegmentType,
													VariableSource:     resolve.VariableSourceObject,
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
											Name:      []byte("non_deferred_posts"),
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
	}

	original := planFactory()
	clone := planFactory()

	proc := &ProcessDefer{}
	actual := proc.Process(original)

	assert.Equal(t, clone, actual)
}

type fakeService struct {
}

func (f *fakeService) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	panic("implement me")
}

func (f *fakeService) UniqueIdentifier() []byte {
	panic("implement me")
}
