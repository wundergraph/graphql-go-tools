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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:          "filters",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "whoop",
												Optional:      true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:          "min_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "minPrice",
															Optional:      true,
														},
														{
															Name:          "max_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "maxPrice",
															Optional:      true,
														},
														{
															Name:          "in_stock",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "inStock",
															Optional:      true,
														},
														{
															Name:          "search_term",
															ProtoTypeName: DataTypeString,
															JSONPath:      "searchTerm",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:          "product_count",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "productCount",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
									Repeated:      true,
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:          "filters",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "p1",
												Optional:      true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:          "min_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "minPrice",
															Optional:      true,
														},
														{
															Name:          "max_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "maxPrice",
															Optional:      true,
														},
														{
															Name:          "in_stock",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "inStock",
															Optional:      true,
														},
														{
															Name:          "search_term",
															ProtoTypeName: DataTypeString,
															JSONPath:      "searchTerm",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:          "product_count",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "productCount1",
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountArgs",
										Fields: []RPCField{
											{
												Name:          "filters",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "p2",
												Optional:      true,
												Message: &RPCMessage{
													Name: "ProductCountFilter",
													Fields: []RPCField{
														{
															Name:          "min_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "minPrice",
															Optional:      true,
														},
														{
															Name:          "max_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "maxPrice",
															Optional:      true,
														},
														{
															Name:          "in_stock",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "inStock",
															Optional:      true,
														},
														{
															Name:          "search_term",
															ProtoTypeName: DataTypeString,
															JSONPath:      "searchTerm",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountResult",
										Fields: []RPCField{
											{
												Name:          "product_count",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "productCount2",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
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
												Name:          "subcategories",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "subcategories",
												Repeated:      false,
												IsListType:    true,
												Optional:      true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo:    []LevelInfo{{Optional: true}},
												},
												Message: &RPCMessage{
													Name: "Subcategory",
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
															Name:          "description",
															ProtoTypeName: DataTypeString,
															JSONPath:      "description",
															Optional:      true,
														},
														{
															Name:          "is_active",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "isActive",
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.@subcategories.id"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountArgs",
										Fields: []RPCField{
											{
												Name:          "filters",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "filter",
												Optional:      true,
												Message: &RPCMessage{
													Name: "SubcategoryItemFilter",
													Fields: []RPCField{
														{
															Name:          "min_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "minPrice",
															Optional:      true,
														},
														{
															Name:          "max_price",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "maxPrice",
															Optional:      true,
														},
														{
															Name:          "in_stock",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "inStock",
															Optional:      true,
														},
														{
															Name:          "is_active",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "isActive",
															Optional:      true,
														},
														{
															Name:          "search_term",
															ProtoTypeName: DataTypeString,
															JSONPath:      "searchTerm",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveSubcategoryItemCountResult",
										Fields: []RPCField{
											{
												Name:          "item_count",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "itemCount",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
									Repeated:      true,
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsArgs",
										Fields: []RPCField{
											{
												Name:          "metric_type",
												ProtoTypeName: DataTypeString,
												JSONPath:      "metricType",
												Optional:      false,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsResult",
										Fields: []RPCField{
											{
												Name:          "category_metrics",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "categoryMetrics",
												Optional:      true,
												Message: &RPCMessage{
													Name: "CategoryMetrics",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
														},
														{
															Name:          "metric_type",
															ProtoTypeName: DataTypeString,
															JSONPath:      "metricType",
														},
														{
															Name:          "value",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "value",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
									Repeated:      true,
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreArgs",
										Fields: []RPCField{
											{
												Name:          "threshold",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "threshold",
												Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryPopularityScoreResult",
										Fields: []RPCField{
											{
												Name:          "popularity_score",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "popularityScore",
												Optional:      true,
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
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsArgs",
										Fields: []RPCField{
											{
												Name:          "metric_type",
												ProtoTypeName: DataTypeString,
												JSONPath:      "metricType",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsResult",
										Fields: []RPCField{
											{
												Name:          "category_metrics",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "categoryMetrics",
												Optional:      true,
												Message: &RPCMessage{
													Name: "CategoryMetrics",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
														},
														{
															Name:          "metric_type",
															ProtoTypeName: DataTypeString,
															JSONPath:      "metricType",
														},
														{
															Name:          "value",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "value",
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

func TestExecutionPlanFieldResolvers_WithNestedResolvers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with nested field resolvers",
			query: "query CategoriesWithNestedResolvers($metricType: String, $baseline: Float!) { categories { categoryMetrics(metricType: $metricType) { id metricType value normalizedScore(baseline: $baseline) } } }",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
									Repeated:      true,
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
						MethodName:     "ResolveCategoryCategoryMetrics",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.categoryMetrics"),
						Request: RPCMessage{
							Name: "ResolveCategoryCategoryMetricsRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("categories.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsArgs",
										Fields: []RPCField{
											{
												Name:          "metric_type",
												ProtoTypeName: DataTypeString,
												JSONPath:      "metricType",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryCategoryMetricsResult",
										Fields: []RPCField{
											{
												Name:          "category_metrics",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "categoryMetrics",
												Optional:      true,
												Message: &RPCMessage{
													Name: "CategoryMetrics",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
														},
														{
															Name:          "metric_type",
															ProtoTypeName: DataTypeString,
															JSONPath:      "metricType",
														},
														{
															Name:          "value",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "value",
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
						DependentCalls: []int{1},
						ServiceName:    "Products",
						MethodName:     "ResolveCategoryMetricsNormalizedScore",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.categoryMetrics.normalizedScore"),
						Request: RPCMessage{
							Name: "ResolveCategoryMetricsNormalizedScoreRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryMetricsNormalizedScoreContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.categoryMetrics.id"),
											},
											{
												Name:          "value",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "value",
												ResolvePath:   buildPath("categories.categoryMetrics.value"),
											},
											{
												Name:          "metric_type",
												ProtoTypeName: DataTypeString,
												JSONPath:      "metricType",
												ResolvePath:   buildPath("categories.categoryMetrics.metricType"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									Message: &RPCMessage{
										Name: "ResolveCategoryMetricsNormalizedScoreArgs",
										Fields: []RPCField{
											{
												Name:          "baseline",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "baseline",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveCategoryMetricsNormalizedScoreResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryMetricsNormalizedScoreResult",
										Fields: []RPCField{
											{
												Name:          "normalized_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "normalizedScore",
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

func TestExecutionPlanFieldResolvers_WithOneOfTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with interface type",
			query: "query CategoriesWithNestedResolvers($includeValue: Boolean!) { categories { mascot(includeVolume: $includeVolume) { ... on Cat { name  } ... on Dog { name } } } }",
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
									Name:          "categories",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "categories",
									Repeated:      true,
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
						MethodName:     "ResolveCategoryMascot",
						Kind:           CallKindResolve,
						ResponsePath:   buildPath("categories.mascot"),
						Request: RPCMessage{
							Name: "ResolveCategoryMascotRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryMascotContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("categories.id"),
											},
											{
												Name:          "kind",
												ProtoTypeName: DataTypeEnum,
												JSONPath:      "kind",
												EnumName:      "CategoryKind",
												ResolvePath:   buildPath("categories.kind"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									Message: &RPCMessage{
										Name: "ResolveCategoryMascotArgs",
										Fields: []RPCField{
											{
												Name:          "include_volume",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "includeVolume",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveCategoryMascotResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveCategoryMascotResult",
										Fields: []RPCField{
											{
												Name:          "mascot",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "mascot",
												Optional:      true,
												Message: &RPCMessage{
													Name:        "Animal",
													OneOfType:   OneOfTypeInterface,
													MemberTypes: []string{"Cat", "Dog"},
													FieldSelectionSet: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "name",
																ProtoTypeName: DataTypeString,
																JSONPath:      "name",
															},
														},
														"Dog": {
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
