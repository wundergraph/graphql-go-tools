package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	federationFactory := &Factory{}

	t.Run("composite keys, provides, requires", func(t *testing.T) {
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
			DataSources:                  dataSources,
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
				{
					TypeName:             "Account",
					FieldName:            "shippingInfo",
					HasAuthorizationRule: true,
				},
			},
		}

		withInfo := func(config plan.Configuration) plan.Configuration {
			config.IncludeInfo = true
			return config
		}

		t.Run("composite keys", func(t *testing.T) {
			RunWithPermutations(
				t,
				definition,
				`
				query CompositeKeys {
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
				"CompositeKeys",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchID:              0,
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
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
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
				planConfiguration)
		})

		t.Run("composite keys with info", func(t *testing.T) {
			t.Run("run", RunTest(
				definition,
				`
				query CompositeKeys {
					user {
						account {
							name
							shippingInfo {
								zip
							}
						}
					}
				}`,
				"CompositeKeys",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Info: &resolve.GraphQLResponseInfo{
							OperationType: ast.OperationTypeQuery,
						},
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchID:              0,
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								Info: &resolve.FetchInfo{
									DataSourceID:  "user.service",
									OperationType: ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "user",
										},
									},
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
										ExactParentTypeName: "Query",
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
													ExactParentTypeName: "User",
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
																ExactParentTypeName: "Account",
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
																ExactParentTypeName:  "Account",
																HasAuthorizationRule: true,
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
																			ExactParentTypeName: "ShippingInfo",
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
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
														Info: &resolve.FetchInfo{
															DataSourceID: "account.service",
															RootFields: []resolve.GraphCoordinate{
																{
																	TypeName:  "Account",
																	FieldName: "name",
																},
																{
																	TypeName:             "Account",
																	FieldName:            "shippingInfo",
																	HasAuthorizationRule: true,
																},
															},
															OperationType: ast.OperationTypeQuery,
														},
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
		})

		t.Run("composite keys variant", func(t *testing.T) {
			definition := `
				type Account {
				  id: ID!
				  type: String!
				}
				
				type User {
				  id: ID!
				  account: Account!
				  name: String!
				  foo: Boolean!
				}
				
				type Query {
				  user: User!
				}`

			subgraphA := `
				type Account @key(fields: "id") {
					id: ID!
					type: String!
				}
				
				type User @key(fields: "id account { id }") {
					id: ID!
					account: Account!
					name: String!
				}
				
				type Query {
					user: User!
				}`

			subgraphADatasourceConfiguration := plan.DataSourceConfiguration{
				ID: "service-a",
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "account", "name"},
					},
					{
						TypeName:   "Account",
						FieldNames: []string{"id", "type"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://service-a",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: subgraphA,
					},
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "Account",
							SelectionSet: "id",
						},
						{
							TypeName:     "User",
							SelectionSet: "account { id } id",
						},
					},
				},
			}

			subgraphB := `
				type Account @key(fields: "id") {
					id: ID! @external
				}
				
				type User @key(fields: "id account { id }") {
					id: ID! @external
					account: Account! @external
					foo: Boolean!
				}`

			subgraphBDatasourceConfiguration := plan.DataSourceConfiguration{
				ID: "service-b",
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "account", "foo"},
					},
					{
						TypeName:   "Account",
						FieldNames: []string{"id"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://service-b",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: subgraphB,
					},
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "Account",
							FieldName:    "",
							SelectionSet: "id",
						},
						{
							TypeName:     "User",
							FieldName:    "",
							SelectionSet: "account { id } id",
						},
					},
				},
			}

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSourceConfiguration{
					subgraphADatasourceConfiguration,
					subgraphBDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			t.Run("query without nested key object fields", func(t *testing.T) {
				t.Run("run", RunTest(
					definition,
					`
				query CompositeKey {
					user {
						id
						name
						foo
					}
				}`,
					"CompositeKey",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchID:              0,
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {id name __typename account {id}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Fetch: &resolve.SingleFetch{
												FetchID:              1,
												DependsOnFetchIDs:    []int{0},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {foo}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: resolve.NewVariables(
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
																		Name: []byte("account"),
																		Value: &resolve.Object{
																			Path: []string{"account"},
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("id"),
																					Value: &resolve.Scalar{
																						Path: []string{"id"},
																					},
																				},
																			},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																},
															}),
														},
													),
													DataSource:                            &Source{},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													SetTemplateOutputToNullOnVariableNull: true,
												},
											},
											Path: []string{"user"},
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
												{
													Name: []byte("foo"),
													Value: &resolve.Boolean{
														Path: []string{"foo"},
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

			t.Run("query with nested key object field", func(t *testing.T) {
				t.Run("run", RunTest(
					definition,
					`
				query CompositeKey {
					user {
						id
						name
						foo
						account {
							type
						}
					}
				}`,
					"CompositeKey",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchID:              0,
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {id name account {type id} __typename}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Fetch: &resolve.SingleFetch{
												FetchID:              1,
												DependsOnFetchIDs:    []int{0},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {foo}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: resolve.NewVariables(
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
																		Name: []byte("account"),
																		Value: &resolve.Object{
																			Path: []string{"account"},
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("id"),
																					Value: &resolve.Scalar{
																						Path: []string{"id"},
																					},
																				},
																			},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																},
															}),
														},
													),
													DataSource:                            &Source{},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													SetTemplateOutputToNullOnVariableNull: true,
												},
											},
											Path: []string{"user"},
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
												{
													Name: []byte("foo"),
													Value: &resolve.Boolean{
														Path: []string{"foo"},
													},
												},
												{
													Name: []byte("account"),
													Value: &resolve.Object{
														Path: []string{"account"},
														Fields: []*resolve.Field{
															{
																Name: []byte("type"),
																Value: &resolve.String{
																	Path: []string{"type"},
																},
															},
														},
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
				}`

				operationName := "Requires"

				expectedPlan := func(input string) *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchID:              0,
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
																				FetchID:              3,
																				DependsOnFetchIDs:    []int{0},
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
																			&resolve.SingleFetch{
																				FetchID:              2,
																				DependsOnFetchIDs:    []int{0, 3},
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
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
																				FetchID:              1,
																				DependsOnFetchIDs:    []int{0, 2},
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
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
																		},
																	},
																},
															},
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

				RunWithPermutations(
					t,
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
					WithMultiFetchPostProcessor(),
				)
			})
		})

		t.Run("provides", func(t *testing.T) {
			t.Run("only fields with provides", func(t *testing.T) {
				RunWithPermutations(
					t,
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
									FetchID:              0,
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
				)
			})

			t.Run("both provided and not provided", func(t *testing.T) {
				RunWithPermutations(
					t,
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
									FetchID:              0,
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
															FetchID:              1,
															DependsOnFetchIDs:    []int{0},
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
				)
			})
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSourceConfiguration{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
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

				variant1 := expectedPlan(`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename}}}"}}`)
				variant2 := expectedPlan(`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename}}}"}}`)

				RunWithPermutationsVariants(
					t,
					definition,
					query,
					"basic",
					[]plan.Plan{
						variant1,
						variant1,
						variant2,
						variant2,
						variant1,
						variant2,
					},
					planConfiguration,
				)
			})

			t.Run("shared and not shared field", func(t *testing.T) {
				t.Run("resolve from single subgraph", func(t *testing.T) {
					RunWithPermutations(
						t,
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
										FetchID:              0,
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
					)
				})

				t.Run("resolve from two subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchID:              0,
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
													FetchID:              1,
													DependsOnFetchIDs:    []int{0},
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

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[$$0$$]}}}`)

					variant2 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {middlename}}}}","variables":{"representations":[$$0$$]}}}`)

					RunWithPermutationsVariants(
						t,
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
						[]plan.Plan{
							variant1,
							variant1,
							variant2,
							variant2,
							variant1,
							variant2,
						},
						planConfiguration,
					)
				})

				t.Run("resolve from two subgraphs - not shared field", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchID:              0,
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
													FetchID:              1,
													DependsOnFetchIDs:    []int{0},
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

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {__typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)
					variant2 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {__typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					RunWithPermutationsVariants(
						t,
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
						[]plan.Plan{
							variant1,
							variant1,
							variant2,
							variant2,
							variant1,
							variant2,
						},
						planConfiguration,
					)
				})

				t.Run("resolve from two subgraphs - shared and not shared field - should not depend on the order of ds", func(t *testing.T) {
					// Note: we use only 2 datasources
					planConfiguration := plan.Configuration{
						DataSources: []plan.DataSourceConfiguration{
							firstDatasourceConfiguration,
							thirdDatasourceConfiguration,
						},
						DisableResolveFieldPositions: true,
					}

					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchID:              0,
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
													FetchID:              1,
													DependsOnFetchIDs:    []int{0},
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

					RunWithPermutations(
						t,
						definition,
						`
						query basic {
							me {
								details {
									forename
									age
								}
							}
						}
					`,
						"basic",
						expectedPlan(
							`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename} __typename id}}"}}`,
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						),
						planConfiguration,
					)
				})

				t.Run("resolve from three subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2, input3 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchID:              0,
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
															FetchID:              1,
															DependsOnFetchIDs:    []int{0},
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
														&resolve.SingleFetch{
															FetchID:              2,
															DependsOnFetchIDs:    []int{0},
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
															FetchConfiguration: resolve.FetchConfiguration{
																Input:                                 input3,
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

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant2 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant3 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {middlename}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant4 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {middlename}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					RunWithPermutationsVariants(
						t,
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
						[]plan.Plan{
							variant1,
							variant2,
							variant3,
							variant4,
							variant2,
							variant4,
						},
						planConfiguration,
						WithMultiFetchPostProcessor(),
					)
				})
			})
		})
	})

	t.Run("rewrite interface/union selections", func(t *testing.T) {
		t.Run("on object", func(t *testing.T) {
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSourceConfiguration{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			t.Run("query with inline fragments on interface - no expanding", func(t *testing.T) {
				RunWithPermutations(
					t,
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
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
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
				)
			})

			t.Run("query with selection on interface - should expand to inline fragments", func(t *testing.T) {
				RunWithPermutations(
					t,
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
													OnTypeNames: [][]byte{[]byte("Admin"), []byte("User")},
												},
											},
											Fetch: &resolve.SingleFetch{
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
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
				)
			})
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSourceConfiguration{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			t.Run("union query on array", func(t *testing.T) {
				RunWithPermutations(
					t,
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
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
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
															FetchID:           2,
															DependsOnFetchIDs: []int{0},
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
					WithMultiFetchPostProcessor(),
				)
			})

			t.Run("interface query on array - with interface selection expanded to inline fragments", func(t *testing.T) {
				RunWithPermutations(
					t,
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
														OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
													},
												},
												Fetch: &resolve.ParallelFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
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
															FetchID:           2,
															DependsOnFetchIDs: []int{0},
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
					WithMultiFetchPostProcessor(),
				)
			})
		})
	})

	t.Run("different entity keys jumps", func(t *testing.T) {
		t.Run("single key - double key - single key", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					uuid: ID!
					title: String!
					name: String!
					address: Address!
				}

				type Address {
					country: String!
				}

				type Query {
					user: User!
				}
			`

			firstSubgraphSDL := `	
				type User @key(fields: "id") {
					id: ID!
				}

				type Query {
					user: User
				}
			`

			firstDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id"},
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
					},
				},
			}

			secondSubgraphSDL := `
				type User @key(fields: "id") @key(fields: "uuid") {
					id: ID!
					uuid: ID!
					name: String!
				}
			`
			secondDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "uuid", "name"},
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
							TypeName:     "User",
							SelectionSet: "uuid",
						},
					},
				},
			}

			thirdSubgraphSDL := `
				type User @key(fields: "uuid") {
					uuid: ID!
					title: String!
				}
			`
			thirdDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"uuid", "title"},
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
							TypeName:     "User",
							SelectionSet: "uuid",
						},
					},
				},
			}

			fourthSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					address: Address!
				}

				type Address {
					country: String!
				}
			`
			fourthDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "address"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Address",
						FieldNames: []string{"country"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://fourth.service",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: fourthSubgraphSDL,
					},
					UpstreamSchema: fourthSubgraphSDL,
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSourceConfiguration{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
					fourthDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			RunWithPermutations(
				t,
				definition,
				`
				query User {
					user {
						id
						name
						title
						address {
							country
						}
					}
				}`,
				"User",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {id __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
									Value: &resolve.Object{
										Path:     []string{"user"},
										Nullable: false,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path: []string{"name"},
												},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
											},
											{
												Name: []byte("address"),
												Value: &resolve.Object{
													Path: []string{"address"},
													Fields: []*resolve.Field{
														{
															Name: []byte("country"),
															Value: &resolve.String{
																Path: []string{"country"},
															},
														},
													},
												},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.ParallelFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name uuid}}}","variables":{"representations":[$$0$$]}}}`,
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
																			},
																		}),
																	},
																},
																PostProcessing: SingleEntityPostProcessingConfiguration,
															},
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
														},
														&resolve.SingleFetch{
															FetchID:           3,
															DependsOnFetchIDs: []int{0},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {address {country}}}}","variables":{"representations":[$$0$$]}}}`,
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
												&resolve.SingleFetch{
													FetchID:           2,
													DependsOnFetchIDs: []int{0, 1},
													FetchConfiguration: resolve.FetchConfiguration{
														RequiresEntityBatchFetch:              false,
														RequiresEntityFetch:                   true,
														Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																			Name: []byte("uuid"),
																			Value: &resolve.String{
																				Path: []string{"uuid"},
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
					},
				},
				planConfiguration,
				WithMultiFetchPostProcessor(),
			)
		})

		t.Run("single key - double key - double key - single key", func(t *testing.T) {
			definition := `
				type User {
					key1: ID!
					key2: ID!
					key3: ID!

					field1: String!
					field2: String!
					field3: String!
					field4: String!
				}

				type Query {
					user: User!
				}
			`

			firstSubgraphSDL := `	
				type User @key(fields: "key1") {
					key1: ID!
					field1: String!
				}

				type Query {
					user: User
				}
			`

			firstDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"key1", "field1"},
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
							SelectionSet: "key1",
						},
					},
				},
			}

			secondSubgraphSDL := `
				type User @key(fields: "key1") @key(fields: "key2") {
					key1: ID!
					key2: ID!
					field2: String!
				}
			`
			secondDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"key1", "key2", "field2"},
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
							SelectionSet: "key1",
						},
						{
							TypeName:     "User",
							SelectionSet: "key2",
						},
					},
				},
			}

			thirdSubgraphSDL := `
				type User @key(fields: "key2") @key(fields: "key3") {
					key2: ID!
					key3: ID!
					field3: String!
				}
			`
			thirdDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"key2", "key3", "field3"},
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
							TypeName:     "User",
							SelectionSet: "key2",
						},
						{
							TypeName:     "User",
							SelectionSet: "key3",
						},
					},
				},
			}

			fourthSubgraphSDL := `
				type User @key(fields: "key3") {
					key3: ID!
					field4: String!
				}
			`
			fourthDatasourceConfiguration := plan.DataSourceConfiguration{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"key3", "field4"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "http://fourth.service",
					},
					Federation: FederationConfiguration{
						Enabled:    true,
						ServiceSDL: fourthSubgraphSDL,
					},
					UpstreamSchema: fourthSubgraphSDL,
				}),
				Factory: federationFactory,
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "key3",
						},
					},
				},
			}

			dataSources := []plan.DataSourceConfiguration{
				firstDatasourceConfiguration,
				secondDatasourceConfiguration,
				thirdDatasourceConfiguration,
				fourthDatasourceConfiguration,
			}

			planConfiguration := plan.Configuration{
				DataSources:                  dataSources,
				DisableResolveFieldPositions: true,
			}

			t.Run("only fields", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
				query User {
					user {
						field1
						field2
						field3
						field4
					}
				}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {field1 __typename key1}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("field1"),
													Value: &resolve.String{
														Path: []string{"field1"},
													},
												},
												{
													Name: []byte("field2"),
													Value: &resolve.String{
														Path: []string{"field2"},
													},
												},
												{
													Name: []byte("field3"),
													Value: &resolve.String{
														Path: []string{"field3"},
													},
												},
												{
													Name: []byte("field4"),
													Value: &resolve.String{
														Path: []string{"field4"},
													},
												},
											},
											Fetch: &resolve.SerialFetch{
												Fetches: []resolve.Fetch{
													&resolve.SingleFetch{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {field2 key2}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key1"),
																				Value: &resolve.String{
																					Path: []string{"key1"},
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
													&resolve.SingleFetch{
														FetchID:           2,
														DependsOnFetchIDs: []int{0, 1},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {field3 key3}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key2"),
																				Value: &resolve.String{
																					Path: []string{"key2"},
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
													&resolve.SingleFetch{
														FetchID:           3,
														DependsOnFetchIDs: []int{0, 2},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {field4}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key3"),
																				Value: &resolve.String{
																					Path: []string{"key3"},
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
						},
					},
					planConfiguration,
					WithMultiFetchPostProcessor(),
				)
			})

			t.Run("fields and keys", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
				query User {
					user {
						key1
						key2
						key3
						field1
						field2
						field3
						field4
					}
				}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {key1 field1 __typename}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("key1"),
													Value: &resolve.Scalar{
														Path: []string{"key1"},
													},
												},
												{
													Name: []byte("key2"),
													Value: &resolve.Scalar{
														Path: []string{"key2"},
													},
												},
												{
													Name: []byte("key3"),
													Value: &resolve.Scalar{
														Path: []string{"key3"},
													},
												},
												{
													Name: []byte("field1"),
													Value: &resolve.String{
														Path: []string{"field1"},
													},
												},
												{
													Name: []byte("field2"),
													Value: &resolve.String{
														Path: []string{"field2"},
													},
												},
												{
													Name: []byte("field3"),
													Value: &resolve.String{
														Path: []string{"field3"},
													},
												},
												{
													Name: []byte("field4"),
													Value: &resolve.String{
														Path: []string{"field4"},
													},
												},
											},
											Fetch: &resolve.SerialFetch{
												Fetches: []resolve.Fetch{
													&resolve.SingleFetch{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {key2 field2}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key1"),
																				Value: &resolve.String{
																					Path: []string{"key1"},
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
													&resolve.SingleFetch{
														FetchID:           2,
														DependsOnFetchIDs: []int{1, 0},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {key3 field3}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key2"),
																				Value: &resolve.String{
																					Path: []string{"key2"},
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
													&resolve.SingleFetch{
														FetchID:           3,
														DependsOnFetchIDs: []int{2, 0},
														FetchConfiguration: resolve.FetchConfiguration{
															RequiresEntityBatchFetch:              false,
															RequiresEntityFetch:                   true,
															Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {field4}}}","variables":{"representations":[$$0$$]}}}`,
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
																				Name: []byte("key3"),
																				Value: &resolve.String{
																					Path: []string{"key3"},
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
						},
					},
					planConfiguration,
					WithMultiFetchPostProcessor(),
				)
			})
		})
	})
}
