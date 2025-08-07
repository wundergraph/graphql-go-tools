package grpcdatasource

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	grpctest "github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

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

			rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      testMapping(),
			})

			plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)

			if err != nil {
				require.NotEmpty(t, tt.expectedError)
				require.Contains(t, err.Error(), tt.expectedError)
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

			rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      testMapping(),
			})

			plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)

			if err != nil {
				require.NotEmpty(t, tt.expectedError)
				require.Contains(t, err.Error(), tt.expectedError)
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
