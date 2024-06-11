package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederationEntityInterfaces(t *testing.T) {
	federationFactory := &Factory[Configuration]{}

	definition := EntityInterfacesDefinition
	planConfiguration := *EntityInterfacesPlanConfiguration(t, federationFactory)

	t.Run("query 0 - Interface object typename", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _0_InterfaceObjectTypename {
					accountLocations {
						id
						__typename
					}
				}`,
			"_0_InterfaceObjectTypename",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
															},
														}),
													},
												},
												RequiresEntityBatchFetch:              true,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
			planConfiguration,
			WithMultiFetchPostProcessor(),
		))
	})

	t.Run("query 1 - Interface to interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _1_InterfaceToInterfaceObject {
					allAccountsInterface {
						id
						locations {
							country
						}
					}
				}`,
			"_1_InterfaceToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
												Variables: []resolve.Variable{
													&resolve.ResolvableObjectVariable{
														Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
															},
														}),
													},
												},
												RequiresEntityBatchFetch:              true,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
			planConfiguration,
		))

	})

	t.Run("query 2 - Interface to interface objects", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _2_InterfaceToInterfaceObjects {
					allAccountsInterface {
						id
						locations {
							country
						}
						age
					}
				}`,
			"_2_InterfaceToInterfaceObjects",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))

	})

	t.Run("query 3 - Interface object to concrete type User", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _3_InterfaceObjectToConcreteType {
					accountLocations {
						id
						... on User {
							title
						}
					}
				}`,
			"_3_InterfaceObjectToConcreteType",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
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
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title __typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
															},
														}),
													},
												},
												RequiresEntityBatchFetch:              true,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
			planConfiguration,
		))

	})

	t.Run("query 4 - Concrete type User to interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _4_ConcreteType_User_ToInterfaceObject {
					user(id: "u1") {
						id
						age
					}
				}`,
			"_4_ConcreteType_User_ToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){user(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
									},
								},
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
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
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
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: []resolve.Variable{
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.StaticString{
																	Path:  []string{"__typename"},
																	Value: "Account",
																},
																OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
															},
														},
													}),
												},
											},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											DataSource:                            &Source{},
											SetTemplateOutputToNullOnVariableNull: true,
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
			WithMultiFetchPostProcessor(),
		))

	})

	t.Run("query 5 - Concrete type User to interface objects", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _5_ConcreteType_User_ToInterfaceObjects {
					user(id: "u1") {
						id
						age
						locations {
							country
						}
					}
				}`,
			"_5_ConcreteType_User_ToInterfaceObjects",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){user(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
									},
								},
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
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("age"),
											Value: &resolve.Integer{
												Path: []string{"age"},
											},
										},
										{
											Name: []byte("locations"),
											Value: &resolve.Array{
												Path:     []string{"locations"},
												Nullable: true,
												Item: &resolve.Object{
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
									},
									Fetch: &resolve.ParallelFetch{
										Fetches: []resolve.Fetch{
											&resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												},
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: []resolve.Variable{
														&resolve.ResolvableObjectVariable{
															Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("__typename"),
																		Value: &resolve.StaticString{
																			Path:  []string{"__typename"},
																			Value: "Account",
																		},
																		OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																	},
																},
															}),
														},
													},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													DataSource:                            &Source{},
													SetTemplateOutputToNullOnVariableNull: true,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											},
											&resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
												},
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: []resolve.Variable{
														&resolve.ResolvableObjectVariable{
															Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("__typename"),
																		Value: &resolve.StaticString{
																			Path:  []string{"__typename"},
																			Value: "Account",
																		},
																		OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																	},
																},
															}),
														},
													},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													DataSource:                            &Source{},
													SetTemplateOutputToNullOnVariableNull: true,
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
		))

	})

	t.Run("query 6 - Concrete type Admin to interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _6_ConcreteType_Admin_ToInterfaceObject {
					admin(id: "a1") {
						id
						age
					}
				}`,
			"_6_ConcreteType_Admin_ToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){admin(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
									},
								},
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("admin"),
								Value: &resolve.Object{
									Path:     []string{"admin"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
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
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: []resolve.Variable{
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.StaticString{
																	Path:  []string{"__typename"},
																	Value: "Account",
																},
																OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
															},
														},
													}),
												},
											},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											DataSource:                            &Source{},
											SetTemplateOutputToNullOnVariableNull: true,
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
			WithMultiFetchPostProcessor(),
		))

	})

	t.Run("query 7 - Concrete type Admin to interface objects", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _7_ConcreteType_Admin_ToInterfaceObjects {
					admin(id: "a1") {
						id
						age
						locations {
							country
						}
					}
				}`,
			"_7_ConcreteType_Admin_ToInterfaceObjects",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){admin(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
									},
								},
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("admin"),
								Value: &resolve.Object{
									Path:     []string{"admin"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("age"),
											Value: &resolve.Integer{
												Path: []string{"age"},
											},
										},
										{
											Name: []byte("locations"),
											Value: &resolve.Array{
												Path:     []string{"locations"},
												Nullable: true,
												Item: &resolve.Object{
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
									},
									Fetch: &resolve.ParallelFetch{
										Fetches: []resolve.Fetch{
											&resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
												},
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: []resolve.Variable{
														&resolve.ResolvableObjectVariable{
															Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("__typename"),
																		Value: &resolve.StaticString{
																			Path:  []string{"__typename"},
																			Value: "Account",
																		},
																		OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																	},
																},
															}),
														},
													},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													DataSource:                            &Source{},
													SetTemplateOutputToNullOnVariableNull: true,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											},
											&resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
												},
												FetchConfiguration: resolve.FetchConfiguration{
													Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
													Variables: []resolve.Variable{
														&resolve.ResolvableObjectVariable{
															Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("__typename"),
																		Value: &resolve.StaticString{
																			Path:  []string{"__typename"},
																			Value: "Account",
																		},
																		OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.String{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																	},
																},
															}),
														},
													},
													RequiresEntityFetch:                   true,
													PostProcessing:                        SingleEntityPostProcessingConfiguration,
													DataSource:                            &Source{},
													SetTemplateOutputToNullOnVariableNull: true,
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
		))

	})

	t.Run("query 8 - Interface object to concrete types User and Admin with external field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _8_InterfaceObjectToConcreteTypeWithExternalField {
					accountLocations {
						id
						... on User {
							title
						}
						... on Admin {
							title
						}
					}
				}`,
			"_8_InterfaceObjectToConcreteTypeWithExternalField",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
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
														Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title __typename} ... on Admin {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{1, 0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))

	})

	t.Run("query 9 - Interface fragment on union to interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _9_InterfaceFragmentOnUnionToInterfaceObject {
					allAccountsUnion {
						... on Account {
							id
							title
							locations {
								country
							}
						}
					}
				}`,
			"_9_InterfaceFragmentOnUnionToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsUnion {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsUnion"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsUnion"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 9.1 - Interface fragment on union to interface objects", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _9_1_InterfaceFragmentOnUnionToInterfaceObjects {
					allAccountsUnion {
						... on Account {
							id
							title
							locations {
								country
							}
							age
						}
					}
				}`,
			"_9_1_InterfaceFragmentOnUnionToInterfaceObjects",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsUnion {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsUnion"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsUnion"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           3,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 10 - Interface object to concrete type with external field - no fragments", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _10_InterfaceObjectToConcreteTypeWithExternalField_No_Fragments {
					accountLocations {
						id
						title
					}
				}`,
			"_10_InterfaceObjectToConcreteTypeWithExternalField_No_Fragments",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")}},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {__typename} ... on Moderator {title __typename} ... on User {title __typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0, 2},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 10.1 - Interface object to concrete type with external field - no fragments, with typename", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _10_InterfaceObjectToConcreteTypeWithExternalField_No_Fragments {
					accountLocations {
						id
						title
						__typename
					}
				}`,
			"_10_InterfaceObjectToConcreteTypeWithExternalField_No_Fragments",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {__typename} ... on Moderator {title __typename} ... on User {title __typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{2, 0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 11 - Interface object to interface object with external field - no fragments", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _11_InterfaceObjectToInterfaceObjectWithExternalField_No_Fragments {
					accountLocations {
						id
						title
						age
					}
				}`,
			"_11_InterfaceObjectToInterfaceObjectWithExternalField_No_Fragments",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.ParallelFetch{
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           2,
																DependsOnFetchIDs: []int{0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
																Variables: []resolve.Variable{
																	&resolve.ResolvableObjectVariable{
																		Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																			Nullable: true,
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("__typename"),
																					Value: &resolve.StaticString{
																						Path:  []string{"__typename"},
																						Value: "Account",
																					},
																					OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																				},
																				{
																					Name: []byte("__typename"),
																					Value: &resolve.StaticString{
																						Path:  []string{"__typename"},
																						Value: "Account",
																					},
																					OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																				},
																				{
																					Name: []byte("__typename"),
																					Value: &resolve.StaticString{
																						Path:  []string{"__typename"},
																						Value: "Account",
																					},
																					OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																				},
																			},
																		}),
																	},
																},
																RequiresEntityBatchFetch:              true,
																PostProcessing:                        EntitiesPostProcessingConfiguration,
																DataSource:                            &Source{},
																SetTemplateOutputToNullOnVariableNull: true,
															},
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
														},
														&resolve.SingleFetch{
															FetchDependencies: resolve.FetchDependencies{
																FetchID:           3,
																DependsOnFetchIDs: []int{0},
															},
															FetchConfiguration: resolve.FetchConfiguration{
																Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {__typename} ... on Moderator {title __typename} ... on User {title __typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																					OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																				},
																				{
																					Name: []byte("__typename"),
																					Value: &resolve.String{
																						Path: []string{"__typename"},
																					},
																					OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																				},
																				{
																					Name: []byte("__typename"),
																					Value: &resolve.String{
																						Path: []string{"__typename"},
																					},
																					OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																				},
																				{
																					Name: []byte("id"),
																					Value: &resolve.String{
																						Path: []string{"id"},
																					},
																					OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																				},
																			},
																		}),
																	},
																},
																RequiresEntityBatchFetch:              true,
																PostProcessing:                        EntitiesPostProcessingConfiguration,
																DataSource:                            &Source{},
																SetTemplateOutputToNullOnVariableNull: true,
															},
															DataSourceIdentifier: []byte("graphql_datasource.Source"),
														},
													},
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0, 3},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 12 - Interface fragment on interface to interface objects", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _12_InterfaceToInterfaceObjects_InterfaceFragment {
					allAccountsInterface {
						... on Account {
							id
							locations {
								country
							}
							age
						}
					}
				}`,
			"_12_InterfaceToInterfaceObjects_InterfaceFragment",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 13 - Interface fragment on interface to interface objects with external field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _13_InterfaceToInterfaceObjects_InterfaceFragment_ExternalField {
					allAccountsInterface {
						... on Account {
							id
							title
							locations {
								country
							}
							age
						}
					}
				}`,
			"_13_InterfaceToInterfaceObjects_InterfaceFragment_ExternalField",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {title}}}","variables":{"representations":[$$0$$]}}}`,
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
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {locations {country}}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           3,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 14 Interface object to Interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _14_InterfaceObjectToInterfaceObject {
					accountLocations {
						age
					}
				}`,
			"_14_InterfaceObjectToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
												Variables: []resolve.Variable{
													&resolve.ResolvableObjectVariable{
														Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
															Nullable: true,
															Fields: []*resolve.Field{
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
															},
														}),
													},
												},
												RequiresEntityBatchFetch:              true,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
			planConfiguration,
			WithMultiFetchPostProcessor(),
		))
	})

	t.Run("query 14.1 Interface object to Interface object with typename", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _14_InterfaceObjectToInterfaceObject {
					accountLocations {
						age
						__typename
					}
				}`,
			"_14_InterfaceObjectToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
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
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.StaticString{
																				Path:  []string{"__typename"},
																				Value: "Account",
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												&resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           2,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Admin {__typename} ... on Moderator {__typename} ... on User {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																		},
																	},
																}),
															},
														},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														DataSource:                            &Source{},
														SetTemplateOutputToNullOnVariableNull: true,
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
		))
	})

	t.Run("query 15 - Interface object to concrete type User", func(t *testing.T) {
		// Note: we overfetch here, as we do not know the type of the object, so we fetch all possible fields from first subgraph,
		// but later we render only fields on a User type
		// Alternative is to fetch id, fetch type and after that fetch remaining fields only for user

		t.Run("run", RunTest(
			definition,
			`
				query _15_InterfaceObjectToConcreteTypeWithFieldFromInterfaceObject {
					accountLocations {
						... on User {
							title
							locations {
								country
							}
						}
					}
				}`,
			"_15_InterfaceObjectToConcreteTypeWithFieldFromInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename locations {country} id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
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
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           1,
												DependsOnFetchIDs: []int{0},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {title __typename}}}","variables":{"representations":[$$0$$]}}}`,
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
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
																},
															},
														}),
													},
												},
												RequiresEntityBatchFetch:              true,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												DataSource:                            &Source{},
												SetTemplateOutputToNullOnVariableNull: true,
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
			planConfiguration,
		))

	})

}
