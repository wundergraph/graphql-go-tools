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
				country: String!
				city: String!
				zip: String!
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
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id",
					},
					{
						TypeName:     "Account",
						SelectionSet: "id info {a b}",
					},
					{
						TypeName:     "Address",
						SelectionSet: "id",
					},
				},
				Provides: plan.FederationFieldConfigurations{
					{
						TypeName:     "User",
						FieldName:    "oldAccount",
						SelectionSet: `name shippingInfo {zip}`,
					},
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
				zip: String! @external
				fullAddress: String! @requires(fields: "line1 line2 line3(test:\"BOOM\") zip")
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
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Account",
						FieldName:    "",
						SelectionSet: "id info {a b}",
					},
					{
						TypeName:     "Address",
						FieldName:    "",
						SelectionSet: "id",
					},
				},
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Address",
						FieldName:    "fullAddress",
						SelectionSet: "line1 line2 line3(test:\"BOOM\") zip",
					},
				},
			},
		}

		addressesSubgraphSDL := `
			extend type Address @key(fields: "id") {
				id: ID!
				line3(test: String!): String!
				country: String! @external
				city: String! @external
				zip: String! @requires(fields: "country city")
			}
		`
		addressesDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "line3", "zip"},
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
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Address",
						FieldName:    "",
						SelectionSet: "id",
					},
				},
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Address",
						FieldName:    "zip",
						SelectionSet: "country city",
					},
				},
			},
		}

		addressesEnricherSubgraphSDL := `
			extend type Address @key(fields: "id") {
				id: ID!
				country: String!
				city: String!
			}
		`
		addressesEnricherDatasourceConfiguration := plan.DataSourceConfiguration{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Address",
					FieldNames: []string{"id", "country", "city"},
				},
			},
			Custom: ConfigJson(Configuration{
				Fetch: FetchConfiguration{
					URL: "http://address-enricher.service",
				},
				Federation: FederationConfiguration{
					Enabled:    true,
					ServiceSDL: addressesEnricherSubgraphSDL,
				},
			}),
			Factory: federationFactory,
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Address",
						FieldName:    "",
						SelectionSet: "id",
					},
				},
			},
		}

		dataSources := []plan.DataSourceConfiguration{
			usersDatasourceConfiguration,
			accountsDatasourceConfiguration,
			addressesDatasourceConfiguration,
			addressesEnricherDatasourceConfiguration,
		}

		planConfiguration := plan.Configuration{
			DataSources:                  ShuffleDS(dataSources),
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintOperationWithRequiredFields: true,
				PrintPlanningPaths:               true,
				PrintQueryPlans:                  true,
				ConfigurationVisitor:             false,
				PlanningVisitor:                  false,
				DatasourceVisitor:                false,
			},
			Fields: plan.FieldConfigurations{
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
												Fetch: &resolve.BatchFetch{
													BatchFactory: batchFactory,
													Fetch: &resolve.SingleFetch{
														BufferId:                              1,
														Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":$$0$$}}}`,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
														Variables: []resolve.Variable{
															&resolve.ListVariable{
																Variables: []resolve.Variable{
																	&resolve.ResolvableObjectVariable{
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
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
														ProcessResponseConfig: resolve.ProcessResponseConfig{
															ExtractGraphqlResponse:    true,
															ExtractFederationEntities: true,
														},
													},
												},
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

		t.Run("requires fields", func(t *testing.T) {
			t.Skip("serial fetch is not supported yet")

			t.Run("from 3 subgraphs: parent and siblings", RunTest(
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
																		BufferID:  1,
																		Name:      []byte("fullAddress"),
																		Value: &resolve.String{
																			Path: []string{"fullAddress"},
																		},
																	},
																},
																Fetch: &resolve.SerialFetch{
																	Fetches: []resolve.Fetch{
																		&resolve.SingleFetch{
																			BufferId:               1,
																			DissallowParallelFetch: true,
																			Input:                  `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":$$0$$}}}`,
																			DataSource:             &Source{},
																			DataSourceIdentifier:   []byte("graphql_datasource.Source"),
																			ProcessResponseConfig:  resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
																			Variables: []resolve.Variable{
																				&resolve.ListVariable{
																					Variables: []resolve.Variable{
																						&resolve.ResolvableObjectVariable{
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
																										Name: []byte("zip"),
																										Value: &resolve.String{
																											Path: []string{"zip"},
																										},
																									},
																								},
																							}),
																						},
																					},
																				},
																			},
																		},
																		&resolve.SingleFetch{
																			BufferId:               2,
																			DissallowParallelFetch: true,
																			Input:                  `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":$$0$$}}}`,
																			DataSource:             &Source{},
																			DataSourceIdentifier:   []byte("graphql_datasource.Source"),
																			ProcessResponseConfig:  resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
																			Variables: []resolve.Variable{
																				&resolve.ListVariable{
																					Variables: []resolve.Variable{
																						&resolve.ResolvableObjectVariable{
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
																										Name: []byte("country"),
																										Value: &resolve.String{
																											Path: []string{"country"},
																										},
																									},
																									{
																										Name: []byte("city"),
																										Value: &resolve.String{
																											Path: []string{"city"},
																										},
																									},
																								},
																							}),
																						},
																					},
																				},
																			},
																		},
																		&resolve.SingleFetch{
																			BufferId:              3,
																			Input:                 `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":$$0$$}}}`,
																			DataSource:            &Source{},
																			DataSourceIdentifier:  []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
																			Variables: []resolve.Variable{
																				&resolve.ListVariable{
																					Variables: []resolve.Variable{
																						&resolve.ResolvableObjectVariable{
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
																			},
																		},
																	},
																},
															},
														},
													},
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
		})

		t.Run("provides", func(t *testing.T) {
			t.Run("only fields with provides", RunTest(
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
				"Provides",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId:              0,
								Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{user {oldAccount {name shippingInfo {zip}}}}"}}`,
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
												Name: []byte("oldAccount"),
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

			// TODO: more complex example both provides and not provides
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
	})

	t.Run("shareable", func(t *testing.T) {
		t.Run("on entity", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					details: Details!
				}
	
				type Details {
					forename: String!
					surname: String!
					middlename: String!
					age: Int!
				}
	
				type Query {
					me: User
				}
			`

			firstSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					details: Details! @shareable
				}
	
				type Details {
					forename: String! @shareable
					middlename: String!
				}
	
				type Query {
					me: User
				}
			`

			firstDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "details"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Details",
						FieldNames: []string{"forename", "middlename"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://first.service",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: firstSubgraphSDL,
					},
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			}

			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					details: Details! @shareable
				}
	
				type Details {
					forename: String! @shareable
					surname: String!
				}
	
				type Query {
					me: User
				}
			`
			secondDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "details"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Details",
						FieldNames: []string{"forename", "surname"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://second.service",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: secondSubgraphSDL,
					},
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			}

			thirdSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					details: Details! @shareable
				}
	
				type Details {
					age: Int!
				}
			`
			thirdDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "details"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Details",
						FieldNames: []string{"age"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://third.service",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: thirdSubgraphSDL,
					},
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			}

			dataSources := []plan.DataSourceConfiguration{
				firstDatasourceConfiguration,
				secondDatasourceConfiguration,
				thirdDatasourceConfiguration,
			}

			planConfiguration := plan.Configuration{
				DataSources:                  ShuffleDS(dataSources),
				DisableResolveFieldPositions: true,
				Debug: plan.DebugConfiguration{
					PrintQueryPlans: true,
				},
			}

			t.Run("only shared field", func(t *testing.T) {
				query := `
					query basic {
						me {
							details {
								forename
							}
						}
					}
				`

				expectedPlan := func(input string) *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId: 0,
									Input:    input,
									ProcessResponseConfig: resolve.ProcessResponseConfig{
										ExtractGraphqlResponse: true,
									},
									DataSource:           &Source{},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  0,
										Name:      []byte("me"),
										Value: &resolve.Object{
											Path:     []string{"me"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("details"),
													Value: &resolve.Object{
														Path: []string{"details"},
														Fields: []*resolve.Field{
															{
																Name: []byte("forename"),
																Value: &resolve.String{
																	Path: []string{"forename"},
																},
															},
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
				}

				t.Run("variant 1", RunTest(
					definition,
					query,
					"basic",

					expectedPlan(`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename}}}"}}`),
					plan.Configuration{
						DataSources: []plan.DataSourceConfiguration{
							firstDatasourceConfiguration,
							secondDatasourceConfiguration,
							thirdDatasourceConfiguration,
						},
						DisableResolveFieldPositions: true,
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: true,
						},
					},
				))

				t.Run("variant 2", RunTest(
					definition,
					query,
					"basic",
					expectedPlan(`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename}}}"}}`),
					plan.Configuration{
						DataSources: []plan.DataSourceConfiguration{
							secondDatasourceConfiguration,
							firstDatasourceConfiguration,
							thirdDatasourceConfiguration,
						},
						DisableResolveFieldPositions: true,
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: true,
						},
					},
				))
			})

			t.Run("shared and not shared field", func(t *testing.T) {
				t.Run("resolve from single subgraph", RunTest(
					definition,
					`
						query basic {
							me {
								details {
									forename
									surname
								}
							}
						}
					`,
					"basic",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									BufferId: 0,
									Input:    `{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname}}}"}}`,
									ProcessResponseConfig: resolve.ProcessResponseConfig{
										ExtractGraphqlResponse: true,
									},
									DataSource:           &Source{},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  0,
										Name:      []byte("me"),
										Value: &resolve.Object{
											Path:     []string{"me"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("details"),
													Value: &resolve.Object{
														Path: []string{"details"},
														Fields: []*resolve.Field{
															{
																Name: []byte("forename"),
																Value: &resolve.String{
																	Path: []string{"forename"},
																},
															},
															{
																Name: []byte("surname"),
																Value: &resolve.String{
																	Path: []string{"surname"},
																},
															},
														},
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

				t.Run("resolve from two subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId: 0,
										Input:    input1,
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse: true,
										},
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  0,
											Name:      []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														HasBuffer: true,
														BufferID:  1,
														Name:      []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("forename"),
																	Value: &resolve.String{
																		Path: []string{"forename"},
																	},
																},
																{
																	Name: []byte("surname"),
																	Value: &resolve.String{
																		Path: []string{"surname"},
																	},
																},
																{
																	Name: []byte("middlename"),
																	Value: &resolve.String{
																		Path: []string{"middlename"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.BatchFetch{
													BatchFactory: batchFactory,
													Fetch: &resolve.SingleFetch{
														BufferId:                              1,
														Input:                                 input2,
														SetTemplateOutputToNullOnVariableNull: true,
														ProcessResponseConfig: resolve.ProcessResponseConfig{
															ExtractGraphqlResponse:    true,
															ExtractFederationEntities: true,
														},
														DataSource:           &Source{},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
														Variables: []resolve.Variable{
															&resolve.ListVariable{
																Variables: []resolve.Variable{
																	&resolve.ResolvableObjectVariable{
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
														},
													},
												},
											},
										},
									},
								},
							},
						}
					}

					t.Run("variant 1", RunTest(
						definition,
						`
						query basic {
							me {
								details {
									forename
									surname
									middlename
								}
							}
						}
					`,
						"basic",
						expectedPlan(
							`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
							`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":$$0$$}}}`),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								firstDatasourceConfiguration,
								secondDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
							Debug: plan.DebugConfiguration{
								PrintQueryPlans: true,
							},
						},
					))

					t.Run("variant 2", RunTest(
						definition,
						`
						query basic {
							me {
								details {
									forename
									surname
									middlename
								}
							}
						}
					`,
						"basic",
						expectedPlan(
							`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
							`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {middlename}}}}","variables":{"representations":$$0$$}}}`),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								secondDatasourceConfiguration,
								firstDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
							Debug: plan.DebugConfiguration{
								PrintQueryPlans: true,
							},
						},
					))
				})

				t.Run("resolve from two subgraphs - not shared field", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId: 0,
										Input:    input1,
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse: true,
										},
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  0,
											Name:      []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														HasBuffer: true,
														BufferID:  1,
														Name:      []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("age"),
																	Value: &resolve.Integer{
																		Path: []string{"age"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.BatchFetch{
													BatchFactory: batchFactory,
													Fetch: &resolve.SingleFetch{
														BufferId:                              1,
														Input:                                 input2,
														SetTemplateOutputToNullOnVariableNull: true,
														ProcessResponseConfig: resolve.ProcessResponseConfig{
															ExtractGraphqlResponse:    true,
															ExtractFederationEntities: true,
														},
														DataSource:           &Source{},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
														Variables: []resolve.Variable{
															&resolve.ListVariable{
																Variables: []resolve.Variable{
																	&resolve.ResolvableObjectVariable{
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
														},
													},
												},
											},
										},
									},
								},
							},
						}
					}

					t.Run("variant 1", RunTest(
						definition,
						`
						query basic {
							me {
								details {
									age
								}
							}
						}
					`,
						"basic",
						expectedPlan(
							`{"method":"POST","url":"http://first.service","body":{"query":"{me {__typename id}}"}}`,
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":$$0$$}}}`,
						),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								firstDatasourceConfiguration,
								secondDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
							Debug: plan.DebugConfiguration{
								PrintQueryPlans: true,
							},
						},
					))

					t.Run("variant 2", RunTest(
						definition,
						`
						query basic {
							me {
								details {
									age
								}
							}
						}
					`,
						"basic",
						expectedPlan(
							`{"method":"POST","url":"http://second.service","body":{"query":"{me {__typename id}}"}}`,
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":$$0$$}}}`,
						),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								secondDatasourceConfiguration,
								firstDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
							Debug: plan.DebugConfiguration{
								PrintQueryPlans: true,
							},
						},
					))
				})

				t.Run("resolve from three subgraphs", func(t *testing.T) {
					planConfiguration := plan.Configuration{
						DataSources: []plan.DataSourceConfiguration{
							firstDatasourceConfiguration,
							secondDatasourceConfiguration,
							thirdDatasourceConfiguration,
						},
						DisableResolveFieldPositions: true,
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: true,
							// DatasourceVisitor: true,
							// PlanningVisitor: true,
						},
					}

					expectedPlan := func(input1 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										BufferId: 0,
										Input:    input1,
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse: true,
										},
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Fields: []*resolve.Field{
										{
											HasBuffer: true,
											BufferID:  0,
											Name:      []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														HasBuffer: true,
														BufferID:  2,
														Name:      []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("forename"),
																	Value: &resolve.String{
																		Path: []string{"forename"},
																	},
																},
																{
																	Name: []byte("surname"),
																	Value: &resolve.String{
																		Path: []string{"surname"},
																	},
																},
																{
																	Name: []byte("middlename"),
																	Value: &resolve.String{
																		Path: []string{"middlename"},
																	},
																},
																{
																	Name: []byte("age"),
																	Value: &resolve.Integer{
																		Path: []string{"age"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.ParallelFetch{
													Fetches: []resolve.Fetch{
														&resolve.BatchFetch{
															BatchFactory: batchFactory,
															Fetch: &resolve.SingleFetch{
																BufferId:                              1,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":$$0$$}}}`,
																SetTemplateOutputToNullOnVariableNull: true,
																ProcessResponseConfig: resolve.ProcessResponseConfig{
																	ExtractGraphqlResponse:    true,
																	ExtractFederationEntities: true,
																},
																DataSource:           &Source{},
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
																Variables: []resolve.Variable{
																	&resolve.ListVariable{
																		Variables: []resolve.Variable{
																			&resolve.ResolvableObjectVariable{
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
																},
															},
														},
														&resolve.BatchFetch{
															BatchFactory: batchFactory,
															Fetch: &resolve.SingleFetch{
																BufferId:                              2,
																Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":$$0$$}}}`,
																SetTemplateOutputToNullOnVariableNull: true,
																ProcessResponseConfig: resolve.ProcessResponseConfig{
																	ExtractGraphqlResponse:    true,
																	ExtractFederationEntities: true,
																},
																DataSource:           &Source{},
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
																Variables: []resolve.Variable{
																	&resolve.ListVariable{
																		Variables: []resolve.Variable{
																			&resolve.ResolvableObjectVariable{
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
																},
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
					}

					t.Run("run", RunTest(
						definition,
						`
						query basic {
							me {
								details {
									forename
									surname
									middlename
									age
								}
							}
						}
					`,
						"basic",
						expectedPlan(`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`),
						planConfiguration,
					))
				})
			})
		})
	})
}
