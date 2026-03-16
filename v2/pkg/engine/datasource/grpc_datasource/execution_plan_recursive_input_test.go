package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestMutationExecutionPlanWithRecursiveInputType(t *testing.T) {
	schemaDoc := mustParseRecursiveInputSchema(t, `
		scalar JSON

		type Query {
			noop: Boolean
		}

		type Mutation {
			updateNode(input: UpdateNodeInput!): Node!
		}

		type Node {
			id: ID!
		}

		input UpdateNodeInput {
			id: ID!
			conditions: RecursiveFilterInput
		}

		input RecursiveFilterInput {
			and: [RecursiveFilterInput!]
			or: [RecursiveFilterInput!]
			key: String
			value: JSON
		}
	`)

	queryDoc, report := astparser.ParseGraphqlDocumentString(`
		mutation UpdateNode($input: UpdateNodeInput!) {
			updateNode(input: $input) {
				id
			}
		}
	`)
	require.False(t, report.HasErrors(), report.Error())

	plan, err := newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName: "Products",
		mapping: &GRPCMapping{
			Service: "Products",
			MutationRPCs: RPCConfigMap[RPCConfig]{
				"updateNode": {
					RPC:      "MutationUpdateNode",
					Request:  "MutationUpdateNodeRequest",
					Response: "MutationUpdateNodeResponse",
				},
			},
		},
	}).PlanOperation(&queryDoc, &schemaDoc)
	require.NoError(t, err)
	require.Len(t, plan.Calls, 1)

	inputField := lookupField(plan.Calls[0].Request.Fields, "input")
	require.NotNil(t, inputField)
	require.NotNil(t, inputField.Message)

	conditionsField := lookupField(inputField.Message.Fields, "conditions")
	require.NotNil(t, conditionsField)
	require.NotNil(t, conditionsField.Message)

	andField := lookupField(conditionsField.Message.Fields, "and")
	require.NotNil(t, andField)
	require.True(t, andField.Repeated || andField.IsListType)
	require.Same(t, conditionsField.Message, andField.Message)

	orField := lookupField(conditionsField.Message.Fields, "or")
	require.NotNil(t, orField)
	require.True(t, orField.Repeated || orField.IsListType)
	require.Same(t, conditionsField.Message, orField.Message)
}

func lookupField(fields RPCFields, name string) *RPCField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}

	return nil
}

func mustParseRecursiveInputSchema(t *testing.T, schema string) ast.Document {
	t.Helper()

	doc, report := astparser.ParseGraphqlDocumentString(schema)
	require.False(t, report.HasErrors(), report.Error())

	err := asttransform.MergeDefinitionWithBaseSchema(&doc)
	require.NoError(t, err)

	report = operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&doc, &report)
	require.False(t, report.HasErrors(), report.Error())

	return doc
}
