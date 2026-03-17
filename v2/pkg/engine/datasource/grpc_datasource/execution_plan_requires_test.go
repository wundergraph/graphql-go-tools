package grpcdatasource

import (
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
		{
			name:    "Should create an execution plan for tagSummary requiring tags list",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name tagSummary } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
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
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageTagSummaryById",
						ResponsePath: buildPath("_entities.tagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "tags",
															ProtoTypeName: DataTypeString,
															Repeated:      true,
															JSONPath:      "tags",
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
							Name: "RequireStorageTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tagSummary",
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
			name:    "Should create an execution plan for optionalTagSummary requiring nullable list",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename optionalTagSummary } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
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
						MethodName:   "RequireStorageOptionalTagSummaryById",
						ResponsePath: buildPath("_entities.optionalTagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageOptionalTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageOptionalTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "optional_tags",
															ProtoTypeName: DataTypeString,
															JSONPath:      "optionalTags",
															Optional:      true,
															IsListType:    true,
															ListMetadata: &ListMetadata{
																NestingLevel: 1,
																LevelInfo:    []LevelInfo{{Optional: true}},
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
							Name: "RequireStorageOptionalTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "optional_tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalTagSummary",
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
		},
		{
			name:    "Should create an execution plan for metadataScore requiring nested object fields",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename metadataScore } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
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
						MethodName:   "RequireStorageMetadataScoreById",
						ResponsePath: buildPath("_entities.metadataScore"),
						Request: RPCMessage{
							Name: "RequireStorageMetadataScoreByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageMetadataScoreByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageMetadataScoreByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
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
							Name: "RequireStorageMetadataScoreByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdResult",
										Fields: RPCFields{
											{
												Name:          "metadata_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "metadataScore",
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
			name:    "Should create an execution plan for processedMetadata returning complex type",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename processedMetadata { capacity zone priority } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadata",
					SelectionSet: "metadata { capacity zone priority }",
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
						MethodName:   "RequireStorageProcessedMetadataById",
						ResponsePath: buildPath("_entities.processedMetadata"),
						Request: RPCMessage{
							Name: "RequireStorageProcessedMetadataByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedMetadataByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageProcessedMetadataByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageProcessedMetadataByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
																	},
																	{
																		Name:          "priority",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "priority",
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
							Name: "RequireStorageProcessedMetadataByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedMetadataByIdResult",
										Fields: RPCFields{
											{
												Name:          "processed_metadata",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "processedMetadata",
												Message: &RPCMessage{
													Name: "StorageMetadata",
													Fields: []RPCField{
														{
															Name:          "capacity",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "capacity",
														},
														{
															Name:          "zone",
															ProtoTypeName: DataTypeString,
															JSONPath:      "zone",
														},
														{
															Name:          "priority",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "priority",
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
			name:    "Should create an execution plan for optionalProcessedMetadata returning nullable complex type",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename optionalProcessedMetadata { capacity zone } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedMetadata",
					SelectionSet: "metadata { capacity zone }",
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
						MethodName:   "RequireStorageOptionalProcessedMetadataById",
						ResponsePath: buildPath("_entities.optionalProcessedMetadata"),
						Request: RPCMessage{
							Name: "RequireStorageOptionalProcessedMetadataByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalProcessedMetadataByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageOptionalProcessedMetadataByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageOptionalProcessedMetadataByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
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
							Name: "RequireStorageOptionalProcessedMetadataByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalProcessedMetadataByIdResult",
										Fields: RPCFields{
											{
												Name:          "optional_processed_metadata",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "optionalProcessedMetadata",
												Optional:      true,
												Message: &RPCMessage{
													Name: "StorageMetadata",
													Fields: []RPCField{
														{
															Name:          "capacity",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "capacity",
														},
														{
															Name:          "zone",
															ProtoTypeName: DataTypeString,
															JSONPath:      "zone",
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
			name:    "Should create an execution plan for processedTags returning list",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename processedTags } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedTags",
					SelectionSet: "tags",
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
						MethodName:   "RequireStorageProcessedTagsById",
						ResponsePath: buildPath("_entities.processedTags"),
						Request: RPCMessage{
							Name: "RequireStorageProcessedTagsByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedTagsByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageProcessedTagsByIdFields",
													Fields: []RPCField{
														{
															Name:          "tags",
															ProtoTypeName: DataTypeString,
															Repeated:      true,
															JSONPath:      "tags",
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
							Name: "RequireStorageProcessedTagsByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedTagsByIdResult",
										Fields: RPCFields{
											{
												Name:          "processed_tags",
												ProtoTypeName: DataTypeString,
												Repeated:      true,
												JSONPath:      "processedTags",
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
			name:    "Should create an execution plan for multiple requires fields in single query",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name tagSummary metadataScore } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
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
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageTagSummaryById",
						ResponsePath: buildPath("_entities.tagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "tags",
															ProtoTypeName: DataTypeString,
															Repeated:      true,
															JSONPath:      "tags",
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
							Name: "RequireStorageTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tagSummary",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageMetadataScoreById",
						ResponsePath: buildPath("_entities.metadataScore"),
						Request: RPCMessage{
							Name: "RequireStorageMetadataScoreByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageMetadataScoreByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageMetadataScoreByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
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
							Name: "RequireStorageMetadataScoreByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdResult",
										Fields: RPCFields{
											{
												Name:          "metadata_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "metadataScore",
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
			name:    "requires with direct enum field",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name kindSummary } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "kindSummary",
					SelectionSet: "storageKind",
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
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageKindSummaryById",
						ResponsePath: buildPath("_entities.kindSummary"),
						Request: RPCMessage{
							Name: "RequireStorageKindSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageKindSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageKindSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "storage_kind",
															ProtoTypeName: DataTypeEnum,
															JSONPath:      "storageKind",
															EnumName:      "CategoryKind",
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
							Name: "RequireStorageKindSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageKindSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "kind_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "kindSummary",
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
			name:    "requires with nested enum in type",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name categoryInfoSummary } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "categoryInfoSummary",
					SelectionSet: "categoryInfo { kind name }",
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
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageCategoryInfoSummaryById",
						ResponsePath: buildPath("_entities.categoryInfoSummary"),
						Request: RPCMessage{
							Name: "RequireStorageCategoryInfoSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageCategoryInfoSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageCategoryInfoSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "category_info",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "categoryInfo",
															Message: &RPCMessage{
																Name: "RequireStorageCategoryInfoSummaryByIdFields.StorageCategoryInfo",
																Fields: []RPCField{
																	{
																		Name:          "kind",
																		ProtoTypeName: DataTypeEnum,
																		JSONPath:      "kind",
																		EnumName:      "CategoryKind",
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
						Response: RPCMessage{
							Name: "RequireStorageCategoryInfoSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageCategoryInfoSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "category_info_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "categoryInfoSummary",
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

func TestExecutionPlan_FederationRequires_AbstractTypes(t *testing.T) {
	t.Parallel()

	// storageEntityLookupCall returns the common entity lookup call shared by all tests
	storageEntityLookupCall := func() RPCCall {
		return RPCCall{
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
									Name:          "name",
									ProtoTypeName: DataTypeString,
									JSONPath:      "name",
								},
							},
						},
					},
				},
			},
		}
	}

	storageKeyMessage := func() *RPCMessage {
		return &RPCMessage{
			Name:        "LookupStorageByIdRequestKey",
			MemberTypes: []string{"Storage"},
			Fields: []RPCField{
				{
					Name:          "id",
					ProtoTypeName: DataTypeString,
					JSONPath:      "id",
				},
			},
		}
	}

	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:    "requires with interface type in selection set",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name itemInfo } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemInfo",
					SelectionSet: `primaryItem { ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageItemInfoById",
						ResponsePath: buildPath("_entities.itemInfo"),
						Request: RPCMessage{
							Name: "RequireStorageItemInfoByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageItemInfoByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageItemInfoByIdFields",
													Fields: []RPCField{
														{
															Name:          "primary_item",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "primaryItem",
															Message: &RPCMessage{
																Name:        "RequireStorageItemInfoByIdFields.StorageItem",
																OneOfType:   OneOfTypeInterface,
																MemberTypes: []string{"PalletItem", "ContainerItem"},
																FragmentFields: RPCFieldSelectionSet{
																	"PalletItem": {
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "pallet_count",
																			ProtoTypeName: DataTypeInt32,
																			JSONPath:      "palletCount",
																		},
																	},
																	"ContainerItem": {
																		{
																			Name:          "name",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "name",
																		},
																		{
																			Name:          "container_size",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "containerSize",
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
						Response: RPCMessage{
							Name: "RequireStorageItemInfoByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageItemInfoByIdResult",
										Fields: RPCFields{
											{
												Name:          "item_info",
												ProtoTypeName: DataTypeString,
												JSONPath:      "itemInfo",
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
			name:    "requires with union type in selection set",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name operationReport } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "operationReport",
					SelectionSet: `lastStorageOperation { ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageOperationReportById",
						ResponsePath: buildPath("_entities.operationReport"),
						Request: RPCMessage{
							Name: "RequireStorageOperationReportByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageOperationReportByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageOperationReportByIdFields",
													Fields: []RPCField{
														{
															Name:          "last_storage_operation",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "lastStorageOperation",
															Message: &RPCMessage{
																Name:        "RequireStorageOperationReportByIdFields.StorageOperationResult",
																OneOfType:   OneOfTypeUnion,
																MemberTypes: []string{"StorageSuccess", "StorageFailure"},
																FragmentFields: RPCFieldSelectionSet{
																	"StorageSuccess": {
																		{
																			Name:          "message",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "message",
																		},
																		{
																			Name:          "completed_at",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "completedAt",
																		},
																	},
																	"StorageFailure": {
																		{
																			Name:          "message",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "message",
																		},
																		{
																			Name:          "error_code",
																			ProtoTypeName: DataTypeString,
																			JSONPath:      "errorCode",
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
						Response: RPCMessage{
							Name: "RequireStorageOperationReportByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageOperationReportByIdResult",
										Fields: RPCFields{
											{
												Name:          "operation_report",
												ProtoTypeName: DataTypeString,
												JSONPath:      "operationReport",
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
			name:    "requires with concrete type wrapping abstract type",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name securitySummary } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "securitySummary",
					SelectionSet: `securitySetup { securityLevel primaryItem { ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageSecuritySummaryById",
						ResponsePath: buildPath("_entities.securitySummary"),
						Request: RPCMessage{
							Name: "RequireStorageSecuritySummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageSecuritySummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageSecuritySummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "security_setup",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "securitySetup",
															Message: &RPCMessage{
																Name: "RequireStorageSecuritySummaryByIdFields.SecuritySetup",
																Fields: []RPCField{
																	{
																		Name:          "security_level",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "securityLevel",
																	},
																	{
																		Name:          "primary_item",
																		ProtoTypeName: DataTypeMessage,
																		JSONPath:      "primaryItem",
																		Message: &RPCMessage{
																			Name:        "RequireStorageSecuritySummaryByIdFields.SecuritySetup.StorageItem",
																			OneOfType:   OneOfTypeInterface,
																			MemberTypes: []string{"PalletItem", "ContainerItem"},
																			FragmentFields: RPCFieldSelectionSet{
																				"PalletItem": {
																					{
																						Name:          "name",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "name",
																					},
																					{
																						Name:          "pallet_count",
																						ProtoTypeName: DataTypeInt32,
																						JSONPath:      "palletCount",
																					},
																				},
																				"ContainerItem": {
																					{
																						Name:          "name",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "name",
																					},
																					{
																						Name:          "container_size",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "containerSize",
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
						Response: RPCMessage{
							Name: "RequireStorageSecuritySummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageSecuritySummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "security_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "securitySummary",
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
			name:    "requires with nested concrete message inside interface fragment",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name itemHandlerInfo } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemHandlerInfo",
					SelectionSet: `primaryItem { ... on PalletItem { handler { name } } ... on ContainerItem { handler { name } } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageItemHandlerInfoById",
						ResponsePath: buildPath("_entities.itemHandlerInfo"),
						Request: RPCMessage{
							Name: "RequireStorageItemHandlerInfoByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageItemHandlerInfoByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageItemHandlerInfoByIdFields",
													Fields: []RPCField{
														{
															Name:          "primary_item",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "primaryItem",
															Message: &RPCMessage{
																Name:        "RequireStorageItemHandlerInfoByIdFields.StorageItem",
																OneOfType:   OneOfTypeInterface,
																MemberTypes: []string{"PalletItem", "ContainerItem"},
																FragmentFields: RPCFieldSelectionSet{
																	"PalletItem": {
																		{
																			Name:          "handler",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "handler",
																			Message: &RPCMessage{
																				Name: "RequireStorageItemHandlerInfoByIdFields.StorageItem.ItemHandler",
																				Fields: []RPCField{
																					{
																						Name:          "name",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "name",
																					},
																				},
																			},
																		},
																	},
																	"ContainerItem": {
																		{
																			Name:          "handler",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "handler",
																			Message: &RPCMessage{
																				Name: "RequireStorageItemHandlerInfoByIdFields.StorageItem.ItemHandler",
																				Fields: []RPCField{
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
						Response: RPCMessage{
							Name: "RequireStorageItemHandlerInfoByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageItemHandlerInfoByIdResult",
										Fields: RPCFields{
											{
												Name:          "item_handler_info",
												ProtoTypeName: DataTypeString,
												JSONPath:      "itemHandlerInfo",
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
			name:    "requires with deep concrete nesting inside interface fragment",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name itemSpecsInfo } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemSpecsInfo",
					SelectionSet: `primaryItem { ... on PalletItem { specs { name dimensions { length width } } } ... on ContainerItem { specs { name dimensions { length width } } } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageItemSpecsInfoById",
						ResponsePath: buildPath("_entities.itemSpecsInfo"),
						Request: RPCMessage{
							Name: "RequireStorageItemSpecsInfoByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageItemSpecsInfoByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageItemSpecsInfoByIdFields",
													Fields: []RPCField{
														{
															Name:          "primary_item",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "primaryItem",
															Message: &RPCMessage{
																Name:        "RequireStorageItemSpecsInfoByIdFields.StorageItem",
																OneOfType:   OneOfTypeInterface,
																MemberTypes: []string{"PalletItem", "ContainerItem"},
																FragmentFields: RPCFieldSelectionSet{
																	"PalletItem": {
																		{
																			Name:          "specs",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "specs",
																			Message: &RPCMessage{
																				Name: "RequireStorageItemSpecsInfoByIdFields.StorageItem.PalletSpecs",
																				Fields: []RPCField{
																					{
																						Name:          "name",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "name",
																					},
																					{
																						Name:          "dimensions",
																						ProtoTypeName: DataTypeMessage,
																						JSONPath:      "dimensions",
																						Message: &RPCMessage{
																							Name: "RequireStorageItemSpecsInfoByIdFields.StorageItem.PalletSpecs.Dimensions",
																							Fields: []RPCField{
																								{
																									Name:          "length",
																									ProtoTypeName: DataTypeDouble,
																									JSONPath:      "length",
																								},
																								{
																									Name:          "width",
																									ProtoTypeName: DataTypeDouble,
																									JSONPath:      "width",
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																	"ContainerItem": {
																		{
																			Name:          "specs",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "specs",
																			Message: &RPCMessage{
																				Name: "RequireStorageItemSpecsInfoByIdFields.StorageItem.ContainerSpecs",
																				Fields: []RPCField{
																					{
																						Name:          "name",
																						ProtoTypeName: DataTypeString,
																						JSONPath:      "name",
																					},
																					{
																						Name:          "dimensions",
																						ProtoTypeName: DataTypeMessage,
																						JSONPath:      "dimensions",
																						Message: &RPCMessage{
																							Name: "RequireStorageItemSpecsInfoByIdFields.StorageItem.ContainerSpecs.Dimensions",
																							Fields: []RPCField{
																								{
																									Name:          "length",
																									ProtoTypeName: DataTypeDouble,
																									JSONPath:      "length",
																								},
																								{
																									Name:          "width",
																									ProtoTypeName: DataTypeDouble,
																									JSONPath:      "width",
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
						Response: RPCMessage{
							Name: "RequireStorageItemSpecsInfoByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageItemSpecsInfoByIdResult",
										Fields: RPCFields{
											{
												Name:          "item_specs_info",
												ProtoTypeName: DataTypeString,
												JSONPath:      "itemSpecsInfo",
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
			name:    "requires with nested abstract type through concrete intermediary",
			query:   `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Storage { __typename name deepItemInfo } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "deepItemInfo",
					SelectionSet: `primaryItem { ... on PalletItem { handler { assignedItem { ... on ContainerItem { name containerSize } ... on PalletItem { name palletCount } } } } ... on ContainerItem { handler { name } } }`,
				},
			},
			expectedPlan: &RPCExecutionPlan{
				Calls: []RPCCall{
					storageEntityLookupCall(),
					{
						ID:           1,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageDeepItemInfoById",
						ResponsePath: buildPath("_entities.deepItemInfo"),
						Request: RPCMessage{
							Name: "RequireStorageDeepItemInfoByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageDeepItemInfoByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
												Message:       storageKeyMessage(),
											},
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageDeepItemInfoByIdFields",
													Fields: []RPCField{
														{
															Name:          "primary_item",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "primaryItem",
															Message: &RPCMessage{
																Name:        "RequireStorageDeepItemInfoByIdFields.StorageItem",
																OneOfType:   OneOfTypeInterface,
																MemberTypes: []string{"PalletItem", "ContainerItem"},
																FragmentFields: RPCFieldSelectionSet{
																	"PalletItem": {
																		{
																			Name:          "handler",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "handler",
																			Message: &RPCMessage{
																				Name: "RequireStorageDeepItemInfoByIdFields.StorageItem.ItemHandler",
																				Fields: []RPCField{
																					{
																						Name:          "assigned_item",
																						ProtoTypeName: DataTypeMessage,
																						JSONPath:      "assignedItem",
																						Message: &RPCMessage{
																							Name:        "RequireStorageDeepItemInfoByIdFields.StorageItem.ItemHandler.StorageItem",
																							OneOfType:   OneOfTypeInterface,
																							MemberTypes: []string{"PalletItem", "ContainerItem"},
																							FragmentFields: RPCFieldSelectionSet{
																								"ContainerItem": {
																									{
																										Name:          "name",
																										ProtoTypeName: DataTypeString,
																										JSONPath:      "name",
																									},
																									{
																										Name:          "container_size",
																										ProtoTypeName: DataTypeString,
																										JSONPath:      "containerSize",
																									},
																								},
																								"PalletItem": {
																									{
																										Name:          "name",
																										ProtoTypeName: DataTypeString,
																										JSONPath:      "name",
																									},
																									{
																										Name:          "pallet_count",
																										ProtoTypeName: DataTypeInt32,
																										JSONPath:      "palletCount",
																									},
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																	"ContainerItem": {
																		{
																			Name:          "handler",
																			ProtoTypeName: DataTypeMessage,
																			JSONPath:      "handler",
																			Message: &RPCMessage{
																				Name: "RequireStorageDeepItemInfoByIdFields.StorageItem.ItemHandler",
																				Fields: []RPCField{
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
						Response: RPCMessage{
							Name: "RequireStorageDeepItemInfoByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageDeepItemInfoByIdResult",
										Fields: RPCFields{
											{
												Name:          "deep_item_info",
												ProtoTypeName: DataTypeString,
												JSONPath:      "deepItemInfo",
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

func TestExecutionPlan_FederationRequires_WithFieldResolvers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		query             string
		expectedPlan      *RPCExecutionPlan
		mapping           *GRPCMapping
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:    "Should create execution plan for tagSummary (requires) + storageStatus (field resolver)",
			query:   `query EntityLookup($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ... on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message } } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
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
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						Kind:           CallKindResolve,
						MethodName:     "ResolveStorageStorageStatus",
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.storageStatus"),
						Request: RPCMessage{
							Name: "ResolveStorageStorageStatusRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageStorageStatusContext",
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
										Name: "ResolveStorageStorageStatusArgs",
										Fields: []RPCField{
											{
												Name:          "check_health",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "checkHealth",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveStorageStorageStatusResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageStorageStatusResult",
										Fields: []RPCField{
											{
												Name:          "storage_status",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "storageStatus",
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
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageTagSummaryById",
						ResponsePath: buildPath("_entities.tagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "tags",
															ProtoTypeName: DataTypeString,
															Repeated:      true,
															JSONPath:      "tags",
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
							Name: "RequireStorageTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tagSummary",
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
			name:    "Should create execution plan for metadataScore (requires) + linkedStorages (field resolver)",
			query:   `query EntityLookup($representations: [_Any!]!, $depth: Int!) { _entities(representations: $representations) { ... on Storage { __typename id metadataScore linkedStorages(depth: $depth) { id name } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
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
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						Kind:           CallKindResolve,
						MethodName:     "ResolveStorageLinkedStorages",
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.linkedStorages"),
						Request: RPCMessage{
							Name: "ResolveStorageLinkedStoragesRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesContext",
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
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
												ResolvePath:   buildPath("result.location"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesArgs",
										Fields: []RPCField{
											{
												Name:          "depth",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "depth",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveStorageLinkedStoragesResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesResult",
										Fields: []RPCField{
											{
												Name:          "linked_storages",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "linkedStorages",
												Repeated:      true,
												Message: &RPCMessage{
													Name: "Storage",
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
					{
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageMetadataScoreById",
						ResponsePath: buildPath("_entities.metadataScore"),
						Request: RPCMessage{
							Name: "RequireStorageMetadataScoreByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageMetadataScoreByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageMetadataScoreByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
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
							Name: "RequireStorageMetadataScoreByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdResult",
										Fields: RPCFields{
											{
												Name:          "metadata_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "metadataScore",
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
			name:    "Should create execution plan for optionalTagSummary (nullable requires) + nearbyStorages (nullable field resolver)",
			query:   `query EntityLookup($representations: [_Any!]!, $radius: Int) { _entities(representations: $representations) { ... on Storage { __typename id optionalTagSummary nearbyStorages(radius: $radius) { id name } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
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
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						Kind:           CallKindResolve,
						MethodName:     "ResolveStorageNearbyStorages",
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.nearbyStorages"),
						Request: RPCMessage{
							Name: "ResolveStorageNearbyStoragesRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageNearbyStoragesContext",
										Fields: []RPCField{
											{
												Name:          "id",
												ProtoTypeName: DataTypeString,
												JSONPath:      "id",
												ResolvePath:   buildPath("result.id"),
											},
											{
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
												ResolvePath:   buildPath("result.location"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveStorageNearbyStoragesArgs",
										Fields: []RPCField{
											{
												Name:          "radius",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "radius",
												Optional:      true,
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveStorageNearbyStoragesResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageNearbyStoragesResult",
										Fields: []RPCField{
											{
												Name:          "nearby_storages",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "nearbyStorages",
												Optional:      true,
												IsListType:    true,
												ListMetadata: &ListMetadata{
													NestingLevel: 1,
													LevelInfo:    []LevelInfo{{Optional: true}},
												},
												Message: &RPCMessage{
													Name: "Storage",
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
					{
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageOptionalTagSummaryById",
						ResponsePath: buildPath("_entities.optionalTagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageOptionalTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageOptionalTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "optional_tags",
															ProtoTypeName: DataTypeString,
															JSONPath:      "optionalTags",
															Optional:      true,
															IsListType:    true,
															ListMetadata: &ListMetadata{
																NestingLevel: 1,
																LevelInfo:    []LevelInfo{{Optional: true}},
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
							Name: "RequireStorageOptionalTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageOptionalTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "optional_tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "optionalTagSummary",
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
		},
		{
			name:    "Should create execution plan for multiple requires (tagSummary + metadataScore) + storageStatus (field resolver)",
			query:   `query EntityLookup($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ... on Storage { __typename id tagSummary metadataScore storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message } } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
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
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						Kind:           CallKindResolve,
						MethodName:     "ResolveStorageStorageStatus",
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.storageStatus"),
						Request: RPCMessage{
							Name: "ResolveStorageStorageStatusRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageStorageStatusContext",
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
										Name: "ResolveStorageStorageStatusArgs",
										Fields: []RPCField{
											{
												Name:          "check_health",
												ProtoTypeName: DataTypeBool,
												JSONPath:      "checkHealth",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveStorageStorageStatusResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageStorageStatusResult",
										Fields: []RPCField{
											{
												Name:          "storage_status",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "storageStatus",
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
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageTagSummaryById",
						ResponsePath: buildPath("_entities.tagSummary"),
						Request: RPCMessage{
							Name: "RequireStorageTagSummaryByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageTagSummaryByIdFields",
													Fields: []RPCField{
														{
															Name:          "tags",
															ProtoTypeName: DataTypeString,
															Repeated:      true,
															JSONPath:      "tags",
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
							Name: "RequireStorageTagSummaryByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageTagSummaryByIdResult",
										Fields: RPCFields{
											{
												Name:          "tag_summary",
												ProtoTypeName: DataTypeString,
												JSONPath:      "tagSummary",
											},
										},
									},
								},
							},
						},
					},
					{
						ID:           3,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageMetadataScoreById",
						ResponsePath: buildPath("_entities.metadataScore"),
						Request: RPCMessage{
							Name: "RequireStorageMetadataScoreByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageMetadataScoreByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageMetadataScoreByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
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
							Name: "RequireStorageMetadataScoreByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageMetadataScoreByIdResult",
										Fields: RPCFields{
											{
												Name:          "metadata_score",
												ProtoTypeName: DataTypeDouble,
												JSONPath:      "metadataScore",
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
			name:    "Should create execution plan for processedMetadata (complex return requires) + linkedStorages (field resolver)",
			query:   `query EntityLookup($representations: [_Any!]!, $depth: Int!) { _entities(representations: $representations) { ... on Storage { __typename id processedMetadata { capacity zone priority } linkedStorages(depth: $depth) { id name } } } }`,
			mapping: testMapping(),
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadata",
					SelectionSet: "metadata { capacity zone priority }",
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
										},
									},
								},
							},
						},
					},
					{
						ID:             1,
						ServiceName:    "Products",
						Kind:           CallKindResolve,
						MethodName:     "ResolveStorageLinkedStorages",
						DependentCalls: []int{0},
						ResponsePath:   buildPath("_entities.linkedStorages"),
						Request: RPCMessage{
							Name: "ResolveStorageLinkedStoragesRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesContext",
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
												Name:          "location",
												ProtoTypeName: DataTypeString,
												JSONPath:      "location",
												ResolvePath:   buildPath("result.location"),
											},
										},
									},
								},
								{
									Name:          "field_args",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "",
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesArgs",
										Fields: []RPCField{
											{
												Name:          "depth",
												ProtoTypeName: DataTypeInt32,
												JSONPath:      "depth",
											},
										},
									},
								},
							},
						},
						Response: RPCMessage{
							Name: "ResolveStorageLinkedStoragesResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									JSONPath:      "result",
									Repeated:      true,
									Message: &RPCMessage{
										Name: "ResolveStorageLinkedStoragesResult",
										Fields: []RPCField{
											{
												Name:          "linked_storages",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "linkedStorages",
												Repeated:      true,
												Message: &RPCMessage{
													Name: "Storage",
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
					{
						ID:           2,
						ServiceName:  "Products",
						Kind:         CallKindRequired,
						MethodName:   "RequireStorageProcessedMetadataById",
						ResponsePath: buildPath("_entities.processedMetadata"),
						Request: RPCMessage{
							Name: "RequireStorageProcessedMetadataByIdRequest",
							Fields: []RPCField{
								{
									Name:          "context",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "representations",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedMetadataByIdContext",
										Fields: []RPCField{
											{
												Name:          "key",
												ProtoTypeName: DataTypeMessage,
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
											{
												Name:          "fields",
												ProtoTypeName: DataTypeMessage,
												Message: &RPCMessage{
													Name: "RequireStorageProcessedMetadataByIdFields",
													Fields: []RPCField{
														{
															Name:          "metadata",
															ProtoTypeName: DataTypeMessage,
															JSONPath:      "metadata",
															Message: &RPCMessage{
																Name: "RequireStorageProcessedMetadataByIdFields.StorageMetadata",
																Fields: []RPCField{
																	{
																		Name:          "capacity",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "capacity",
																	},
																	{
																		Name:          "zone",
																		ProtoTypeName: DataTypeString,
																		JSONPath:      "zone",
																	},
																	{
																		Name:          "priority",
																		ProtoTypeName: DataTypeInt32,
																		JSONPath:      "priority",
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
							Name: "RequireStorageProcessedMetadataByIdResponse",
							Fields: []RPCField{
								{
									Name:          "result",
									ProtoTypeName: DataTypeMessage,
									Repeated:      true,
									JSONPath:      "result",
									Message: &RPCMessage{
										Name: "RequireStorageProcessedMetadataByIdResult",
										Fields: RPCFields{
											{
												Name:          "processed_metadata",
												ProtoTypeName: DataTypeMessage,
												JSONPath:      "processedMetadata",
												Message: &RPCMessage{
													Name: "StorageMetadata",
													Fields: []RPCField{
														{
															Name:          "capacity",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "capacity",
														},
														{
															Name:          "zone",
															ProtoTypeName: DataTypeString,
															JSONPath:      "zone",
														},
														{
															Name:          "priority",
															ProtoTypeName: DataTypeInt32,
															JSONPath:      "priority",
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
