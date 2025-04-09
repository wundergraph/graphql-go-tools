package grpcdatasource

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var testSchema = `
	scalar ID
	scalar String
	scalar Float

type Product @key(fields: "id") {
	id: ID!
	name: String!
	price: Float!
	shippingEstimate(input: ShippingEstimateInput!): Float!
}

type User {
	id: ID!
	name: String!
}

type NestedTypeA {
	id: ID!
	name: String!
	b: NestedTypeB!
}

type NestedTypeB {
	id: ID!
	name: String!
	c: NestedTypeC!
}

type NestedTypeC {
	id: ID!
	name: String!
}

type RecursiveType {
	id: ID!
	name: String!
	recursiveType: RecursiveType!
}

type TypeWithMultipleFilterFields {
	id: ID!
	name: String!
	filterField1: String!
	filterField2: String!
}

input FilterTypeInput {
	filterField1: String!
	filterField2: String!
}

type TypeWithComplexFilterInput {
	id: ID!
	name: String!
}

input FilterType {
	name: String!
	filterField1: String!
	filterField2: String!
	pagination: Pagination!
}

input Pagination {
	page: Int!
	perPage: Int!
}

input ComplexFilterTypeInput {
	filter: FilterType!
}


type Query {
	_entities(representations: [_Any!]!): [_Entity!]!
	users: [User!]!
	user(id: ID!): User
	nestedType: [NestedTypeA!]!
	recursiveType: RecursiveType!
	typeFilterWithArguments(filterField1: String!, filterField2: String!): [TypeWithMultipleFilterFields!]!
	typeWithMultipleFilterFields(filter: FilterTypeInput!): [TypeWithMultipleFilterFields!]!
	complexFilterType(filter: ComplexFilterTypeInput!): [TypeWithComplexFilterInput!]!
}

union _Entity = Product
scalar _Any
`

func TestEntityLookup(t *testing.T) {
	t.Run("Should create an execution plan for an entity lookup", func(t *testing.T) {
		// GraphQL Query
		query := `
query EntityLookup($representations: [_Any!]!) {
	_entities(representations: $representations) {
		... on Product {
			id
			name
			price
		}
	}
}
`

		expectedPlan := &RPCExecutionPlan{
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
										Name:     "inputs",
										TypeName: string(DataTypeMessage),
										Repeated: true,
										JSONPath: "representations", // Path to extract data from GraphQL variables
										Index:    0,
										Message: &RPCMessage{
											Name: "LookupProductByIdInput",
											Fields: []RPCField{
												{
													Name:     "key",
													TypeName: string(DataTypeMessage),
													Index:    0,
													Message: &RPCMessage{
														Name: "ProductByIdKey",
														Fields: []RPCField{
															{
																Name:     "id",
																TypeName: string(DataTypeString),
																JSONPath: "id", // Extract 'id' from each representation
																Index:    0,
															},
														},
													},
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
										Name:     "results",
										TypeName: string(DataTypeMessage),
										Repeated: true,
										Index:    0,
										JSONPath: "results",
										Message: &RPCMessage{
											Name: "LookupProductByIdResult",
											Fields: []RPCField{
												{
													Name:     "product",
													TypeName: string(DataTypeMessage),
													Index:    0,
													Message: &RPCMessage{
														Name: "Product",
														Fields: []RPCField{
															{
																Name:     "id",
																TypeName: string(DataTypeString),
																JSONPath: "id",
																Index:    0,
															},
															{
																Name:     "name",
																TypeName: string(DataTypeString),
																JSONPath: "name",
																Index:    1,
															},
															{
																Name:     "price",
																TypeName: string(DataTypeFloat),
																JSONPath: "price",
																Index:    2,
															},
														},
													},
												},
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

		report := &operationreport.Report{}

		// Parse the GraphQL schema
		schemaDoc := ast.NewDocument()
		schemaDoc.Input.ResetInputString(testSchema)
		astparser.NewParser().Parse(schemaDoc, report)
		if report.HasErrors() {
			t.Fatalf("failed to parse schema: %s", report.Error())
		}

		// Parse the GraphQL query
		queryDoc := ast.NewDocument()
		queryDoc.Input.ResetInputString(query)
		astparser.NewParser().Parse(queryDoc, report)
		if report.HasErrors() {
			t.Fatalf("failed to parse query: %s", report.Error())
		}
		// Transform the GraphQL ASTs
		err := asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
		if err != nil {
			t.Fatalf("failed to merge schema with base: %s", err)
		}

		walker := astvisitor.NewWalker(48)

		rpcPlanVisitor := NewRPCPlanVisitor(&walker, "Products")

		walker.Walk(queryDoc, schemaDoc, report)

		if report.HasErrors() {
			t.Fatalf("failed to walk AST: %s", report.Error())
		}

		diff := cmp.Diff(expectedPlan, rpcPlanVisitor.plan)
		if diff != "" {
			t.Fatalf("execution plan mismatch: %s", diff)
		}
	})
}

func TestQueryExecutionPlans(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedPlan *RPCExecutionPlan
	}{
		{
			name:  "Should call query with two arguments and no variables",
			query: `query QueryWithTwoArguments { typeFilterWithArguments(filterField1: "test1", filterField2: "test2") { id name } }`,
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
											Name:     "filterField1",
											TypeName: string(DataTypeString),
											JSONPath: "filterField1",
											Index:    0,
										},
										{
											Name:     "filterField2",
											TypeName: string(DataTypeString),
											JSONPath: "filterField2",
											Index:    1,
										},
									},
								},
								Response: RPCMessage{
									Name: "QueryTypeFilterWithArgumentsResponse",
									Fields: []RPCField{
										{
											Name:     "typeFilterWithArguments",
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
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
			name:  "Should create an execution plan for a query with a complex input type and no variables",
			query: `query ComplexFilterTypeQuery { complexFilterType(filter: { name: "test", filterField1: "test1", filterField2: "test2", pagination: { page: 1, perPage: 10 } }) { id name } }`,
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
											Index:    0,
											Message: &RPCMessage{
												Name: "ComplexFilterTypeInput",
												Fields: []RPCField{
													{
														Name:     "filter",
														TypeName: string(DataTypeMessage),
														JSONPath: "filter",
														Index:    0,
														Message: &RPCMessage{
															Name: "FilterType",
															Fields: []RPCField{
																{
																	Name:     "name",
																	TypeName: string(DataTypeString),
																	JSONPath: "name",
																	Index:    0,
																},
																{
																	Name:     "filterField1",
																	TypeName: string(DataTypeString),
																	JSONPath: "filterField1",
																	Index:    1,
																},
																{
																	Name:     "filterField2",
																	TypeName: string(DataTypeString),
																	JSONPath: "filterField2",
																	Index:    2,
																},
																{
																	Name:     "pagination",
																	TypeName: string(DataTypeMessage),
																	JSONPath: "pagination",
																	Index:    3,
																	Message: &RPCMessage{
																		Name: "Pagination",
																		Fields: []RPCField{
																			{
																				Name:     "page",
																				TypeName: string(DataTypeInt32),
																				JSONPath: "page",
																				Index:    0,
																			},
																			{
																				Name:     "perPage",
																				TypeName: string(DataTypeInt32),
																				JSONPath: "perPage",
																				Index:    1,
																			},
																		},
																	},
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
											Index:    0,
											TypeName: string(DataTypeMessage),
											JSONPath: "complexFilterType",
											Message: &RPCMessage{
												Name: "TypeWithComplexFilterInput",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
											Index:    0,
											Message: &RPCMessage{
												Name: "ComplexFilterTypeInput",
												Fields: []RPCField{
													{
														Name:     "filter",
														TypeName: string(DataTypeMessage),
														JSONPath: "filter",
														Index:    0,
														Message: &RPCMessage{
															Name: "FilterType",
															Fields: []RPCField{
																{
																	Name:     "name",
																	TypeName: string(DataTypeString),
																	JSONPath: "name",
																	Index:    0,
																},
																{
																	Name:     "filterField1",
																	TypeName: string(DataTypeString),
																	JSONPath: "filterField1",
																	Index:    1,
																},
																{
																	Name:     "filterField2",
																	TypeName: string(DataTypeString),
																	JSONPath: "filterField2",
																	Index:    2,
																},
																{
																	Name:     "pagination",
																	TypeName: string(DataTypeMessage),
																	JSONPath: "pagination",
																	Index:    3,
																	Message: &RPCMessage{
																		Name: "Pagination",
																		Fields: []RPCField{
																			{
																				Name:     "page",
																				TypeName: string(DataTypeInt32),
																				JSONPath: "page",
																				Index:    0,
																			},
																			{
																				Name:     "perPage",
																				TypeName: string(DataTypeInt32),
																				JSONPath: "perPage",
																				Index:    1,
																			},
																		},
																	},
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
											Index:    0,
											TypeName: string(DataTypeMessage),
											JSONPath: "complexFilterType",
											Message: &RPCMessage{
												Name: "TypeWithComplexFilterInput",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
											Index:    0,
											Message: &RPCMessage{
												Name: "FilterTypeInput",
												Fields: []RPCField{
													{
														Repeated: false,
														Name:     "filterField1",
														TypeName: string(DataTypeString),
														JSONPath: "filterField1",
														Index:    0,
													},
													{
														Repeated: false,
														Name:     "filterField2",
														TypeName: string(DataTypeString),
														JSONPath: "filterField2",
														Index:    1,
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
											Index:    0,
											JSONPath: "typeWithMultipleFilterFields",
											Message: &RPCMessage{
												Name: "TypeWithMultipleFilterFields",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
											Index:    0,
											Message: &RPCMessage{
												Name: "User",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
			name:  "Should create an execution plan for a query with a user",
			query: `query UserQuery { user(id: "1") { id name } }`,
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
											Index:    0,
										},
									},
								},
								Response: RPCMessage{
									Name: "QueryUserResponse",
									Fields: []RPCField{
										{
											Name:     "user",
											TypeName: string(DataTypeMessage),
											Index:    0,
											JSONPath: "user",
											Message: &RPCMessage{
												Name: "User",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
												},
											},
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
											Index:    0,
											Message: &RPCMessage{
												Name: "NestedTypeA",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
													{
														Name:     "b",
														TypeName: string(DataTypeMessage),
														JSONPath: "b",
														Index:    2,
														Message: &RPCMessage{
															Name: "NestedTypeB",
															Fields: []RPCField{
																{
																	Name:     "id",
																	TypeName: string(DataTypeString),
																	JSONPath: "id",
																	Index:    0,
																},
																{
																	Name:     "name",
																	TypeName: string(DataTypeString),
																	JSONPath: "name",
																	Index:    1,
																},
																{
																	Name:     "c",
																	TypeName: string(DataTypeMessage),
																	JSONPath: "c",
																	Index:    2,
																	Message: &RPCMessage{
																		Name: "NestedTypeC",
																		Fields: []RPCField{
																			{
																				Name:     "id",
																				TypeName: string(DataTypeString),
																				JSONPath: "id",
																				Index:    0,
																			},
																			{
																				Name:     "name",
																				TypeName: string(DataTypeString),
																				JSONPath: "name",
																				Index:    1,
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
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
											Index:    0,
											Message: &RPCMessage{
												Name: "RecursiveType",
												Fields: []RPCField{
													{
														Name:     "id",
														TypeName: string(DataTypeString),
														JSONPath: "id",
														Index:    0,
													},
													{
														Name:     "name",
														TypeName: string(DataTypeString),
														JSONPath: "name",
														Index:    1,
													},
													{
														Name:     "recursiveType",
														TypeName: string(DataTypeMessage),
														JSONPath: "recursiveType",
														Index:    2,
														Message: &RPCMessage{
															Name: "RecursiveType",
															Fields: []RPCField{
																{
																	Name:     "id",
																	TypeName: string(DataTypeString),
																	JSONPath: "id",
																	Index:    0,
																},
																{
																	Name:     "recursiveType",
																	TypeName: string(DataTypeMessage),
																	JSONPath: "recursiveType",
																	Index:    1,
																	Message: &RPCMessage{
																		Name: "RecursiveType",
																		Fields: []RPCField{
																			{
																				Name:     "id",
																				TypeName: string(DataTypeString),
																				JSONPath: "id",
																				Index:    0,
																			},
																			{
																				Name:     "name",
																				TypeName: string(DataTypeString),
																				JSONPath: "name",
																				Index:    1,
																			},
																			{
																				Name:     "recursiveType",
																				TypeName: string(DataTypeMessage),
																				JSONPath: "recursiveType",
																				Index:    2,
																				Message: &RPCMessage{
																					Name: "RecursiveType",
																					Fields: []RPCField{
																						{
																							Name:     "id",
																							TypeName: string(DataTypeString),
																							JSONPath: "id",
																							Index:    0,
																						},
																						{
																							Name:     "name",
																							TypeName: string(DataTypeString),
																							JSONPath: "name",
																							Index:    1,
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
																	Index:    2,
																},
															},
														},
													},
												},
											},
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
			schemaDoc := ast.NewDocument()
			schemaDoc.Input.ResetInputString(testSchema)
			astparser.NewParser().Parse(schemaDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to parse schema: %s", report.Error())
			}

			// Parse the GraphQL query
			queryDoc := ast.NewDocument()
			queryDoc.Input.ResetInputString(tt.query)
			astparser.NewParser().Parse(queryDoc, report)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}
			// Transform the GraphQL ASTs
			err := asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
			if err != nil {
				t.Fatalf("failed to merge schema with base: %s", err)
			}

			walker := astvisitor.NewWalker(48)

			rpcPlanVisitor := NewRPCPlanVisitor(&walker, "Products")

			walker.Walk(queryDoc, schemaDoc, report)

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
