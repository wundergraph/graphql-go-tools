package grpcdatasource

import (
	"testing"
)

func TestExecutionPlanFieldResolvers(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with nullable fields type",
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
						Request: RPCMessage{
							Name: "ResolveCategoryProductCountRequest",
							Fields: []RPCField{
								{
									Name:     "key",
									TypeName: string(DataTypeMessage),
									JSONPath: "key",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveCategoryProductCountRequestKey",
										Fields: []RPCField{
											{
												Name:     "context",
												TypeName: string(DataTypeMessage),
												JSONPath: "",
												Message: &RPCMessage{
													Name: "CategoryProductCountContext",
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
													Name: "CategoryProductCountArgs",
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
										Name: "ResolveCategoryProductCountResponseResult",
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
