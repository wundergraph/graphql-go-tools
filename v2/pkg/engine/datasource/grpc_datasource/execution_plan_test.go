package grpcdatasource

import (
	"fmt"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestConstructExecutionPlan(t *testing.T) {
	schema := `
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

type Query {
	_entities(representations: [_Any!]!): [_Entity!]!
	user(id: ID!): User
	users: [User!]!
}

union _Entity = Product
scalar _Any
`

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
		report := &operationreport.Report{}

		// Parse the GraphQL schema
		schemaDoc := ast.NewDocument()
		schemaDoc.Input.ResetInputString(schema)
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

		fmt.Println(rpcPlanVisitor.plan)

	})

}
