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
															TypeName: string(DataTypeString),
															JSONPath: "modifiers",
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
											{
												Name:     "meow_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "meowVolume",
											},
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
											{
												Name:     "meow_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "meowVolume",
											},
											{
												Name:     "bark_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "barkVolume",
											},
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
			name:  "Should create an execution plan for a query with a union type",
			query: "query UnionQuery { randomPet { id name kind ... on Cat { meowVolume } ... on Dog { barkVolume } } }",
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
											{
												Name:     "meow_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "meowVolume",
											},
											{
												Name:     "bark_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "barkVolume",
											},
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
											{
												Name:     "meow_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "meowVolume",
											},
											{
												Name:     "bark_volume",
												TypeName: string(DataTypeInt32),
												JSONPath: "barkVolume",
											},
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
												Name:     "price",
												TypeName: string(DataTypeDouble),
												JSONPath: "price",
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
												Name:     "price",
												TypeName: string(DataTypeDouble),
												JSONPath: "price",
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
										Fields: []RPCField{
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
										Fields: []RPCField{
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
										Fields: []RPCField{
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

// TODO: Define test cases for product execution plans
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

func testMapping() *GRPCMapping {
	return &GRPCMapping{
		Service: "Products",
		QueryRPCs: map[string]RPCConfig{
			"users": {
				RPC:      "QueryUsers",
				Request:  "QueryUsersRequest",
				Response: "QueryUsersResponse",
			},
			"user": {
				RPC:      "QueryUser",
				Request:  "QueryUserRequest",
				Response: "QueryUserResponse",
			},
			"nestedType": {
				RPC:      "QueryNestedType",
				Request:  "QueryNestedTypeRequest",
				Response: "QueryNestedTypeResponse",
			},
			"recursiveType": {
				RPC:      "QueryRecursiveType",
				Request:  "QueryRecursiveTypeRequest",
				Response: "QueryRecursiveTypeResponse",
			},
			"typeFilterWithArguments": {
				RPC:      "QueryTypeFilterWithArguments",
				Request:  "QueryTypeFilterWithArgumentsRequest",
				Response: "QueryTypeFilterWithArgumentsResponse",
			},
			"typeWithMultipleFilterFields": {
				RPC:      "QueryTypeWithMultipleFilterFields",
				Request:  "QueryTypeWithMultipleFilterFieldsRequest",
				Response: "QueryTypeWithMultipleFilterFieldsResponse",
			},
			"complexFilterType": {
				RPC:      "QueryComplexFilterType",
				Request:  "QueryComplexFilterTypeRequest",
				Response: "QueryComplexFilterTypeResponse",
			},
			"calculateTotals": {
				RPC:      "QueryCalculateTotals",
				Request:  "QueryCalculateTotalsRequest",
				Response: "QueryCalculateTotalsResponse",
			},
			"randomPet": {
				RPC:      "QueryRandomPet",
				Request:  "QueryRandomPetRequest",
				Response: "QueryRandomPetResponse",
			},
			"allPets": {
				RPC:      "QueryAllPets",
				Request:  "QueryAllPetsRequest",
				Response: "QueryAllPetsResponse",
			},
			"categories": {
				RPC:      "QueryCategories",
				Request:  "QueryCategoriesRequest",
				Response: "QueryCategoriesResponse",
			},
			"categoriesByKind": {
				RPC:      "QueryCategoriesByKind",
				Request:  "QueryCategoriesByKindRequest",
				Response: "QueryCategoriesByKindResponse",
			},
			"categoriesByKinds": {
				RPC:      "QueryCategoriesByKinds",
				Request:  "QueryCategoriesByKindsRequest",
				Response: "QueryCategoriesByKindsResponse",
			},
			"filterCategories": {
				RPC:      "QueryFilterCategories",
				Request:  "QueryFilterCategoriesRequest",
				Response: "QueryFilterCategoriesResponse",
			},
			"randomSearchResult": {
				RPC:      "QueryRandomSearchResult",
				Request:  "QueryRandomSearchResultRequest",
				Response: "QueryRandomSearchResultResponse",
			},
			"search": {
				RPC:      "QuerySearch",
				Request:  "QuerySearchRequest",
				Response: "QuerySearchResponse",
			},
		},
		MutationRPCs: RPCConfigMap{
			"createUser": {
				RPC:      "CreateUser",
				Request:  "CreateUserRequest",
				Response: "CreateUserResponse",
			},
			"performAction": {
				RPC:      "MutationPerformAction",
				Request:  "MutationPerformActionRequest",
				Response: "MutationPerformActionResponse",
			},
		},
		SubscriptionRPCs: RPCConfigMap{},
		EntityRPCs: map[string]EntityRPCConfig{
			"Product": {
				Key: "id",
				RPCConfig: RPCConfig{
					RPC:      "LookupProductById",
					Request:  "LookupProductByIdRequest",
					Response: "LookupProductByIdResponse",
				},
			},
			"Storage": {
				Key: "id",
				RPCConfig: RPCConfig{
					RPC:      "LookupStorageById",
					Request:  "LookupStorageByIdRequest",
					Response: "LookupStorageByIdResponse",
				},
			},
		},
		EnumValues: map[string][]EnumValueMapping{
			"CategoryKind": {
				{Value: "BOOK", TargetValue: "CATEGORY_KIND_BOOK"},
				{Value: "ELECTRONICS", TargetValue: "CATEGORY_KIND_ELECTRONICS"},
				{Value: "FURNITURE", TargetValue: "CATEGORY_KIND_FURNITURE"},
				{Value: "OTHER", TargetValue: "CATEGORY_KIND_OTHER"},
			},
		},
		Fields: map[string]FieldMap{
			"Query": {
				"user": {
					TargetName: "user",
					ArgumentMappings: map[string]string{
						"id": "id",
					},
				},
				"nestedType": {
					TargetName: "nested_type",
				},
				"recursiveType": {
					TargetName: "recursive_type",
				},
				"randomPet": {
					TargetName: "random_pet",
				},
				"allPets": {
					TargetName: "all_pets",
				},
				"categories": {
					TargetName: "categories",
				},
				"categoriesByKind": {
					TargetName: "categories_by_kind",
					ArgumentMappings: map[string]string{
						"kind": "kind",
					},
				},
				"categoriesByKinds": {
					TargetName: "categories_by_kinds",
					ArgumentMappings: map[string]string{
						"kinds": "kinds",
					},
				},
				"filterCategories": {
					TargetName: "filter_categories",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"typeFilterWithArguments": {
					TargetName: "type_filter_with_arguments",
					ArgumentMappings: map[string]string{
						"filterField1": "filter_field_1",
						"filterField2": "filter_field_2",
					},
				},
				"typeWithMultipleFilterFields": {
					TargetName: "type_with_multiple_filter_fields",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"complexFilterType": {
					TargetName: "complex_filter_type",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"calculateTotals": {
					TargetName: "calculate_totals",
					ArgumentMappings: map[string]string{
						"orders": "orders",
					},
				},
				"search": {
					TargetName: "search",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"randomSearchResult": {
					TargetName: "random_search_result",
				},
			},
			"Mutation": {
				"createUser": {
					TargetName: "create_user",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"performAction": {
					TargetName: "perform_action",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
			},
			"UserInput": {
				"name": {
					TargetName: "name",
				},
			},
			"Product": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"price": {
					TargetName: "price",
				},
			},
			"Storage": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"location": {
					TargetName: "location",
				},
			},
			"User": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"NestedTypeA": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"b": {
					TargetName: "b",
				},
			},
			"NestedTypeB": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"c": {
					TargetName: "c",
				},
			},
			"NestedTypeC": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"RecursiveType": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"recursiveType": {
					TargetName: "recursive_type",
				},
			},
			"TypeWithMultipleFilterFields": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"filterField1": {
					TargetName: "filter_field_1",
				},
				"filterField2": {
					TargetName: "filter_field_2",
				},
			},
			"TypeWithComplexFilterInput": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"Cat": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
				"meowVolume": {
					TargetName: "meow_volume",
				},
			},
			"Dog": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
				"barkVolume": {
					TargetName: "bark_volume",
				},
			},
			"Animal": {
				"cat": {
					TargetName: "cat",
				},
				"dog": {
					TargetName: "dog",
				},
			},
			"FilterType": {
				"name": {
					TargetName: "name",
				},
				"filterField1": {
					TargetName: "filter_field_1",
				},
				"filterField2": {
					TargetName: "filter_field_2",
				},
				"pagination": {
					TargetName: "pagination",
				},
			},
			"Pagination": {
				"page": {
					TargetName: "page",
				},
				"perPage": {
					TargetName: "per_page",
				},
			},
			"ComplexFilterTypeInput": {
				"filter": {
					TargetName: "filter",
				},
			},
			"Category": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
			},
			"CategoryFilter": {
				"category": {
					TargetName: "category",
				},
				"pagination": {
					TargetName: "pagination",
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
				"orderLines": {
					TargetName: "order_lines",
				},
			},
			"OrderLine": {
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
			"ActionSuccess": {
				"message": {
					TargetName: "message",
				},
				"timestamp": {
					TargetName: "timestamp",
				},
			},
			"ActionError": {
				"message": {
					TargetName: "message",
				},
				"code": {
					TargetName: "code",
				},
			},
			"SearchInput": {
				"query": {
					TargetName: "query",
				},
				"limit": {
					TargetName: "limit",
				},
			},
			"ActionInput": {
				"type": {
					TargetName: "type",
				},
				"payload": {
					TargetName: "payload",
				},
			},
			"SearchResult": {
				"product": {
					TargetName: "product",
				},
				"user": {
					TargetName: "user",
				},
				"category": {
					TargetName: "category",
				},
			},
			"ActionResult": {
				"actionSuccess": {
					TargetName: "action_success",
				},
				"actionError": {
					TargetName: "action_error",
				},
			},
		},
	}
}
