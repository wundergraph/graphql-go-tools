package grpcdatasource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"

	"github.com/google/go-cmp/cmp"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
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

	walker := astvisitor.NewWalker(48)

	rpcPlanVisitor := newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
		subgraphName: "Products",
		mapping:      testMapping(),
	})

	walker.Walk(&queryDoc, &schemaDoc, &report)

	if report.HasErrors() {
		require.NotEmpty(t, testCase.expectedError)
		require.Contains(t, report.Error(), testCase.expectedError)
		return
	}

	require.Empty(t, testCase.expectedError)
	diff := cmp.Diff(testCase.expectedPlan, rpcPlanVisitor.plan)
	if diff != "" {
		t.Fatalf("execution plan mismatch: %s", diff)
	}
}

func TestEntityLookup(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedPlan *RPCExecutionPlan
		mapping      *GRPCMapping
	}{
		{
			name:  "Should create an execution plan for an entity lookup with one key field",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Product { __typename id name price } } }`,
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string]EntityRPCConfig{
					"Product": {
						Key: "id",
						RPCConfig: RPCConfig{
							RPC:      "LookupProductById",
							Request:  "LookupProductByIdRequest",
							Response: "LookupProductByIdResponse",
						},
					},
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						// Define the structure of the request message
						Request: RPCMessage{
							Name: "LookupProductByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name: "LookupProductByIdKey",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
										},
									},
								},
							},
						},
						// Define the structure of the response message
						Response: RPCMessage{
							Name: "LookupProductByIdResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:        "__typename",
												TypeName:    string(DataTypeString),
												JSONPath:    "__typename",
												StaticValue: "Product",
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
											{
												Name:     "price",
												TypeName: string(DataTypeDouble),
												JSONPath: "price",
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

		// TODO implement multiple entity lookup types
		// 		{
		// 			name: "Should create an execution plan for an entity lookup multiple types",
		// 			query: `
		// query EntityLookup($representations: [_Any!]!) {
		// 	_entities(representations: $representations) {
		// 		... on Product {
		// 			id
		// 			name
		// 			price
		// 		}
		// 		... on Storage {
		// 			id
		// 			name
		// 			location
		// 		}
		// 	}
		// }
		// `,
		// 			expectedPlan: &RPCExecutionPlan{
		// 				Groups: []RPCCallGroup{
		// 					{
		// 						Calls: []RPCCall{
		// 							{
		// 								ServiceName: "Products",
		// 								MethodName:  "LookupProductById",
		// 								// Define the structure of the request message
		// 								Request: RPCMessage{
		// 									Name: "LookupProductByIdRequest",
		// 									Fields: []RPCField{
		// 										{
		// 											Name:     "inputs",
		// 											TypeName: string(DataTypeMessage),
		// 											Repeated: true,
		// 											JSONPath: "representations", // Path to extract data from GraphQL variables
		//

		// 											Message: &RPCMessage{
		// 												Name: "LookupProductByIdInput",
		// 												Fields: []RPCField{
		// 													{
		// 														Name:     "key",
		// 														TypeName: string(DataTypeMessage),
		//

		// 														Message: &RPCMessage{
		// 															Name: "ProductByIdKey",
		// 															Fields: []RPCField{
		// 																{
		// 																	Name:     "id",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "id", // Extract 'id' from each representation
		//

		// 																},
		// 															},
		// 														},
		// 													},
		// 												},
		// 											},
		// 										},
		// 									},
		// 								},
		// 								// Define the structure of the response message
		// 								Response: RPCMessage{
		// 									Name: "LookupProductByIdResponse",
		// 									Fields: []RPCField{
		// 										{
		// 											Name:     "results",
		// 											TypeName: string(DataTypeMessage),
		// 											Repeated: true,
		//

		// 											JSONPath: "results",
		// 											Message: &RPCMessage{
		// 												Name: "LookupProductByIdResult",
		// 												Fields: []RPCField{
		// 													{
		// 														Name:     "product",
		// 														TypeName: string(DataTypeMessage),
		//

		// 														Message: &RPCMessage{
		// 															Name: "Product",
		// 															Fields: []RPCField{
		// 																{
		// 																	Name:     "id",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "id",
		// 																},
		// 																{
		// 																	Name:     "name",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "name",
		// 																},
		// 																{
		// 																	Name:     "price",
		// 																	TypeName: string(DataTypeFloat),
		// 																	JSONPath: "price",
		// 																},
		// 															},
		// 														},
		// 													},
		// 												},
		// 											},
		// 										},
		// 									},
		// 								},
		// 							},
		// 							{
		// 								ServiceName: "Products",
		// 								MethodName:  "LookupStorageById",
		// 								Request: RPCMessage{
		// 									Name: "LookupStorageByIdRequest",
		// 									Fields: []RPCField{
		// 										{
		// 											Name:     "inputs",
		// 											TypeName: string(DataTypeMessage),
		// 											Repeated: true,
		// 											JSONPath: "representations",
		// 											Message: &RPCMessage{
		// 												Name: "LookupStorageByIdInput",
		// 												Fields: []RPCField{
		// 													{
		// 														Name:     "key",
		// 														TypeName: string(DataTypeMessage),
		// 														Message: &RPCMessage{
		// 															Name: "StorageByIdKey",
		// 															Fields: []RPCField{
		// 																{
		// 																	Name:     "id",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "id",
		// 																},
		// 															},
		// 														},
		// 													},
		// 												},
		// 											},
		// 										},
		// 									},
		// 								},
		// 								Response: RPCMessage{
		// 									Name: "LookupStorageByIdResponse",
		// 									Fields: []RPCField{
		// 										{
		// 											Name:     "results",
		// 											TypeName: string(DataTypeMessage),
		// 											Repeated: true,
		// 											JSONPath: "results",
		// 											Message: &RPCMessage{
		// 												Name: "LookupStorageByIdResult",
		// 												Fields: []RPCField{
		// 													{
		// 														Name:     "storage",
		// 														TypeName: string(DataTypeMessage),
		// 														Message: &RPCMessage{
		// 															Name: "Storage",
		// 															Fields: []RPCField{
		// 																{
		// 																	Name:     "id",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "id",
		// 																},
		// 																{
		// 																	Name:     "name",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "name",
		// 																},
		// 																{
		// 																	Name:     "location",
		// 																	TypeName: string(DataTypeString),
		// 																	JSONPath: "location",
		// 																},
		// 															},
		// 														},
		// 													},
		// 												},
		// 											},
		// 										},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
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

			walker := astvisitor.NewWalker(48)

			rpcPlanVisitor := newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      tt.mapping,
			})

			walker.Walk(&queryDoc, &schemaDoc, &report)

			if report.HasErrors() {
				t.Fatalf("failed to walk AST: %s", report.Error())
			}

			diff := cmp.Diff(tt.expectedPlan, rpcPlanVisitor.plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestQueryExecutionPlans(t *testing.T) {
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
						CallID:      1,
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
			name:  "Should create an execution plan for a query with a complex input type and variables",
			query: `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
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
															Name:     "filterField1",
															TypeName: string(DataTypeString),
															JSONPath: "filterField1",
														},
														{
															Name:     "filterField2",
															TypeName: string(DataTypeString),
															JSONPath: "filterField2",
														},
														{
															Name:     "pagination",
															TypeName: string(DataTypeMessage),
															JSONPath: "pagination",
															Message: &RPCMessage{
																Name: "Pagination",
																Fields: []RPCField{
																	{
																		Name:     "page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "page",
																	},
																	{
																		Name:     "perPage",
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
									Name:     "complexFilterType",
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
			name:  "Should create an execution plan for a query with a complex input type and variables with different name",
			query: `query ComplexFilterTypeQuery($foobar: ComplexFilterTypeInput!) { complexFilterType(filter: $foobar) { id name } }`,
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
															Name:     "filterField1",
															TypeName: string(DataTypeString),
															JSONPath: "filterField1",
														},
														{
															Name:     "filterField2",
															TypeName: string(DataTypeString),
															JSONPath: "filterField2",
														},
														{
															Name:     "pagination",
															TypeName: string(DataTypeMessage),
															JSONPath: "pagination",
															Message: &RPCMessage{
																Name: "Pagination",
																Fields: []RPCField{
																	{
																		Name:     "page",
																		TypeName: string(DataTypeInt32),
																		JSONPath: "page",
																	},
																	{
																		Name:     "perPage",
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
									Name:     "complexFilterType",
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
			name:  "Should create an execution plan for a query with a type filter with arguments and variables",
			query: "query TypeWithMultipleFilterFieldsQuery($filter: FilterTypeInput!) { typeWithMultipleFilterFields(filter: $filter) { id name } }",
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
												Name:     "filterField1",
												TypeName: string(DataTypeString),
												JSONPath: "filterField1",
											},
											{
												Repeated: false,
												Name:     "filterField2",
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
									Name:     "typeWithMultipleFilterFields",
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
			name:  "Should create an execution plan for a query",
			query: "query UserQuery { users { id name } }",
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
			name:  "Should create an execution plan for a query with a user",
			query: `query UserQuery { user(id: "abc123") { id name } }`,
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
			name:  "Should create an execution plan for a query with a nested type",
			query: "query NestedTypeQuery { nestedType { id name b { id name c { id name } } } }",
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
									Name:     "nestedType",
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
			name:  "Should create an execution plan for a query with a recursive type",
			query: "query RecursiveTypeQuery { recursiveType { id name recursiveType { id recursiveType { id name recursiveType { id name } } name } } }",
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
									Name:     "recursiveType",
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
												Name:     "recursiveType",
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
															Name:     "recursiveType",
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
																		Name:     "recursiveType",
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
															Name:     "modifiers",
															TypeName: string(DataTypeMessage),
															Repeated: false,
															Optional: true,
															Message: &RPCMessage{
																Name: "ListOfString",
																Fields: []RPCField{
																	{
																		Name:     "items",
																		TypeName: string(DataTypeString),
																		Repeated: true,
																		JSONPath: "modifiers",
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

			walker := astvisitor.NewWalker(48)

			rpcPlanVisitor := newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      tt.mapping,
			})

			walker.Walk(&queryDoc, &schemaDoc, &report)

			if report.HasErrors() {
				require.Contains(t, report.Error(), tt.expectedError)
				require.NotEmpty(t, tt.expectedError)
				return
			}

			require.Empty(t, tt.expectedError)
			diff := cmp.Diff(tt.expectedPlan, rpcPlanVisitor.plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestCompositeTypeExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with a random cat",
			query: "query RandomCatQuery { randomPet { id name kind ... on Cat { meowVolume } } }",
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
							Fields: []RPCField{
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
												},
											},
										},
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
			name:  "Should create an execution plan for a query with a random cat and dog",
			query: "query CatAndDogQuery { randomPet { id name kind ... on Cat { meowVolume } ... on Dog { barkVolume } } }",
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
							Fields: []RPCField{
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
												},
											},
											"Dog": {
												{
													Name:     "bark_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "barkVolume",
												},
											},
										},
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
			name:  "Should create an execution plan for a query with all pets (interface list)",
			query: "query AllPetsQuery { allPets { id name kind ... on Cat { meowVolume } ... on Dog { barkVolume } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryAllPets",
						Request: RPCMessage{
							Name: "QueryAllPetsRequest",
						},
						Response: RPCMessage{
							Name: "QueryAllPetsResponse",
							Fields: []RPCField{
								{
									Name:     "all_pets",
									TypeName: string(DataTypeMessage),
									JSONPath: "allPets",
									Repeated: true,
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
												},
											},
											"Dog": {
												{
													Name:     "bark_volume",
													TypeName: string(DataTypeInt32),
													JSONPath: "barkVolume",
												},
											},
										},
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
			name:  "Should create an execution plan for a query with all pets using an interface fragment",
			query: "query AllPetsQuery { allPets { ... on Animal { id name kind } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryAllPets",
						Request: RPCMessage{
							Name: "QueryAllPetsRequest",
						},
						Response: RPCMessage{
							Name: "QueryAllPetsResponse",
							Fields: []RPCField{
								{
									Name:     "all_pets",
									TypeName: string(DataTypeMessage),
									JSONPath: "allPets",
									Repeated: true,
									Message: &RPCMessage{
										Name:      "Animal",
										OneOfType: OneOfTypeInterface,
										MemberTypes: []string{
											"Cat",
											"Dog",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"Animal": {
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
		},
		{
			name:  "Should create an execution plan for a query with interface selecting only common fields",
			query: "query CommonFieldsQuery { randomPet { id name kind } }",
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
							Fields: []RPCField{
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
			name:  "Should create an execution plan for a SearchResult union query",
			query: "query SearchQuery($input: SearchInput!) { search(input: $input) { ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QuerySearch",
						Request: RPCMessage{
							Name: "QuerySearchRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "SearchInput",
										Fields: []RPCField{
											{
												Name:     "query",
												TypeName: string(DataTypeString),
												JSONPath: "query",
											},
											{
												Name:     "limit",
												TypeName: string(DataTypeInt32),
												JSONPath: "limit",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QuerySearchResponse",
							Fields: []RPCField{
								{
									Name:     "search",
									TypeName: string(DataTypeMessage),
									JSONPath: "search",
									Repeated: true,
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
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
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
		},
		{
			name:  "Should create an execution plan for a single SearchResult union query",
			query: "query RandomSearchQuery { randomSearchResult { ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }",
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
							Fields: []RPCField{
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
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
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
		},
		{
			name:  "Should create an execution plan for a SearchResult union with partial selection",
			query: "query PartialSearchQuery($input: SearchInput!) { search(input: $input) { ... on Product { id name } ... on User { id name } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QuerySearch",
						Request: RPCMessage{
							Name: "QuerySearchRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "SearchInput",
										Fields: []RPCField{
											{
												Name:     "query",
												TypeName: string(DataTypeString),
												JSONPath: "query",
											},
											{
												Name:     "limit",
												TypeName: string(DataTypeInt32),
												JSONPath: "limit",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QuerySearchResponse",
							Fields: []RPCField{
								{
									Name:     "search",
									TypeName: string(DataTypeMessage),
									JSONPath: "search",
									Repeated: true,
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
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tt.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			walker := astvisitor.NewWalker(48)

			rpcPlanVisitor := newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      testMapping(),
			})

			walker.Walk(&queryDoc, &schemaDoc, &report)

			if report.HasErrors() {
				require.NotEmpty(t, tt.expectedError)
				require.Contains(t, report.Error(), tt.expectedError)
				return
			}

			require.Empty(t, tt.expectedError)
			diff := cmp.Diff(tt.expectedPlan, rpcPlanVisitor.plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestMutationUnionExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for ActionResult union mutation",
			query: "mutation PerformActionMutation($input: ActionInput!) { performAction(input: $input) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationPerformAction",
						Request: RPCMessage{
							Name: "MutationPerformActionRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "ActionInput",
										Fields: []RPCField{
											{
												Name:     "type",
												TypeName: string(DataTypeString),
												JSONPath: "type",
											},
											{
												Name:     "payload",
												TypeName: string(DataTypeString),
												JSONPath: "payload",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationPerformActionResponse",
							Fields: []RPCField{
								{
									Name:     "perform_action",
									TypeName: string(DataTypeMessage),
									JSONPath: "performAction",
									Message: &RPCMessage{
										Name:      "ActionResult",
										OneOfType: OneOfTypeUnion,
										MemberTypes: []string{
											"ActionSuccess",
											"ActionError",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"ActionSuccess": {
												{
													Name:     "message",
													TypeName: string(DataTypeString),
													JSONPath: "message",
												},
												{
													Name:     "timestamp",
													TypeName: string(DataTypeString),
													JSONPath: "timestamp",
												},
											},
											"ActionError": {
												{
													Name:     "message",
													TypeName: string(DataTypeString),
													JSONPath: "message",
												},
												{
													Name:     "code",
													TypeName: string(DataTypeString),
													JSONPath: "code",
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
			name:  "Should create an execution plan for ActionResult union with only success case",
			query: "mutation PerformSuccessActionMutation($input: ActionInput!) { performAction(input: $input) { ... on ActionSuccess { message timestamp } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationPerformAction",
						Request: RPCMessage{
							Name: "MutationPerformActionRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "ActionInput",
										Fields: []RPCField{
											{
												Name:     "type",
												TypeName: string(DataTypeString),
												JSONPath: "type",
											},
											{
												Name:     "payload",
												TypeName: string(DataTypeString),
												JSONPath: "payload",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationPerformActionResponse",
							Fields: []RPCField{
								{
									Name:     "perform_action",
									TypeName: string(DataTypeMessage),
									JSONPath: "performAction",
									Message: &RPCMessage{
										Name:      "ActionResult",
										OneOfType: OneOfTypeUnion,
										MemberTypes: []string{
											"ActionSuccess",
											"ActionError",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"ActionSuccess": {
												{
													Name:     "message",
													TypeName: string(DataTypeString),
													JSONPath: "message",
												},
												{
													Name:     "timestamp",
													TypeName: string(DataTypeString),
													JSONPath: "timestamp",
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
			name:  "Should create an execution plan for ActionResult union with only error case",
			query: "mutation PerformErrorActionMutation($input: ActionInput!) { performAction(input: $input) { ... on ActionError { message code } } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationPerformAction",
						Request: RPCMessage{
							Name: "MutationPerformActionRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "ActionInput",
										Fields: []RPCField{
											{
												Name:     "type",
												TypeName: string(DataTypeString),
												JSONPath: "type",
											},
											{
												Name:     "payload",
												TypeName: string(DataTypeString),
												JSONPath: "payload",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationPerformActionResponse",
							Fields: []RPCField{
								{
									Name:     "perform_action",
									TypeName: string(DataTypeMessage),
									JSONPath: "performAction",
									Message: &RPCMessage{
										Name:      "ActionResult",
										OneOfType: OneOfTypeUnion,
										MemberTypes: []string{
											"ActionSuccess",
											"ActionError",
										},
										Fields: RPCFields{},
										FieldSelectionSet: RPCFieldSelectionSet{
											"ActionError": {
												{
													Name:     "message",
													TypeName: string(DataTypeString),
													JSONPath: "message",
												},
												{
													Name:     "code",
													TypeName: string(DataTypeString),
													JSONPath: "code",
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
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tt.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			walker := astvisitor.NewWalker(48)

			rpcPlanVisitor := newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      testMapping(),
			})

			walker.Walk(&queryDoc, &schemaDoc, &report)

			if report.HasErrors() {
				require.NotEmpty(t, tt.expectedError)
				require.Contains(t, report.Error(), tt.expectedError)
				return
			}

			require.Empty(t, tt.expectedError)
			diff := cmp.Diff(tt.expectedPlan, rpcPlanVisitor.plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestProductExecutionPlan(t *testing.T) {
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

			planner := NewPlanner("Products", testMapping())
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
						CallID:      1,
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
						CallID:      1,
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
						CallID:      2,
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

			planner := NewPlanner("Products", testMapping())
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

func TestNullableFieldsExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with nullable fields type",
			query: "query NullableFieldsTypeQuery { nullableFieldsType { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:     "nullable_fields_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
											},
											{
												Name:     "required_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "requiredInt",
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
			name:  "Should create an execution plan for a query with nullable fields in the request",
			query: `query NullableFieldsTypeWithFilterQuery($filter: NullableFieldsFilter!) { nullableFieldsTypeWithFilter(filter: $filter) { id name optionalString optionalInt optionalFloat optionalBoolean } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsTypeWithFilter",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeWithFilterRequest",
							Fields: []RPCField{
								{
									Name:     "filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "filter",
									Message: &RPCMessage{
										Name: "NullableFieldsFilter",
										Fields: []RPCField{
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
												Optional: true,
											},
											{
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "include_nulls",
												TypeName: string(DataTypeBool),
												JSONPath: "includeNulls",
												Optional: true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeWithFilterResponse",
							Fields: []RPCField{
								{
									Name:     "nullable_fields_type_with_filter",
									TypeName: string(DataTypeMessage),
									JSONPath: "nullableFieldsTypeWithFilter",
									Repeated: true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
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
		{
			name:  "Should create an execution plan for nullable fields type by ID query",
			query: `query NullableFieldsTypeByIdQuery($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString requiredString } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsTypeById",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeByIdRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
							},
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeByIdResponse",
							Fields: []RPCField{
								{
									Name:     "nullable_fields_type_by_id",
									TypeName: string(DataTypeMessage),
									JSONPath: "nullableFieldsTypeById",
									Optional: true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
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
			name:  "Should create an execution plan for all nullable fields types query",
			query: "query AllNullableFieldsTypesQuery { allNullableFieldsTypes { id name optionalString optionalInt requiredString requiredInt } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryAllNullableFieldsTypes",
						Request: RPCMessage{
							Name: "QueryAllNullableFieldsTypesRequest",
						},
						Response: RPCMessage{
							Name: "QueryAllNullableFieldsTypesResponse",
							Fields: []RPCField{
								{
									Name:     "all_nullable_fields_types",
									TypeName: string(DataTypeMessage),
									JSONPath: "allNullableFieldsTypes",
									Repeated: true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
											},
											{
												Name:     "required_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "requiredInt",
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
			name:  "Should create an execution plan for create nullable fields type mutation",
			query: `mutation CreateNullableFieldsType($input: NullableFieldsInput!) { createNullableFieldsType(input: $input) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationCreateNullableFieldsType",
						Request: RPCMessage{
							Name: "MutationCreateNullableFieldsTypeRequest",
							Fields: []RPCField{
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "NullableFieldsInput",
										Fields: []RPCField{
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
											},
											{
												Name:     "required_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "requiredInt",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationCreateNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:     "create_nullable_fields_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "createNullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
											},
											{
												Name:     "required_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "requiredInt",
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
			name:  "Should create an execution plan for update nullable fields type mutation",
			query: `mutation UpdateNullableFieldsType($id: ID!, $input: NullableFieldsInput!) { updateNullableFieldsType(id: $id, input: $input) { id name optionalString requiredString } }`,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "MutationUpdateNullableFieldsType",
						Request: RPCMessage{
							Name: "MutationUpdateNullableFieldsTypeRequest",
							Fields: []RPCField{
								{
									Name:     "id",
									TypeName: string(DataTypeString),
									JSONPath: "id",
								},
								{
									Name:     "input",
									TypeName: string(DataTypeMessage),
									JSONPath: "input",
									Message: &RPCMessage{
										Name: "NullableFieldsInput",
										Fields: []RPCField{
											{
												Name:     "name",
												TypeName: string(DataTypeString),
												JSONPath: "name",
											},
											{
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
											},
											{
												Name:     "required_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "requiredInt",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "MutationUpdateNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:     "update_nullable_fields_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "updateNullableFieldsType",
									Optional: true,
									Message: &RPCMessage{
										Name: "NullableFieldsType",
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
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "required_string",
												TypeName: string(DataTypeString),
												JSONPath: "requiredString",
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
			name:  "Should create an execution plan for nullable fields with partial field selection",
			query: "query PartialNullableFieldsQuery { nullableFieldsType { id optionalString } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:     "nullable_fields_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
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
		{
			name:  "Should create an execution plan for nullable fields with only optional fields",
			query: "query OptionalFieldsOnlyQuery { nullableFieldsType { optionalString optionalInt optionalFloat optionalBoolean } }",
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "QueryNullableFieldsType",
						Request: RPCMessage{
							Name: "QueryNullableFieldsTypeRequest",
						},
						Response: RPCMessage{
							Name: "QueryNullableFieldsTypeResponse",
							Fields: []RPCField{
								{
									Name:     "nullable_fields_type",
									TypeName: string(DataTypeMessage),
									JSONPath: "nullableFieldsType",
									Message: &RPCMessage{
										Name: "NullableFieldsType",
										Fields: []RPCField{
											{
												Name:     "optional_string",
												TypeName: string(DataTypeString),
												JSONPath: "optionalString",
												Optional: true,
											},
											{
												Name:     "optional_int",
												TypeName: string(DataTypeInt32),
												JSONPath: "optionalInt",
												Optional: true,
											},
											{
												Name:     "optional_float",
												TypeName: string(DataTypeDouble),
												JSONPath: "optionalFloat",
												Optional: true,
											},
											{
												Name:     "optional_boolean",
												TypeName: string(DataTypeBool),
												JSONPath: "optionalBoolean",
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
