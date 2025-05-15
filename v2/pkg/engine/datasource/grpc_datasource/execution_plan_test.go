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
				Groups: []RPCCallGroup{
					{
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
			name:  "Should include typename when requested",
			query: `query UsersWithTypename { users { __typename id name } }`,
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should call query with two arguments and no variables and mapping for field names",
			query: `query QueryWithTwoArguments { typeFilterWithArguments(filterField1: "test1", filterField2: "test2") { id name filterField1 filterField2 } }`,
			mapping: &GRPCMapping{
				QueryRPCs: map[string]RPCConfig{
					"Query": {
						RPC:      "QueryTypeFilterWithArguments",
						Request:  "QueryTypeFilterWithArgumentsRequest",
						Response: "QueryTypeFilterWithArgumentsResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"typeFilterWithArguments": {
							TargetName: "type_filter_with_arguments",
							ArgumentMappings: map[string]string{
								"filterField1": "filter_field_1",
								"filterField2": "filter_field_2",
							},
						},
					},
					"TypeWithMultipleFilterFields": {
						"filterField1": {
							TargetName: "filter_field_1",
						},
						"filterField2": {
							TargetName: "filter_field_2",
						},
					},
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a complex input type and no variables and mapping for field names",
			query: `query ComplexFilterTypeQuery { complexFilterType(filter: { name: "test", filterField1: "test1", filterField2: "test2", pagination: { page: 1, perPage: 10 } }) { id name } }`,
			mapping: &GRPCMapping{
				Fields: map[string]FieldMap{
					"FilterType": {
						"filterField1": {
							TargetName: "filter_field1",
						},
						"filterField2": {
							TargetName: "filter_field2",
						},
					},
					"Pagination": {
						"perPage": {
							TargetName: "per_page",
						},
					},
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
																	Name:     "filter_field1",
																	TypeName: string(DataTypeString),
																	JSONPath: "filterField1",
																},
																{
																	Name:     "filter_field2",
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a complex input type and variables",
			query: `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a complex input type and variables with different name",
			query: `query ComplexFilterTypeQuery($foobar: ComplexFilterTypeInput!) { complexFilterType(filter: $foobar) { id name } }`,
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a type filter with arguments and variables",
			query: "query TypeWithMultipleFilterFieldsQuery($filter: FilterTypeInput!) { typeWithMultipleFilterFields(filter: $filter) { id name } }",
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query",
			query: "query UserQuery { users { id name } }",
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a nested type",
			query: "query NestedTypeQuery { nestedType { id name b { id name c { id name } } } }",
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
			},
		},
		{
			name:  "Should create an execution plan for a query with a recursive type",
			query: "query RecursiveTypeQuery { recursiveType { id name recursiveType { id recursiveType { id name recursiveType { id name } } name } } }",
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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

// TODO: Define test cases for interface execution plans
func TestInterfaceExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		mapping       *GRPCMapping
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with a random cat",
			query: "query RandomCatQuery { randomPet { id name kind ... on Cat { meowVolume } } }",
			mapping: &GRPCMapping{
				QueryRPCs: map[string]RPCConfig{
					"Query": {
						RPC:      "QueryRandomPet",
						Request:  "QueryRandomPetRequest",
						Response: "QueryRandomPetResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"randomPet": {
							TargetName: "random_pet",
						},
					},
					"Cat": {
						"meowVolume": {
							TargetName: "meow_volume",
						},
					},
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
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
												Name:  "Animal",
												OneOf: true,
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
		mapping       *GRPCMapping
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name:  "Should create an execution plan for a query with categories by kind",
			query: "query CategoriesQuery($kind: CategoryKind!) { categoriesByKind(kind: $kind) { id name kind } }",
			mapping: &GRPCMapping{
				Service: "ProductService",
				QueryRPCs: map[string]RPCConfig{
					"categoriesByKind": {
						RPC:      "QueryCategoriesByKind",
						Request:  "QueryCategoriesByKindRequest",
						Response: "QueryCategoriesByKindResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"categoriesByKind": {
							TargetName: "categories_by_kind",
							ArgumentMappings: map[string]string{
								"kind": "kind",
							},
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
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
						Calls: []RPCCall{
							{
								ServiceName: "ProductService",
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
			},
		},
		{
			name:  "Should create an execution plan for a query with categories by kinds",
			query: "query CategoriesQuery($kinds: [CategoryKind!]!) { categoriesByKinds(kinds: $kinds) { id name kind } }",
			mapping: &GRPCMapping{
				Service: "ProductService",
				QueryRPCs: map[string]RPCConfig{
					"categoriesByKinds": {
						RPC:      "QueryCategoriesByKinds",
						Request:  "QueryCategoriesByKindsRequest",
						Response: "QueryCategoriesByKindsResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"categoriesByKinds": {
							TargetName: "categories_by_kinds",
							ArgumentMappings: map[string]string{
								"kinds": "kinds",
							},
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
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
						Calls: []RPCCall{
							{
								ServiceName: "ProductService",
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
			},
		},
		{
			name:  "Should create an execution plan for a query with filtered categories",
			query: "query FilterCategoriesQuery($filter: CategoryFilter!) { filterCategories(filter: $filter) { id name kind } }",
			mapping: &GRPCMapping{
				Service: "ProductService",
				QueryRPCs: map[string]RPCConfig{
					"filterCategories": {
						RPC:      "QueryFilterCategories",
						Request:  "QueryFilterCategoriesRequest",
						Response: "QueryFilterCategoriesResponse",
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"filterCategories": {
							TargetName: "filter_categories",
							ArgumentMappings: map[string]string{
								"filter": "filter",
							},
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
					"Pagination": {
						"page": {
							TargetName: "page",
						},
						"perPage": {
							TargetName: "per_page",
						},
					},
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Groups: []RPCCallGroup{
					{
						Calls: []RPCCall{
							{
								ServiceName: "ProductService",
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

			planner := NewPlanner("Products", tt.mapping)
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
