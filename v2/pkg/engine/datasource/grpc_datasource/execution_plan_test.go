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

type Query {
	_entities(representations: [_Any!]!): [_Entity!]!
	user(id: ID!): User
	users: [User!]!
}

union _Entity = Product
scalar _Any
`

func TestConstructExecutionPlan(t *testing.T) {

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
								JSONPath: "variables.representations", // Path to extract data from GraphQL variables
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

		// fmt.Println(rpcPlanVisitor.plan.String())
		// fmt.Println(expectedPlan.String())

	})

}
