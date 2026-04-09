package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

func TestExecutionPlan_RecursiveInputTypes_String(t *testing.T) {
	// Verify stringer method does not overflow on recursive inputs
	t.Parallel()

	schema := `
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
			}`

	mapping := &GRPCMapping{
		Service: "Search",
		QueryRPCs: map[string]RPCConfig{
			"search": {
				RPC:      "Search",
				Request:  "SearchRequest",
				Response: "SearchResponse",
			},
		},
	}

	query := `query SearchQuery($conditions: ConditionsInput) { search(conditions: $conditions) { id name } }`

	plan := planRecursiveTest(t, query, schema, mapping)

	result := plan.String()
	// formatRPCMessage must emit the recursive placeholder instead of overflowing.
	require.Contains(t, result, "ConditionsInput")
	require.Contains(t, result, "<recursive: ConditionsInput>")
}

func TestExecutionPlan_RecursiveInputTypes(t *testing.T) {
	t.Parallel()

	t.Run("Should not stack overflow on recursive input object with and/or fields", func(t *testing.T) {
		t.Parallel()

		schema := `
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
			}`

		mapping := &GRPCMapping{
			Service: "Search",
			QueryRPCs: map[string]RPCConfig{
				"search": {
					RPC:      "Search",
					Request:  "SearchRequest",
					Response: "SearchResponse",
				},
			},
		}

		query := `query SearchQuery($conditions: ConditionsInput) { search(conditions: $conditions) { id name } }`

		plan := planRecursiveTest(t, query, schema, mapping)

		require.Len(t, plan.Calls, 1)
		call := plan.Calls[0]
		require.Equal(t, "Search", call.MethodName)

		// The request should have a conditions field with a recursive message.
		require.Len(t, call.Request.Fields, 1)
		conditionsField := call.Request.Fields[0]
		require.Equal(t, "conditions", conditionsField.JSONPath)
		require.NotNil(t, conditionsField.Message)
		require.Equal(t, "ConditionsInput", conditionsField.Message.Name)
		require.Len(t, conditionsField.Message.Fields, 4)

		// The and/or fields should reference the same ConditionsInput message (cycle).
		andField := findField(t, conditionsField.Message.Fields, "and")
		orField := findField(t, conditionsField.Message.Fields, "or")
		require.True(t, andField.Message == conditionsField.Message, "and field should reference the same ConditionsInput message")
		require.True(t, orField.Message == conditionsField.Message, "or field should reference the same ConditionsInput message")
	})

	t.Run("Should not stack overflow on self-referencing input object", func(t *testing.T) {
		t.Parallel()

		schema := `
			type Query {
				filter(input: FilterInput): [Item!]!
			}

			type Item {
				id: ID!
			}

			input FilterInput {
				child: FilterInput
				value: String
			}`

		mapping := &GRPCMapping{
			Service: "Items",
			QueryRPCs: map[string]RPCConfig{
				"filter": {
					RPC:      "Filter",
					Request:  "FilterRequest",
					Response: "FilterResponse",
				},
			},
		}

		query := `query FilterQuery($input: FilterInput) { filter(input: $input) { id } }`

		plan := planRecursiveTest(t, query, schema, mapping)

		require.Len(t, plan.Calls, 1)
		call := plan.Calls[0]
		require.Equal(t, "Filter", call.MethodName)

		require.Len(t, call.Request.Fields, 1)
		inputField := call.Request.Fields[0]
		require.Equal(t, "input", inputField.JSONPath)
		require.NotNil(t, inputField.Message)
		require.Equal(t, "FilterInput", inputField.Message.Name)
		require.Len(t, inputField.Message.Fields, 2)

		// The child field should reference the same FilterInput message.
		childField := findField(t, inputField.Message.Fields, "child")
		require.True(t, childField.Message == inputField.Message, "child field should reference the same FilterInput message")
	})

	t.Run("Should not stack overflow on mutually recursive input objects", func(t *testing.T) {
		t.Parallel()

		schema := `
			type Query {
				evaluate(expr: ExprInput): Boolean!
			}

			input ExprInput {
				not: NotExprInput
				value: String
			}

			input NotExprInput {
				expr: ExprInput
			}`

		mapping := &GRPCMapping{
			Service: "Eval",
			QueryRPCs: map[string]RPCConfig{
				"evaluate": {
					RPC:      "Evaluate",
					Request:  "EvaluateRequest",
					Response: "EvaluateResponse",
				},
			},
		}

		query := `query EvalQuery($expr: ExprInput) { evaluate(expr: $expr) }`

		plan := planRecursiveTest(t, query, schema, mapping)

		require.Len(t, plan.Calls, 1)
		call := plan.Calls[0]
		require.Equal(t, "Evaluate", call.MethodName)

		require.Len(t, call.Request.Fields, 1)
		exprField := call.Request.Fields[0]
		require.Equal(t, "expr", exprField.JSONPath)
		require.NotNil(t, exprField.Message)
		require.Equal(t, "ExprInput", exprField.Message.Name)
		require.Len(t, exprField.Message.Fields, 2)

		// ExprInput.not -> NotExprInput.expr -> ExprInput (cycle)
		notField := findField(t, exprField.Message.Fields, "not")
		require.NotNil(t, notField.Message)
		require.Equal(t, "NotExprInput", notField.Message.Name)
		require.Len(t, notField.Message.Fields, 1)

		backRef := findField(t, notField.Message.Fields, "expr")
		require.True(t, backRef.Message == exprField.Message, "NotExprInput.expr should reference the same ExprInput message")
	})
}

func findField(t *testing.T, fields RPCFields, jsonPath string) RPCField {
	t.Helper()

	for _, f := range fields {
		if f.JSONPath == jsonPath {
			return f
		}
	}

	t.Fatalf("field with JSONPath %q not found", jsonPath)
	return RPCField{}
}

func planRecursiveTest(t *testing.T, query, schema string, mapping *GRPCMapping) *RPCExecutionPlan {
	t.Helper()

	schemaDoc := testSchema(t, schema)

	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName: mapping.Service,
		mapping:      mapping,
	})

	plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)
	require.NoError(t, err)

	return plan
}
