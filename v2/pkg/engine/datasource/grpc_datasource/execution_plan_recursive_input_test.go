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
	require.NotNil(t, plan)
	require.NotPanics(t, func() { _ = plan.String() })
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

	keyField := lookupField(conditionsField.Message.Fields, "key")
	require.NotNil(t, keyField)

	valueField := lookupField(conditionsField.Message.Fields, "value")
	require.NotNil(t, valueField)
}

func TestMutationExecutionPlanWithNestedRecursiveInputTypes(t *testing.T) {
	schemaDoc := mustParseRecursiveInputSchema(t, `
		type Query {
			noop: Boolean
		}

		type Mutation {
			processA(input: A!): Boolean!
		}

		input A {
			b: B
		}

		input B {
			c: C
		}

		input C {
			a: A
			b: B
		}
	`)

	queryDoc, report := astparser.ParseGraphqlDocumentString(`
		mutation ProcessA($input: A!) {
			processA(input: $input)
		}
	`)
	require.False(t, report.HasErrors(), report.Error())

	plan, err := newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName: "Products",
		mapping: &GRPCMapping{
			Service: "Products",
			MutationRPCs: RPCConfigMap[RPCConfig]{
				"processA": {
					RPC:      "MutationProcessA",
					Request:  "MutationProcessARequest",
					Response: "MutationProcessAResponse",
				},
			},
		},
	}).PlanOperation(&queryDoc, &schemaDoc)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotPanics(t, func() { _ = plan.String() })
	require.Len(t, plan.Calls, 1)

	inputField := lookupField(plan.Calls[0].Request.Fields, "input")
	require.NotNil(t, inputField)
	require.NotNil(t, inputField.Message)

	// A.b -> B
	aMessage := inputField.Message
	require.Equal(t, "A", aMessage.Name)
	bFieldInA := lookupField(aMessage.Fields, "b")
	require.NotNil(t, bFieldInA)
	require.NotNil(t, bFieldInA.Message)

	// B.c -> C
	bMessage := bFieldInA.Message
	require.Equal(t, "B", bMessage.Name)
	cFieldInB := lookupField(bMessage.Fields, "c")
	require.NotNil(t, cFieldInB)
	require.NotNil(t, cFieldInB.Message)

	// C.a -> A (same pointer as top-level A)
	cMessage := cFieldInB.Message
	require.Equal(t, "C", cMessage.Name)
	aFieldInC := lookupField(cMessage.Fields, "a")
	require.NotNil(t, aFieldInC)
	require.NotNil(t, aFieldInC.Message)
	require.Same(t, aMessage, aFieldInC.Message)

	// C.b -> B (same pointer as A.b's message)
	bFieldInC := lookupField(cMessage.Fields, "b")
	require.NotNil(t, bFieldInC)
	require.NotNil(t, bFieldInC.Message)
	require.Same(t, bMessage, bFieldInC.Message)
}

func TestMutationExecutionPlanWithMultipleRecursiveArguments(t *testing.T) {
	schemaDoc := mustParseRecursiveInputSchema(t, `
		type Query {
			noop: Boolean
		}

		type Mutation {
			processFilters(filter: RecursiveFilter!, exclude: RecursiveFilter!): Boolean!
		}

		input RecursiveFilter {
			and: [RecursiveFilter!]
			or: [RecursiveFilter!]
			key: String
		}
	`)

	queryDoc, report := astparser.ParseGraphqlDocumentString(`
		mutation ProcessFilters($filter: RecursiveFilter!, $exclude: RecursiveFilter!) {
			processFilters(filter: $filter, exclude: $exclude)
		}
	`)
	require.False(t, report.HasErrors(), report.Error())

	plan, err := newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName: "Products",
		mapping: &GRPCMapping{
			Service: "Products",
			MutationRPCs: RPCConfigMap[RPCConfig]{
				"processFilters": {
					RPC:      "MutationProcessFilters",
					Request:  "MutationProcessFiltersRequest",
					Response: "MutationProcessFiltersResponse",
				},
			},
		},
	}).PlanOperation(&queryDoc, &schemaDoc)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotPanics(t, func() { _ = plan.String() })
	require.Len(t, plan.Calls, 1)

	filterField := lookupField(plan.Calls[0].Request.Fields, "filter")
	require.NotNil(t, filterField)
	require.NotNil(t, filterField.Message)

	excludeField := lookupField(plan.Calls[0].Request.Fields, "exclude")
	require.NotNil(t, excludeField)
	require.NotNil(t, excludeField.Message)

	// Both arguments share the same RecursiveFilter message via cache
	require.Same(t, filterField.Message, excludeField.Message)

	// Verify self-referencing fields
	andField := lookupField(filterField.Message.Fields, "and")
	require.NotNil(t, andField)
	require.True(t, andField.Repeated || andField.IsListType)
	require.Same(t, filterField.Message, andField.Message)

	orField := lookupField(filterField.Message.Fields, "or")
	require.NotNil(t, orField)
	require.True(t, orField.Repeated || orField.IsListType)
	require.Same(t, filterField.Message, orField.Message)

	keyField := lookupField(filterField.Message.Fields, "key")
	require.NotNil(t, keyField)
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
