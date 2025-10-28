package grpcdatasource

import "testing"

func TestListExecutionPlan(t *testing.T) {
	t.Parallel()
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
												Name:       "optional_tags",
												TypeName:   string(DataTypeString),
												Optional:   true,
												JSONPath:   "optionalTags",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
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
												Name:       "tag_groups",
												TypeName:   string(DataTypeString),
												Repeated:   false,
												JSONPath:   "tagGroups",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: false,
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

func TestListParametersExecutionPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for query with required list of required enums parameter",
			query: `query GetCategoriesByKinds($kinds: [CategoryKind!]!) { categoriesByKinds(kinds: $kinds) { id name kind } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategoriesByKinds",
						Request: RPCMessage{
							Name: "QueryCategoriesByKindsRequest",
							Fields: []RPCField{
								{
									Name:     "kinds",
									TypeName: string(DataTypeEnum),
									JSONPath: "kinds",
									EnumName: "CategoryKind",
									Repeated: true,
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryCategoriesByKindsResponse",
							Fields: []RPCField{
								{
									Name:     "categories_by_kinds",
									TypeName: string(DataTypeMessage),
									JSONPath: "categoriesByKinds",
									Repeated: true,
									Message: &RPCMessage{
										Name: "Category",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "kind",
												TypeName: string(DataTypeEnum),
												JSONPath: "kind",
												EnumName: "CategoryKind",
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
			name:  "Should create an execution plan for query with required list of required input objects parameter",
			query: `query CalculateTotals($orders: [OrderInput!]!) { calculateTotals(orders: $orders) { orderId customerName totalItems } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCalculateTotals",
						Request: RPCMessage{
							Name: "QueryCalculateTotalsRequest",
							Fields: []RPCField{
								{
									Name:     "orders",
									TypeName: string(DataTypeMessage),
									JSONPath: "orders",
									Repeated: true,
									Message: &RPCMessage{
										Name: "OrderInput",
										Fields: []RPCField{
											{
												Name:     "order_id",
												TypeName: string(DataTypeString),
												JSONPath: "orderId",
											},
											{
												Name:     "customer_name",
												TypeName: string(DataTypeString),
												JSONPath: "customerName",
											},
											{
												Name:     "lines",
												TypeName: string(DataTypeMessage),
												JSONPath: "lines",
												Repeated: true,
												Message: &RPCMessage{
													Name: "OrderLineInput",
													Fields: []RPCField{
														{
															Name:     "product_id",
															TypeName: string(DataTypeString),
															JSONPath: "productId",
														},
														{
															Name:     "quantity",
															TypeName: string(DataTypeInt32),
															JSONPath: "quantity",
														},
														{
															Name:       "modifiers",
															TypeName:   string(DataTypeString),
															JSONPath:   "modifiers",
															Optional:   true,
															IsListType: true,
															ListMetadata: &ListMetadata{
																NestingLevel: 1,
																LevelInfo: []LevelInfo{
																	{
																		Optional: true,
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
						Response: RPCMessage{
							Name: "QueryCalculateTotalsResponse",
							Fields: []RPCField{
								{
									Name:     "calculate_totals",
									TypeName: string(DataTypeMessage),
									JSONPath: "calculateTotals",
									Repeated: true,
									Message: &RPCMessage{
										Name: "Order",
										Fields: []RPCField{
											{
												Name:     "order_id",
												TypeName: string(DataTypeString),
												JSONPath: "orderId",
											},
											{
												Name:     "customer_name",
												TypeName: string(DataTypeString),
												JSONPath: "customerName",
											},
											{
												Name:     "total_items",
												TypeName: string(DataTypeInt32),
												JSONPath: "totalItems",
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
			name:  "Should create an execution plan for BlogPost mutation with single list parameters",
			query: `mutation CreateBlogPost($input: BlogPostInput!) { createBlogPost(input: $input) { id title tags optionalTags categories keywords } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationCreateBlogPost",
						Request: RPCMessage{
							Name: "MutationCreateBlogPostRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "BlogPostInput",
										Fields: []RPCField{
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
											},
											{
												Name:     "content",
												TypeName: string(DataTypeString),
												JSONPath: "content",
											},
											{
												Name:     "tags",
												TypeName: string(DataTypeString),
												JSONPath: "tags",
												Repeated: true,
											},
											{
												Name:       "optional_tags",
												TypeName:   string(DataTypeString),
												JSONPath:   "optionalTags",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:     "categories",
												TypeName: string(DataTypeString),
												JSONPath: "categories",
												Repeated: true,
											},
											{
												Name:       "keywords",
												TypeName:   string(DataTypeString),
												JSONPath:   "keywords",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:     "view_counts",
												TypeName: string(DataTypeInt32),
												JSONPath: "viewCounts",
												Repeated: true,
											},
											{
												Name:       "ratings",
												TypeName:   string(DataTypeDouble),
												JSONPath:   "ratings",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:       "is_published",
												TypeName:   string(DataTypeBool),
												JSONPath:   "isPublished",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:       "tag_groups",
												TypeName:   string(DataTypeString),
												JSONPath:   "tagGroups",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: false,
														},
													},
												},
											},
											{
												Name:       "related_topics",
												TypeName:   string(DataTypeString),
												JSONPath:   "relatedTopics",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:       "comment_threads",
												TypeName:   string(DataTypeString),
												JSONPath:   "commentThreads",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: false,
														},
													},
												},
											},
											{
												Name:       "suggestions",
												TypeName:   string(DataTypeString),
												JSONPath:   "suggestions",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:       "related_categories",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "relatedCategories",
												Repeated:   false,
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
												Message: &RPCMessage{
													Name: "CategoryInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "kind",
															TypeName: string(DataTypeEnum),
															JSONPath: "kind",
															EnumName: "CategoryKind",
														},
													},
												},
											},
											{
												Name:       "contributors",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "contributors",
												Repeated:   false,
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
												Message: &RPCMessage{
													Name: "UserInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
													},
												},
											},
											{
												Name:       "category_groups",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "categoryGroups",
												Repeated:   false,
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
														},
													},
												},
												Message: &RPCMessage{
													Name: "CategoryInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "kind",
															TypeName: string(DataTypeEnum),
															JSONPath: "kind",
															EnumName: "CategoryKind",
														},
													},
												},
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationCreateBlogPostResponse",
							Fields: []RPCField{
								{
									Name:     "create_blog_post",
									TypeName: string(DataTypeMessage),
									JSONPath: "createBlogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
											},
											{
												Name:     "tags",
												TypeName: string(DataTypeString),
												JSONPath: "tags",
												Repeated: true,
											},
											{
												Name:       "optional_tags",
												TypeName:   string(DataTypeString),
												JSONPath:   "optionalTags",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:     "categories",
												TypeName: string(DataTypeString),
												JSONPath: "categories",
												Repeated: true,
											},
											{
												Name:       "keywords",
												TypeName:   string(DataTypeString),
												JSONPath:   "keywords",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
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
			name:  "Should create an execution plan for Author mutation with nested list parameters",
			query: `mutation CreateAuthor($input: AuthorInput!) { createAuthor(input: $input) { id name skills teamsByProject collaborations } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationCreateAuthor",
						Request: RPCMessage{
							Name: "MutationCreateAuthorRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "AuthorInput",
										Fields: []RPCField{
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "email",
												TypeName: string(DataTypeString),
												JSONPath: "email",
												Optional: true,
											},
											{
												Name:     "skills",
												TypeName: string(DataTypeString),
												JSONPath: "skills",
												Repeated: true,
											},
											{
												Name:     "languages",
												TypeName: string(DataTypeString),
												JSONPath: "languages",
												Repeated: true,
											},
											{
												Name:       "social_links",
												TypeName:   string(DataTypeString),
												JSONPath:   "socialLinks",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:       "teams_by_project",
												TypeName:   string(DataTypeString),
												JSONPath:   "teamsByProject",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: false,
														},
													},
												},
											},
											{
												Name:       "collaborations",
												TypeName:   string(DataTypeString),
												JSONPath:   "collaborations",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
														},
													},
												},
											},
											{
												Name:     "favorite_categories",
												TypeName: string(DataTypeMessage),
												JSONPath: "favoriteCategories",
												Repeated: true,
												Message: &RPCMessage{
													Name: "CategoryInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "kind",
															TypeName: string(DataTypeEnum),
															JSONPath: "kind",
															EnumName: "CategoryKind",
														},
													},
												},
											},
											{
												Name:       "author_groups",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "authorGroups",
												Repeated:   false,
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
														},
													},
												},
												Message: &RPCMessage{
													Name: "UserInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
													},
												},
											},
											{
												Name:       "project_teams",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "projectTeams",
												Repeated:   false,
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
														},
													},
												},
												Message: &RPCMessage{
													Name: "UserInput",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
													},
												},
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationCreateAuthorResponse",
							Fields: []RPCField{
								{
									Name:     "create_author",
									TypeName: string(DataTypeMessage),
									JSONPath: "createAuthor",
									Message: &RPCMessage{
										Name: "Author",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "skills",
												TypeName: string(DataTypeString),
												JSONPath: "skills",
												Repeated: true,
											},
											{
												Name:       "teams_by_project",
												TypeName:   string(DataTypeString),
												JSONPath:   "teamsByProject",
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: false,
														},
														{
															Optional: false,
														},
													},
												},
											},
											{
												Name:       "collaborations",
												TypeName:   string(DataTypeString),
												JSONPath:   "collaborations",
												Optional:   true,
												IsListType: true,
												ListMetadata: &ListMetadata{
													NestingLevel: 2,
													LevelInfo: []LevelInfo{
														{
															Optional: true,
														},
														{
															Optional: true,
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
			name:  "Should create an execution plan for filtered BlogPost query with simple parameters",
			query: `query FilteredBlogPosts($filter: BlogPostFilter!) { blogPostsWithFilter(filter: $filter) { id title tags } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryBlogPostsWithFilter",
						Request: RPCMessage{
							Name: "QueryBlogPostsWithFilterRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "BlogPostFilter",
										Fields: []RPCField{
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
												Optional: true,
											},
											{
												Name:     "has_categories",
												TypeName: string(DataTypeBool),
												JSONPath: "hasCategories",
												Optional: true,
											},
											{
												Name:     "min_tags",
												TypeName: string(DataTypeInt32),
												JSONPath: "minTags",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryBlogPostsWithFilterResponse",
							Fields: []RPCField{
								{
									Name:     "blog_posts_with_filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "blogPostsWithFilter",
									Repeated: true,
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "title",
												TypeName: string(DataTypeString),
												JSONPath: "title",
											},
											{
												Name:     "tags",
												TypeName: string(DataTypeString),
												JSONPath: "tags",
												Repeated: true,
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
			name:  "Should create an execution plan for bulk search authors",
			query: `query BulkSearchAuthors($filters: [AuthorFilter!]) { bulkSearchAuthors(filters: $filters) { id name email skills } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryBulkSearchAuthors",
						Request: RPCMessage{
							Name: "QueryBulkSearchAuthorsRequest",
							Fields: []RPCField{
								{
									Name:       "filters",
									TypeName:   string(DataTypeMessage),
									JSONPath:   "filters",
									IsListType: true,
									Optional:   true,
									ListMetadata: &ListMetadata{
										NestingLevel: 1,
										LevelInfo: []LevelInfo{
											{
												Optional: true,
											},
										},
									},
									Message: &RPCMessage{
										Name: "AuthorFilter",
										Fields: []RPCField{
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Optional: true,
											},
											{
												Name:     "has_teams",
												TypeName: string(DataTypeBool),
												JSONPath: "hasTeams",
												Optional: true,
											},
											{
												Name:     "skill_count",
												TypeName: string(DataTypeInt32),
												JSONPath: "skillCount",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryBulkSearchAuthorsResponse",
							Fields: []RPCField{
								{
									Name:     "bulk_search_authors",
									TypeName: string(DataTypeMessage),
									JSONPath: "bulkSearchAuthors",
									Repeated: true,
									Message: &RPCMessage{
										Name: "Author",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "email",
												TypeName: string(DataTypeString),
												JSONPath: "email",
												Optional: true,
											},
											{
												Name:     "skills",
												TypeName: string(DataTypeString),
												JSONPath: "skills",
												Repeated: true,
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
