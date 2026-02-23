package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation_NestedRequiresProvides(t *testing.T) {
	t.Run("ignore requires due to implicit provide by parent", func(t *testing.T) {
		definition := `
			type Query {
				order: Order
			}

			type Order {
				id: ID!
				shippingInfo: ShippingInfo
			}

			type ShippingInfo {
				id: ID!
				details: DeliveryDetails
				status: String!
				fullLog: String!
			}

			type DeliveryDetails {
				trackingNumber: String!
				carrier: String!
				estimatedDelivery: String!
			}
		`

		service1SDL := `
			type Query {
				order: Order
			}

			type Order @key(fields: "id") {
				id: ID!
				shippingInfo: ShippingInfo @provides(fields: "details { trackingNumber carrier }")
			}

			type ShippingInfo @key(fields: "id") {
				id: ID!
				details: DeliveryDetails @external
				status: String! @requires(fields: "details { trackingNumber carrier }")
				fullLog: String! @requires(fields: "details { trackingNumber carrier estimatedDelivery }")
			}

			type DeliveryDetails {
				trackingNumber: String!
				carrier: String!
				estimatedDelivery: String!
			}
		`

		service1DataSourceConfig := mustDataSourceConfiguration(
			t,
			"service1",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"order"},
					},
					{
						TypeName:   "Order",
						FieldNames: []string{"id", "shippingInfo"},
					},
					{
						TypeName:           "ShippingInfo",
						FieldNames:         []string{"id", "status", "fullLog"},
						ExternalFieldNames: []string{"details"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "DeliveryDetails",
						FieldNames: []string{"trackingNumber", "carrier", "estimatedDelivery"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "Order",
							FieldName:    "",
							SelectionSet: "id",
						},
						{
							TypeName:     "ShippingInfo",
							FieldName:    "",
							SelectionSet: "id",
						},
					},
					Requires: plan.FederationFieldConfigurations{
						{
							TypeName:     "ShippingInfo",
							FieldName:    "status",
							SelectionSet: "details { trackingNumber carrier }",
						},
						{
							TypeName:     "ShippingInfo",
							FieldName:    "fullLog",
							SelectionSet: "details { trackingNumber carrier estimatedDelivery }",
						},
					},
					Provides: plan.FederationFieldConfigurations{
						{
							TypeName:     "Order",
							FieldName:    "shippingInfo",
							SelectionSet: "details { trackingNumber carrier }",
						},
					},
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://service1",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: service1SDL,
						},
						service1SDL,
					),
				},
			),
		)

		service2SDL := `
			type ShippingInfo @key(fields: "id") {
				id: ID!
				details: DeliveryDetails
			}

			type DeliveryDetails {
				trackingNumber: String!
				carrier: String!
				estimatedDelivery: String!
			}
		`

		service2DataSourceConfig := mustDataSourceConfiguration(
			t,
			"service2",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "ShippingInfo",
						FieldNames: []string{"id", "details"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "DeliveryDetails",
						FieldNames: []string{"trackingNumber", "carrier", "estimatedDelivery"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "ShippingInfo",
							FieldName:    "",
							SelectionSet: "id",
						},
					},
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://service2",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: service2SDL,
						},
						service2SDL,
					),
				},
			),
		)

		planConfiguration := plan.Configuration{
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintQueryPlans:    false,
				PrintPlanningPaths: false,
			},
			DataSources: []plan.DataSource{
				service1DataSourceConfig,
				service2DataSourceConfig,
			},
		}

		t.Run("query nested fields with required fields which provided", func(t *testing.T) {
			t.Run("run", RunTest(
				definition,
				`
				query NestedRequires {
					order {
						shippingInfo {
							status
							fullLog
						}
					}
				}`,
				"NestedRequires",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://service1","body":{"query":"{order {shippingInfo {status fullLog}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
							}),
							/*
									// Nested fetch won't happen, because we provide sibling fields, so parent shippingInfo - is provided and could be selected.
								    // "fullLog" is a sibling of provided fields, and it is not marked as external, so basically we should be able to query it.

									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:               `{"method":"POST","url":"http://service1","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on ShippingInfo {__typename fullLog}}}","variables":{"representations":[$$0$$]}}}`,
											DataSource:          &Source{},
											PostProcessing:      SingleEntityPostProcessingConfiguration,
											RequiresEntityFetch: true,
											Variables: []resolve.Variable{
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("ShippingInfo")},
															},
															{
																Name: []byte("details"),
																Value: &resolve.Object{
																	Path:     []string{"details"},
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("trackingNumber"),
																			Value: &resolve.String{
																				Path: []string{"trackingNumber"},
																			},
																		},
																		{
																			Name: []byte("carrier"),
																			Value: &resolve.String{
																				Path: []string{"carrier"},
																			},
																		},
																		{
																			Name: []byte("estimatedDelivery"),
																			Value: &resolve.String{
																				Path: []string{"estimatedDelivery"},
																			},
																		},
																	},
																},
																OnTypeNames: [][]byte{[]byte("ShippingInfo")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("ShippingInfo")},
															},
														},
													}),
												},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
									}, "order.shippingInfo", resolve.ObjectPath("order"), resolve.ObjectPath("shippingInfo")),
							*/
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("order"),
									Value: &resolve.Object{
										Path:          []string{"order"},
										Nullable:      true,
										TypeName:      "Order",
										PossibleTypes: map[string]struct{}{"Order": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("shippingInfo"),
												Value: &resolve.Object{
													Path:          []string{"shippingInfo"},
													Nullable:      true,
													TypeName:      "ShippingInfo",
													PossibleTypes: map[string]struct{}{"ShippingInfo": {}},
													Fields: []*resolve.Field{
														{
															Name: []byte("status"),
															Value: &resolve.String{
																Path: []string{"status"},
															},
														},
														{
															Name: []byte("fullLog"),
															Value: &resolve.String{
																Path: []string{"fullLog"},
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
				planConfiguration,
				WithDefaultPostProcessor(),
			))
		})

		t.Run("query nested fields which should be provided", func(t *testing.T) {
			// technically estimated delivery is not provided
			// but as it is not marked external on the DeliveryDetails type
			// it could be selected by planner as it's external parent ShippingInfo.details selectable
			// due to provides directive on the parent Order.shippingInfo

			t.Run("run", RunTest(
				definition,
				`
				query NestedRequires {
					order {
						shippingInfo {
							details {
								trackingNumber
								carrier
								estimatedDelivery
							}
						}
					}
				}`,
				"NestedRequires",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://service1","body":{"query":"{order {shippingInfo {details {trackingNumber carrier estimatedDelivery}}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("order"),
									Value: &resolve.Object{
										Path:          []string{"order"},
										Nullable:      true,
										TypeName:      "Order",
										PossibleTypes: map[string]struct{}{"Order": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("shippingInfo"),
												Value: &resolve.Object{
													Path:          []string{"shippingInfo"},
													Nullable:      true,
													TypeName:      "ShippingInfo",
													PossibleTypes: map[string]struct{}{"ShippingInfo": {}},
													Fields: []*resolve.Field{
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path:          []string{"details"},
																Nullable:      true,
																TypeName:      "DeliveryDetails",
																PossibleTypes: map[string]struct{}{"DeliveryDetails": {}},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("trackingNumber"),
																		Value: &resolve.String{
																			Path: []string{"trackingNumber"},
																		},
																	},
																	{
																		Name: []byte("carrier"),
																		Value: &resolve.String{
																			Path: []string{"carrier"},
																		},
																	},
																	{
																		Name: []byte("estimatedDelivery"),
																		Value: &resolve.String{
																			Path: []string{"estimatedDelivery"},
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
				planConfiguration,
				WithDefaultPostProcessor(),
			))
		})

	})
}
