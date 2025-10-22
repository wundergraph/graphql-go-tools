package grpcdatasource

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type testCase struct {
	query         string
	expectedPlan  *RPCExecutionPlan
	expectedError string
}

func runTest(t *testing.T, testCase testCase) {
	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(testCase.query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName: "Products",
		mapping:      testMapping(),
	})

	plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)

	if err != nil {
		require.NotEmpty(t, testCase.expectedError, "expected error to be empty, got: %s", err.Error())
		require.Contains(t, err.Error(), testCase.expectedError, "expected error to contain: %s, got: %s", testCase.expectedError, err.Error())
		return
	}

	require.Empty(t, testCase.expectedError)
	diff := cmp.Diff(testCase.expectedPlan, plan)
	if diff != "" {
		t.Fatalf("execution plan mismatch: %s", diff)
	}
}

// buildPath builds a path from a string which is a dot-separated list of field names.
func buildPath(path string) ast.Path {
	b := make([]byte, len(path))
	copy(b, path)
	n := 1
	for i := 0; i < len(b); i++ {
		if b[i] == '.' {
			n++
		}
	}
	items := make([]ast.PathItem, n)
	start, seg := 0, 0
	for i := 0; i <= len(b); i++ {
		if i == len(b) || b[i] == '.' {
			items[seg] = ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: b[start:i],
			}
			seg++
			start = i + 1
		}
	}
	return items
}

func TestQueryExecutionPlans(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		mapping       *GRPCMapping
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:    "Should include typename when requested",
			query:   `query UsersWithTypename { users { __typename id name } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: []RPCField{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "users",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:        "__typename",
												TypeName:    string(DataTypeString),
												JSONPath:    "__typename",
												StaticValue: "User",
											},
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
			name: "Should allow multiple user calls in a single query",
			query: `query MultipleUserCalls { 
				users { id name }
				user(id: "1") { id name }
				}`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: []RPCField{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "users",
									Message: &RPCMessage{
										Name: "User",
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
										},
									},
								},
							},
						},
					},
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: []RPCField{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
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
			name:    "Should call query with two arguments and no variables and mapping for field names",
			query:   `query QueryWithTwoArguments { typeFilterWithArguments(filterField1: "test1", filterField2: "test2") { id name filterField1 filterField2 } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryTypeFilterWithArguments",
						Request: RPCMessage{
							Name: "QueryTypeFilterWithArgumentsRequest",
							Fields: []RPCField{
								{
									Name:     "filter_field_1",
									TypeName: string(DataTypeString),
									JSONPath: "filterField1",
								},
								{
									Name:     "filter_field_2",
									TypeName: string(DataTypeString),
									JSONPath: "filterField2",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryTypeFilterWithArgumentsResponse",
							Fields: []RPCField{
								{
									Name:     "type_filter_with_arguments",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "typeFilterWithArguments",
									Message: &RPCMessage{
										Name: "TypeWithMultipleFilterFields",
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
												Name:     "filter_field_1",
												TypeName: string(DataTypeString),
												JSONPath: "filterField1",
											},
											{
												Name:     "filter_field_2",
												TypeName: string(DataTypeString),
												JSONPath: "filterField2",
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
			name:    "Should create an execution plan for a query with a complex input type and no variables and mapping for field names",
			query:   `query ComplexFilterTypeQuery { complexFilterType(filter: { name: "test", filterField1: "test1", filterField2: "test2", pagination: { page: 1, perPage: 10 } }) { id name } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryComplexFilterType",
						Request: RPCMessage{
							Name: "QueryComplexFilterTypeRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "ComplexFilterTypeInput",
										Fields: []RPCField{
											{
												Name:     "filter",
												TypeName: string(DataTypeMessage),
												JSONPath: "filter",
												Message: &RPCMessage{
													Name: "FilterType",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "filter_field_1",
															TypeName: string(DataTypeString),
															JSONPath: "filterField1",
														},
														{
															Name:     "filter_field_2",
															TypeName: string(DataTypeString),
															JSONPath: "filterField2",
														},
														{
															Name:     "pagination",
															TypeName: string(DataTypeMessage),
															JSONPath: "pagination",
															Optional: true,
															Message: &RPCMessage{
																Name: "Pagination",
																Fields: []RPCField{
																	{
																		Name:     "page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "page",
																	},
																	{
																		Name:     "per_page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "perPage",
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
							Name: "QueryComplexFilterTypeResponse",
							Fields: []RPCField{
								{
									Repeated: true,
									Name:     "complex_filter_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "complexFilterType",
									Message: &RPCMessage{
										Name: "TypeWithComplexFilterInput",
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
			name:    "Should create an execution plan for a query with a complex input type and variables",
			query:   `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryComplexFilterType",
						Request: RPCMessage{
							Name: "QueryComplexFilterTypeRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "ComplexFilterTypeInput",
										Fields: []RPCField{
											{
												Name:     "filter",
												TypeName: string(DataTypeMessage),
												JSONPath: "filter",
												Message: &RPCMessage{
													Name: "FilterType",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "filter_field_1",
															TypeName: string(DataTypeString),
															JSONPath: "filterField1",
														},
														{
															Name:     "filter_field_2",
															TypeName: string(DataTypeString),
															JSONPath: "filterField2",
														},
														{
															Name:     "pagination",
															TypeName: string(DataTypeMessage),
															JSONPath: "pagination",
															Optional: true,
															Message: &RPCMessage{
																Name: "Pagination",
																Fields: []RPCField{
																	{
																		Name:     "page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "page",
																	},
																	{
																		Name:     "per_page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "perPage",
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
							Name: "QueryComplexFilterTypeResponse",
							Fields: []RPCField{
								{
									Repeated: true,
									Name:     "complex_filter_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "complexFilterType",
									Message: &RPCMessage{
										Name: "TypeWithComplexFilterInput",
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
			name:    "Should create an execution plan for a query with a complex input type and variables with different name",
			query:   `query ComplexFilterTypeQuery($foobar: ComplexFilterTypeInput!) { complexFilterType(filter: $foobar) { id name } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryComplexFilterType",
						Request: RPCMessage{
							Name: "QueryComplexFilterTypeRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "foobar",
									Message: &RPCMessage{
										Name: "ComplexFilterTypeInput",
										Fields: []RPCField{
											{
												Name:     "filter",
												TypeName: string(DataTypeMessage),
												JSONPath: "filter",
												Message: &RPCMessage{
													Name: "FilterType",
													Fields: []RPCField{
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
														},
														{
															Name:     "filter_field_1",
															TypeName: string(DataTypeString),
															JSONPath: "filterField1",
														},
														{
															Name:     "filter_field_2",
															TypeName: string(DataTypeString),
															JSONPath: "filterField2",
														},
														{
															Name:     "pagination",
															TypeName: string(DataTypeMessage),
															JSONPath: "pagination",
															Optional: true,
															Message: &RPCMessage{
																Name: "Pagination",
																Fields: []RPCField{
																	{
																		Name:     "page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "page",
																	},
																	{
																		Name:     "per_page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "perPage",
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
							Name: "QueryComplexFilterTypeResponse",
							Fields: []RPCField{
								{
									Repeated: true,
									Name:     "complex_filter_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "complexFilterType",
									Message: &RPCMessage{
										Name: "TypeWithComplexFilterInput",
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
			name:    "Should create an execution plan for a query with a type filter with arguments and variables",
			query:   "query TypeWithMultipleFilterFieldsQuery($filter: FilterTypeInput!) { typeWithMultipleFilterFields(filter: $filter) { id name } }",
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryTypeWithMultipleFilterFields",
						Request: RPCMessage{
							Name: "QueryTypeWithMultipleFilterFieldsRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "FilterTypeInput",
										Fields: []RPCField{
											{
												Repeated: false,
												Name:     "filter_field_1",
												TypeName: string(DataTypeString),
												JSONPath: "filterField1",
											},
											{
												Repeated: false,
												Name:     "filter_field_2",
												TypeName: string(DataTypeString),
												JSONPath: "filterField2",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryTypeWithMultipleFilterFieldsResponse",
							Fields: []RPCField{
								{
									Name:     "type_with_multiple_filter_fields",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "typeWithMultipleFilterFields",
									Message: &RPCMessage{
										Name: "TypeWithMultipleFilterFields",
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
			name:    "Should create an execution plan for a query",
			query:   "query UserQuery { users { id name } }",
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: []RPCField{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "users",
									Message: &RPCMessage{
										Name: "User",
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
			name:  "Should stop when no mapping is found for the operation request",
			query: `query UserQuery { user(id: "1") { id name } }`,
			mapping: &GRPCMapping{
				QueryRPCs: map[string]RPCConfig{
					"user": {
						RPC:      "QueryUser",
						Request:  "",
						Response: "QueryUserResponse",
					},
				},
			},
			expectedError: "no request message name mapping found for operation user",
		},
		{
			name:    "Should create an execution plan for a query with a user",
			query:   `query UserQuery { user(id: "abc123") { id name } }`,
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: []RPCField{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
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
			name:    "Should create an execution plan for a query with a nested type",
			query:   "query NestedTypeQuery { nestedType { id name b { id name c { id name } } } }",
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNestedType",
						Request: RPCMessage{
							Name: "QueryNestedTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNestedTypeResponse",
							Fields: []RPCField{
								{
									Name:     "nested_type",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "nestedType",
									Message: &RPCMessage{
										Name: "NestedTypeA",
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
												Name:     "b",
												TypeName: string(DataTypeMessage),
												JSONPath: "b",
												Message: &RPCMessage{
													Name: "NestedTypeB",
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
															Name:     "c",
															TypeName: string(DataTypeMessage),
															JSONPath: "c",
															Message: &RPCMessage{
																Name: "NestedTypeC",
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
		{
			name:    "Should create an execution plan for a query with a recursive type",
			query:   "query RecursiveTypeQuery { recursiveType { id name recursiveType { id recursiveType { id name recursiveType { id name } } name } } }",
			mapping: testMapping(),
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryRecursiveType",
						Request: RPCMessage{
							Name: "QueryRecursiveTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryRecursiveTypeResponse",
							Fields: []RPCField{
								{
									Name:     "recursive_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "recursiveType",
									Message: &RPCMessage{
										Name: "RecursiveType",
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
												Name:     "recursive_type",
												TypeName: string(DataTypeMessage),
												JSONPath: "recursiveType",
												Message: &RPCMessage{
													Name: "RecursiveType",
													Fields: []RPCField{
														{
															Name:     "id",
															TypeName: string(DataTypeString),
															JSONPath: "id",
														},
														{
															Name:     "recursive_type",
															TypeName: string(DataTypeMessage),
															JSONPath: "recursiveType",
															Message: &RPCMessage{
																Name: "RecursiveType",
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
																		Name:     "recursive_type",
																		TypeName: string(DataTypeMessage),
																		JSONPath: "recursiveType",
																		Message: &RPCMessage{
																			Name: "RecursiveType",
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
																			},
																		},
																	},
																},
															},
														},
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
					},
				},
			},
		},
		{
			name: "Should query total orders by using input type with lists",
			query: `
			query CalculateTotalsQuery { 
				calculateTotals(
					orders: [
						{ orderId: "1", customerName: "John", lines: [{ productId: "1", quantity: 10 }, { productId: "2", quantity: 20 }] }, 
						{ orderId: "2", customerName: "Jane", lines: [{ productId: "3", quantity: 30 }, { productId: "4", quantity: 40 }] }
						]
					) { 
						orderId 
						customerName 
						totalItems 
					} 
				}`,
			mapping: &GRPCMapping{
				Service: "Products",
				QueryRPCs: map[string]RPCConfig{
					"calculateTotals": {
						RPC:      "QueryCalculateTotals",
						Request:  "QueryCalculateTotalsRequest",
						Response: "QueryCalculateTotalsResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"calculateTotals": {
							TargetName: "calculate_totals",
						},
					},
					"Order": {
						"orderId": {
							TargetName: "order_id",
						},
						"customerName": {
							TargetName: "customer_name",
						},
						"totalItems": {
							TargetName: "total_items",
						},
					},
					"OrderInput": {
						"orderId": {
							TargetName: "order_id",
						},
						"customerName": {
							TargetName: "customer_name",
						},
						"lines": {
							TargetName: "lines",
						},
					},
					"OrderLineInput": {
						"productId": {
							TargetName: "product_id",
						},
						"quantity": {
							TargetName: "quantity",
						},
						"modifiers": {
							TargetName: "modifiers",
						},
					},
				},
			},
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
															Repeated:   false,
															Optional:   true,
															IsListType: true,
															JSONPath:   "modifiers",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)
			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tt.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      tt.mapping,
			})

			plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)

			if err != nil {
				require.Contains(t, err.Error(), tt.expectedError)
				require.NotEmpty(t, tt.expectedError)
				return
			}

			require.Empty(t, tt.expectedError)
			diff := cmp.Diff(tt.expectedPlan, plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestProductExecutionPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with categories by kind",
			query: "query CategoriesQuery($kind: CategoryKind!) { categoriesByKind(kind: $kind) { id name kind } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategoriesByKind",
						Request: RPCMessage{
							Name: "QueryCategoriesByKindRequest",
							Fields: []RPCField{
								{
									Name:     "kind",
									TypeName: string(DataTypeEnum),
									JSONPath: "kind",
									EnumName: "CategoryKind",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryCategoriesByKindResponse",
							Fields: []RPCField{
								{
									Name:     "categories_by_kind",
									TypeName: string(DataTypeMessage),
									JSONPath: "categoriesByKind",
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
			name:  "Should create an execution plan for a query with categories by kinds",
			query: "query CategoriesQuery($kinds: [CategoryKind!]!) { categoriesByKinds(kinds: $kinds) { id name kind } }",
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
			name:  "Should create an execution plan for a query with filtered categories",
			query: "query FilterCategoriesQuery($filter: CategoryFilter!) { filterCategories(filter: $filter) { id name kind } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryFilterCategories",
						Request: RPCMessage{
							Name: "QueryFilterCategoriesRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "CategoryFilter",
										Fields: []RPCField{
											{
												Name:     "category",
												TypeName: string(DataTypeEnum),
												JSONPath: "category",
												EnumName: "CategoryKind",
											},
											{
												Name:     "pagination",
												TypeName: string(DataTypeMessage),
												JSONPath: "pagination",
												Optional: true,
												Message: &RPCMessage{
													Name: "Pagination",
													Fields: []RPCField{
														{
															Name:     "page",
															TypeName: string(DataTypeInt32),
															JSONPath: "page",
														},
														{
															Name:     "per_page",
															TypeName: string(DataTypeInt32),
															JSONPath: "perPage",
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
							Name: "QueryFilterCategoriesResponse",
							Fields: []RPCField{
								{
									Name:     "filter_categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "filterCategories",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := &operationreport.Report{}
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			astvalidation.DefaultDefinitionValidator().Validate(&schemaDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to validate schema: %s", report.Error())
			}

			// Parse the GraphQL query
			queryDoc, queryReport := astparser.ParseGraphqlDocumentString(tt.query)
			if queryReport.HasErrors() {
				t.Fatalf("failed to parse query: %s", queryReport.Error())
			}

			astvalidation.DefaultOperationValidator().Validate(&queryDoc, &schemaDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to validate query: %s", report.Error())
			}

			planner := NewPlanner("Products", testMapping(), nil)
			outPlan, err := planner.PlanOperation(&queryDoc, &schemaDoc)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			diff := cmp.Diff(tt.expectedPlan, outPlan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestProductExecutionPlanWithAliases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with an alias on the users root field",
			query: "query { foo: users { id name } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: RPCFields{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									JSONPath: "users",
									Alias:    "foo",
									Repeated: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
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
			name:  "Should create an execution plan for a query with an alias on a field with arguments",
			query: `query { specificUser: user(id: "123") { userId: id userName: name } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: RPCFields{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Alias:    "specificUser",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "userId",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "userName",
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
			name:  "Should create an execution plan for a query with multiple aliases on the same level",
			query: "query { allUsers: users { id name } allCategories: categories { id name categoryType: kind } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: RPCFields{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									JSONPath: "users",
									Alias:    "allUsers",
									Repeated: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
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
										},
									},
								},
							},
						},
					},
					{
						ServiceName: "Products",
						MethodName:  "QueryCategories",
						Request: RPCMessage{
							Name: "QueryCategoriesRequest",
						},
						Response: RPCMessage{
							Name: "QueryCategoriesResponse",
							Fields: RPCFields{
								{
									Name:     "categories",
									TypeName: string(DataTypeMessage),
									JSONPath: "categories",
									Alias:    "allCategories",
									Repeated: true,
									Message: &RPCMessage{
										Name: "Category",
										Fields: RPCFields{
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
												Alias:    "categoryType",
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
			name:  "Should create an execution plan for a query with aliases on nested object fields",
			query: "query { nestedData: nestedType { identifier: id title: name childB: b { identifier: id title: name grandChild: c { identifier: id title: name } } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNestedType",
						Request: RPCMessage{
							Name: "QueryNestedTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNestedTypeResponse",
							Fields: RPCFields{
								{
									Name:     "nested_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "nestedType",
									Alias:    "nestedData",
									Repeated: true,
									Message: &RPCMessage{
										Name: "NestedTypeA",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "identifier",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "title",
											},
											{
												Name:     "b",
												TypeName: string(DataTypeMessage),
												JSONPath: "b",
												Alias:    "childB",
												Message: &RPCMessage{
													Name: "NestedTypeB",
													Fields: RPCFields{
														{
															Name:     "id",
															TypeName: string(DataTypeString),
															JSONPath: "id",
															Alias:    "identifier",
														},
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
															Alias:    "title",
														},
														{
															Name:     "c",
															TypeName: string(DataTypeMessage),
															JSONPath: "c",
															Alias:    "grandChild",
															Message: &RPCMessage{
																Name: "NestedTypeC",
																Fields: RPCFields{
																	{
																		Name:     "id",
																		TypeName: string(DataTypeString),
																		JSONPath: "id",
																		Alias:    "identifier",
																	},
																	{
																		Name:     "name",
																		TypeName: string(DataTypeString),
																		JSONPath: "name",
																		Alias:    "title",
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
		{
			name:  "Should create an execution plan for a query with aliases on interface fields",
			query: "query { pet: randomPet { identifier: id petName: name animalKind: kind ... on Cat { volumeLevel: meowVolume } ... on Dog { volumeLevel: barkVolume } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryRandomPet",
						Request: RPCMessage{
							Name: "QueryRandomPetRequest",
						},
						Response: RPCMessage{
							Name: "QueryRandomPetResponse",
							Fields: RPCFields{
								{
									Name:     "random_pet",
									TypeName: string(DataTypeMessage),
									JSONPath: "randomPet",
									Alias:    "pet",
									Message: &RPCMessage{
										Name:      "Animal",
										OneOfType: OneOfTypeInterface,
										MemberTypes: []string{
											"Cat",
											"Dog",
										},
										FieldSelectionSet: RPCFieldSelectionSet{
											"Cat": {
												{
													Name:     "meow_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "meowVolume",
													Alias:    "volumeLevel",
												},
											},
											"Dog": {
												{
													Name:     "bark_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "barkVolume",
													Alias:    "volumeLevel",
												},
											},
										},
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "identifier",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "petName",
											},
											{
												Name:     "kind",
												TypeName: string(DataTypeString),
												JSONPath: "kind",
												Alias:    "animalKind",
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
			name:  "Should create an execution plan for a query with aliases on union type fields",
			query: "query { searchResults: randomSearchResult { ... on Product { productId: id productName: name cost: price } ... on User { userId: id userName: name } ... on Category { categoryId: id categoryName: name categoryType: kind } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryRandomSearchResult",
						Request: RPCMessage{
							Name: "QueryRandomSearchResultRequest",
						},
						Response: RPCMessage{
							Name: "QueryRandomSearchResultResponse",
							Fields: RPCFields{
								{
									Name:     "random_search_result",
									TypeName: string(DataTypeMessage),
									JSONPath: "randomSearchResult",
									Alias:    "searchResults",
									Message: &RPCMessage{
										Name:      "SearchResult",
										OneOfType: OneOfTypeUnion,
										MemberTypes: []string{
											"Product",
											"User",
											"Category",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"Product": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
													Alias:    "productId",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "productName",
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
													Alias:    "cost",
												},
											},
											"User": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
													Alias:    "userId",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "userName",
												},
											},
											"Category": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
													Alias:    "categoryId",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "categoryName",
												},
												{
													Name:     "kind",
													TypeName: string(DataTypeEnum),
													JSONPath: "kind",
													Alias:    "categoryType",
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
		},
		{
			name:  "Should create an execution plan for a mutation with aliases",
			query: `mutation { newUser: createUser(input: { name: "John Doe" }) { userId: id fullName: name } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationCreateUser",
						Request: RPCMessage{
							Name: "MutationCreateUserRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
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
						Response: RPCMessage{
							Name: "MutationCreateUserResponse",
							Fields: RPCFields{
								{
									Name:     "create_user",
									TypeName: string(DataTypeMessage),
									JSONPath: "createUser",
									Alias:    "newUser",
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "userId",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "fullName",
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
			name:  "Should create an execution plan for a query with aliases on field with complex input type",
			query: `query { bookCategories: categoriesByKind(kind: BOOK) { identifier: id title: name type: kind } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryCategoriesByKind",
						Request: RPCMessage{
							Name: "QueryCategoriesByKindRequest",
							Fields: []RPCField{
								{
									Name:     "kind",
									TypeName: string(DataTypeEnum),
									JSONPath: "kind",
									EnumName: "CategoryKind",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryCategoriesByKindResponse",
							Fields: RPCFields{
								{
									Name:     "categories_by_kind",
									TypeName: string(DataTypeMessage),
									JSONPath: "categoriesByKind",
									Alias:    "bookCategories",
									Repeated: true,
									Message: &RPCMessage{
										Name: "Category",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "identifier",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "title",
											},
											{
												Name:     "kind",
												TypeName: string(DataTypeEnum),
												JSONPath: "kind",
												Alias:    "type",
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
			name:  "Should create an execution plan for a query with multiple aliases for the same field",
			query: `query { users { id name1: name name2: name name3: name } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUsers",
						Request: RPCMessage{
							Name: "QueryUsersRequest",
						},
						Response: RPCMessage{
							Name: "QueryUsersResponse",
							Fields: RPCFields{
								{
									Name:     "users",
									TypeName: string(DataTypeMessage),
									JSONPath: "users",
									Repeated: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name1",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name2",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name3",
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
			name:  "Should create an execution plan for a query with multiple aliases for the same field with arguments",
			query: `query { user1: user(id: "123") { id name } user2: user(id: "456") { id name } sameUser: user(id: "123") { userId: id userName: name } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: RPCFields{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Alias:    "user1",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
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
										},
									},
								},
							},
						},
					},
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: RPCFields{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Alias:    "user2",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
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
										},
									},
								},
							},
						},
					},
					{
						ServiceName: "Products",
						MethodName:  "QueryUser",
						Request: RPCMessage{
							Name: "QueryUserRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryUserResponse",
							Fields: RPCFields{
								{
									Name:     "user",
									TypeName: string(DataTypeMessage),
									JSONPath: "user",
									Alias:    "sameUser",
									Optional: true,
									Message: &RPCMessage{
										Name: "User",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
												Alias:    "userId",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "userName",
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
			name:  "Should create an execution plan for a query with multiple aliases for the same field in nested objects",
			query: `query { nestedType { id name1: name name2: name b { id title1: name title2: name c { id label1: name label2: name } } } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNestedType",
						Request: RPCMessage{
							Name: "QueryNestedTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNestedTypeResponse",
							Fields: RPCFields{
								{
									Name:     "nested_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "nestedType",
									Repeated: true,
									Message: &RPCMessage{
										Name: "NestedTypeA",
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name1",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name2",
											},
											{
												Name:     "b",
												TypeName: string(DataTypeMessage),
												JSONPath: "b",
												Message: &RPCMessage{
													Name: "NestedTypeB",
													Fields: RPCFields{
														{
															Name:     "id",
															TypeName: string(DataTypeString),
															JSONPath: "id",
														},
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
															Alias:    "title1",
														},
														{
															Name:     "name",
															TypeName: string(DataTypeString),
															JSONPath: "name",
															Alias:    "title2",
														},
														{
															Name:     "c",
															TypeName: string(DataTypeMessage),
															JSONPath: "c",
															Message: &RPCMessage{
																Name: "NestedTypeC",
																Fields: RPCFields{
																	{
																		Name:     "id",
																		TypeName: string(DataTypeString),
																		JSONPath: "id",
																	},
																	{
																		Name:     "name",
																		TypeName: string(DataTypeString),
																		JSONPath: "name",
																		Alias:    "label1",
																	},
																	{
																		Name:     "name",
																		TypeName: string(DataTypeString),
																		JSONPath: "name",
																		Alias:    "label2",
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
		{
			name:  "Should create an execution plan for a query with multiple aliases for the same field in interface fragments",
			query: `query { randomPet { id name1: name name2: name kind ... on Cat { volume1: meowVolume volume2: meowVolume } ... on Dog { volume1: barkVolume volume2: barkVolume } } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryRandomPet",
						Request: RPCMessage{
							Name: "QueryRandomPetRequest",
						},
						Response: RPCMessage{
							Name: "QueryRandomPetResponse",
							Fields: RPCFields{
								{
									Name:     "random_pet",
									TypeName: string(DataTypeMessage),
									JSONPath: "randomPet",
									Message: &RPCMessage{
										Name:      "Animal",
										OneOfType: OneOfTypeInterface,
										MemberTypes: []string{
											"Cat",
											"Dog",
										},
										FieldSelectionSet: RPCFieldSelectionSet{
											"Cat": {
												{
													Name:     "meow_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "meowVolume",
													Alias:    "volume1",
												},
												{
													Name:     "meow_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "meowVolume",
													Alias:    "volume2",
												},
											},
											"Dog": {
												{
													Name:     "bark_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "barkVolume",
													Alias:    "volume1",
												},
												{
													Name:     "bark_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "barkVolume",
													Alias:    "volume2",
												},
											},
										},
										Fields: RPCFields{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name1",
											},
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Alias:    "name2",
											},
											{
												Name:     "kind",
												TypeName: string(DataTypeString),
												JSONPath: "kind",
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
			name:  "Should create an execution plan for a query with multiple aliases for the same field in union fragments",
			query: `query { randomSearchResult { ... on Product { id name1: name name2: name price1: price price2: price } ... on User { id name1: name name2: name } ... on Category { id name1: name name2: name kind1: kind kind2: kind } } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryRandomSearchResult",
						Request: RPCMessage{
							Name: "QueryRandomSearchResultRequest",
						},
						Response: RPCMessage{
							Name: "QueryRandomSearchResultResponse",
							Fields: RPCFields{
								{
									Name:     "random_search_result",
									TypeName: string(DataTypeMessage),
									JSONPath: "randomSearchResult",
									Message: &RPCMessage{
										Name:      "SearchResult",
										OneOfType: OneOfTypeUnion,
										MemberTypes: []string{
											"Product",
											"User",
											"Category",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"Product": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name1",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name2",
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
													Alias:    "price1",
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
													Alias:    "price2",
												},
											},
											"User": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name1",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name2",
												},
											},
											"Category": {
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name1",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Alias:    "name2",
												},
												{
													Name:     "kind",
													TypeName: string(DataTypeEnum),
													JSONPath: "kind",
													Alias:    "kind1",
													EnumName: "CategoryKind",
												},
												{
													Name:     "kind",
													TypeName: string(DataTypeEnum),
													JSONPath: "kind",
													Alias:    "kind2",
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := &operationreport.Report{}
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			astvalidation.DefaultDefinitionValidator().Validate(&schemaDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to validate schema: %s", report.Error())
			}

			// Parse the GraphQL query
			queryDoc, queryReport := astparser.ParseGraphqlDocumentString(tt.query)
			if queryReport.HasErrors() {
				t.Fatalf("failed to parse query: %s", queryReport.Error())
			}

			astvalidation.DefaultOperationValidator().Validate(&queryDoc, &schemaDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to validate query: %s", report.Error())
			}

			planner := NewPlanner("Products", testMapping(), nil)
			outPlan, err := planner.PlanOperation(&queryDoc, &schemaDoc)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			diff := cmp.Diff(tt.expectedPlan, outPlan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}

}
