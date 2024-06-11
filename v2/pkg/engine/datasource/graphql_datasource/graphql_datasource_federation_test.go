package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	t.Run("composite keys, provides, requires", func(t *testing.T) {
		definition := `
			type Account {
				id: ID!
				name: String!
				info: Info
				address: Address
				deliveryAddress: Address
				secretAddress: Address
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
				secretLine: String!
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
			type Query {
				user: User
			}

			type User @key(fields: "id") {
				id: ID!
				account: Account
				oldAccount: Account @provides(fields: "name shippingInfo {zip}")
			}

			type Account @key(fields: "id info {a b}") {
				id: ID!
				info: Info
				name: String! @external
				shippingInfo: ShippingInfo @external
				address: Address
				deliveryAddress: Address @requires(fields: "shippingInfo {zip}")
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
		// TODO: add test for requires when query already has the required field with the different argument - it is using field from a query not with default arg

		usersDatasourceConfiguration := mustDataSourceConfiguration(t,
			"user.service",
			&plan.DataSourceMetadata{
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
						FieldNames: []string{"id", "info", "address", "deliveryAddress"},
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
					Requires: plan.FederationFieldConfigurations{
						{
							TypeName:     "Account",
							FieldName:    "deliveryAddress",
							SelectionSet: "shippingInfo {zip}",
						},
					},
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://user.service",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: usersSubgraphSDL,
						},
						usersSubgraphSDL,
					),
				},
			),
		)
		accountsSubgraphSDL := `
			type Query {
				account: Account
			}

			type Account @key(fields: "id info {a b}") {
				id: ID!
				name: String!
				info: Info
				shippingInfo: ShippingInfo
				secretAddress: Address
			}

			type Info {
				a: ID!
				b: ID!
			}

			type ShippingInfo {
				zip: String!
			}

			type Address @key(fields: "id") {
				id: ID!
				line1: String! @external
				line2: String! @external
				line3(test: String!): String! @external
				zip: String! @external
				secretLine: String! @requires(fields: "zip")
				fullAddress: String! @requires(fields: "line1 line2 line3(test:\"BOOM\") zip")
			}
		`

		accountsDatasourceConfiguration := mustDataSourceConfiguration(
			t,
			"account.service",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"account"},
					},
					{
						TypeName:   "Account",
						FieldNames: []string{"id", "name", "info", "shippingInfo", "secretAddress"},
					},
					{
						TypeName:   "Address",
						FieldNames: []string{"id", "fullAddress", "secretLine"},
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
						{
							TypeName:     "Address",
							FieldName:    "secretLine",
							SelectionSet: "zip",
						},
					},
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://account.service",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: accountsSubgraphSDL,
						},
						accountsSubgraphSDL,
					),
				},
			),
		)

		addressesSubgraphSDL := `
			type Address @key(fields: "id") {
				id: ID!
				line3(test: String!): String!
				country: String! @external
				city: String! @external
				zip: String! @requires(fields: "country city")
			}
		`

		addressesDatasourceConfiguration := mustDataSourceConfiguration(
			t,
			"address.service",

			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Address",
						FieldNames: []string{"id", "line3", "zip"},
					},
				},
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
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://address.service",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: addressesSubgraphSDL,
						},
						addressesSubgraphSDL,
					),
				},
			),
		)

		addressesEnricherSubgraphSDL := `
			type Address @key(fields: "id") {
				id: ID!
				country: String!
				city: String!
			}
		`

		addressesEnricherDatasourceConfiguration := mustDataSourceConfiguration(
			t,
			"address-enricher.service",

			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Address",
						FieldNames: []string{"id", "country", "city"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "Address",
							FieldName:    "",
							SelectionSet: "id",
						},
					},
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://address-enricher.service",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: addressesEnricherSubgraphSDL,
						},
						addressesEnricherSubgraphSDL,
					),
				},
			),
		)

		dataSources := []plan.DataSource{
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
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
														},
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
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
														},
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
				  otherUser: User!
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
					otherUser: User!
				}`

			subgraphADatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"service-a",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"user", "otherUser"},
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://service-a",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: subgraphA,
							},
							subgraphA,
						),
					},
				),
			)

			subgraphB := `
				type Account @key(fields: "id") {
					id: ID! @external
				}
				
				type User @key(fields: "id account { id }") {
					id: ID! @external
					account: Account! @external
					foo: Boolean!
				}`

			subgraphBDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"service-b",

				&plan.DataSourceMetadata{
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://service-b",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: subgraphB,
							},
							subgraphB,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					subgraphADatasourceConfiguration,
					subgraphBDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			t.Run("query having a fetch after fetch with composite key", func(t *testing.T) {
				t.Run("run", RunTest(
					definition,
					`
				query CompositeKey {
					user {
						foo
					}
					otherUser {
						foo
					}
				}`,
					"CompositeKey",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {__typename account {id} id} otherUser {__typename account {id} id}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												},
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
													Name: []byte("foo"),
													Value: &resolve.Boolean{
														Path: []string{"foo"},
													},
												},
											},
										},
									},
									{
										Name: []byte("otherUser"),
										Value: &resolve.Object{
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
												},
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
											Path: []string{"otherUser"},
											Fields: []*resolve.Field{
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
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												},
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
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												},
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

				expectedPlan := func() *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {line1 line2 __typename id}}}}"}}`,
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           2,
																					DependsOnFetchIDs: []int{0},
																				},
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           1,
																					DependsOnFetchIDs: []int{0, 2},
																				},
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           3,
																					DependsOnFetchIDs: []int{0, 1},
																				},
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
					expectedPlan(),
					plan.Configuration{
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: false,
						},
						DataSources: []plan.DataSource{
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

			t.Run("complex requires chain", func(t *testing.T) {
				// this tests illustrates a complex requires chain on Address object
				//
				// secretAddress.secret -> requires secretAddress.zip -> requires secretAddress.city secretAddress.country
				//
				// The tricky part here - we should get representation of Address {id, __typename} from accounts subgraph but via Account entity
				// After gathering all requirements we should send a final request again to accounts subgraph but for the Address entity
				// We should never try to get secretAddress.secret from the account subgraph via the first query, because we don't have requirements for it

				operation := `
				query Requires {
					user {
						account {
							secretAddress {
								secretLine
							}
						}
					}
				}`

				operationName := "Requires"

				expectedPlan := func() *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
																Name: []byte("secretAddress"),
																Value: &resolve.Object{
																	Path:     []string{"secretAddress"},
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("secretLine"),
																			Value: &resolve.String{
																				Path: []string{"secretLine"},
																			},
																		},
																	},
																	Fetch: &resolve.SerialFetch{
																		Fetches: []resolve.Fetch{
																			&resolve.SingleFetch{
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           2,
																					DependsOnFetchIDs: []int{1},
																				},
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           3,
																					DependsOnFetchIDs: []int{2, 1},
																				},
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
																					Input:               `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {zip}}}","variables":{"representations":[$$0$$]}}}`,
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           4,
																					DependsOnFetchIDs: []int{3, 1},
																				},
																				DataSourceIdentifier: []byte("graphql_datasource.Source"),
																				FetchConfiguration: resolve.FetchConfiguration{
																					Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {secretLine}}}","variables":{"representations":[$$0$$]}}}`,
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
														Fetch: &resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															},
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
															FetchConfiguration: resolve.FetchConfiguration{
																Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {secretAddress {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
																					OnTypeNames: [][]byte{[]byte("Account")},
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
					}
				}

				RunWithPermutations(
					t,
					definition,
					operation,
					operationName,
					expectedPlan(),
					plan.Configuration{
						DataSources: []plan.DataSource{
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

			t.Run("nested selection set in requires", func(t *testing.T) {
				operation := `
				query Requires {
					user {
						account {
							deliveryAddress {
								line1
							}
						}
					}
				}`

				operationName := "Requires"

				expectedPlan := func() *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
																Name: []byte("deliveryAddress"),
																Value: &resolve.Object{
																	Path:     []string{"deliveryAddress"},
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("line1"),
																			Value: &resolve.String{
																				Path: []string{"line1"},
																			},
																		},
																	},
																},
															},
														},
														Fetch: &resolve.SerialFetch{
															Fetches: []resolve.Fetch{
																&resolve.SingleFetch{
																	FetchDependencies: resolve.FetchDependencies{
																		FetchID:           1,
																		DependsOnFetchIDs: []int{0},
																	},
																	DataSourceIdentifier: []byte("graphql_datasource.Source"),
																	FetchConfiguration: resolve.FetchConfiguration{
																		Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
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
																							OnTypeNames: [][]byte{[]byte("Account")},
																						},
																					},
																				}),
																			},
																		},
																		SetTemplateOutputToNullOnVariableNull: true,
																	},
																},
																&resolve.SingleFetch{
																	FetchDependencies: resolve.FetchDependencies{
																		FetchID:           2,
																		DependsOnFetchIDs: []int{1, 0},
																	},
																	DataSourceIdentifier: []byte("graphql_datasource.Source"),
																	FetchConfiguration: resolve.FetchConfiguration{
																		Input:               `{"method":"POST","url":"http://user.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {deliveryAddress {line1}}}}","variables":{"representations":[$$0$$]}}}`,
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
																							OnTypeNames: [][]byte{[]byte("Account")},
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
																							OnTypeNames: [][]byte{[]byte("Account")},
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
					}
				}

				RunWithPermutations(
					t,
					definition,
					operation,
					operationName,
					expectedPlan(),
					plan.Configuration{
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: true,
						},
						DataSources: []plan.DataSource{
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

			t.Run("nested selection set - but requirements are provided", func(t *testing.T) {
				t.Skip("fixme")

				operation := `
				query Requires {
					user {
						oldAccount {
							deliveryAddress {
								line1
							}
						}
					}
				}`

				operationName := "Requires"

				expectedPlan := func() *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {line1 line2 __typename id}}}}"}}`,
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           3,
																					DependsOnFetchIDs: []int{0},
																				},
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           2,
																					DependsOnFetchIDs: []int{0, 3},
																				},
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
																				FetchDependencies: resolve.FetchDependencies{
																					FetchID:           1,
																					DependsOnFetchIDs: []int{0, 2},
																				},
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
					expectedPlan(),
					plan.Configuration{
						Debug: plan.DebugConfiguration{
							PrintQueryPlans: true,
						},
						DataSources: []plan.DataSource{
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

			t.Run("requires fields from the root query subgraph", func(t *testing.T) {
				definition := `
					type User {
						id: ID!
						firstName: String!
						lastName: String!
						fullName: String!
					}
	
					type Query {
						user: User!
					}
				`

				firstSubgraphSDL := `	
					type User @key(fields: "id") {
						id: ID!
						fullName: String! @requires(fields: "firstName lastName")
						firstName: String! @external
						lastName: String! @external
					}
	
					type Query {
						user: User
					}
				`

				firstDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"first-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"user"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "fullName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "fullName",
									SelectionSet: "firstName lastName",
								},
							},
						},
					},
					mustCustomConfiguration(t,
						ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "http://first.service",
							},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{
									Enabled:    true,
									ServiceSDL: firstSubgraphSDL,
								},
								firstSubgraphSDL,
							),
						},
					),
				)

				secondSubgraphSDL := `
					type User @key(fields: "id") {
						id: ID!
						firstName: String!
						lastName: String!
					}
				`

				secondDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"second-service",

					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "firstName", "lastName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t,
						ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "http://second.service",
							},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{
									Enabled:    true,
									ServiceSDL: secondSubgraphSDL,
								},
								secondSubgraphSDL,
							),
						},
					),
				)

				planConfiguration := plan.Configuration{
					DataSources: []plan.DataSource{
						firstDatasourceConfiguration,
						secondDatasourceConfiguration,
					},
					DisableResolveFieldPositions: true,
					Debug: plan.DebugConfiguration{
						PrintQueryPlans: true,
					},
				}

				t.Run("selected only field with requires directive", func(t *testing.T) {
					RunWithPermutations(
						t,
						definition,
						`
						query User {
							user {
								fullName
							}
						}`,
						"User",
						&plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
														Name: []byte("fullName"),
														Value: &resolve.String{
															Path: []string{"fullName"},
														},
													},
												},
												Fetch: &resolve.SerialFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{1, 0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
																					Name: []byte("firstName"),
																					Value: &resolve.String{
																						Path: []string{"firstName"},
																					},
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																				{
																					Name: []byte("lastName"),
																					Value: &resolve.String{
																						Path: []string{"lastName"},
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

				t.Run("selected field with requires directive and required fields", func(t *testing.T) {
					expectedNestedFetch := &resolve.SerialFetch{
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{1, 0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name: []byte("firstName"),
														Value: &resolve.String{
															Path: []string{"firstName"},
														},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name: []byte("lastName"),
														Value: &resolve.String{
															Path: []string{"lastName"},
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
					}

					t.Run("requires after required", func(t *testing.T) {
						RunWithPermutations(
							t,
							definition,
							`
						query User {
							user {
								firstName
								lastName
								fullName
							}
						}`,
							"User",
							&plan.SynchronousResponsePlan{
								Response: &resolve.GraphQLResponse{
									Data: &resolve.Object{
										Fetch: &resolve.SingleFetch{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
															Name: []byte("firstName"),
															Value: &resolve.String{
																Path: []string{"firstName"},
															},
														},
														{
															Name: []byte("lastName"),
															Value: &resolve.String{
																Path: []string{"lastName"},
															},
														},
														{
															Name: []byte("fullName"),
															Value: &resolve.String{
																Path: []string{"fullName"},
															},
														},
													},
													Fetch: expectedNestedFetch,
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

					t.Run("requires before required", func(t *testing.T) {
						RunWithPermutations(
							t,
							definition,
							`
						query User {
							user {
								fullName
								firstName
								lastName
							}
						}`,
							"User",
							&plan.SynchronousResponsePlan{
								Response: &resolve.GraphQLResponse{
									Data: &resolve.Object{
										Fetch: &resolve.SingleFetch{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
															Name: []byte("fullName"),
															Value: &resolve.String{
																Path: []string{"fullName"},
															},
														},
														{
															Name: []byte("firstName"),
															Value: &resolve.String{
																Path: []string{"firstName"},
															},
														},
														{
															Name: []byte("lastName"),
															Value: &resolve.String{
																Path: []string{"lastName"},
															},
														},
													},
													Fetch: expectedNestedFetch,
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

					t.Run("requires between required", func(t *testing.T) {
						RunWithPermutations(
							t,
							definition,
							`
						query User {
							user {
								firstName
								fullName
								lastName
							}
						}`,
							"User",
							&plan.SynchronousResponsePlan{
								Response: &resolve.GraphQLResponse{
									Data: &resolve.Object{
										Fetch: &resolve.SingleFetch{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
															Name: []byte("firstName"),
															Value: &resolve.String{
																Path: []string{"firstName"},
															},
														},
														{
															Name: []byte("fullName"),
															Value: &resolve.String{
																Path: []string{"fullName"},
															},
														},
														{
															Name: []byte("lastName"),
															Value: &resolve.String{
																Path: []string{"lastName"},
															},
														},
													},
													Fetch: expectedNestedFetch,
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

			t.Run("requires fields from 2 subgraphs from root query subgraph and sibling", func(t *testing.T) {
				definition := `
					type User {
						id: ID!
						firstName: String!
						lastName: String!
						fullName: String!
					}
	
					type Query {
						user: User!
					}
				`

				firstSubgraphSDL := `	
					type User @key(fields: "id") {
						id: ID!
						firstName: String!
					}
	
					type Query {
						user: User
					}
				`

				firstDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"first-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"user"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "firstName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t,
						ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "http://first.service",
							},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{
									Enabled:    true,
									ServiceSDL: firstSubgraphSDL,
								},
								firstSubgraphSDL,
							),
						},
					),
				)

				secondSubgraphSDL := `
					type User @key(fields: "id") {
						id: ID!
						firstName: String! @external
						lastName: String! @external
						fullName: String! @requires(fields: "firstName lastName")
					}
				`

				secondDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"second-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "fullName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									FieldName:    "fullName",
									SelectionSet: "firstName lastName",
								},
							},
						},
					},
					mustCustomConfiguration(t,
						ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "http://second.service",
							},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{
									Enabled:    true,
									ServiceSDL: secondSubgraphSDL,
								},
								secondSubgraphSDL,
							),
						},
					),
				)

				thirdSubgraphSDL := `
					type User @key(fields: "id") {
						id: ID!
						lastName: String!
					}
				`

				thirdDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"third-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "lastName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t,
						ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "http://third.service",
							},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{
									Enabled:    true,
									ServiceSDL: thirdSubgraphSDL,
								},
								thirdSubgraphSDL,
							),
						},
					),
				)

				planConfiguration := plan.Configuration{
					DataSources: []plan.DataSource{
						firstDatasourceConfiguration,
						secondDatasourceConfiguration,
						thirdDatasourceConfiguration,
					},
					DisableResolveFieldPositions: true,
					Debug:                        plan.DebugConfiguration{},
				}

				t.Run("selected only fields with requires", func(t *testing.T) {
					RunWithPermutations(
						t,
						definition,
						`
						query User {
							user {
								fullName
							}
						}`,
						"User",
						&plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {firstName __typename id}}"}}`,
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
														Name: []byte("fullName"),
														Value: &resolve.String{
															Path: []string{"fullName"},
														},
													},
												},
												Fetch: &resolve.SerialFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0, 1},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
																					Name: []byte("firstName"),
																					Value: &resolve.String{
																						Path: []string{"firstName"},
																					},
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																				{
																					Name: []byte("lastName"),
																					Value: &resolve.String{
																						Path: []string{"lastName"},
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

				t.Run("selected field with requires and required fields", func(t *testing.T) {
					RunWithPermutations(
						t,
						definition,
						`
						query User {
							user {
								firstName
								lastName
								fullName
							}
						}`,
						"User",
						&plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Data: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {firstName __typename id}}"}}`,
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
														Name: []byte("firstName"),
														Value: &resolve.String{
															Path: []string{"firstName"},
														},
													},
													{
														Name: []byte("lastName"),
														Value: &resolve.String{
															Path: []string{"lastName"},
														},
													},
													{
														Name: []byte("fullName"),
														Value: &resolve.String{
															Path: []string{"fullName"},
														},
													},
												},
												Fetch: &resolve.SerialFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0, 1},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
																					Name: []byte("firstName"),
																					Value: &resolve.String{
																						Path: []string{"firstName"},
																					},
																					OnTypeNames: [][]byte{[]byte("User")},
																				},
																				{
																					Name: []byte("lastName"),
																					Value: &resolve.String{
																						Path: []string{"lastName"},
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
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															},
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

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first.service",

				&plan.DataSourceMetadata{
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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

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

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					details: Details! @shareable
				}
	
				type Details {
					age: Int!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug: plan.DebugConfiguration{
					PrintQueryPlans:      false,
					PrintNodeSuggestions: false,
					PrintPlanningPaths:   false,
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
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
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
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
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
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
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
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
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
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
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
						DataSources: []plan.DataSource{
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
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
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
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
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
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															}, DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0},
															}, DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
					some: User!
				}

				type Admin implements Node {
					id: ID!
					title: String!
					adminName: String!
					some: User!
				}

				interface Node {
					id: ID!
					title: String!
					some: User!
				}

				type Query {
					account: Node!
				}
			`

			firstSubgraphSDL := `	
				type User implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
					some: User!
				}

				type Admin implements Node @key(fields: "id") {
					id: ID!
					title: String! @external
					some: User!
				}

				interface Node {
					id: ID!
					title: String!
					some: User!
				}

				type Query {
					account: Node
				}
			`

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"account"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "some"},
						},
						{
							TypeName:   "Admin",
							FieldNames: []string{"id", "some"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Node",
							FieldNames: []string{"id", "title", "some"},
						},
					},
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

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

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug: plan.DebugConfiguration{
					PrintPlanningPaths: false,
				},
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
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
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

			t.Run("query on interface - no expanding - with nested fetch", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
					query Accounts {
						account {
							some {
								name
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
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {some {__typename id}}}"}}`,
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
													Name: []byte("some"),
													Value: &resolve.Object{
														Path: []string{"some"},
														Fields: []*resolve.Field{
															{
																Name: []byte("name"),
																Value: &resolve.String{
																	Path: []string{"name"},
																},
															},
														},
														Fetch: &resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
																RequiresEntityBatchFetch:              false,
																RequiresEntityFetch:                   true,
																Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[$$0$$]}}}`,
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
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
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

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

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

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type Moderator @key(fields: "moderatorID") {
					moderatorID: ID!
					subject: String!
					title: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Moderator",
							FieldNames: []string{"moderatorID", "subject", "title"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Moderator",
								SelectionSet: "moderatorID",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0},
															},
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
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

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type User @key(fields: "id") @key(fields: "uuid") {
					id: ID!
					uuid: ID!
					name: String!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "uuid", "name"},
						},
					},
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type User @key(fields: "uuid") {
					uuid: ID!
					title: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"uuid", "title"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "uuid",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			fourthSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					address: Address!
				}

				type Address {
					country: String!
				}
			`

			fourthDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"fourth-service",

				&plan.DataSourceMetadata{

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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://fourth.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: fourthSubgraphSDL,
							},
							fourthSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           1,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
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
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           3,
																DependsOnFetchIDs: []int{0},
															}, FetchConfiguration: resolve.FetchConfiguration{
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
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0, 1},
													}, FetchConfiguration: resolve.FetchConfiguration{
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

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
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
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "key1",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type User @key(fields: "key1") @key(fields: "key2") {
					key1: ID!
					key2: ID!
					field2: String!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"key1", "key2", "field2"},
						},
					},
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type User @key(fields: "key2") @key(fields: "key3") {
					key2: ID!
					key3: ID!
					field3: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"key2", "key3", "field3"},
						},
					},
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
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			fourthSubgraphSDL := `
				type User @key(fields: "key3") {
					key3: ID!
					field4: String!
				}
			`

			fourthDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"fourth-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"key3", "field4"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "key3",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://fourth.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: fourthSubgraphSDL,
							},
							fourthSubgraphSDL,
						),
					},
				),
			)

			dataSources := []plan.DataSource{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
														}, FetchConfiguration: resolve.FetchConfiguration{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           2,
															DependsOnFetchIDs: []int{0, 1},
														}, FetchConfiguration: resolve.FetchConfiguration{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           3,
															DependsOnFetchIDs: []int{0, 2},
														}, FetchConfiguration: resolve.FetchConfiguration{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           1,
															DependsOnFetchIDs: []int{0},
														}, FetchConfiguration: resolve.FetchConfiguration{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           2,
															DependsOnFetchIDs: []int{1, 0},
														}, FetchConfiguration: resolve.FetchConfiguration{
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
														FetchDependencies: resolve.FetchDependencies{
															FetchID:           3,
															DependsOnFetchIDs: []int{2, 0},
														}, FetchConfiguration: resolve.FetchConfiguration{
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

	t.Run("key resolvable false", func(t *testing.T) {
		t.Run("example 1", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					user: User!
					userWithName: User!
				}
			`

			firstSubgraphSDL := `	
				type User @key(fields: "id") {
					id: ID!
					title: String!
				}

				type Query {
					user: User
				}
			`

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"user"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "title"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type User @key(fields: "id" resolvable: false) {
					id: ID!
					name: String!
				}

				type Query {
					userWithName: User
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name"},
						},
						{
							TypeName:   "Query",
							FieldNames: []string{"userWithName"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:              "User",
								SelectionSet:          "id",
								DisableEntityResolver: true,
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					name: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug:                        plan.DebugConfiguration{},
			}

			t.Run("do not jump to resolvable false", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								id
								name
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
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													RequiresEntityBatchFetch:              false,
													RequiresEntityFetch:                   true,
													Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[$$0$$]}}}`,
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
								},
							},
						},
					},
					planConfiguration,
				)
			})

			t.Run("jump from resolvable false", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							userWithName {
								name
								title
							}
						}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{userWithName {name __typename id}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("userWithName"),
										Value: &resolve.Object{
											Path:     []string{"userWithName"},
											Nullable: false,
											Fields: []*resolve.Field{
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
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													RequiresEntityBatchFetch:              false,
													RequiresEntityFetch:                   true,
													Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title}}}","variables":{"representations":[$$0$$]}}}`,
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
								},
							},
						},
					},
					planConfiguration,
				)
			})
		})

		t.Run("example 2", func(t *testing.T) {
			definition := `
				type Entity {
				  id: ID!
				  name: String!
				  age: Int!
				}
				
				type Query {
				  entity: Entity!
				}
			`

			firstSubgraphSDL := `	
				type Query {
					entity: Entity!
				}
				
				type Entity {
					id: ID!
					name: String!
				}
			`

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"entity"},
						},
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "name"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:              "Entity",
								SelectionSet:          "id",
								DisableEntityResolver: true,
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type Entity @key(fields: "id") {
					id: ID!
					age: Int!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "age"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Entity",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug:                        plan.DebugConfiguration{},
			}

			t.Run("query", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query Query {
							entity {
								id
								name
								age
							}
						}
					`,
					"Query",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entity {id name __typename}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("entity"),
										Value: &resolve.Object{
											Path:     []string{"entity"},
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
													Name: []byte("age"),
													Value: &resolve.Integer{
														Path: []string{"age"},
													},
												},
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													RequiresEntityBatchFetch:              false,
													RequiresEntityFetch:                   true,
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {age}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("Entity")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("Entity")},
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

		t.Run("example 3", func(t *testing.T) {
			definition := `
				type Entity {
				  id: ID!
				  name: String!
				  isEntity: Boolean!
				  age: Int!
				}
				
				type Query {
				  entity: Entity!
				}
			`

			firstSubgraphSDL := `	
				type Query {
					entity: Entity!
				}
				
				type Entity @key(fields: "id", resolvable: false) @key(fields: "name") {
					id: ID!
					name: String!
					isEntity: Boolean!
				}
			`

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"entity"},
						},
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "name", "isEntity"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:              "Entity",
								SelectionSet:          "id",
								DisableEntityResolver: true,
							},
							{
								TypeName:              "Entity",
								SelectionSet:          "name",
								DisableEntityResolver: true,
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type Entity @key(fields: "id") @key(fields: "name") {
					id: ID!
					name: String!
					age: Int!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "name", "age"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Entity",
								SelectionSet: "id",
							},
							{
								TypeName:     "Entity",
								SelectionSet: "name",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug:                        plan.DebugConfiguration{},
			}

			t.Run("query", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query Query {
							entity {
								id
								name
								isEntity
								age
							}
						}
					`,
					"Query",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entity {id name isEntity __typename}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("entity"),
										Value: &resolve.Object{
											Path:     []string{"entity"},
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
													Name: []byte("isEntity"),
													Value: &resolve.Boolean{
														Path: []string{"isEntity"},
													},
												},
												{
													Name: []byte("age"),
													Value: &resolve.Integer{
														Path: []string{"age"},
													},
												},
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													RequiresEntityBatchFetch:              false,
													RequiresEntityFetch:                   true,
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {age}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("Entity")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("Entity")},
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

		t.Run("example 4 - leap frogging", func(t *testing.T) {
			definition := `
				type Entity {
				  id: ID!
				  uuid: ID!
				  name: String!
				  isEntity: Boolean!
				  age: Int!
				  rating: Float!
				  isImportant: Boolean!
				}
				
				type Query {
				  entityOne: Entity!
				  entityTwo: Entity!
				  entityThree: Entity!
				}
			`

			firstSubgraphSDL := `	
				type Query {
					entityOne: Entity!
				}
				
				type Entity @key(fields: "id") @key(fields: "name", resolvable: false) @key(fields: "uuid", resolvable: false) {
					id: ID!
					uuid: ID!
					name: String!
					isEntity: Boolean!
				}
			`

			firstDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"first-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"entityOne"},
						},
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "uuid", "name", "isEntity"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Entity",
								SelectionSet: "id",
							},
							{
								TypeName:              "Entity",
								SelectionSet:          "name",
								DisableEntityResolver: true,
							},
							{
								TypeName:              "Entity",
								SelectionSet:          "uuid",
								DisableEntityResolver: true,
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://first.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: firstSubgraphSDL,
							},
							firstSubgraphSDL,
						),
					},
				),
			)

			secondSubgraphSDL := `
				type Query {
					entityTwo: Entity!
				}
				
				type Entity @key(fields: "id", resolvable: false) @key(fields: "name", resolvable: false) @key(fields: "uuid") {
					id: ID!
					uuid: ID!
					name: String!
					age: Int!
					rating: Float!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"entityTwo"},
						},
						{
							TypeName:   "Entity",
							FieldNames: []string{"id", "uuid", "name", "age", "rating"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:              "Entity",
								SelectionSet:          "id",
								DisableEntityResolver: true,
							},
							{
								TypeName:              "Entity",
								SelectionSet:          "name",
								DisableEntityResolver: true,
							},
							{
								TypeName:     "Entity",
								SelectionSet: "uuid",
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://second.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: secondSubgraphSDL,
							},
							secondSubgraphSDL,
						),
					},
				),
			)

			thirdSubgraphSDL := `
				type Query {
					entityThree: Entity!
				}
				
				type Entity @key(fields: "id", resolvable: false) @key(fields: "name") @key(fields: "uuid", resolvable: false) {
					id: ID!
					uuid: ID!
					name: String!
					age: Int!
					isImportant: Boolean!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"entityThree"},
					},
					{
						TypeName:   "Entity",
						FieldNames: []string{"id", "uuid", "name", "age", "isImportant"},
					},
				},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:              "Entity",
								SelectionSet:          "id",
								DisableEntityResolver: true,
							},
							{
								TypeName:     "Entity",
								SelectionSet: "name",
							},
							{
								TypeName:              "Entity",
								SelectionSet:          "uuid",
								DisableEntityResolver: true,
							},
						},
					},
				},
				mustCustomConfiguration(t,
					ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://third.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: thirdSubgraphSDL,
							},
							thirdSubgraphSDL,
						),
					},
				),
			)

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
					thirdDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug:                        plan.DebugConfiguration{},
			}

			t.Run("query", func(t *testing.T) {
				entityOneNestedFetch2Second := func(fetchID int) resolve.Fetch {
					var entitySelectionSet string
					if fetchID == 1 {
						entitySelectionSet = "age rating"
					} else {
						entitySelectionSet = "rating"
					}

					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{0},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {` + entitySelectionSet + `}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("uuid"),
												Value: &resolve.String{
													Path: []string{"uuid"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}
				entityOneNestedFetch2Third := func(fetchID int) resolve.Fetch {
					var entitySelectionSet string
					if fetchID == 2 {
						entitySelectionSet = "isImportant"
					} else {
						entitySelectionSet = "age isImportant"
					}

					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{0},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {` + entitySelectionSet + `}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path: []string{"name"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}
				entityTwoNestedFetch2First := func(fetchID int) resolve.Fetch {
					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{3},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {isEntity}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.String{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}
				entityTwoNestedFetch2Third := func(fetchID int) resolve.Fetch {
					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{3},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {isImportant}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path: []string{"name"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}
				entityThreeNestedFetch2First := func(fetchID int) resolve.Fetch {
					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{6},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {isEntity}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.String{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}
				entityThreeNestedFetch2Second := func(fetchID int) resolve.Fetch {
					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{6},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Entity {rating}}}","variables":{"representations":[$$0$$]}}}`,
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
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
											{
												Name: []byte("uuid"),
												Value: &resolve.String{
													Path: []string{"uuid"},
												},
												OnTypeNames: [][]byte{[]byte("Entity")},
											},
										},
									}),
								},
							},
							PostProcessing: SingleEntityPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}
				}

				expectedPlan := func(
					entityOneFetchOne resolve.Fetch,
					entityOneFetchTwo resolve.Fetch,
					entityTwoFetchOne resolve.Fetch,
					entityTwoFetchTwo resolve.Fetch,
					entityThreeFetchOne resolve.Fetch,
					entityThreeFetchTwo resolve.Fetch,
				) plan.Plan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 0,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entityOne {id name isEntity __typename uuid}}"}}`,
												PostProcessing: DefaultPostProcessingConfiguration,
												DataSource:     &Source{},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 3,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{entityTwo {id name age rating __typename}}"}}`,
												PostProcessing: DefaultPostProcessingConfiguration,
												DataSource:     &Source{},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 6,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://third.service","body":{"query":"{entityThree {id name age isImportant __typename uuid}}"}}`,
												PostProcessing: DefaultPostProcessingConfiguration,
												DataSource:     &Source{},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("entityOne"),
										Value: &resolve.Object{
											Path:     []string{"entityOne"},
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
													Name: []byte("age"),
													Value: &resolve.String{
														Path: []string{"age"},
													},
												},
												{
													Name: []byte("isEntity"),
													Value: &resolve.Boolean{
														Path: []string{"isEntity"},
													},
												},
												{
													Name: []byte("isImportant"),
													Value: &resolve.Boolean{
														Path: []string{"isImportant"},
													},
												},
												{
													Name: []byte("rating"),
													Value: &resolve.Float{
														Path: []string{"rating"},
													},
												},
											},
											Fetch: &resolve.ParallelFetch{
												Fetches: []resolve.Fetch{
													entityOneFetchOne,
													entityOneFetchTwo,
												},
											},
										},
									},
									{
										Name: []byte("entityTwo"),
										Value: &resolve.Object{
											Path:     []string{"entityTwo"},
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
													Name: []byte("age"),
													Value: &resolve.String{
														Path: []string{"age"},
													},
												},
												{
													Name: []byte("isEntity"),
													Value: &resolve.Boolean{
														Path: []string{"isEntity"},
													},
												},
												{
													Name: []byte("isImportant"),
													Value: &resolve.Boolean{
														Path: []string{"isImportant"},
													},
												},
												{
													Name: []byte("rating"),
													Value: &resolve.Float{
														Path: []string{"rating"},
													},
												},
											},
											Fetch: &resolve.ParallelFetch{
												Fetches: []resolve.Fetch{
													entityTwoFetchOne,
													entityTwoFetchTwo,
												},
											},
										},
									},
									{
										Name: []byte("entityThree"),
										Value: &resolve.Object{
											Path:     []string{"entityThree"},
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
													Name: []byte("age"),
													Value: &resolve.String{
														Path: []string{"age"},
													},
												},
												{
													Name: []byte("isEntity"),
													Value: &resolve.Boolean{
														Path: []string{"isEntity"},
													},
												},
												{
													Name: []byte("isImportant"),
													Value: &resolve.Boolean{
														Path: []string{"isImportant"},
													},
												},
												{
													Name: []byte("rating"),
													Value: &resolve.Float{
														Path: []string{"rating"},
													},
												},
											},
											Fetch: &resolve.ParallelFetch{
												Fetches: []resolve.Fetch{
													entityThreeFetchOne,
													entityThreeFetchTwo,
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
					entityOneNestedFetch2Second(1), entityOneNestedFetch2Third(2),
					entityTwoNestedFetch2First(4), entityTwoNestedFetch2Third(5),
					entityThreeNestedFetch2First(7), entityThreeNestedFetch2Second(8),
				)

				variant2 := expectedPlan(
					entityOneNestedFetch2Third(1), entityOneNestedFetch2Second(2),
					entityTwoNestedFetch2First(4), entityTwoNestedFetch2Third(5),
					entityThreeNestedFetch2First(7), entityThreeNestedFetch2Second(8),
				)

				expectedPlans := []plan.Plan{
					variant1,
					variant2,
					variant1,
					variant1,
					variant2,
					variant2,
				}

				RunWithPermutationsVariants(
					t,
					definition,
					`
						query Query {
							entityOne {
								id
								name
								age
								isEntity
								isImportant
								rating
							}
							entityTwo {
								id
								name
								age
								isEntity
								isImportant
								rating
							}
							entityThree {
								id
								name
								age
								isEntity
								isImportant
								rating
							}
						}
					`,
					"Query",
					expectedPlans,
					planConfiguration,
					WithMultiFetchPostProcessor(),
				)
			})
		})
	})

	t.Run("fragments on a root query type", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			def := `
			schema {
				query: Query
			}
		
			type Query {
				a: String!
				b: String!
			}`

			op := `
			fragment A on Query {
				a
			}
			fragment B on Query {
				b
			}
			query conditions($skipA: Boolean!, $includeB: Boolean!) {
				...A @skip(if: $skipA)
				...B @include(if: $includeB)
			}
		`

			t.Run("same datasource", func(t *testing.T) {
				t.Run("run", RunTest(
					def, op,
					"conditions", &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
										Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"query($skipA: Boolean!, $includeB: Boolean!){__typename ... on Query @skip(if: $skipA) {a} ... on Query @include(if: $includeB){b}}","variables":{"includeB":$$1$$,"skipA":$$0$$}}}`,
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"skipA"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"includeB"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
										),
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("a"),
										Value: &resolve.String{
											Path: []string{"a"},
										},
										SkipDirectiveDefined: true,
										SkipVariableName:     "skipA",
										OnTypeNames:          [][]byte{[]byte("Query")},
									},
									{
										Name: []byte("b"),
										Value: &resolve.String{
											Path: []string{"b"},
										},
										IncludeDirectiveDefined: true,
										IncludeVariableName:     "includeB",
										OnTypeNames:             [][]byte{[]byte("Query")},
									},
								},
							},
						},
					}, plan.Configuration{
						DataSources: []plan.DataSource{
							mustDataSourceConfiguration(
								t,
								"ds-id",
								&plan.DataSourceMetadata{
									RootNodes: []plan.TypeField{
										{
											TypeName:   "Query",
											FieldNames: []string{"a", "b"},
										},
									},
								},
								mustCustomConfiguration(t, ConfigurationInput{
									Fetch: &FetchConfiguration{
										URL: "https://example.com/graphql",
									},
									SchemaConfiguration: mustSchema(t, nil, def),
								}),
							),
						},
						DisableResolveFieldPositions: true,
					}))
			})

			t.Run("different datasource", func(t *testing.T) {
				t.Run("run", RunTest(
					def, op,
					"conditions", &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 0,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												DataSource:     &Source{},
												PostProcessing: DefaultPostProcessingConfiguration,
												Input:          `{"method":"POST","url":"https://example-1.com/graphql","body":{"query":"query($skipA: Boolean!){__typename ... on Query @skip(if: $skipA){a}}","variables":{"skipA":$$0$$}}}`,
												Variables: resolve.NewVariables(
													&resolve.ContextVariable{
														Path:     []string{"skipA"},
														Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
													},
												),
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 1,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												DataSource:     &Source{},
												PostProcessing: DefaultPostProcessingConfiguration,
												Input:          `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($includeB: Boolean!){__typename ... on Query @include(if: $includeB){b}}","variables":{"includeB":$$0$$}}}`,
												Variables: resolve.NewVariables(
													&resolve.ContextVariable{
														Path:     []string{"includeB"},
														Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
													},
												),
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										},
									},
									Trace: nil,
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("a"),
										Value: &resolve.String{
											Path: []string{"a"},
										},
										SkipDirectiveDefined: true,
										SkipVariableName:     "skipA",
										OnTypeNames:          [][]byte{[]byte("Query")},
									},
									{
										Name: []byte("b"),
										Value: &resolve.String{
											Path: []string{"b"},
										},
										IncludeDirectiveDefined: true,
										IncludeVariableName:     "includeB",
										OnTypeNames:             [][]byte{[]byte("Query")},
									},
								},
							},
						},
					}, plan.Configuration{
						DataSources: []plan.DataSource{
							mustDataSourceConfiguration(
								t,
								"ds-id-1",
								&plan.DataSourceMetadata{
									RootNodes: []plan.TypeField{
										{
											TypeName:   "Query",
											FieldNames: []string{"a"},
										},
									},
								},
								mustCustomConfiguration(t, ConfigurationInput{
									Fetch: &FetchConfiguration{
										URL: "https://example-1.com/graphql",
									},
									SchemaConfiguration: mustSchema(t, nil, def),
								}),
							),
							mustDataSourceConfiguration(
								t,
								"ds-id-2",
								&plan.DataSourceMetadata{
									RootNodes: []plan.TypeField{
										{
											TypeName:   "Query",
											FieldNames: []string{"b"},
										},
									},
								},
								mustCustomConfiguration(t, ConfigurationInput{
									Fetch: &FetchConfiguration{
										URL: "https://example-2.com/graphql",
									},
									SchemaConfiguration: mustSchema(t, nil, def),
								}),
							),
						},
						DisableResolveFieldPositions: true,
					}, WithMultiFetchPostProcessor()))
			})
		})

		t.Run("with entities requests", func(t *testing.T) {
			def := `
				schema {
					query: Query
				}
			
				type Query {
					currentUser: User!
				}
	
				type User {
					id: ID!
					a: String!
					b: String!
				}`

			firstSubgraphSDL := `
				type Query {
					currentUser: User!
				}
	
				type User @key(fields: "id") {
					id: ID!
				}`

			secondSubgraphSDL := `	
				type User @key(fields: "id") {
					id: ID!
					a: String!
					b: String!
				}`

			op := `
				fragment A on Query {
					currentUser {
						a
					}
				}
				fragment B on Query {
					currentUser {
						b
					}
				}
				query conditions($skipA: Boolean!, $includeB: Boolean!) {
					...A @skip(if: $skipA)
					...B @include(if: $includeB)
				}`

			t.Run("2 datasources", func(t *testing.T) {
				t.Run("run", RunTest(
					def, op,
					"conditions", &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Data: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
										Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"query($skipA: Boolean!, $includeB: Boolean!){__typename ... on Query @skip(if: $skipA) {currentUser {__typename id}} ... on Query @include(if: $includeB){currentUser {__typename id}}}","variables":{"includeB":$$1$$,"skipA":$$0$$}}}`,
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"skipA"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"includeB"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
										),
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("currentUser"),
										Value: &resolve.Object{
											Path:     []string{"currentUser"},
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("a"),
													Value: &resolve.String{
														Path: []string{"a"},
													},
													SkipDirectiveDefined: true,
													SkipVariableName:     "skipA",
												},
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													DataSource:                            &Source{},
													RequiresEntityFetch:                   true,
													SetTemplateOutputToNullOnVariableNull: true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													Input:                                 `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {a}}}","variables":{"representations":[$$0$$]}}}`,
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
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																},
															}),
														},
													),
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											},
										},
										OnTypeNames:          [][]byte{[]byte("Query")},
										SkipDirectiveDefined: true,
										SkipVariableName:     "skipA",
									},
									{
										Name: []byte("currentUser"),
										Value: &resolve.Object{
											Path:     []string{"currentUser"},
											Nullable: false,
											Fields: []*resolve.Field{
												{
													Name: []byte("b"),
													Value: &resolve.String{
														Path: []string{"b"},
													},
													IncludeDirectiveDefined: true,
													IncludeVariableName:     "includeB",
												},
											},
											Fetch: &resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													DataSource:                            &Source{},
													RequiresEntityFetch:                   true,
													SetTemplateOutputToNullOnVariableNull: true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													Input:                                 `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {b}}}","variables":{"representations":[$$0$$]}}}`,
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
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User")},
																	},
																},
															}),
														},
													),
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											},
										},
										OnTypeNames:             [][]byte{[]byte("Query")},
										IncludeDirectiveDefined: true,
										IncludeVariableName:     "includeB",
									},
								},
							},
						},
					}, plan.Configuration{
						DataSources: []plan.DataSource{
							mustDataSourceConfiguration(
								t,
								"ds-id-1",
								&plan.DataSourceMetadata{
									RootNodes: []plan.TypeField{
										{
											TypeName:   "Query",
											FieldNames: []string{"currentUser"},
										},
										{
											TypeName:   "User",
											FieldNames: []string{"id"},
										},
									},
									FederationMetaData: plan.FederationMetaData{
										Keys: plan.FederationFieldConfigurations{
											{
												TypeName:     "User",
												SelectionSet: "id",
											},
										},
									},
								},
								mustCustomConfiguration(t, ConfigurationInput{
									Fetch: &FetchConfiguration{
										URL: "https://example.com/graphql",
									},
									SchemaConfiguration: mustSchema(t,
										&FederationConfiguration{
											Enabled:    true,
											ServiceSDL: firstSubgraphSDL,
										},
										firstSubgraphSDL,
									),
								}),
							),
							mustDataSourceConfiguration(
								t,
								"ds-id-2",
								&plan.DataSourceMetadata{
									RootNodes: []plan.TypeField{
										{
											TypeName:   "User",
											FieldNames: []string{"id", "a", "b"},
										},
									},
									FederationMetaData: plan.FederationMetaData{
										Keys: plan.FederationFieldConfigurations{
											{
												TypeName:     "User",
												SelectionSet: "id",
											},
										},
									},
								},
								mustCustomConfiguration(t, ConfigurationInput{
									Fetch: &FetchConfiguration{
										URL: "https://example-2.com/graphql",
									},
									SchemaConfiguration: mustSchema(t,
										&FederationConfiguration{
											Enabled:    true,
											ServiceSDL: secondSubgraphSDL,
										},
										secondSubgraphSDL,
									),
								}),
							),
						},
						DisableResolveFieldPositions: true,
					}))
			})
		})
	})
}
