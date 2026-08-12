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

func TestGraphQLDataSourceFederation_ProvidesFieldSetOverUnionTypedField(t *testing.T) {
	definition := `
		type Query {
			media: [Media]
		}

		union Media = Book | Movie

		type Book {
			id: ID!
			title: String!
		}

		type Movie {
			id: ID!
			title: String!
		}
	`

	service1SDL := `
		type Query {
			media: [Media] @shareable @provides(fields: "... on Book { title }")
		}

		union Media = Book | Movie

		type Book @key(fields: "id") {
			id: ID!
			title: String! @external
		}

		type Movie @key(fields: "id") {
			id: ID!
		}
	`

	service1DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service1",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id"}, ExternalFieldNames: []string{"title"}},
				{TypeName: "Movie", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
				},
				Provides: plan.FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "media", SelectionSet: "... on Book { title }"},
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
		type Query {
			_empty: String
		}

		type Book @key(fields: "id") {
			id: ID!
			title: String!
		}

		type Movie @key(fields: "id") {
			id: ID!
			title: String!
		}
	`

	service2DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service2",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"_empty"}},
				{TypeName: "Book", FieldNames: []string{"id", "title"}},
				{TypeName: "Movie", FieldNames: []string{"id", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
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
		DataSources: []plan.DataSource{
			service1DataSourceConfig,
			service2DataSourceConfig,
		},
	}

	t.Run("should not plan to retrieve title from service2 when it should be inlined with @provides", RunTest(
		definition,
		`
			query ProvidesUnion {
				media {
					... on Book {
						id
						title
					}
					... on Movie {
						id
					}
				}
			}
		`,
		"ProvidesUnion",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:      `{"method":"POST","url":"http://service1","body":{"query":"{media {__typename ... on Book {id title} ... on Movie {id}}}"}}`,
						DataSource: &Source{},
						PostProcessing: resolve.PostProcessingConfiguration{
							SelectResponseDataPath:   []string{"data"},
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					FetchDependencies: resolve.FetchDependencies{
						FetchID: 0,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				})),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("media"),
							Value: &resolve.Array{
								Path:     []string{"media"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Book":  {},
										"Movie": {},
									},
									TypeName: "Media",
									Fields: []*resolve.Field{
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.String{
												Path: []string{"title"},
											},
										},
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
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

	t.Run("should plan to retrieve title from service2 when requesting title from the movie type", RunTest(
		definition,
		`
			query ProvidesUnion {
				media {
					... on Book {
						id
					}
					... on Movie {
						id
						title
					}
				}
			}
		`,
		"ProvidesUnion",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://service1","body":{"query":"{media {__typename ... on Book {id} ... on Movie {id __typename}}}"}}`,
							DataSource: &Source{},
							PostProcessing: resolve.PostProcessingConfiguration{
								SelectResponseDataPath:   []string{"data"},
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						FetchDependencies: resolve.FetchDependencies{
							FetchID: 0,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					resolve.SingleWithPath(&resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:                                 `{"method":"POST","url":"http://service2","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Movie {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
							DataSource:                            &Source{},
							SetTemplateOutputToNullOnVariableNull: true,
							RequiresEntityBatchFetch:              true,
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
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
										},
									}),
								},
							},
							PostProcessing: EntitiesPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "media", resolve.ArrayPath("media")),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("media"),
							Value: &resolve.Array{
								Path:     []string{"media"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Book":  {},
										"Movie": {},
									},
									TypeName: "Media",
									Fields: []*resolve.Field{
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.String{
												Path: []string{"title"},
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
}

func TestGraphQLDataSourceFederation_ProvidesFieldSetOverInterfaceTypeField(t *testing.T) {
	definition := `
		type Query {
			media: [Media]
		}

		interface Media {
			id: ID!
			title: String!
        }
		

		type Book implements Media {
			id: ID!
			title: String!
		}

		type Movie implements Media {
			id: ID!
			title: String!
		}
	`

	service1SDL := `
		type Query {
			media: [Media] @shareable @provides(fields: "... on Book { title }")
		}

		interface Media {
			id: ID!
			title: String!
		}

		type Book implements Media @key(fields: "id") {
			id: ID!
			title: String! @external
		}

		type Movie implements Media @key(fields: "id") {
			id: ID!
			title: String! @external
		}
	`

	service1DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service1",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id"}, ExternalFieldNames: []string{"title"}},
				{TypeName: "Movie", FieldNames: []string{"id"}, ExternalFieldNames: []string{"title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
				},
				Provides: plan.FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "media", SelectionSet: "... on Book { title }"},
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
		type Query {
			_empty: String
		}

		type Book @key(fields: "id") {
			id: ID!
			title: String!
		}

		type Movie @key(fields: "id") {
			id: ID!
			title: String!
		}
	`

	service2DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service2",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"_empty"}},
				{TypeName: "Media", FieldNames: []string{"id", "title"}},
				{TypeName: "Book", FieldNames: []string{"id", "title"}},
				{TypeName: "Movie", FieldNames: []string{"id", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
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
		DataSources: []plan.DataSource{
			service1DataSourceConfig,
			service2DataSourceConfig,
		},
	}

	t.Run("should not plan to retrieve title from service2 when it should be inlined with @provides", RunTest(
		definition,
		`
			query ProvidesInterface {
				media {
					... on Book {
						id
						title
					}
					... on Movie {
						id
					}
				}
			}
		`,
		"ProvidesInterface",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:      `{"method":"POST","url":"http://service1","body":{"query":"{media {__typename ... on Book {id title} ... on Movie {id}}}"}}`,
						DataSource: &Source{},
						PostProcessing: resolve.PostProcessingConfiguration{
							SelectResponseDataPath:   []string{"data"},
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					FetchDependencies: resolve.FetchDependencies{
						FetchID: 0,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				})),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("media"),
							Value: &resolve.Array{
								Path:     []string{"media"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Book":  {},
										"Movie": {},
									},
									TypeName: "Media",
									Fields: []*resolve.Field{
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.String{
												Path: []string{"title"},
											},
										},
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
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

	t.Run("should plan to retrieve title from service2 when requesting title from the movie type", RunTest(
		definition,
		`
			query ProvidesInterface {
				media {
					title
					... on Book {
						id
					}
					... on Movie {
						id
					}
				}
			}
		`,
		"ProvidesInterface",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://service1","body":{"query":"{media {__typename ... on Book {title id} ... on Movie {id __typename}}}"}}`,
							DataSource: &Source{},
							PostProcessing: resolve.PostProcessingConfiguration{
								SelectResponseDataPath:   []string{"data"},
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						FetchDependencies: resolve.FetchDependencies{
							FetchID: 0,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					resolve.SingleWithPath(&resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:                                 `{"method":"POST","url":"http://service2","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Movie {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
							DataSource:                            &Source{},
							SetTemplateOutputToNullOnVariableNull: true,
							RequiresEntityBatchFetch:              true,
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
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
										},
									}),
								},
							},
							PostProcessing: EntitiesPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "media", resolve.ArrayPath("media")),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("media"),
							Value: &resolve.Array{
								Path:     []string{"media"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Book":  {},
										"Movie": {},
									},
									TypeName: "Media",
									Fields: []*resolve.Field{
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.String{
												Path: []string{"title"},
											},
										},
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.String{
												Path: []string{"title"},
											},
										},
										{
											Name:        []byte("id"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.Scalar{
												Path: []string{"id"},
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
}

func TestGraphQLDataSourceFederation_ProvidesFieldSetOverInterfaceTypeFieldAndAbstractSelectionRewriting(t *testing.T) {
	definition := `
		type Query {
			media: [Media]
		}

		interface Media {
			id: ID!
		}

		interface Titled {
			title: String!
		}

		type Book implements Media {
			id: ID!
			title: String!
		}

		type Movie implements Media & Titled {
			id: ID!
			title: String!
		}
	`

	service1SDL := `
		type Query {
			media: [Media] @shareable @provides(fields: "... on Book { title }")
		}

		interface Media {
			id: ID!
		}

		interface Titled {
			title: String!
		}

		type Book implements Media @key(fields: "id") {
			id: ID!
			title: String! @external
		}

		type Movie implements Media & Titled @key(fields: "id") {
			id: ID!
			title: String! @external
		}
	`

	service1DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service1",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id"}, ExternalFieldNames: []string{"title"}},
				{TypeName: "Movie", FieldNames: []string{"id"}, ExternalFieldNames: []string{"title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
				},
				Provides: plan.FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "media", SelectionSet: "... on Book { title }"},
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
		type Query {
			_empty: String
		}

		type Book @key(fields: "id") {
			id: ID!
			title: String!
		}

		type Movie @key(fields: "id") {
			id: ID!
			title: String!
		}
	`

	service2DataSourceConfig := mustDataSourceConfiguration(
		t,
		"service2",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"_empty"}},
				{TypeName: "Media", FieldNames: []string{"id", "title"}},
				{TypeName: "Book", FieldNames: []string{"id", "title"}},
				{TypeName: "Movie", FieldNames: []string{"id", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
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
		DataSources: []plan.DataSource{
			service1DataSourceConfig,
			service2DataSourceConfig,
		},
	}

	t.Run("should correctly plan to fetches for proveded and external fields using nested interface selections", RunTest(
		definition,
		`
			query ProvidesInterface {
				media {
					... on Media {
						... on Book {
							title
						}
						... on Titled {
							... on Movie {
								title
							}
						}
					}
				}
			}
		`,
		"ProvidesInterface",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://service1","body":{"query":"{media {__typename ... on Book {title} ... on Movie {__typename id}}}"}}`,
							DataSource: &Source{},
							PostProcessing: resolve.PostProcessingConfiguration{
								SelectResponseDataPath:   []string{"data"},
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
						FetchDependencies: resolve.FetchDependencies{
							FetchID: 0,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					resolve.SingleWithPath(&resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           1,
							DependsOnFetchIDs: []int{0},
						},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:                                 `{"method":"POST","url":"http://service2","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Movie {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
							DataSource:                            &Source{},
							SetTemplateOutputToNullOnVariableNull: true,
							RequiresEntityBatchFetch:              true,
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
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Movie")},
											},
										},
									}),
								},
							},
							PostProcessing: EntitiesPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}, "media", resolve.ArrayPath("media")),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("media"),
							Value: &resolve.Array{
								Path:     []string{"media"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Book":  {},
										"Movie": {},
									},
									TypeName: "Media",
									Fields: []*resolve.Field{
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Book")},
											Value: &resolve.String{
												Path: []string{"title"},
											},
										},
										{
											Name:        []byte("title"),
											OnTypeNames: [][]byte{[]byte("Movie")},
											Value: &resolve.String{
												Path: []string{"title"},
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
}

func TestGraphQLDataSourceFederation_RequiresSameFieldWithDifferentArguments(t *testing.T) {
	definition := `
		type Query {
			products: [Product]
		}

		type Product {
			upc: String!
			weight: Int
			price(currency: String!): Int
			estimateA: Int
			estimateB: Int
			estimateC: Int
			estimateD: Int
		}
	`

	catalogSDL := `
		type Query {
			products: [Product]
		}

		type Product @key(fields: "upc") {
			upc: String!
			weight: Int
			price(currency: String!): Int
		}
	`

	inventorySDL := `
		type Product @key(fields: "upc") {
			upc: String!
			weight: Int @external
			price(currency: String!): Int @external
			estimateA: Int @requires(fields: "price(currency: \"USD\") weight")
			estimateB: Int @requires(fields: "price(currency: \"EUR\") weight")
			estimateC: Int @requires(fields: "price(currency: \"CAD\") weight")
			estimateD: Int @requires(fields: "price(currency: \"UAH\") weight")
		}
	`

	catalog := mustDataSourceConfiguration(
		t,
		"catalog",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"products"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "weight", "price"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://catalog.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: catalogSDL,
				},
				catalogSDL,
			),
		}),
	)

	inventory := mustDataSourceConfiguration(
		t,
		"inventory",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Product",
					FieldNames: []string{"upc", "estimateA", "estimateB", "estimateC", "estimateD"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "upc",
					},
				},
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						FieldName:    "estimateA",
						SelectionSet: `price(currency: "USD") weight`,
					},
					{
						TypeName:     "Product",
						FieldName:    "estimateB",
						SelectionSet: `price(currency: "EUR") weight`,
					},
					{
						TypeName:     "Product",
						FieldName:    "estimateC",
						SelectionSet: `price(currency: "CAD") weight`,
					},
					{
						TypeName:     "Product",
						FieldName:    "estimateD",
						SelectionSet: `price(currency: "UAH") weight`,
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://inventory.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: inventorySDL,
				},
				inventorySDL,
			),
		}),
	)

	estimateFetch := func(fetchID int, fieldName string, pricePath string) *resolve.FetchTreeNode {
		return resolve.SingleWithPath(&resolve.SingleFetch{
			FetchDependencies: resolve.FetchDependencies{
				FetchID:           fetchID,
				DependsOnFetchIDs: []int{0},
			},
			FetchConfiguration: resolve.FetchConfiguration{
				RequiresEntityBatchFetch:              true,
				Input:                                 `{"method":"POST","url":"http://inventory.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename ` + fieldName + `}}}","variables":{"representations":[$$0$$]}}}`,
				DataSource:                            &Source{},
				PostProcessing:                        EntitiesPostProcessingConfiguration,
				SetTemplateOutputToNullOnVariableNull: true,
				Variables: resolve.NewVariables(
					resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("__typename"),
								Value: &resolve.String{
									Path: []string{"__typename"},
								},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name: []byte("price"),
								Value: &resolve.Integer{
									Path:     []string{pricePath},
									Nullable: true,
								},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name: []byte("weight"),
								Value: &resolve.Integer{
									Path:     []string{"weight"},
									Nullable: true,
								},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name: []byte("upc"),
								Value: &resolve.String{
									Path: []string{"upc"},
								},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
						},
					}),
				),
			},
			DataSourceIdentifier: []byte("graphql_datasource.Source"),
		}, "products", resolve.ArrayPath("products"))
	}

	RunWithPermutations(
		t,
		definition,
		`
		query Products {
			products {
				upc
				estimateA
				estimateB
				estimateC
				estimateD
			}
		}`,
		"Products",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://catalog.service","body":{"query":"query($a: String!, $b: String!, $c: String!, $d: String!){products {upc price(currency: $a) weight __internal_price: price(currency: $b) __internal_price_1: price(currency: $c) __internal_price_2: price(currency: $d) __typename}}","variables":{"a":"USD","b":"EUR","c":"CAD","d":"UAH"}}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					estimateFetch(1, "estimateA", "price"),
					estimateFetch(2, "estimateB", "__internal_price"),
					estimateFetch(3, "estimateC", "__internal_price_1"),
					estimateFetch(4, "estimateD", "__internal_price_2"),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("products"),
							Value: &resolve.Array{
								Path:     []string{"products"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("upc"),
											Value: &resolve.String{
												Path: []string{"upc"},
											},
										},
										{
											Name: []byte("estimateA"),
											Value: &resolve.Integer{
												Path:     []string{"estimateA"},
												Nullable: true,
											},
										},
										{
											Name: []byte("estimateB"),
											Value: &resolve.Integer{
												Path:     []string{"estimateB"},
												Nullable: true,
											},
										},
										{
											Name: []byte("estimateC"),
											Value: &resolve.Integer{
												Path:     []string{"estimateC"},
												Nullable: true,
											},
										},
										{
											Name: []byte("estimateD"),
											Value: &resolve.Integer{
												Path:     []string{"estimateD"},
												Nullable: true,
											},
										},
									},
									TypeName: "Product",
									PossibleTypes: map[string]struct{}{
										"Product": {},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				catalog,
				inventory,
			},
			Fields: plan.FieldConfigurations{
				{
					TypeName:  "Product",
					FieldName: "price",
					Arguments: plan.ArgumentsConfigurations{
						{
							Name:       "currency",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
		WithDefaultPostProcessor(),
	)
}
