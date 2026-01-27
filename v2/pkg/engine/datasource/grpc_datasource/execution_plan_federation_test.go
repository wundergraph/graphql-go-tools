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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},

											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:          1,
						ServiceName: "Products",
						MethodName:  "LookupStorageById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupStorageByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupStorageByIdRequestKey",
										MemberTypes: []string{"Storage"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Storage",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Storage",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
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

			planner, err := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			if err != nil {
				t.Fatalf("failed to create planner %s", err)
			}

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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndAddressRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "address",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "address",
												Message: &RPCMessage{
													Name: "Address",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdAndNameAndAddressRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "address",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "address",
												Message: &RPCMessage{
													Name: "Address",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
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
			name:  "Should create an execution plan for an entity lookup with a key field and nested field",
			query: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id  name address { street } } } }`,
			schema: testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			type User @key(fields: "id") {
				id: ID!
				name: String!
				address: Address!
			}
			
			type Address {
				id: ID!
				street: String!
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdRequestKey",
										MemberTypes: []string{"User"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "User",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "User",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "address",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "address",
												Message: &RPCMessage{
													Name: "Address",
													Fields: []RPCField{
														{
															Name:          "street",
															ProtoTypeName: DataTypeString,
															JSONPath:      "street",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},

											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						MethodName:     "ResolveProductShippingEstimate",
						Kind:           CallKindResolve,
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.shippingEstimate"),
						Request: RPCMessage{
							Name: "ResolveProductShippingEstimateRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
												ResolvePath:   buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateArgs",
										Fields: []RPCField{
											{
												Name:          "input",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "input",
												Message: &RPCMessage{
													Name: "ShippingEstimateInput",
													Fields: []RPCField{
														{
															Name:          "destination",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "destination",
															EnumName:      "ShippingDestination",
														},
														{
															Name:          "weight",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "weight",
														},
														{
															Name:          "expedited",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "expedited",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateResult",
										Fields: []RPCField{
											{
												Name:          "shipping_estimate",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "shippingEstimate",
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupStorageByIdRequestKey",
										MemberTypes: []string{"Storage"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Storage",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Storage",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:          1,
						ServiceName: "Products",
						MethodName:  "LookupProductById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupProductByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:             2,
						ServiceName:    "Products",
						MethodName:     "ResolveProductShippingEstimate",
						Kind:           CallKindResolve,
						DependentCalls: []int{1},
						ResponsePath:   buildPath("_entities.shippingEstimate"),
						Request: RPCMessage{
							Name: "ResolveProductShippingEstimateRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
												ResolvePath:   buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateArgs",
										Fields: []RPCField{
											{
												Name:          "input",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "input",
												Message: &RPCMessage{
													Name: "ShippingEstimateInput",
													Fields: []RPCField{
														{
															Name:          "destination",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "destination",
															EnumName:      "ShippingDestination",
														},
														{
															Name:          "weight",
															ProtoTypeName: DataTypeDouble,
															JSONPath:      "weight",
														},
														{
															Name:          "expedited",
															ProtoTypeName: DataTypeBool,
															JSONPath:      "expedited",
															Optional:      true,
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductShippingEstimateResult",
										Fields: []RPCField{
											{
												Name:          "shipping_estimate",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "shippingEstimate",
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

			planner, err := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			if err != nil {
				t.Fatalf("failed to create planner: %s", err)
			}
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

func TestEntityLookupWithFieldResolvers_WithCompositeTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:    "Should create an execution plan for an entity lookup with a field resolver returning interface type",
			query:   `query EntityLookupWithInterface($representations: [_Any!]!, $includeDetails: Boolean!) { _entities(representations: $representations) { ... on Product { __typename id name mascotRecommendation(includeDetails: $includeDetails) { ... on Cat { name meowVolume } ... on Dog { name barkVolume } } } } }`,
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						MethodName:     "ResolveProductMascotRecommendation",
						Kind:           CallKindResolve,
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.mascotRecommendation"),
						Request: RPCMessage{
							Name: "ResolveProductMascotRecommendationRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductMascotRecommendationContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("result.name"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveProductMascotRecommendationArgs",
										Fields: []RPCField{
											{
												Name:          "include_details",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "includeDetails",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveProductMascotRecommendationResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductMascotRecommendationResult",
										Fields: []RPCField{
											{
												Name:          "mascot_recommendation",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "mascotRecommendation",
												Optional:      true,
												Message: &RPCMessage{
													Name:        "Animal",
													OneOfType:   OneOfTypeInterface,
													MemberTypes: []string{"Cat", "Dog"},
													FieldSelectionSet: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "name",
																ProtoTypeName: DataTypeString,
																JSONPath:      "name",
															},
															{
																Name:          "meow_volume",
																ProtoTypeName: DataTypeInt32,
																JSONPath:      "meowVolume",
															},
														},
														"Dog": {
															{
																Name:          "name",
																ProtoTypeName: DataTypeString,
																JSONPath:      "name",
															},
															{
																Name:          "bark_volume",
																ProtoTypeName: DataTypeInt32,
																JSONPath:      "barkVolume",
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
			name:    "Should create an execution plan for an entity lookup with a field resolver returning union type",
			query:   `query EntityLookupWithUnion($representations: [_Any!]!, $checkAvailability: Boolean!) { _entities(representations: $representations) { ... on Product { __typename id name stockStatus(checkAvailability: $checkAvailability) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } }`,
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						MethodName:     "ResolveProductStockStatus",
						Kind:           CallKindResolve,
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.stockStatus"),
						Request: RPCMessage{
							Name: "ResolveProductStockStatusRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductStockStatusContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("result.name"),
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
												ResolvePath:   buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveProductStockStatusArgs",
										Fields: []RPCField{
											{
												Name:          "check_availability",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "checkAvailability",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveProductStockStatusResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductStockStatusResult",
										Fields: []RPCField{
											{
												Name:          "stock_status",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "stockStatus",
												Message: &RPCMessage{
													Name:        "ActionResult",
													OneOfType:   OneOfTypeUnion,
													MemberTypes: []string{"ActionSuccess", "ActionError"},
													FieldSelectionSet: RPCFieldSelectionSet{
														"ActionSuccess": {
															{
																Name:          "message",
																ProtoTypeName: DataTypeString,
																JSONPath:      "message",
															},
															{
																Name:          "timestamp",
																ProtoTypeName: DataTypeString,
																JSONPath:      "timestamp",
															},
														},
														"ActionError": {
															{
																Name:          "message",
																ProtoTypeName: DataTypeString,
																JSONPath:      "message",
															},
															{
																Name:          "code",
																ProtoTypeName: DataTypeString,
																JSONPath:      "code",
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
			name:    "Should create an execution plan for an entity lookup with a field resolver returning nested composite types",
			query:   `query EntityLookupWithNested($representations: [_Any!]!, $includeExtended: Boolean!) { _entities(representations: $representations) { ... on Product { __typename id name price productDetails(includeExtended: $includeExtended) { id description recommendedPet { ... on Cat { name meowVolume } ... on Dog { name barkVolume } } reviewSummary { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } } }`,
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
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupProductByIdRequestKey",
										MemberTypes: []string{"Product"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
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
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Product",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Product",
											},
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						MethodName:     "ResolveProductProductDetails",
						Kind:           CallKindResolve,
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.productDetails"),
						Request: RPCMessage{
							Name: "ResolveProductProductDetailsRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductProductDetailsContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
												ResolvePath:   buildPath("result.name"),
											},
											{
												Name:          "price",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "price",
												ResolvePath:   buildPath("result.price"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveProductProductDetailsArgs",
										Fields: []RPCField{
											{
												Name:          "include_extended",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "includeExtended",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveProductProductDetailsResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveProductProductDetailsResult",
										Fields: []RPCField{
											{
												Name:          "product_details",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "productDetails",
												Optional:      true,
												Message: &RPCMessage{
													Name: "ProductDetails",
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
														},
														{
															Name:          "description",
															ProtoTypeName: DataTypeString,
															JSONPath:      "description",
														},
														{
															Name:          "recommended_pet",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "recommendedPet",
															Message: &RPCMessage{
																Name:        "Animal",
																OneOfType:   OneOfTypeInterface,
																MemberTypes: []string{"Cat", "Dog"},
																FieldSelectionSet: RPCFieldSelectionSet{
																	"Cat": {
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "meow_volume",
																			ProtoTypeName: DataTypeInt32,
																			JSONPath:      "meowVolume",
																		},
																	},
																	"Dog": {
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "bark_volume",
																			ProtoTypeName: DataTypeInt32,
																			JSONPath:      "barkVolume",
																		},
																	},
																},
															},
														},
														{
															Name:          "review_summary",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "reviewSummary",
															Message: &RPCMessage{
																Name:        "ActionResult",
																OneOfType:   OneOfTypeUnion,
																MemberTypes: []string{"ActionSuccess", "ActionError"},
																FieldSelectionSet: RPCFieldSelectionSet{
																	"ActionSuccess": {
																		{
																			Name:          "message",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "message",
																		},
																		{
																			Name:          "timestamp",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "timestamp",
																		},
																	},
																	"ActionError": {
																		{
																			Name:          "message",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "message",
																		},
																		{
																			Name:          "code",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "code",
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

			planner, err := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			if err != nil {
				t.Fatalf("failed to create planner: %s", err)
			}
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

func TestEntityLookupWithRequiredFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:    "Should create an execution plan for an entity lookup with required fields",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Warehouse { __typename name location stockHealthScore } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Warehouse",
					SelectionSet: "id",
				},
				{
					TypeName:     "Warehouse",
					FieldName:    "stockHealthScore",
					SelectionSet: "inventoryCount restockData { lastRestockDate }",
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupWarehouseById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupWarehouseByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupWarehouseByIdRequestKey",
										MemberTypes: []string{"Warehouse"},
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "LookupWarehouseByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "_entities",
									Message: &RPCMessage{
										Name: "Warehouse",
										Fields: []RPCField{
											{
												Name:          "__typename",
												ProtoTypeName: DataTypeString,
												JSONPath:      "__typename",
												StaticValue:   "Warehouse",
											},
											{
												Name:          "name",
												ProtoTypeName: DataTypeString,
												JSONPath:      "name",
											},
											{
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
											},
										},
									},
								},
							},
						},
					},
					{
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireWarehouseStockHealthScoreById",
						ResponsePath: buildPath("_entities.stockHealthScore"),
						Request: RPCMessage{
							Name: "RequireWarehouseStockHealthScoreByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireWarehouseStockHealthScoreByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name:        "LookupWarehouseByIdRequestKey",
													MemberTypes: []string{"Warehouse"},
													Fields: []RPCField{
														{
															Name:          "id",
															ProtoTypeName: DataTypeString,
															JSONPath:      "id",
														},
													},
												},
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireWarehouseStockHealthScoreByIdFields",
													Fields: []RPCField{
														{
															Name:          "inventory_count",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "inventoryCount",
														},
														{
															Name:          "restock_data",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "restockData",
															Message: &RPCMessage{
																Name: "RequireWarehouseStockHealthScoreByIdFields.RestockData",
																Fields: []RPCField{
																	{
																		Name:          "last_restock_date",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "lastRestockDate",
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
							Name: "RequireWarehouseStockHealthScoreByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireWarehouseStockHealthScoreByIdResult",
										Fields: RPCFields{
											{
												Name:          "stock_health_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "stockHealthScore",
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

			planner, err := NewPlanner("Products", tt.mapping, tt.federationConfigs)
			if err != nil {
				t.Fatalf("failed to create planner: %s", err)
			}
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

		planner, err := NewPlanner("Products", tt.mapping, tt.federationConfigs)
		if err != nil {
			t.Fatalf("failed to create planner: %s", err)
		}
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
