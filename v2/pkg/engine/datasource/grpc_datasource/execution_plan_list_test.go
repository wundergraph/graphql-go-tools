package grpcdatasource

import "testing"

func TestListExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a blog post with a non-nullable list",
			query: `query GetBlogPost { blogPost { title tags } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryBlogPost",
						Request: RPCMessage{
							Name: "QueryBlogPostRequest",
						},
						Response: RPCMessage{
							Name: "QueryBlogPostResponse",
							Fields: RPCFields{
								{
									Name:     "blog_post",
									TypeName: string(DataTypeMessage),
									JSONPath: "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
											},
											{
												Name:     "tags",
												TypeName: string(DataTypeString),
												Repeated: true,
												JSONPath: "tags",
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
		{
			name:  "Should create an execution plan for a blog post with a nullable list",
			query: `query GetBlogPost { blogPost { title optionalTags } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryBlogPost",
						Request: RPCMessage{
							Name: "QueryBlogPostRequest",
						},
						Response: RPCMessage{
							Name: "QueryBlogPostResponse",
							Fields: RPCFields{
								{
									Name:     "blog_post",
									TypeName: string(DataTypeMessage),
									JSONPath: "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
											},
											{
												Name:     "optional_tags",
												TypeName: string(DataTypeMessage),
												Optional: true,
												Message: &RPCMessage{
													Name: "ListOfString",
													Fields: RPCFields{
														{
															Name:     "items",
															TypeName: string(DataTypeString),
															Repeated: true,
															JSONPath: "optionalTags",
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
		{
			name:  "Should create an execution plan for a blog post with a nested list",
			query: `query GetBlogPost { blogPost { tagGroups } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryBlogPost",
						Request: RPCMessage{
							Name: "QueryBlogPostRequest",
						},
						Response: RPCMessage{
							Name: "QueryBlogPostResponse",
							Fields: RPCFields{
								{
									Name:     "blog_post",
									TypeName: string(DataTypeMessage),
									JSONPath: "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:     "tag_groups",
												TypeName: string(DataTypeMessage),
												Repeated: false,
												JSONPath: "",
												Message: &RPCMessage{
													Name: "ListOfListOfString",
													Fields: RPCFields{
														{
															Name:     "list",
															TypeName: string(DataTypeMessage),
															Repeated: false,
															JSONPath: "",
															Message: &RPCMessage{
																Name: "List",
																Fields: RPCFields{
																	{
																		Name:     "items",
																		TypeName: string(DataTypeMessage),
																		Repeated: true,
																		JSONPath: "",
																		Message: &RPCMessage{
																			Name: "ListOfString",
																			Fields: RPCFields{
																				{
																					Name:     "items",
																					TypeName: string(DataTypeString),
																					Repeated: true,
																					JSONPath: "tagGroups",
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runTest(t, testCase{
				query:         tt.query,
				expectedPlan:  tt.expectedPlan,
				expectedError: tt.expectedError,
			})
		})
	}
}
