package grpcdatasource

import (
	"testing"
)

func TestExecutionPlanFieldResolvers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with a field resolver",
			query: "query CategoriesWithFieldResolvers($whoop: ProductCountFilter) { categories { id name kind productCount(filters: $whoop) } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
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
					{
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryProductCount",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.productCount"),
						Request: RPCMessage{
							Name: "ResolveCategoryProductCountRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
											{
												Name:        "name",
												TypeName:    string(DataTypeString),
												JSONPath:    "name",
												ResolvePath: buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:     "filters",
												TypeName: string(DataTypeMessage),
												JSONPath: "whoop",
												Optional: true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:     "min_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "minPrice",
															Optional: true,
														},
														{
															Name:     "max_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "maxPrice",
															Optional: true,
														},
														{
															Name:     "in_stock",
															TypeName: string(DataTypeBool),
															JSONPath: "inStock",
															Optional: true,
														},
														{
															Name:     "search_term",
															TypeName: string(DataTypeString),
															JSONPath: "searchTerm",
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
						Response: RPCMessage{
							Name: "ResolveCategoryProductCountResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:     "product_count",
												TypeName: string(DataTypeInt32),
												JSONPath: "productCount",
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
			name:  "Should create an execution plan for a query with a field resolver and aliases",
			query: "query CategoriesWithFieldResolversAndAliases($p1: ProductCountFilter, $p2: ProductCountFilter) { categories { productCount1: productCount(filters: $p1) productCount2: productCount(filters: $p2) } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
									Repeated: true,
									Message: &RPCMessage{
										Name:   "Category",
										Fields: []RPCField{},
									},
								},
							},
						},
					},
					{
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryProductCount",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.productCount1"),
						Request: RPCMessage{
							Name: "ResolveCategoryProductCountRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
											{
												Name:        "name",
												TypeName:    string(DataTypeString),
												JSONPath:    "name",
												ResolvePath: buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:     "filters",
												TypeName: string(DataTypeMessage),
												JSONPath: "p1",
												Optional: true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:     "min_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "minPrice",
															Optional: true,
														},
														{
															Name:     "max_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "maxPrice",
															Optional: true,
														},
														{
															Name:     "in_stock",
															TypeName: string(DataTypeBool),
															JSONPath: "inStock",
															Optional: true,
														},
														{
															Name:     "search_term",
															TypeName: string(DataTypeString),
															JSONPath: "searchTerm",
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
						Response: RPCMessage{
							Name: "ResolveCategoryProductCountResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:     "product_count",
												TypeName: string(DataTypeInt32),
												JSONPath: "productCount1",
											},
										},
									},
								},
							},
						},
					},
					{
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryProductCount",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.productCount2"),
						Request: RPCMessage{
							Name: "ResolveCategoryProductCountRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
											{
												Name:        "name",
												TypeName:    string(DataTypeString),
												JSONPath:    "name",
												ResolvePath: buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:     "filters",
												TypeName: string(DataTypeMessage),
												JSONPath: "p2",
												Optional: true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:     "min_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "minPrice",
															Optional: true,
														},
														{
															Name:     "max_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "maxPrice",
															Optional: true,
														},
														{
															Name:     "in_stock",
															TypeName: string(DataTypeBool),
															JSONPath: "inStock",
															Optional: true,
														},
														{
															Name:     "search_term",
															TypeName: string(DataTypeString),
															JSONPath: "searchTerm",
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
						Response: RPCMessage{
							Name: "ResolveCategoryProductCountResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:     "product_count",
												TypeName: string(DataTypeInt32),
												JSONPath: "productCount2",
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
			name:  "Should create an execution plan for a query with nullable lists type",
			query: "query SubcategoriesWithFieldResolvers($filter: SubcategoryItemFilter) { categories { id subcategories { id name description isActive itemCount(filters: $filter) } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
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
												Name:       "subcategories",
												TypeName:   string(DataTypeMessage),
												JSONPath:   "subcategories",
												Repeated:   false,
												IsListType: true,
												Optional:   true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo:    []LevelInfo{{Optional: true}},
												},
												Message: &RPCMessage{
													Name: "Subcategory",
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
															Name:     "description",
															TypeName: string(DataTypeString),
															JSONPath: "description",
															Optional: true,
														},
														{
															Name:     "is_active",
															TypeName: string(DataTypeBool),
															JSONPath: "isActive",
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
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveSubcategoryItemCount",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.subcategories.itemCount"),
						Request: RPCMessage{
							Name: "ResolveSubcategoryItemCountRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.@subcategories.id"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountArgs",
										Fields: []RPCField{
											{
												Name:     "filters",
												TypeName: string(DataTypeMessage),
												JSONPath: "filter",
												Optional: true,
												Message: &RPCMessage{
													Name: "SubcategoryItemFilter",
													Fields: []RPCField{
														{
															Name:     "min_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "minPrice",
															Optional: true,
														},
														{
															Name:     "max_price",
															TypeName: string(DataTypeDouble),
															JSONPath: "maxPrice",
															Optional: true,
														},
														{
															Name:     "in_stock",
															TypeName: string(DataTypeBool),
															JSONPath: "inStock",
															Optional: true,
														},
														{
															Name:     "is_active",
															TypeName: string(DataTypeBool),
															JSONPath: "isActive",
															Optional: true,
														},
														{
															Name:     "search_term",
															TypeName: string(DataTypeString),
															JSONPath: "searchTerm",
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
						Response: RPCMessage{
							Name: "ResolveSubcategoryItemCountResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountResult",
										Fields: []RPCField{
											{
												Name:     "item_count",
												TypeName: string(DataTypeInt32),
												JSONPath: "itemCount",
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
			name:  "Should create an execution plan for a query a field resolver with a message type",
			query: "query CategoriesWithNullableTypes($metricType: String) { categories { categoryMetrics(metricType: $metricType) { id metricType value } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
									Repeated: true,
									Message: &RPCMessage{
										Name:   "Category",
										Fields: []RPCField{},
									},
								},
							},
						},
					},
					{
						DependentCalls: []int{0},
						Kind:           CallKindResolve,
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryCategoryMetrics",
						ResponsePath:   buildPath("categories.categoryMetrics"),
						Request: RPCMessage{
							Name: "ResolveCategoryCategoryMetricsRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
											{
												Name:        "name",
												TypeName:    string(DataTypeString),
												JSONPath:    "name",
												ResolvePath: buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsArgs",
										Fields: []RPCField{
											{
												Name:     "metric_type",
												TypeName: string(DataTypeString),
												JSONPath: "metricType",
												Optional: false,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveCategoryCategoryMetricsResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsResult",
										Fields: []RPCField{
											{
												Name:     "category_metrics",
												TypeName: string(DataTypeMessage),
												JSONPath: "categoryMetrics",
												Optional: true,
												Message: &RPCMessage{
													Name: "CategoryMetrics",
													Fields: []RPCField{
														{
															Name:     "id",
															TypeName: string(DataTypeString),
															JSONPath: "id",
														},
														{
															Name:     "metric_type",
															TypeName: string(DataTypeString),
															JSONPath: "metricType",
														},
														{
															Name:     "value",
															TypeName: string(DataTypeDouble),
															JSONPath: "value",
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
			name:  "Should create an execution plan for a query with nullable types",
			query: "query CategoriesWithNullableTypes($threshold: Int, $metricType: String) { categories { popularityScore(threshold: $threshold) categoryMetrics(metricType: $metricType) { id metricType value } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
									Repeated: true,
									Message: &RPCMessage{
										Name:   "Category",
										Fields: []RPCField{},
									},
								},
							},
						},
					},
					{
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryPopularityScore",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.popularityScore"),
						Request: RPCMessage{
							Name: "ResolveCategoryPopularityScoreRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreArgs",
										Fields: []RPCField{
											{
												Name:     "threshold",
												TypeName: string(DataTypeInt32),
												JSONPath: "threshold",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveCategoryPopularityScoreResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreResult",
										Fields: []RPCField{
											{
												Name:     "popularity_score",
												TypeName: string(DataTypeInt32),
												JSONPath: "popularityScore",
												Optional: true,
											},
										},
									},
								},
							},
						},
					},
					{
						DependentCalls: []int{0},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryCategoryMetrics",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.categoryMetrics"),
						Request: RPCMessage{
							Name: "ResolveCategoryCategoryMetricsRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("categories.id"),
											},
											{
												Name:        "name",
												TypeName:    string(DataTypeString),
												JSONPath:    "name",
												ResolvePath: buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsArgs",
										Fields: []RPCField{
											{
												Name:     "metric_type",
												TypeName: string(DataTypeString),
												JSONPath: "metricType",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveCategoryCategoryMetricsResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsResult",
										Fields: []RPCField{
											{
												Name:     "category_metrics",
												TypeName: string(DataTypeMessage),
												JSONPath: "categoryMetrics",
												Optional: true,
												Message: &RPCMessage{
													Name: "CategoryMetrics",
													Fields: []RPCField{
														{
															Name:     "id",
															TypeName: string(DataTypeString),
															JSONPath: "id",
														},
														{
															Name:     "metric_type",
															TypeName: string(DataTypeString),
															JSONPath: "metricType",
														},
														{
															Name:     "value",
															TypeName: string(DataTypeDouble),
															JSONPath: "value",
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
