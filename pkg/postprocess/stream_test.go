package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

func TestProcessStream_Process(t *testing.T) {

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
									PatchIndex:       0,
								},
							},
						},
					},
				},
			},
			Patches: []*resolve.GraphQLResponsePatch{
				{
					Operation: literal.ADD,
					Value: &resolve.Object{
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
	}

	proc := &ProcessStream{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}

func TestProcessStream_Process_BatchSize_1(t *testing.T) {

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
							InitialBatchSize: 1,
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
									InitialBatchSize: 1,
								},
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
					Operation: literal.ADD,
					Value: &resolve.Object{
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
	}

	proc := &ProcessStream{}
	actual := proc.Process(original)

	assert.Equal(t, expected, actual)
}
