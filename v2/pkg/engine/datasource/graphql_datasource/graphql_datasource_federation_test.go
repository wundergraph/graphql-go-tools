package graphql_datasource

import (
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	. "github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	federationFactory := &Factory{}

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
			ID: "user.service",
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
			ID: "account.service",
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
			ID: "address.service",
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
			ID: "address-enricher.service",
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

		withInfo := func(config plan.Configuration) plan.Configuration {
			config.IncludeInfo = true
			return config
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
							SerialID:             0,
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
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
												Fetch: &resolve.SingleFetch{
													SerialID: 1,
													FetchConfiguration: resolve.FetchConfiguration{
														Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
																			OnTypeNames: [][]byte{[]byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Account")},
																		},
																		{
																			Name:        []byte("info"),
																			OnTypeNames: [][]byte{[]byte("Account")},
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
														PostProcessing:      SingleEntityPostProcessingConfiguration,
														RequiresEntityFetch: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
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

		t.Run("composed keys with info", RunTest(
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
					Info: &resolve.GraphQLResponseInfo{
						OperationType: ast.OperationTypeQuery,
					},
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							SerialID:             0,
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Info: &resolve.FieldInfo{
									Name:            "user",
									ParentTypeNames: []string{"Query"},
									NamedType:       "User",
									Source: resolve.TypeFieldSource{
										IDs: []string{"user.service"},
									},
								},
								Value: &resolve.Object{
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("account"),
											Info: &resolve.FieldInfo{
												Name:            "account",
												NamedType:       "Account",
												ParentTypeNames: []string{"User"},
												Source: resolve.TypeFieldSource{
													IDs: []string{"user.service"},
												},
											},
											Value: &resolve.Object{
												Path:     []string{"account"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Info: &resolve.FieldInfo{
															Name:            "name",
															NamedType:       "String",
															ParentTypeNames: []string{"Account"},
															Source: resolve.TypeFieldSource{
																IDs: []string{"account.service"},
															},
														},
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("shippingInfo"),
														Info: &resolve.FieldInfo{
															Name:            "shippingInfo",
															NamedType:       "ShippingInfo",
															ParentTypeNames: []string{"Account"},
															Source: resolve.TypeFieldSource{
																IDs: []string{"account.service"},
															},
														},
														Value: &resolve.Object{
															Path:     []string{"shippingInfo"},
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Info: &resolve.FieldInfo{
																		Name:            "zip",
																		NamedType:       "String",
																		ParentTypeNames: []string{"ShippingInfo"},
																		Source: resolve.TypeFieldSource{
																			IDs: []string{"account.service"},
																		},
																	},
																	Value: &resolve.String{
																		Path: []string{"zip"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.SingleFetch{
													SerialID:             1,
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
													FetchConfiguration: resolve.FetchConfiguration{
														Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
														RequiresEntityFetch:                   true,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name:        []byte("__typename"),
																			OnTypeNames: [][]byte{[]byte("Account")},
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																		},
																		{
																			Name:        []byte("id"),
																			OnTypeNames: [][]byte{[]byte("Account")},
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																		},
																		{
																			Name:        []byte("info"),
																			OnTypeNames: [][]byte{[]byte("Account")},
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
														PostProcessing: SingleEntityPostProcessingConfiguration,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			withInfo(planConfiguration),
		))

		t.Run("requires fields", func(t *testing.T) {
			t.Run("from 3 subgraphs: parent and siblings", func(t *testing.T) {
				operation := `
				query Requires {
					user {
						account {
							address {
								fullAddress
							}
						}
					}
				}
			`
				operationName := "Requires"

				expectedPlan := func(input string) *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									SerialID:             0,
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          input,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
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
																			Name: []byte("fullAddress"),
																			Value: &resolve.String{
																				Path: []string{"fullAddress"},
																			},
																		},
																	},
																	Fetch: &resolve.SerialFetch{
																		Fetches: []resolve.Fetch{
																			&resolve.SingleFetch{
																				SerialID:             1,
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
																					RequiresSerialFetch: true,
																					Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":[$$0$$]}}}`,
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
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("id"),
																										Value: &resolve.String{
																											Path: []string{"id"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("line1"),
																										Value: &resolve.String{
																											Path: []string{"line1"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("line2"),
																										Value: &resolve.String{
																											Path: []string{"line2"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("line3"),
																										Value: &resolve.String{
																											Path: []string{"line3"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("zip"),
																										Value: &resolve.String{
																											Path: []string{"zip"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																								},
																							}),
																						},
																					},
																					SetTemplateOutputToNullOnVariableNull: true,
																				},
																			},
																			&resolve.SingleFetch{
																				SerialID:             2,
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
																					RequiresSerialFetch: true,
																					Input:               `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":[$$0$$]}}}`,
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
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("id"),
																										Value: &resolve.String{
																											Path: []string{"id"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("country"),
																										Value: &resolve.String{
																											Path: []string{"country"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("city"),
																										Value: &resolve.String{
																											Path: []string{"city"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																								},
																							}),
																						},
																					},
																					SetTemplateOutputToNullOnVariableNull: true,
																				},
																			},
																			&resolve.SingleFetch{
																				SerialID:             3,
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
																					Input:               `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":[$$0$$]}}}`,
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
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																									{
																										Name: []byte("id"),
																										Value: &resolve.String{
																											Path: []string{"id"},
																										},
																										OnTypeNames: [][]byte{[]byte("Address")},
																									},
																								},
																							}),
																						},
																					},
																					SetTemplateOutputToNullOnVariableNull: true,
																				},
																			},
																		},
																	},
																},
															},
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
					operation,
					operationName,
					expectedPlan(`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`),
					plan.Configuration{
						DataSources: []plan.DataSourceConfiguration{
							usersDatasourceConfiguration,
							accountsDatasourceConfiguration,
							addressesDatasourceConfiguration,
							addressesEnricherDatasourceConfiguration,
						},
						DisableResolveFieldPositions: true,
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
					},
				))
			})
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
								SerialID:             0,
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {oldAccount {name shippingInfo {zip}}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
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

			t.Run("both provided and not provided", RunTest(
				definition,
				`
				query Provides {
					user {
						account {
							name
							shippingInfo {
								zip
							}
						}
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
								SerialID:             0,
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}} oldAccount {name shippingInfo {zip}}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
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
													Fetch: &resolve.SingleFetch{
														SerialID:             1,
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
														FetchConfiguration: resolve.FetchConfiguration{
															Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource:                            &Source{},
															PostProcessing:                        SingleEntityPostProcessingConfiguration,
															RequiresEntityFetch:                   true,
															SetTemplateOutputToNullOnVariableNull: true,
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
																				OnTypeNames: [][]byte{[]byte("Account")},
																			},
																			{
																				Name: []byte("id"),
																				Value: &resolve.String{
																					Path: []string{"id"},
																				},
																				OnTypeNames: [][]byte{[]byte("Account")},
																			},
																			{
																				Name:        []byte("info"),
																				OnTypeNames: [][]byte{[]byte("Account")},
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
												},
											},
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
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          input,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("me"),
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
					},
				))
			})

			t.Run("shared and not shared field", func(t *testing.T) {
				t.Run("resolve from single subgraph", func(t *testing.T) {
					dataSources := []plan.DataSourceConfiguration{
						firstDatasourceConfiguration,
						secondDatasourceConfiguration,
						thirdDatasourceConfiguration,
					}

					planConfiguration := plan.Configuration{
						DataSources:                  ShuffleDS(dataSources),
						DisableResolveFieldPositions: true,
					}

					t.Run("run", RunTest(
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
										SerialID:             0,
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname}}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									},
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
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
				})

				t.Run("resolve from two subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										SerialID:             0,
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									},
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
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
												Fetch: &resolve.SingleFetch{
													SerialID:             1,
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
													FetchConfiguration: resolve.FetchConfiguration{
														Input:                                 input2,
														SetTemplateOutputToNullOnVariableNull: true,
														PostProcessing:                        SingleEntityPostProcessingConfiguration,
														RequiresEntityFetch:                   true,
														DataSource:                            &Source{},
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
																			OnTypeNames: [][]byte{[]byte("User")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User")},
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
							`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[$$0$$]}}}`),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								firstDatasourceConfiguration,
								secondDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
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
							`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {middlename}}}}","variables":{"representations":[$$0$$]}}}`),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								secondDatasourceConfiguration,
								firstDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
						},
					))
				})

				t.Run("resolve from two subgraphs - not shared field", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										SerialID:             0,
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									},
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
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
																	Name: []byte("age"),
																	Value: &resolve.Integer{
																		Path: []string{"age"},
																	},
																},
															},
														},
													},
												},
												Fetch: &resolve.SingleFetch{
													SerialID:             1,
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
													FetchConfiguration: resolve.FetchConfiguration{
														Input:                                 input2,
														SetTemplateOutputToNullOnVariableNull: true,
														PostProcessing:                        SingleEntityPostProcessingConfiguration,
														RequiresEntityFetch:                   true,
														DataSource:                            &Source{},
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
																			OnTypeNames: [][]byte{[]byte("User")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User")},
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
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								firstDatasourceConfiguration,
								secondDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
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
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						),
						plan.Configuration{
							DataSources: []plan.DataSourceConfiguration{
								secondDatasourceConfiguration,
								firstDatasourceConfiguration,
								thirdDatasourceConfiguration,
							},
							DisableResolveFieldPositions: true,
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
					}

					expectedPlan := func(input1 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										SerialID:             0,
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									},
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
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
														&resolve.SingleFetch{
															SerialID:             1,
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
															FetchConfiguration: resolve.FetchConfiguration{
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[$$0$$]}}}`,
																SetTemplateOutputToNullOnVariableNull: true,
																PostProcessing:                        SingleEntityPostProcessingConfiguration,
																RequiresEntityFetch:                   true,
																DataSource:                            &Source{},
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
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																			},
																		}),
																	},
																},
															},
														},
														&resolve.SingleFetch{
															SerialID:             2,
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
															FetchConfiguration: resolve.FetchConfiguration{
																Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
																SetTemplateOutputToNullOnVariableNull: true,
																PostProcessing:                        SingleEntityPostProcessingConfiguration,
																RequiresEntityFetch:                   true,
																DataSource:                            &Source{},
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
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("User")},
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

	t.Run("plan with few entities from the same datasource", func(t *testing.T) {

		t.Run("on interface object", func(t *testing.T) {
			definition := `
				type User implements Node {
					id: ID!
					title: String!
					name: String!
				}

				type Admin implements Node {
					id: ID!
					title: String!
					adminName: String!
				}

				interface Node {
					id: ID!
					title: String!
				}

				type Query {
					account: Node!
				}
			`

			firstSubgraphSDL := `	
				type User implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
				}

				type Admin implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
				}

				interface Node {
					id: ID!
					title: String!
				}

				type Query {
					account: Node
				}
			`

			firstDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"account"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id"},
					},
					{
						TypeName:   "Admin",
						FieldNames: []string{"id"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Node",
						FieldNames: []string{"id", "title"},
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
					UpstreamSchema: firstSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
						{
							TypeName:     "Admin",
							SelectionSet: "id",
						},
					},
				},
			}

			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					name: String!
					title: String!
				}

				type Admin @key(fields: "id") {
					id: ID!
					adminName: String!
					title: String!
				}
			`
			secondDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "name", "title"},
					},
					{
						TypeName:   "Admin",
						FieldNames: []string{"id", "adminName", "title"},
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
					UpstreamSchema: secondSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
						{
							TypeName:     "Admin",
							SelectionSet: "id",
						},
					},
				},
			}

			dataSources := []plan.DataSourceConfiguration{
				firstDatasourceConfiguration,
				secondDatasourceConfiguration,
			}

			planConfiguration := plan.Configuration{
				DataSources:                  ShuffleDS(dataSources),
				DisableResolveFieldPositions: true,
			}

			t.Run("query with inline fragments on interface - no expanding", RunTest(
				definition,
				`
					query Accounts {
						account {
							... on User {
								name
							}
							... on Admin {
								adminName
							}
						}
					}
				`,
				"Accounts",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {__typename ... on User {__typename id} ... on Admin {__typename id}}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("account"),
									Value: &resolve.Object{
										Path:     []string{"account"},
										Nullable: false,
										Fields: []*resolve.Field{
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path: []string{"name"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("adminName"),
												Value: &resolve.String{
													Path: []string{"adminName"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
										},
										Fetch: &resolve.SingleFetch{
											SerialID: 1,
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name} ... on Admin {adminName}}}","variables":{"representations":[$$0$$]}}}`,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
																	OnTypeNames: [][]byte{[]byte("User")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.String{
																		Path: []string{"__typename"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin")},
																},
															},
														}),
													},
												},
												PostProcessing: SingleEntityPostProcessingConfiguration,
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
									},
								},
							},
						},
					},
				},
				planConfiguration,
			))

			t.Run("query with selection on interface - should expand to inline fragments", RunTest(
				definition,
				`
					query Accounts {
						account {
							title
						}
					}
				`,
				"Accounts",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {__typename ... on Admin {__typename id} ... on User {__typename id}}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("account"),
									Value: &resolve.Object{
										Path:     []string{"account"},
										Nullable: false,
										Fields: []*resolve.Field{
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
										},
										Fetch: &resolve.SingleFetch{
											SerialID: 1,
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title} ... on User {title}}}","variables":{"representations":[$$0$$]}}}`,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
																	OnTypeNames: [][]byte{[]byte("Admin")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.String{
																		Path: []string{"__typename"},
																	},
																	OnTypeNames: [][]byte{[]byte("User")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User")},
																},
															},
														}),
													},
												},
												PostProcessing: SingleEntityPostProcessingConfiguration,
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
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

		t.Run("on array", func(t *testing.T) {
			definition := `
				type User implements Node {
					id: ID!
					name: String!
					title: String!
				}

				type Admin implements Node {
					adminID: ID!
					adminName: String!
					title: String!
				}

				type Moderator implements Node {
					moderatorID: ID!
					subject: String!
					title: String!
				}
	
				union Account = User | Admin | Moderator
				
				interface Node {
					title: String!
				}

				type Query {
					accounts: [Account!]
					nodes: [Node!]
				}
			`

			firstSubgraphSDL := `	
				type User implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
				}

				type Admin implements Node @key(fields: "adminID") {
					adminID: ID!
					title: String!
				}

				type Moderator implements Node @key(fields: "moderatorID") {
					moderatorID: ID!
					title: String! @external
				}
	
				union Account = User | Admin | Moderator

				interface Node {
					title: String!
				}

				type Query {
					accounts: [Account!]
					nodes: [Node!]
				}
			`

			firstDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"accounts", "nodes"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id"},
					},
					{
						TypeName:   "Admin",
						FieldNames: []string{"adminID", "title"},
					},
					{
						TypeName:   "Moderator",
						FieldNames: []string{"moderatorID"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Node",
						FieldNames: []string{"title"},
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
					UpstreamSchema: firstSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
						{
							TypeName:     "Admin",
							SelectionSet: "adminID",
						},
						{
							TypeName:     "Moderator",
							SelectionSet: "moderatorID",
						},
					},
				},
			}

			secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					name: String!
					title: String!
				}

				type Admin @key(fields: "adminID") {
					adminID: ID!
					adminName: String!
				}
			`
			secondDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "name", "title"},
					},
					{
						TypeName:   "Admin",
						FieldNames: []string{"adminID", "adminName"},
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
					UpstreamSchema: secondSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
						{
							TypeName:     "Admin",
							SelectionSet: "adminID",
						},
					},
				},
			}

			thirdSubgraphSDL := `
				type Moderator @key(fields: "moderatorID") {
					moderatorID: ID!
					subject: String!
					title: String!
				}
			`
			thirdDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Moderator",
						FieldNames: []string{"moderatorID", "subject", "title"},
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
					UpstreamSchema: thirdSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "Moderator",
							SelectionSet: "moderatorID",
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
			}

			t.Run("union query on array", RunTest(
				definition,
				`
					query Accounts {
						accounts {
							... on User {
								name
							}
							... on Admin {
								adminName
							}
							... on Moderator {
								subject
							}
						}
					}
				`,
				"Accounts",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{accounts {__typename ... on User {__typename id} ... on Admin {__typename adminID} ... on Moderator {__typename moderatorID}}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("accounts"),
									Value: &resolve.Array{
										Path:     []string{"accounts"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
													OnTypeNames: [][]byte{[]byte("User")},
												},
												{
													Name: []byte("adminName"),
													Value: &resolve.String{
														Path: []string{"adminName"},
													},
													OnTypeNames: [][]byte{[]byte("Admin")},
												},
												{
													Name: []byte("subject"),
													Value: &resolve.String{
														Path: []string{"subject"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator")},
												},
											},
											Fetch: &resolve.ParallelFetch{
												Fetches: []resolve.Fetch{
													&resolve.SingleFetch{
														SerialID: 1,
														FetchConfiguration: resolve.FetchConfiguration{
															Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name} ... on Admin {adminName}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource:                            &Source{},
															SetTemplateOutputToNullOnVariableNull: true,
															RequiresEntityBatchFetch:              true,
															PostProcessing:                        EntitiesPostProcessingConfiguration,
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
																				OnTypeNames: [][]byte{[]byte("User")},
																			},
																			{
																				Name: []byte("id"),
																				Value: &resolve.String{
																					Path: []string{"id"},
																				},
																				OnTypeNames: [][]byte{[]byte("User")},
																			},
																			{
																				Name: []byte("__typename"),
																				Value: &resolve.String{
																					Path: []string{"__typename"},
																				},
																				OnTypeNames: [][]byte{[]byte("Admin")},
																			},
																			{
																				Name: []byte("adminID"),
																				Value: &resolve.String{
																					Path: []string{"adminID"},
																				},
																				OnTypeNames: [][]byte{[]byte("Admin")},
																			},
																		},
																	}),
																},
															},
														},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
													},
													&resolve.SingleFetch{
														SerialID: 2,
														FetchConfiguration: resolve.FetchConfiguration{
															Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Moderator {subject}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource:                            &Source{},
															SetTemplateOutputToNullOnVariableNull: true,
															RequiresEntityBatchFetch:              true,
															PostProcessing:                        EntitiesPostProcessingConfiguration,
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
																				OnTypeNames: [][]byte{[]byte("Moderator")},
																			},
																			{
																				Name: []byte("moderatorID"),
																				Value: &resolve.String{
																					Path: []string{"moderatorID"},
																				},
																				OnTypeNames: [][]byte{[]byte("Moderator")},
																			},
																		},
																	}),
																},
															},
														},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
													},
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

			t.Run("interface query on array - with interface selection expanded to inline fragments", RunTest(
				definition,
				`
					query Accounts {
						nodes {
							title
						}
					}
				`,
				"Accounts",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{nodes {__typename ... on Admin {title} ... on Moderator {__typename moderatorID} ... on User {__typename id}}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("nodes"),
									Value: &resolve.Array{
										Path:     []string{"nodes"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Admin")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("User")},
												},
											},
											Fetch: &resolve.ParallelFetch{
												Fetches: []resolve.Fetch{
													&resolve.SingleFetch{
														SerialID: 1,
														FetchConfiguration: resolve.FetchConfiguration{
															Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Moderator {title}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource:                            &Source{},
															SetTemplateOutputToNullOnVariableNull: true,
															RequiresEntityBatchFetch:              true,
															PostProcessing:                        EntitiesPostProcessingConfiguration,
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
																				OnTypeNames: [][]byte{[]byte("Moderator")},
																			},
																			{
																				Name: []byte("moderatorID"),
																				Value: &resolve.String{
																					Path: []string{"moderatorID"},
																				},
																				OnTypeNames: [][]byte{[]byte("Moderator")},
																			},
																		},
																	}),
																},
															},
														},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
													},
													&resolve.SingleFetch{
														SerialID: 2,
														FetchConfiguration: resolve.FetchConfiguration{
															Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title}}}","variables":{"representations":[$$0$$]}}}`,
															DataSource:                            &Source{},
															SetTemplateOutputToNullOnVariableNull: true,
															RequiresEntityBatchFetch:              true,
															PostProcessing:                        EntitiesPostProcessingConfiguration,
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
																				OnTypeNames: [][]byte{[]byte("User")},
																			},
																			{
																				Name: []byte("id"),
																				Value: &resolve.String{
																					Path: []string{"id"},
																				},
																				OnTypeNames: [][]byte{[]byte("User")},
																			},
																		},
																	}),
																},
															},
														},
														DataSourceIdentifier: []byte("graphql_datasource.Source"),
													},
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
	})
}
