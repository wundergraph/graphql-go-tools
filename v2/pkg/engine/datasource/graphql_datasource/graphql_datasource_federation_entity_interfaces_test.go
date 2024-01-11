package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederationEntityInterfaces(t *testing.T) {
	federationFactory := &Factory{}

	definition := EntityInterfacesDefinition
	planConfiguration := *EntityInterfacesPlanConfiguration(federationFactory)

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
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
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
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.ParallelFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchID:           1,
													DependsOnFetchIDs: []int{0},
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
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
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
													FetchID:           3,
													DependsOnFetchIDs: []int{0},
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

	t.Run("query 10 - InterfaceObjectToConcreteTypeWithExternalField - no fragments", func(t *testing.T) {
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
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
													FetchID:           1,
													DependsOnFetchIDs: []int{0, 2},
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

	t.Run("query 10.1 - InterfaceObjectToConcreteTypeWithExternalField - no fragments, with typename", func(t *testing.T) {
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("title"),
												Value: &resolve.String{
													Path: []string{"title"},
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
												},
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.SerialFetch{
											Fetches: []resolve.Fetch{
												&resolve.SingleFetch{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
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
													FetchID:           1,
													DependsOnFetchIDs: []int{0, 2},
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.SingleFetch{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
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

	t.Run("query 14 Interface object to Interface object with typename", func(t *testing.T) {
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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account"), []byte("Moderator"), []byte("User")},
											},
											{
												Name: []byte("__typename"),
												Value: &resolve.String{
													Path:       []string{"__typename"},
													IsTypeName: true,
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
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
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

}
