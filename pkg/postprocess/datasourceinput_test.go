package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

func TestDataSourceInput_Process(t *testing.T) {
	pre := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					BufferId:   0,
					Input:      `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`,
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
								Input:    `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}},"extract_entities":true}`,
								Variables: resolve.NewVariables(
									&resolve.ObjectVariable{
										Path: []string{"id"},
									},
								),
								DataSource: nil,
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
															Input:      `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}},"extract_entities":true}`,
															DataSource: nil,
															Variables: resolve.NewVariables(
																&resolve.ObjectVariable{
																	Path: []string{"upc"},
																},
															),
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
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
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
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableSource:     resolve.VariableSourceObject,
											VariableSourcePath: []string{"id"},
										},
										{
											Data:        []byte(`","__typename":"User"}]}},"extract_entities":true}`),
											SegmentType: resolve.StaticSegmentType,
										},
									},
								},
								DataSource: nil,
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
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType:        resolve.VariableSegmentType,
																		VariableSource:     resolve.VariableSourceObject,
																		VariableSourcePath: []string{"upc"},
																	},
																	{
																		Data:        []byte(`","__typename":"Product"}]}},"extract_entities":true}`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																},
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

	assert.Equal(t, expected, actual)
}
