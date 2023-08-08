package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	batchFactory := NewBatchFactory()
	federationFactory := &Factory{BatchFactory: batchFactory}

	t.Run("composed keys, provides, requires", func(t *testing.T) {
		definition := `
			type Account {
				id: ID!
				name: String!
				info: Info
				address: Address
				shippingInfo: ShippingInfo
			}
			type Address {
				id: ID!
				line1: String!
				line2: String!
				line3(test: String!): String!
				fullAddress: String!
			}
			type Info {
				a: ID!
				b: ID!
			}
			type ShippingInfo {
				zip: String!
			}
			type User {
				id: ID!
				account: Account
				oldAccount: Account
			}
			type Query {
				user: User
				account: Account
			}
		`

		usersSubgraphSDL := `
			extend type Query {
				user: User
			}

			type User @key(fields: "id") {
				id: ID!
				account: Account
				oldAccount: Account @provides(fields: "name shippingInfo {zip}")
			}

			extend type Account @key(fields: "id info {a b}") {
				id: ID!
				info: Info
				name: String! @external
				shippingInfo: ShippingInfo @external
				address: Address
			}

			type ShippingInfo {
				zip: String! @external
			}

			type Info {
				a: ID!
				b: ID!
			}

			type Address @key(fields: "id") {
				id: ID!
				line1: String!
				line2: String!
			}
		`

		// TODO: add test for requires from 2 sibling subgraphs - should be Serial: Parallel -> Single
		// TODO: add test for requires from 1 parent and 2 sibling subgraphs
		// TODO: add test for requires when query already has the required field with the different argument - potentially requires alias?
		// TODO: add test for requires+provides

		usersDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"user"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id", "account", "oldAccount"},
				},
				{
					TypeName:   "Account",
					FieldNames: []string{"id", "info", "address"},
				},
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "line1", "line2"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "ShippingInfo",
					FieldNames: []string{"zip"},
				},
				{
					TypeName:   "Info",
					FieldNames: []string{"a", "b"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://user.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: usersSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FieldConfigurations: plan.FieldConfigurations{
				{
					TypeName:                   "User",
					RequiresFieldsSelectionSet: "id",
				},
				{
					TypeName:                   "Account",
					RequiresFieldsSelectionSet: "id info {a b}",
				},
				{
					TypeName:                   "Address",
					RequiresFieldsSelectionSet: "id",
				},
			},
		}

		accountsSubgraphSDL := `
			extend type Query {
				account: Account
			}

			type Account @key(fields: "id info {a b}") {
				id: ID!
				name: String!
				info: Info
				shippingInfo: ShippingInfo
			}

			type Info {
				a: ID!
				b: ID!
			}

			type ShippingInfo {
				zip: String!
			}

			extend type Address @key(fields: "id") {
				id: ID!
				line1: String! @external
				line2: String! @external
				line3(test: String!): String! @external
				fullAddress: String! @requires(fields: "line1 line2 line3(test:\"BOOM\")")
			}
		`
		accountsDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"account"},
				},
				{
					TypeName:   "Account",
					FieldNames: []string{"id", "name", "info", "shippingInfo"},
				},
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "fullAddress"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Info",
					FieldNames: []string{"a", "b"},
				},
				{
					TypeName:   "ShippingInfo",
					FieldNames: []string{"zip"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://account.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: accountsSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FieldConfigurations: plan.FieldConfigurations{
				{
					TypeName:                   "Account",
					FieldName:                  "",
					RequiresFieldsSelectionSet: "id info {a b}",
				},
				{
					TypeName:                   "Address",
					FieldName:                  "",
					RequiresFieldsSelectionSet: "id",
				},
				{
					TypeName:                   "Address",
					FieldName:                  "fullAddress",
					RequiresFieldsSelectionSet: "line1 line2 line3(test:\"BOOM\")",
				},
			},
		}

		addressesSubgraphSDL := `
			extend type Address @key(fields: "id") {
				id: ID!
				line3(test: String!): String!
			}
		`
		addressesDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "line3"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://address.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: addressesSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FieldConfigurations: plan.FieldConfigurations{
				{
					TypeName:                   "Address",
					FieldName:                  "",
					RequiresFieldsSelectionSet: "id",
				},
				{
					TypeName:  "Address",
					FieldName: "line3",
					Arguments: plan.ArgumentsConfigurations{
						{
							Name:       "test",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
		}

		// addressesEnricherSubgraphSDL := `
		// 	extend type Address @key(fields: "id") {
		// 		id: ID!
		// 		line3(test: String!): String!
		// 	}
		// `
		// addressesEnricherDatasourceConfiguration := plan.DataSourceConfiguration{
		// 	RootNodes: []plan.TypeField{
		// 		{
		// 			TypeName:   "Address",
		// 			FieldNames: []string{"id", "line3"},
		// 		},
		// 	},
		// 	Custom: ConfigJson(Configuration{
		// 		Fetch: FetchConfiguration{
		// 			URL: "http://address.service",
		// 		},
		// 		Federation: FederationConfiguration{
		// 			Enabled:    true,
		// 			ServiceSDL: addressesSubgraphSDL,
		// 		},
		// 	}),
		// 	Factory: federationFactory,
		// 	FieldConfigurations: plan.FieldConfigurations{
		// 		{
		// 			TypeName:                   "Address",
		// 			FieldName:                  "",
		// 			RequiresFieldsSelectionSet: "id",
		// 		},
		// 		{
		// 			TypeName:  "Address",
		// 			FieldName: "line3",
		// 			Arguments: plan.ArgumentsConfigurations{
		// 				{
		// 					Name:       "test",
		// 					SourceType: plan.FieldArgumentSource,
		// 				},
		// 			},
		// 		},
		// 	},
		// }

		dataSources := []plan.DataSourceConfiguration{
			usersDatasourceConfiguration,
			accountsDatasourceConfiguration,
			addressesDatasourceConfiguration,
		}

		// // shuffle dataSources to ensure that the order doesn't matter
		// rand.Seed(time.Now().UnixNano())
		// rand.Shuffle(len(dataSources), func(i, j int) {
		// 	dataSources[i], dataSources[j] = dataSources[j], dataSources[i]
		// })

		planConfiguration := plan.Configuration{
			DataSources:                  dataSources,
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintOperationWithRequiredFields: true,
				PrintPlanningPaths:               true,
				PrintQueryPlans:                  true,
				ConfigurationVisitor:             true,
				PlanningVisitor:                  true,
				DatasourceVisitor:                true,
			},
		}

		t.Run("composed keys", RunTest(
			definition,
			`
				query ComposedKeys {
					user {
						account {
							name
							shippingInfo {
								zip
							}
						}
					}
				}
			`,
			"ComposedKeys",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							BufferId:              0,
							Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
							DataSource:            &Source{},
							DataSourceIdentifier:  []byte("graphql_datasource.Source"),
							ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
						},
						Fields: []*resolve.Field{
							{
								HasBuffer: true,
								BufferID:  0,
								Name:      []byte("user"),
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("account"),
											Value: &resolve.Object{
												Path:     []string{"account"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														HasBuffer: true,
														BufferID:  1,
														Name:      []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														HasBuffer: true,
														BufferID:  1,
														Name:      []byte("shippingInfo"),
														Value: &resolve.Object{
															Path:     []string{"shippingInfo"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Value: &resolve.String{
																		Path: []string{"zip"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.SingleFetch{
													BufferId:   1,
													Input:      `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":$$0$$}}}`,
													DataSource: &Source{},
													Variables: []resolve.Variable{
														&resolve.ListVariable{
															Variables: []resolve.Variable{
																&resolve.ResolvableObjectVariable{
																	Path: []string{"account"},
																	Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																		Fields: []*resolve.Field{
																			{
																				Name: []byte("__typename"),
																				Value: &resolve.String{
																					Path: []string{"__typename"},
																				},
																			},
																			{
																				Name: []byte("id"),
																				Value: &resolve.String{
																					Path: []string{"id"},
																				},
																			},
																			{
																				Name: []byte("info"),
																				Value: &resolve.Object{
																					Path:     []string{"info"},
																					Nullable: true,
																					Fields: []*resolve.Field{
																						{
																							Name: []byte("a"),
																							Value: &resolve.String{
																								Path: []string{"a"},
																							},
																						},
																						{
																							Name: []byte("b"),
																							Value: &resolve.String{
																								Path: []string{"b"},
																							},
																						},
																					},
																				},
																			},
																		},
																	}),
																},
															},
														},
													},
													DataSourceIdentifier:  []byte("graphql_datasource.Source"),
													ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
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
		))

		t.Run("requires fields from 2 subgraphs: parent and sibling", RunTest(
			definition,
			`
				query Requires {
					user {
						account {
							address {
								fullAddress
							}
						}
					}
				}
			`,
			"Requires",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							BufferId:              0,
							Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`,
							DataSource:            &Source{},
							DataSourceIdentifier:  []byte("graphql_datasource.Source"),
							ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
						},
						Fields: []*resolve.Field{
							{
								HasBuffer: true,
								BufferID:  0,
								Name:      []byte("user"),
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("account"),
											Value: &resolve.Object{
												Path:     []string{"account"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("address"),
														Value: &resolve.Object{
															Path:     []string{"address"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("fullAddress"),
																	Value: &resolve.String{
																		Path: []string{"fullAddress"},
																	},
																},
															},
															Fetch: &resolve.SerialFetch{
																Fetches: []resolve.Fetch{
																	&resolve.SingleFetch{
																		BufferId:              1,
																		Input:                 `{"method":"POST","url":"http://address.service","body":{"query":"query($test: String! $representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test:$test)}}}","variables":{"test":"BOOM","representations":$$1$$}}}"}}`,
																		DataSource:            &Source{},
																		DataSourceIdentifier:  []byte("graphql_datasource.Source"),
																		ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
																		Variables: []resolve.Variable{
																			&resolve.ListVariable{
																				Variables: []resolve.Variable{
																					&resolve.ResolvableObjectVariable{
																						Path: []string{"address"},
																						Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																							Fields: []*resolve.Field{
																								{
																									Name: []byte("__typename"),
																									Value: &resolve.String{
																										Path: []string{"__typename"},
																									},
																								},
																								{
																									Name: []byte("id"),
																									Value: &resolve.String{
																										Path: []string{"id"},
																									},
																								},
																							},
																						}),
																					},
																				},
																			},
																			&resolve.ContextVariable{
																				Path:     []string{"test"},
																				Renderer: nil,
																			},
																		},
																	},
																	&resolve.SingleFetch{
																		BufferId:              2,
																		Input:                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":$$0$$}}}`,
																		DataSource:            &Source{},
																		DataSourceIdentifier:  []byte("graphql_datasource.Source"),
																		ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
																		Variables: []resolve.Variable{
																			&resolve.ListVariable{
																				Variables: []resolve.Variable{
																					&resolve.ResolvableObjectVariable{
																						Path: []string{"address"},
																						Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																							Fields: []*resolve.Field{
																								{
																									Name: []byte("__typename"),
																									Value: &resolve.String{
																										Path: []string{"__typename"},
																									},
																								},
																								{
																									Name: []byte("line1"),
																									Value: &resolve.String{
																										Path: []string{"line1"},
																									},
																								},
																								{
																									Name: []byte("line2"),
																									Value: &resolve.String{
																										Path: []string{"line2"},
																									},
																								},
																								{
																									Name: []byte("line3"),
																									Value: &resolve.String{
																										Path: []string{"line3"},
																									},
																								},
																								{
																									Name: []byte("id"),
																									Value: &resolve.String{
																										Path: []string{"id"},
																									},
																								},
																							},
																						}),
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
			planConfiguration,
		))

		t.Run("provides", RunTest(
			definition,
			`
				query Provides {
					user {
						oldAccount {
							name
							shippingInfo {
								zip
							}
						}
					}
				}
			`,
			"ComposedKeys",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.ParallelFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									BufferId:              0,
									Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"query{user{oldAccount{name ShippingInfo{zip}}}}"}}`,
									DataSource:            &Source{},
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},
							},
						},
						Fields: []*resolve.Field{
							{
								HasBuffer: true,
								BufferID:  0,
								Name:      []byte("user"),
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  1,
											Name:      []byte("oldAccount"),
											Value: &resolve.Object{
												Path:     []string{"oldAccount"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("shippingInfo"),
														Value: &resolve.Object{
															Path:     []string{"shippingInfo"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Value: &resolve.String{
																		Path: []string{"zip"},
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
		))

		// TODO: more complex example
		// `query ComposedKeys {
		// 	user {
		// 		account {
		// 			name
		// 			shippingInfo {
		// 				zip
		// 			}
		// 			address {
		// 				line1
		// 			}
		// 		}
		// 		oldAccount {
		// 			name
		// 			shippingInfo {
		// 				zip
		// 			}
		// 			address {
		// 				line1
		// 			}
		// 		}
		// 	}
		// }`
	})
}
