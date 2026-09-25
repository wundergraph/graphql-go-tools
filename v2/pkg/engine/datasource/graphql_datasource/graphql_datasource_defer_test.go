package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceDefer(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		t.Run("on root query node", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					name: String!
					title: String!
				}

				type Query {
					user: User!
				}
			`

			firstSubgraphSDL := `	
				type User {
					id: ID!
					name: String!
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
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "title", "name"},
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug: plan.DebugConfiguration{
					PrintQueryPlans:    true,
					PrintPlanningPaths: true,

					PlanningVisitor: true,
				},
			}

			t.Run("defer User.title - defer postprocess disabled", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								name
								... @defer {
									title
								}
							}
						}`,
					"User",
					&plan.DeferResponsePlan{
						Response: &resolve.GraphQLDeferResponse{
							DeferDescriptors: map[int]resolve.DeferDescriptor{
								1: {
									ID:       1,
									ParentID: 0,
									Label:    "",
									Path:     []string{"user"},
								},
							},
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
											DeferID: 1,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {title}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 1,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {name}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("title"),
														Defer: &resolve.DeferField{
															DeferID: 1,
														},
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
					WithPostProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableCreateParallelNodes(), postprocess.DisableMergeFields(), postprocess.DisableExtractDeferFetches()),
					WithDefer(),
					WithCalculateFieldDependencies(),
				)
			})

			t.Run("defer User.title", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								name
								... @defer {
									title
								}
							}
						}`,
					"User",
					&plan.DeferResponsePlan{
						Response: &resolve.GraphQLDeferResponse{
							DeferDescriptors: map[int]resolve.DeferDescriptor{
								1: {
									ID:       1,
									ParentID: 0,
									Label:    "",
									Path:     []string{"user"},
								},
							},
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 1,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {name}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("title"),
														Defer: &resolve.DeferField{
															DeferID: 1,
														},
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
							DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
								DeferID: 1,
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
											DeferID: 1,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {title}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
								),
							}),
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
					WithDefer(),
					WithCalculateFieldDependencies(),
				)
			})
		})

		t.Run("nested defer on single subgraph", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					name: String!
					title: String!
					description: String!
				}

				type Query {
					user: User!
				}
			`

			firstSubgraphSDL := `
				type User {
					id: ID!
					name: String!
					title: String!
					description: String!
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
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "name", "title", "description"},
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

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
			}

			t.Run("nested defer User.title and User.description", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								name
								... @defer {
									title
									... @defer {
										description
									}
								}
							}
						}`,
					"User",
					&plan.DeferResponsePlan{
						Response: &resolve.GraphQLDeferResponse{
							DeferDescriptors: map[int]resolve.DeferDescriptor{
								1: {
									ID:       1,
									ParentID: 0,
									Label:    "",
									Path:     []string{"user"},
								},
								2: {
									ID:       2,
									ParentID: 1,
									Label:    "",
									Path:     []string{"user"},
								},
							},
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 2,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {name}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("title"),
														Defer: &resolve.DeferField{
															DeferID: 1,
														},
														Value: &resolve.String{
															Path: []string{"title"},
														},
													},
													{
														Name: []byte("description"),
														Defer: &resolve.DeferField{
															DeferID: 2,
														},
														Value: &resolve.String{
															Path: []string{"description"},
														},
													},
												},
											},
										},
									},
								},
							},
							DeferTree: resolve.DeferSequence(
								resolve.DeferSingle(&resolve.DeferFetchGroup{
									DeferID: 1,
									Fetches: resolve.Sequence(
										resolve.Single(&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 0,
												DeferID: 1,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {title}}"}}`,
												PostProcessing: DefaultPostProcessingConfiguration,
												DataSource:     &Source{},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}),
									),
								}),
								resolve.DeferSingle(&resolve.DeferFetchGroup{
									DeferID: 2,
									Fetches: resolve.Sequence(
										resolve.Single(&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 1,
												DeferID: 2,
											},
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {description}}"}}`,
												PostProcessing: DefaultPostProcessingConfiguration,
												DataSource:     &Source{},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}),
									),
								}),
							),
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
					WithDefer(),
					WithCalculateFieldDependencies(),
				)
			})
		})

		t.Run("on entity from other subgraph", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					title: String!
					firstName: String!
					lastName: String!
				}

				type Query {
					user: User!
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
					PrintQueryPlans:    true,
					PrintPlanningPaths: true,
				},
			}

			t.Run("defer User.lastName. defer postprocess disabled", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								title
								firstName
								... @defer {
									lastName
								}
							}
						}`,
					"User",
					&plan.DeferResponsePlan{
						Response: &resolve.GraphQLDeferResponse{
							DeferDescriptors: map[int]resolve.DeferDescriptor{
								1: {
									ID:       1,
									ParentID: 0,
									Label:    "",
									Path:     []string{"user"},
								},
							},
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {title __typename id}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										}, FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName}}}","variables":{"representations":[$$0$$]}}}`,
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
																Value: &resolve.Scalar{
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
									}, "user", resolve.ObjectPath("user")),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{0},
											DeferID:           1,
										}, FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
																Value: &resolve.Scalar{
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
									}, "user", resolve.ObjectPath("user")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
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
														Defer: &resolve.DeferField{
															DeferID: 1,
														},
														Value: &resolve.String{
															Path: []string{"lastName"},
														},
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
					WithPostProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableCreateParallelNodes(), postprocess.DisableMergeFields(), postprocess.DisableExtractDeferFetches()),
					WithDefer(),
					WithCalculateFieldDependencies(),
				)
			})

			t.Run("defer User.lastName", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								title
								firstName
								... @defer {
									lastName
								}
							}
						}`,
					"User",
					&plan.DeferResponsePlan{
						Response: &resolve.GraphQLDeferResponse{
							DeferDescriptors: map[int]resolve.DeferDescriptor{
								1: {
									ID:       1,
									ParentID: 0,
									Label:    "",
									Path:     []string{"user"},
								},
							},
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {title __typename id}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										}, FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName}}}","variables":{"representations":[$$0$$]}}}`,
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
																Value: &resolve.Scalar{
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
									}, "user", resolve.ObjectPath("user")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
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
														Defer: &resolve.DeferField{
															DeferID: 1,
														},
														Value: &resolve.String{
															Path: []string{"lastName"},
														},
													},
												},
											},
										},
									},
								},
							},
							DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
								DeferID: 1,
								Fetches: resolve.Sequence(
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{0},
											DeferID:           1,
										}, FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
																Value: &resolve.Scalar{
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
									}, "user", resolve.ObjectPath("user")),
								),
							}),
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
					WithDefer(),
					WithCalculateFieldDependencies(),
				)
			})
		})
	})
}
