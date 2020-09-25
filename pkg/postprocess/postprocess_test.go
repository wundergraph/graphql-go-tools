package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

func TestDefaultProcessor_Process(t *testing.T) {

	userService := &fakeService{}
	postsService := &fakeService{}

	original := &plan.SynchronousResponsePlan{
		FlushInterval: 500,
		Response: resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				FieldSets: []resolve.FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []resolve.Field{
							{
								Name: []byte("users"),
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
														VariableSource:     resolve.VariableSourceObject,
														VariableSourcePath: []string{"id"},
													},
												},
											},
										},
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
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
												},
											},
											{
												HasBuffer: true,
												BufferID:  1,
												Fields: []resolve.Field{
													{
														Name:  []byte("posts"),
														Defer: &resolve.DeferField{},
														Value: &resolve.Array{
															Item: &resolve.Object{
																FieldSets: []resolve.FieldSet{
																	{
																		Fields: []resolve.Field{
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
			InitialResponse: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSource: userService,
						BufferId:   0,
					},
					FieldSets: []resolve.FieldSet{
						{
							HasBuffer: true,
							BufferID:  0,
							Fields: []resolve.Field{
								{
									Name: []byte("users"),
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
							FieldSets: []resolve.FieldSet{
								{
									Fields: []resolve.Field{
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
				{
					Operation: literal.ADD,
					Value: &resolve.Object{
						FieldSets: []resolve.FieldSet{
							{
								Fields: []resolve.Field{
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
								},
							},
							{
								Fields: []resolve.Field{
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
	}

	processor := DefaultProcessor()
	actual := processor.Process(original)

	assert.Equal(t, expected, actual)
}
