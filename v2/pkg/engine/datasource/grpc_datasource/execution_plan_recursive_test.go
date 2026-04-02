package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

func TestExecutionPlan_RecursiveInputTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		schema        string
		mapping       *GRPCMapping
		query         string
		expectedPlan  *RPCExecutionPlan
		expectedError string
	}{
		{
			name: "Should not stack overflow on recursive input object with and/or fields",
			schema: `
				type Query {
					search(conditions: ConditionsInput): [Result!]!
				}

				type Result {
					id: ID!
					name: String!
				}

				input ConditionsInput {
					and: [ConditionsInput!]
					or: [ConditionsInput!]
					key: String
					value: String
				}`,
			mapping: &GRPCMapping{
				Service: "Search",
				QueryRPCs: map[string]RPCConfig{
					"search": {
						RPC:      "Search",
						Request:  "SearchRequest",
						Response: "SearchResponse",
					},
				},
			},
			query:         `query SearchQuery($conditions: ConditionsInput) { search(conditions: $conditions) { id name } }`,
			expectedError: "recursive input type",
		},
		{
			name: "Should not stack overflow on self-referencing input object",
			schema: `
				type Query {
					filter(input: FilterInput): [Item!]!
				}

				type Item {
					id: ID!
				}

				input FilterInput {
					child: FilterInput
					value: String
				}`,
			mapping: &GRPCMapping{
				Service: "Items",
				QueryRPCs: map[string]RPCConfig{
					"filter": {
						RPC:      "Filter",
						Request:  "FilterRequest",
						Response: "FilterResponse",
					},
				},
			},
			query:         `query FilterQuery($input: FilterInput) { filter(input: $input) { id } }`,
			expectedError: "recursive input type",
		},
		{
			name: "Should not stack overflow on mutually recursive input objects",
			schema: `
				type Query {
					evaluate(expr: ExprInput): Boolean!
				}

				input ExprInput {
					not: NotExprInput
					value: String
				}

				input NotExprInput {
					expr: ExprInput
				}`,
			mapping: &GRPCMapping{
				Service: "Eval",
				QueryRPCs: map[string]RPCConfig{
					"evaluate": {
						RPC:      "Evaluate",
						Request:  "EvaluateRequest",
						Response: "EvaluateResponse",
					},
				},
			},
			query:         `query EvalQuery($expr: ExprInput) { evaluate(expr: $expr) }`,
			expectedError: "recursive input type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schemaDoc := testSchema(t, tt.schema)

			queryDoc, report := astparser.ParseGraphqlDocumentString(tt.query)
			require.False(t, report.HasErrors())

			runTestWithConfig(t, testCase{
				query:         tt.query,
				expectedPlan:  tt.expectedPlan,
				expectedError: tt.expectedError,
			}, testConfig{
				subgraphName: tt.mapping.Service,
				mapping:      tt.mapping,
				schemaDoc:    schemaDoc,
				operationDoc: queryDoc,
			})
		})
	}
}
