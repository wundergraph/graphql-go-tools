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
										Name:        "LookupProductByIdKey",
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
										Name:        "LookupProductByIdKey",
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
										Name:        "LookupStorageByIdKey",
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
										Name:        "LookupUserByIdKey",
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
										Name:        "LookupUserByIdAndAddressKey",
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
										Name:        "LookupUserByIdAndNameKey",
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
										Name:        "LookupUserByIdAndNameKey",
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
										Name:        "LookupUserByIdAndNameAndAddressKey",
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
										Name:        "LookupUserByIdKey",
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
										Name:        "LookupProductByIdKey",
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
										Name:        "LookupStorageByIdKey",
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
										Name:        "LookupProductByIdKey",
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
										Name:        "LookupProductByIdKey",
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
													FragmentFields: RPCFieldSelectionSet{
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
										Name:        "LookupProductByIdKey",
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
													FragmentFields: RPCFieldSelectionSet{
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
										Name:        "LookupProductByIdKey",
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
																FragmentFields: RPCFieldSelectionSet{
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
																FragmentFields: RPCFieldSelectionSet{
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

var nestedInlineFragmentFederationSchema = testFederationSchemaString(`
			type Query {
				_entities(representations: [_Any!]!): [_Entity]!
			}
			type User @key(fields: "id") {
				id: ID!
				name: String!
				pet: Animal
			}
			interface Animal {
				id: ID!
				name: String!
				kind: String!
			}
			type Cat implements Animal {
				id: ID!
				name: String!
				kind: String!
				meowVolume: Int!
				owner: Owner!
				breed: CatBreed!
			}
			type Dog implements Animal {
				id: ID!
				name: String!
				kind: String!
				barkVolume: Int!
			}
			type Owner {
				id: ID!
				name: String!
				pet: Animal!
			}
			type CatBreed {
				id: ID!
				name: String!
				origin: String!
				characteristics: BreedCharacteristics!
			}
			type BreedCharacteristics {
				temperament: String!
			}
			`, []string{"User"})

var nestedInlineFragmentFederationMapping = &GRPCMapping{
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
	Fields: map[string]FieldMap{
		"Cat": {
			"meowVolume": {TargetName: "meow_volume"},
		},
		"Dog": {
			"barkVolume": {TargetName: "bark_volume"},
		},
	},
}

var nestedInlineFragmentFederationConfigs = plan.FederationFieldConfigurations{
	{
		TypeName:     "User",
		SelectionSet: "id",
	},
}

func TestEntityLookupWithNestedInlineFragments(t *testing.T) {
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
			name:              "Should create an execution plan for a nested message inside an entity with interface fragment and common fields",
			query:             `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id name pet { name kind ... on Cat { meowVolume owner { id name } } } } } }`,
			schema:            nestedInlineFragmentFederationSchema,
			mapping:           nestedInlineFragmentFederationMapping,
			federationConfigs: nestedInlineFragmentFederationConfigs,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdKey",
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
												Name:          "pet",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "pet",
												Optional:      true,
												Message: &RPCMessage{
													Name:      "Animal",
													OneOfType: OneOfTypeInterface,
													MemberTypes: []string{
														"Cat",
														"Dog",
													},
													Fields: []RPCField{
														{
															Name:          "name",
															ProtoTypeName: DataTypeString,
															JSONPath:      "name",
														},
														{
															Name:          "kind",
															ProtoTypeName: DataTypeString,
															JSONPath:      "kind",
														},
													},
													FragmentFields: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "meow_volume",
																ProtoTypeName: DataTypeInt32,
																JSONPath:      "meowVolume",
															},
															{
																Name:          "owner",
																ProtoTypeName: DataTypeMessage,
																JSONPath:      "owner",
																Message: &RPCMessage{
																	Name: "Owner",
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
			name:              "Should create an execution plan for a nested message inside an entity with interface fragment without common fields",
			query:             `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id pet { ... on Cat { breed { name origin } } } } } }`,
			schema:            nestedInlineFragmentFederationSchema,
			mapping:           nestedInlineFragmentFederationMapping,
			federationConfigs: nestedInlineFragmentFederationConfigs,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdKey",
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
												Name:          "pet",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "pet",
												Optional:      true,
												Message: &RPCMessage{
													Name:      "Animal",
													OneOfType: OneOfTypeInterface,
													MemberTypes: []string{
														"Cat",
														"Dog",
													},
													Fields: RPCFields{},
													FragmentFields: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "breed",
																ProtoTypeName: DataTypeMessage,
																JSONPath:      "breed",
																Message: &RPCMessage{
																	Name: "CatBreed",
																	Fields: []RPCField{
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "origin",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "origin",
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
			name:              "Should create an execution plan for a deeply nested message inside an entity with inline fragment",
			query:             `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id pet { ... on Cat { breed { characteristics { temperament } } } } } } }`,
			schema:            nestedInlineFragmentFederationSchema,
			mapping:           nestedInlineFragmentFederationMapping,
			federationConfigs: nestedInlineFragmentFederationConfigs,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdKey",
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
												Name:          "pet",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "pet",
												Optional:      true,
												Message: &RPCMessage{
													Name:      "Animal",
													OneOfType: OneOfTypeInterface,
													MemberTypes: []string{
														"Cat",
														"Dog",
													},
													Fields: RPCFields{},
													FragmentFields: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "breed",
																ProtoTypeName: DataTypeMessage,
																JSONPath:      "breed",
																Message: &RPCMessage{
																	Name: "CatBreed",
																	Fields: []RPCField{
																		{
																			Name:          "characteristics",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "characteristics",
																			Message: &RPCMessage{
																				Name: "BreedCharacteristics",
																				Fields: []RPCField{
																					{
																						Name:          "temperament",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "temperament",
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
			},
		},
		{
			name:              "Should create an execution plan for nested inline fragments through an intermediate regular message in entity",
			query:             `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on User { __typename id pet { ... on Cat { owner { name pet { ... on Cat { breed { name origin } } ... on Dog { barkVolume } } } } } } } }`,
			schema:            nestedInlineFragmentFederationSchema,
			mapping:           nestedInlineFragmentFederationMapping,
			federationConfigs: nestedInlineFragmentFederationConfigs,
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					{
						ServiceName: "Products",
						MethodName:  "LookupUserById",
						Kind:        CallKindEntity,
						Request: RPCMessage{
							Name: "LookupUserByIdRequest",
							Fields: []RPCField{
								{
									Name:          "keys",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name:        "LookupUserByIdKey",
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
												Name:          "pet",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "pet",
												Optional:      true,
												Message: &RPCMessage{
													Name:      "Animal",
													OneOfType: OneOfTypeInterface,
													MemberTypes: []string{
														"Cat",
														"Dog",
													},
													Fields: RPCFields{},
													FragmentFields: RPCFieldSelectionSet{
														"Cat": {
															{
																Name:          "owner",
																ProtoTypeName: DataTypeMessage,
																JSONPath:      "owner",
																Message: &RPCMessage{
																	Name: "Owner",
																	Fields: []RPCField{
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "pet",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "pet",
																			Message: &RPCMessage{
																				Name:      "Animal",
																				OneOfType: OneOfTypeInterface,
																				MemberTypes: []string{
																					"Cat",
																					"Dog",
																				},
																				Fields: RPCFields{},
																				FragmentFields: RPCFieldSelectionSet{
																					"Cat": {
																						{
																							Name:          "breed",
																							ProtoTypeName: DataTypeMessage,
																							JSONPath:      "breed",
																							Message: &RPCMessage{
																								Name: "CatBreed",
																								Fields: []RPCField{
																									{
																										Name:          "name",
																										ProtoTypeName: DataTypeString,
																										JSONPath:      "name",
																									},
																									{
																										Name:          "origin",
																										ProtoTypeName: DataTypeString,
																										JSONPath:      "origin",
																									},
																								},
																							},
																						},
																					},
																					"Dog": {
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

// complexResolverInNestedMessageFederationSchema defines an entity Product that has a
// regular (non-resolver) nested field "specs: ProductSpecs!". ProductSpecs contains a
// resolver field "relatedProduct" that returns another Product (complex return type),
// followed by a plain scalar field "dimensions". This combination is used to reproduce
// the bug where LeaveSelectionSet incorrectly pops the responseMessageAncestors stack
// for the resolver's selection set, causing "dimensions" to land in the wrong message.
var complexResolverInNestedMessageFederationSchema = `
scalar connect__FieldSet
directive @connect__fieldResolver(context: connect__FieldSet!) on FIELD_DEFINITION

schema {
	query: Query
}

type Query {
	_entities(representations: [_Any!]!): [_Entity]!
}

type Product @key(fields: "id") {
	id: ID!
	name: String!
	specs: ProductSpecs!
}

type ProductSpecs {
	id: ID!
	weight: Float!
	relatedProduct(category: String!): Product @connect__fieldResolver(context: "id")
	dimensions: String!
}

union _Entity = Product
scalar _Any
`

var complexResolverInNestedMessageFederationMapping = &GRPCMapping{
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
	ResolveRPCs: RPCConfigMap[ResolveRPCMapping]{
		"ProductSpecs": {
			"relatedProduct": ResolveRPCTypeField{
				FieldMappingData: FieldMapData{
					TargetName: "related_product",
					ArgumentMappings: FieldArgumentMap{
						"category": "category",
					},
				},
				RPC:      "ResolveProductSpecsRelatedProduct",
				Request:  "ResolveProductSpecsRelatedProductRequest",
				Response: "ResolveProductSpecsRelatedProductResponse",
			},
		},
	},
	Fields: map[string]FieldMap{
		"Product": {
			"id":    {TargetName: "id"},
			"name":  {TargetName: "name"},
			"specs": {TargetName: "specs"},
		},
		"ProductSpecs": {
			"id":        {TargetName: "id"},
			"weight":    {TargetName: "weight"},
			"dimensions": {TargetName: "dimensions"},
			"relatedProduct": {
				TargetName: "related_product",
				ArgumentMappings: FieldArgumentMap{
					"category": "category",
				},
			},
		},
	},
}

var complexResolverInNestedMessageFederationConfigs = plan.FederationFieldConfigurations{
	{
		TypeName:     "Product",
		SelectionSet: "id",
	},
}

// TestEntityLookupWithFieldResolvers_ComplexResolverInNestedMessage tests that fields
// following a complex-return-type resolver inside a nested message of an entity are placed
// into the correct parent message. This is a regression test for a bug where
// LeaveSelectionSet in the federation visitor incorrectly called leaveNestedField for
// a resolver field whose selection set never called enterNestedField.
//
// With the bug, the "dimensions" field that comes after "relatedProduct" in the
// "specs" selection set ends up in Product.Fields instead of ProductSpecs.Fields.
func TestEntityLookupWithFieldResolvers_ComplexResolverInNestedMessage(t *testing.T) {
	t.Parallel()

	query := `query EntityLookup($representations: [_Any!]!, $category: String!) {
		_entities(representations: $representations) {
			... on Product {
				__typename
				id
				specs {
					weight
					relatedProduct(category: $category) {
						id
						name
					}
					dimensions
				}
			}
		}
	}`

	expectedPlan := &RPCExecutionPlan{
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
								Name:        "LookupProductByIdKey",
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
										Name:          "specs",
										ProtoTypeName: DataTypeMessage,
										JSONPath:      "specs",
										// Both "weight" and "dimensions" must be in ProductSpecs.Fields.
										// The bug causes "dimensions" to be placed in Product.Fields
										// instead, because LeaveSelectionSet for the relatedProduct
										// resolver selection set incorrectly pops the ProductSpecs
										// message off responseMessageAncestors.
										Message: &RPCMessage{
											Name: "ProductSpecs",
											Fields: []RPCField{
												{
													Name:          "weight",
													ProtoTypeName: DataTypeDouble,
													JSONPath:      "weight",
												},
												{
													Name:          "dimensions",
													ProtoTypeName: DataTypeString,
													JSONPath:      "dimensions",
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
				ID:             1,
				DependentCalls: []int{0},
				ServiceName:    "Products",
				MethodName:     "ResolveProductSpecsRelatedProduct",
				Kind:           CallKindResolve,
				ResponsePath:   buildPath("_entities.specs.relatedProduct"),
				Request: RPCMessage{
					Name: "ResolveProductSpecsRelatedProductRequest",
					Fields: []RPCField{
						{
							Name:          "context",
							ProtoTypeName: DataTypeMessage,
							Repeated:      true,
							Message: &RPCMessage{
								Name: "ResolveProductSpecsRelatedProductContext",
								Fields: []RPCField{
									{
										Name:          "id",
										ProtoTypeName: DataTypeString,
										JSONPath:      "id",
										ResolvePath:   buildPath("result.specs.id"),
									},
								},
							},
						},
						{
							Name:          "field_args",
							ProtoTypeName: DataTypeMessage,
							Message: &RPCMessage{
								Name: "ResolveProductSpecsRelatedProductArgs",
								Fields: []RPCField{
									{
										Name:          "category",
										ProtoTypeName: DataTypeString,
										JSONPath:      "category",
									},
								},
							},
						},
					},
				},
				Response: RPCMessage{
					Name: "ResolveProductSpecsRelatedProductResponse",
					Fields: []RPCField{
						{
							Name:          "result",
							ProtoTypeName: DataTypeMessage,
							JSONPath:      "result",
							Repeated:      true,
							Message: &RPCMessage{
								Name: "ResolveProductSpecsRelatedProductResult",
								Fields: []RPCField{
									{
										Name:          "related_product",
										ProtoTypeName: DataTypeMessage,
										JSONPath:      "relatedProduct",
										Optional:      true,
										Message: &RPCMessage{
											Name: "Product",
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
						},
					},
				},
			},
		},
	}

	t.Run("Should place fields after a complex resolver correctly in the parent message", func(t *testing.T) {
		t.Parallel()

		definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(complexResolverInNestedMessageFederationSchema)
		report := operationreport.Report{}
		astvalidation.DefaultDefinitionValidator().Validate(&definition, &report)
		if report.HasErrors() {
			t.Fatalf("failed to validate schema: %s", report.Error())
		}

		operation, report := astparser.ParseGraphqlDocumentString(query)
		if report.HasErrors() {
			t.Fatalf("failed to parse query: %s", report.Error())
		}

		planner, err := NewPlanner("Products", complexResolverInNestedMessageFederationMapping, complexResolverInNestedMessageFederationConfigs)
		if err != nil {
			t.Fatalf("failed to create planner: %s", err)
		}
		plan, err := planner.PlanOperation(&operation, &definition)
		if err != nil {
			t.Fatalf("failed to plan operation: %s", err)
		}

		diff := cmp.Diff(expectedPlan, plan)
		if diff != "" {
			t.Fatalf("execution plan mismatch: %s", diff)
		}
	})
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
