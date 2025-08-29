package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
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
				},
			}

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
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImageWithProvides {image {__typename id}}}}}","variables":{"representations":[$$0$$]}}}`,
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
										DependsOnFetchIDs: []int{1},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Image {__typename cdnUrl}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("Image")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Image")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImageWithProvides.image", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImageWithProvides"), resolve.ObjectPath("image")),
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
													Name: []byte("hostedImageWithProvides"),
													Value: &resolve.Object{
														Path:     []string{"hostedImageWithProvides"},
														Nullable: false,
														PossibleTypes: map[string]struct{}{
															"HostedImage": {},
														},
														TypeName: "HostedImage",
														Fields: []*resolve.Field{
															{
																Name: []byte("image"),
																Value: &resolve.Object{
																	Path: []string{"image"},
																	PossibleTypes: map[string]struct{}{
																		"Image": {},
																	},
																	TypeName: "Image",
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("cdnUrl"),
																			Value: &resolve.String{
																				Path: []string{"cdnUrl"},
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
					WithDefer(),
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
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename id}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImageWithProvides {image {__typename id}}}}}","variables":{"representations":[$$0$$]}}}`,
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
										DependsOnFetchIDs: []int{1},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Image {__typename cdnUrl}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("Image")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Image")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImageWithProvides.image", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImageWithProvides"), resolve.ObjectPath("image")),
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
													Name: []byte("hostedImageWithProvides"),
													Value: &resolve.Object{
														Path:     []string{"hostedImageWithProvides"},
														Nullable: false,
														PossibleTypes: map[string]struct{}{
															"HostedImage": {},
														},
														TypeName: "HostedImage",
														Fields: []*resolve.Field{
															{
																Name: []byte("image"),
																Value: &resolve.Object{
																	Path: []string{"image"},
																	PossibleTypes: map[string]struct{}{
																		"Image": {},
																	},
																	TypeName: "Image",
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("cdnUrl"),
																			Value: &resolve.String{
																				Path: []string{"cdnUrl"},
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
					WithDefer(),
				)
			})
		})
	})
}
