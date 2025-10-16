package grpcdatasource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	grpctest "github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestEntityLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:  "Should create an execution plan for an entity lookup with one key field",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Product { __typename id name price } } }`,
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"Product": {
						{
							Key: "id",
							RPCConfig: RPCConfig{
								RPC:      "LookupProductById",
								Request:  "LookupProductByIdRequest",
								Response: "LookupProductByIdResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						Kind:        CallKindEntity,
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
										Name:        "LookupProductByIdKey",
										MemberTypes: []string{"Product"},
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
		{
			name:    "Should create an execution plan for an entity lookup multiple types",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Product { __typename id name price } ... on Storage { __typename id name location } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupProductByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdKey",
										MemberTypes: []string{"Product"},
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
					{
						ServiceName: "Products",
						MethodName:  "LookupStorageById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupStorageByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupStorageByIdKey",
										MemberTypes: []string{"Storage"},
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
						Response: RPCMessage{
							Name: "LookupStorageByIdResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
									Message: &RPCMessage{
										Name: "Storage",
										Fields: []RPCField{
											{
												Name:        "__typename",
												TypeName:    string(DataTypeString),
												JSONPath:    "__typename",
												StaticValue: "Storage",
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
												Name:     "location",
												TypeName: string(DataTypeString),
												JSONPath: "location",
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

			planner := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			plan, err := planner.PlanOperation(&queryDoc, &schemaDoc)
			if err != nil {
				t.Fatalf("failed to plan operation: %s", err)
			}

			diff := cmp.Diff(tt.expectedPlan, plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func TestEntityKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		query             string
		schema            string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:  "Should create an execution plan for an entity lookup with a key field",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			type User @key(fields: "id") {
				id: ID!
				name: String!
			}
			`, []string{"User"}),
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"User": {
						{
							Key: "id",
							RPCConfig: RPCConfig{
								RPC:      "LookupUserById",
								Request:  "LookupUserByIdRequest",
								Response: "LookupUserByIdResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "User",
					SelectionSet: "id",
				},
			},

			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserById",
						Kind:        CallKindEntity,
						// Define the structure of the request message
						Request: RPCMessage{
							Name: "LookupUserByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdKey",
										MemberTypes: []string{"User"},
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
							Name: "LookupUserByIdResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
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
			name:  "Should create an execution plan for an entity lookup with a nested key",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}

			type Address {
				id: ID!
				street: String!
				city: String!
				state: String!
				zip: String!
			}

			type User @key(fields: "id address { id }") {
				id: ID!
				name: String!
				address: Address!
			}
			`, []string{"User"}),
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"User": {
						{
							Key: "id address { id }",
							RPCConfig: RPCConfig{
								RPC:      "LookupUserByIdAndAddress",
								Request:  "LookupUserByIdAndAddressRequest",
								Response: "LookupUserByIdAndAddressResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "User",
					SelectionSet: "id address { id }",
				},
			},

			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserByIdAndAddress",
						Kind:        CallKindEntity,
						// Define the structure of the request message
						Request: RPCMessage{
							Name: "LookupUserByIdAndAddressRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndAddressKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:     "id",
												TypeName: string(DataTypeString),
												JSONPath: "id",
											},
											{
												Name:     "address",
												TypeName: string(DataTypeMessage),
												JSONPath: "address",
												Message: &RPCMessage{
													Name: "Address",
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
								},
							},
						},
						// Define the structure of the response message
						Response: RPCMessage{
							Name: "LookupUserByIdAndAddressResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
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
			name:  "Should create an execution plan for an entity lookup with a compound key",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name  } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			type User @key(fields: "id name") {
				id: ID!
				name: String!
			}
			`, []string{"User"}),
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"User": {
						{
							Key: "id name",
							RPCConfig: RPCConfig{
								RPC:      "LookupUserByIdAndName",
								Request:  "LookupUserByIdAndNameRequest",
								Response: "LookupUserByIdAndNameResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "User",
					SelectionSet: "id name",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserByIdAndName",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdAndNameRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameKey",
										MemberTypes: []string{"User"},
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
						Response: RPCMessage{
							Name: "LookupUserByIdAndNameResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
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
			name:  "Order in a compound key should not matter",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name  } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			type User @key(fields: "id name") {
				id: ID!
				name: String!
			}
			`, []string{"User"}),
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"User": {
						{
							Key: "name id",
							RPCConfig: RPCConfig{
								RPC:      "LookupUserByIdAndName",
								Request:  "LookupUserByIdAndNameRequest",
								Response: "LookupUserByIdAndNameResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "User",
					SelectionSet: "id name",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserByIdAndName",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdAndNameRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameKey",
										MemberTypes: []string{"User"},
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
						Response: RPCMessage{
							Name: "LookupUserByIdAndNameResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
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
			name:  "Nested fields in a compound key should be ignored",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name  } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			
			type Address {
				id: ID!
				street: String!
			}
			type User @key(fields: "id name address { id }") {
				id: ID!
				name: String!
				address: Address!
			}
			`, []string{"User"}),
			mapping: &GRPCMapping{
				Service: "Products",
				EntityRPCs: map[string][]EntityRPCConfig{
					"User": {
						{
							Key: "name id address",
							RPCConfig: RPCConfig{
								RPC:      "LookupUserByIdAndNameAndAddress",
								Request:  "LookupUserByIdAndNameAndAddressRequest",
								Response: "LookupUserByIdAndNameAndAddressResponse",
							},
						},
					},
				},
			},
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "User",
					SelectionSet: "id name address { id }",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserByIdAndNameAndAddress",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdAndNameAndAddressRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameAndAddressKey",
										MemberTypes: []string{"User"},
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
												Name:     "address",
												TypeName: string(DataTypeMessage),
												JSONPath: "address",
												Message: &RPCMessage{
													Name: "Address",
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
								},
							},
						},
						Response: RPCMessage{
							Name: "LookupUserByIdAndNameAndAddressResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
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
	}

	for _, tt := range tests {
		runFederationTest(t, tt)
	}
}

func TestEntityLookupWithFieldResolvers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{

		{
			name:    "Should create an execution plan for an entity lookup with a field resolver",
			query:   `query EntityLookup($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ... on Product { __typename id name price shippingEstimate(input: $input) } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupProductByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdKey",
										MemberTypes: []string{"Product"},
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
					{
						ServiceName:    "Products",
						MethodName:     "ResolveProductShippingEstimate",
						Kind:           CallKindResolve,
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.shippingEstimate"),
						Request: RPCMessage{
							Name: "ResolveProductShippingEstimateRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("result.id"),
											},
											{
												Name:        "price",
												TypeName:    string(DataTypeDouble),
												JSONPath:    "price",
												ResolvePath: buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateArgs",
										Fields: []RPCField{
											{
												Name:     "input",
												TypeName: string(DataTypeMessage),
												JSONPath: "input",
												Message: &RPCMessage{
													Name: "ShippingEstimateInput",
													Fields: []RPCField{
														{
															Name:     "destination",
															TypeName: string(DataTypeEnum),
															JSONPath: "destination",
															EnumName: "ShippingDestination",
														},
														{
															Name:     "weight",
															TypeName: string(DataTypeDouble),
															JSONPath: "weight",
														},
														{
															Name:     "expedited",
															TypeName: string(DataTypeBool),
															JSONPath: "expedited",
															Optional: true,
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
							Name: "ResolveProductShippingEstimateResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateResult",
										Fields: []RPCField{
											{
												Name:     "shipping_estimate",
												TypeName: string(DataTypeDouble),
												JSONPath: "shippingEstimate",
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
			name:    "Should create an execution plan for multiple entity lookups with field resolvers",
			query:   `query MultiEntityLookup($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ... on Storage { __typename id name location } ... on Product { __typename id name price shippingEstimate(input: $input) } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupStorageById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupStorageByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupStorageByIdKey",
										MemberTypes: []string{"Storage"},
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
						Response: RPCMessage{
							Name: "LookupStorageByIdResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "_entities",
									Message: &RPCMessage{
										Name: "Storage",
										Fields: []RPCField{
											{
												Name:        "__typename",
												TypeName:    string(DataTypeString),
												JSONPath:    "__typename",
												StaticValue: "Storage",
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
												Name:     "location",
												TypeName: string(DataTypeString),
												JSONPath: "location",
											},
										},
									},
								},
							},
						},
					},
					{
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupProductByIdRequest",
							Fields: []RPCField{
								{
									Name:     "keys",
									TypeName: string(DataTypeMessage),
									Repeated: true,
									JSONPath: "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdKey",
										MemberTypes: []string{"Product"},
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
					{
						ServiceName:    "Products",
						MethodName:     "ResolveProductShippingEstimate",
						Kind:           CallKindResolve,
						DependentCalls: []int{1},
						ResponsePath:   buildPath("_entities.shippingEstimate"),
						Request: RPCMessage{
							Name: "ResolveProductShippingEstimateRequest",
							Fields: []RPCField{
								{
									Name:     "context",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateContext",
										Fields: []RPCField{
											{
												Name:        "id",
												TypeName:    string(DataTypeString),
												JSONPath:    "id",
												ResolvePath: buildPath("result.id"),
											},
											{
												Name:        "price",
												TypeName:    string(DataTypeDouble),
												JSONPath:    "price",
												ResolvePath: buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:     "field_args",
									TypeName: string(DataTypeMessage),
									JSONPath: "",
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateArgs",
										Fields: []RPCField{
											{
												Name:     "input",
												TypeName: string(DataTypeMessage),
												JSONPath: "input",
												Message: &RPCMessage{
													Name: "ShippingEstimateInput",
													Fields: []RPCField{
														{
															Name:     "destination",
															TypeName: string(DataTypeEnum),
															JSONPath: "destination",
															EnumName: "ShippingDestination",
														},
														{
															Name:     "weight",
															TypeName: string(DataTypeDouble),
															JSONPath: "weight",
														},
														{
															Name:     "expedited",
															TypeName: string(DataTypeBool),
															JSONPath: "expedited",
															Optional: true,
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
							Name: "ResolveProductShippingEstimateResponse",
							Fields: []RPCField{
								{
									Name:     "result",
									TypeName: string(DataTypeMessage),
									JSONPath: "result",
									Repeated: true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateResult",
										Fields: []RPCField{
											{
												Name:     "shipping_estimate",
												TypeName: string(DataTypeDouble),
												JSONPath: "shippingEstimate",
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

			planner := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			plan, err := planner.PlanOperation(&queryDoc, &schemaDoc)
			if err != nil {
				t.Fatalf("failed to plan operation: %s", err)
			}

			diff := cmp.Diff(tt.expectedPlan, plan)
			if diff != "" {
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}

func runFederationTest(t *testing.T, tt struct {
	name              string
	query             string
	schema            string
	expectedPlan      *RPCExecutionPlan
	mapping           *GRPCMapping
	federationConfigs plan.FederationFieldConfigurations
}) {

	t.Helper()

	t.Run(tt.name, func(t *testing.T) {
		t.Parallel()

		var operation, definition ast.Document

		definition = unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tt.schema)

		report := operationreport.Report{}
		astvalidation.DefaultDefinitionValidator().Validate(&definition, &report)
		if report.HasErrors() {
			t.Fatalf("failed to validate schema: %s", report.Error())
		}

		operation, report = astparser.ParseGraphqlDocumentString(tt.query)
		if report.HasErrors() {
			t.Fatalf("failed to parse query: %s", report.Error())
		}

		astvalidation.DefaultOperationValidator().Validate(&operation, &definition, &report)
		if report.HasErrors() {
			t.Fatalf("failed to validate query: %s", report.Error())
		}

		planner := NewPlanner("Products", tt.mapping, tt.federationConfigs)
		plan, err := planner.PlanOperation(&operation, &definition)
		if err != nil {
			t.Fatalf("failed to plan operation: %s", err)
		}

		diff := cmp.Diff(tt.expectedPlan, plan)
		if diff != "" {
			t.Fatalf("execution plan mismatch: %s", diff)
		}
	})

}

func testFederationSchemaString(schema string, entities []string) string {
	entityUnion := strings.Join(entities, " | ")
	return fmt.Sprintf(`
	schema {
		query: Query
	}
	%s

	union _Entity = %s
	scalar _Any
	`, schema, entityUnion)
}
