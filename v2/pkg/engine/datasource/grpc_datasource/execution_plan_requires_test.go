package grpcdatasource

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func TestExecutionPlan_FederationRequires(t *testing.T) {
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
						ID:           1,
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
				fmt.Println(tt.expectedPlan.String())
				fmt.Println(plan.String())
				t.Fatalf("execution plan mismatch: %s", diff)
			}
		})
	}
}
