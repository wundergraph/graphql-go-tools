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
									Name:          "blog_post",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
											},
											{
												Name:          "tags",
												ProtoTypeName: DataTypeString,
												Repeated:      true,
												JSONPath:      "tags",
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
									Name:          "blog_post",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
											},
											{
												Name:          "optional_tags",
												ProtoTypeName: DataTypeString,
												Optional:      true,
												JSONPath:      "optionalTags",
												IsListType:    true,
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
									Name:          "blog_post",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "blogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: RPCFields{
											{
												Name:          "tag_groups",
												ProtoTypeName: DataTypeString,
												Repeated:      false,
												JSONPath:      "tagGroups",
												IsListType:    true,
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
									Name:          "kinds",
									ProtoTypeName: DataTypeEnum,
									JSONPath:      "kinds",
									EnumName:      "CategoryKind",
									Repeated:      true,
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryCategoriesByKindsResponse",
							Fields: []RPCField{
								{
									Name:          "categories_by_kinds",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categoriesByKinds",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "Category",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "kind",
												ProtoTypeName: DataTypeEnum,
												JSONPath:      "kind",
												EnumName:      "CategoryKind",
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
									Name:          "orders",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "orders",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "OrderInput",
										Fields: []RPCField{
											{
												Name:          "order_id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "orderId",
											},
											{
												Name:          "customer_name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "customerName",
											},
											{
												Name:          "lines",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "lines",
												Repeated:      true,
												Message: &RPCMessage{
													Name: "OrderLineInput",
													Fields: []RPCField{
														{
															Name:          "product_id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "productId",
														},
														{
															Name:          "quantity",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "quantity",
														},
														{
															Name:          "modifiers",
															ProtoTypeName: DataTypeString,
															JSONPath:      "modifiers",
															Optional:      true,
															IsListType:    true,
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
									Name:          "calculate_totals",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "calculateTotals",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "Order",
										Fields: []RPCField{
											{
												Name:          "order_id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "orderId",
											},
											{
												Name:          "customer_name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "customerName",
											},
											{
												Name:          "total_items",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "totalItems",
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
									Name:          "input",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "input",
									Message: &RPCMessage{
										Name: "BlogPostInput",
										Fields: []RPCField{
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
											},
											{
												Name:          "content",
												ProtoTypeName: DataTypeString,
												JSONPath:      "content",
											},
											{
												Name:          "tags",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tags",
												Repeated:      true,
											},
											{
												Name:          "optional_tags",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalTags",
												Optional:      true,
												IsListType:    true,
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
												Name:          "categories",
												ProtoTypeName: DataTypeString,
												JSONPath:      "categories",
												Repeated:      true,
											},
											{
												Name:          "keywords",
												ProtoTypeName: DataTypeString,
												JSONPath:      "keywords",
												Optional:      true,
												IsListType:    true,
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
												Name:          "view_counts",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "viewCounts",
												Repeated:      true,
											},
											{
												Name:          "ratings",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "ratings",
												Optional:      true,
												IsListType:    true,
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
												Name:          "is_published",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "isPublished",
												Optional:      true,
												IsListType:    true,
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
												Name:          "tag_groups",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tagGroups",
												IsListType:    true,
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
												Name:          "related_topics",
												ProtoTypeName: DataTypeString,
												JSONPath:      "relatedTopics",
												IsListType:    true,
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
												Name:          "comment_threads",
												ProtoTypeName: DataTypeString,
												JSONPath:      "commentThreads",
												IsListType:    true,
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
												Name:          "suggestions",
												ProtoTypeName: DataTypeString,
												JSONPath:      "suggestions",
												Optional:      true,
												IsListType:    true,
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
												Name:          "related_categories",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "relatedCategories",
												Repeated:      false,
												Optional:      true,
												IsListType:    true,
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
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
														{
															Name:          "kind",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "kind",
															EnumName:      "CategoryKind",
														},
													},
												},
											},
											{
												Name:          "contributors",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "contributors",
												Repeated:      false,
												Optional:      true,
												IsListType:    true,
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
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
													},
												},
											},
											{
												Name:          "category_groups",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "categoryGroups",
												Repeated:      false,
												Optional:      true,
												IsListType:    true,
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
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
														{
															Name:          "kind",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "kind",
															EnumName:      "CategoryKind",
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
									Name:          "create_blog_post",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "createBlogPost",
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
											},
											{
												Name:          "tags",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tags",
												Repeated:      true,
											},
											{
												Name:          "optional_tags",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalTags",
												Optional:      true,
												IsListType:    true,
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
												Name:          "categories",
												ProtoTypeName: DataTypeString,
												JSONPath:      "categories",
												Repeated:      true,
											},
											{
												Name:          "keywords",
												ProtoTypeName: DataTypeString,
												JSONPath:      "keywords",
												Optional:      true,
												IsListType:    true,
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
									Name:          "input",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "input",
									Message: &RPCMessage{
										Name: "AuthorInput",
										Fields: []RPCField{
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "email",
												ProtoTypeName: DataTypeString,
												JSONPath:      "email",
												Optional:      true,
											},
											{
												Name:          "skills",
												ProtoTypeName: DataTypeString,
												JSONPath:      "skills",
												Repeated:      true,
											},
											{
												Name:          "languages",
												ProtoTypeName: DataTypeString,
												JSONPath:      "languages",
												Repeated:      true,
											},
											{
												Name:          "social_links",
												ProtoTypeName: DataTypeString,
												JSONPath:      "socialLinks",
												Optional:      true,
												IsListType:    true,
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
												Name:          "teams_by_project",
												ProtoTypeName: DataTypeString,
												JSONPath:      "teamsByProject",
												IsListType:    true,
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
												Name:          "collaborations",
												ProtoTypeName: DataTypeString,
												JSONPath:      "collaborations",
												Optional:      true,
												IsListType:    true,
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
												Name:          "favorite_categories",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "favoriteCategories",
												Repeated:      true,
												Message: &RPCMessage{
													Name: "CategoryInput",
													Fields: []RPCField{
														{
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
														{
															Name:          "kind",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "kind",
															EnumName:      "CategoryKind",
														},
													},
												},
											},
											{
												Name:          "author_groups",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "authorGroups",
												Repeated:      false,
												Optional:      true,
												IsListType:    true,
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
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
													},
												},
											},
											{
												Name:          "project_teams",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "projectTeams",
												Repeated:      false,
												Optional:      true,
												IsListType:    true,
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
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
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
									Name:          "create_author",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "createAuthor",
									Message: &RPCMessage{
										Name: "Author",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "skills",
												ProtoTypeName: DataTypeString,
												JSONPath:      "skills",
												Repeated:      true,
											},
											{
												Name:          "teams_by_project",
												ProtoTypeName: DataTypeString,
												JSONPath:      "teamsByProject",
												IsListType:    true,
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
												Name:          "collaborations",
												ProtoTypeName: DataTypeString,
												JSONPath:      "collaborations",
												Optional:      true,
												IsListType:    true,
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
									Name:          "filter",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "filter",
									Message: &RPCMessage{
										Name: "BlogPostFilter",
										Fields: []RPCField{
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
												Optional:      true,
											},
											{
												Name:          "has_categories",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "hasCategories",
												Optional:      true,
											},
											{
												Name:          "min_tags",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "minTags",
												Optional:      true,
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
									Name:          "blog_posts_with_filter",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "blogPostsWithFilter",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "BlogPost",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "title",
												ProtoTypeName: DataTypeString,
												JSONPath:      "title",
											},
											{
												Name:          "tags",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tags",
												Repeated:      true,
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
									Name:          "filters",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "filters",
									IsListType:    true,
									Optional:      true,
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
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												Optional:      true,
											},
											{
												Name:          "has_teams",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "hasTeams",
												Optional:      true,
											},
											{
												Name:          "skill_count",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "skillCount",
												Optional:      true,
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
									Name:          "bulk_search_authors",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "bulkSearchAuthors",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "Author",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "email",
												ProtoTypeName: DataTypeString,
												JSONPath:      "email",
												Optional:      true,
											},
											{
												Name:          "skills",
												ProtoTypeName: DataTypeString,
												JSONPath:      "skills",
												Repeated:      true,
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
