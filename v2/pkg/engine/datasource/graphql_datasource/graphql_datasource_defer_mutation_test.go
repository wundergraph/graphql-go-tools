package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceDeferMutation(t *testing.T) {
	definition := `
		type Thing {
			id: ID!
			slow: String!
			owner: User!
		}

		type User {
			id: ID!
			title: String!
			lastName: String!
		}

		type Query {
			thing: Thing!
		}

		type Mutation {
			createThing: Thing!
			createUser: User!
		}
	`

	mutationSubgraphSDL := `
		type Thing {
			id: ID!
			slow: String!
			owner: User!
		}

		type User @key(fields: "id") {
			id: ID!
			title: String!
		}

		type Query {
			thing: Thing!
		}

		type Mutation {
			createThing: Thing!
			createUser: User!
		}
	`

	usersSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String!
		}
	`

	mutationDataSource := mustDataSourceConfiguration(
		t,
		"mutation-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"thing"}},
				{TypeName: "Mutation", FieldNames: []string{"createThing", "createUser"}},
				{TypeName: "User", FieldNames: []string{"id", "title"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Thing", FieldNames: []string{"id", "slow", "owner"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://mutation.service"},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{Enabled: true, ServiceSDL: mutationSubgraphSDL},
				mutationSubgraphSDL,
			),
		}),
	)

	usersDataSource := mustDataSourceConfiguration(
		t,
		"users-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "lastName"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://users.service"},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{Enabled: true, ServiceSDL: usersSubgraphSDL},
				usersSubgraphSDL,
			),
		}),
	)

	planConfiguration := plan.Configuration{
		DataSources:                  []plan.DataSource{mutationDataSource, usersDataSource},
		DisableResolveFieldPositions: true,
	}

	t.Run("[P1] planner moves a non-entity deferred field below a mutation root field into the initial response", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateThing {
					a: createThing {
						id
						... @defer {
							slow
						}
					}
				}`,
			"CreateThing",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id slow}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("a"),
								Value: &resolve.Object{
									Path:          []string{"a"},
									PossibleTypes: map[string]struct{}{"Thing": {}},
									TypeName:      "Thing",
									Fields: []*resolve.Field{
										{
											Name:  []byte("id"),
											Value: &resolve.Scalar{Path: []string{"id"}},
										},
										{
											Name:  []byte("slow"),
											Value: &resolve.String{Path: []string{"slow"}},
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
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P2] entity-backed defer below a mutation root field stays deferred", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						title
						... @defer {
							lastName
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {title __typename id}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("u"),
									Value: &resolve.Object{
										Path:          []string{"u"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("title"),
												Value: &resolve.String{Path: []string{"title"}},
											},
											{
												Name:  []byte("lastName"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.String{Path: []string{"lastName"}},
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
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
									DataSource:                            &Source{},
									SetTemplateOutputToNullOnVariableNull: true,
									Variables: []resolve.Variable{
										&resolve.ResolvableObjectVariable{
											Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name:        []byte("__typename"),
														Value:       &resolve.String{Path: []string{"__typename"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"id"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "u", resolve.ObjectPath("u")),
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

	t.Run("[P3] two mutation root fields drop the non-entity defer and keep the entity-backed defer", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateBoth {
					a: createThing {
						id
						... @defer {
							slow
						}
					}
					u: createUser {
						title
						... @defer {
							lastName
						}
					}
				}`,
			"CreateBoth",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						2: {ID: 2, ParentID: 0, Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id slow}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {title __typename id}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("a"),
									Value: &resolve.Object{
										Path:          []string{"a"},
										PossibleTypes: map[string]struct{}{"Thing": {}},
										TypeName:      "Thing",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("slow"),
												Value: &resolve.String{Path: []string{"slow"}},
											},
										},
									},
								},
								{
									Name: []byte("u"),
									Value: &resolve.Object{
										Path:          []string{"u"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("title"),
												Value: &resolve.String{Path: []string{"title"}},
											},
											{
												Name:  []byte("lastName"),
												Defer: &resolve.DeferField{DeferID: 2},
												Value: &resolve.String{Path: []string{"lastName"}},
											},
										},
									},
								},
							},
						},
					},
					DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
						DeferID: 2,
						Fetches: resolve.Sequence(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{1},
									DeferID:           2,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
									DataSource:                            &Source{},
									SetTemplateOutputToNullOnVariableNull: true,
									Variables: []resolve.Variable{
										&resolve.ResolvableObjectVariable{
											Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name:        []byte("__typename"),
														Value:       &resolve.String{Path: []string{"__typename"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"id"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "u", resolve.ObjectPath("u")),
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

	t.Run("[P4] declared entity key on the mutation subgraph does not keep the defer", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer {
							title
						}
					}
				}`,
			"CreateUser",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id title}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("u"),
								Value: &resolve.Object{
									Path:          []string{"u"},
									PossibleTypes: map[string]struct{}{"User": {}},
									TypeName:      "User",
									Fields: []*resolve.Field{
										{
											Name:  []byte("id"),
											Value: &resolve.Scalar{Path: []string{"id"}},
										},
										{
											Name:  []byte("title"),
											Value: &resolve.String{Path: []string{"title"}},
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
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P5] unsafe nested defer moves to the initial response and the entity-backed outer defer stays", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer(label: "outer") {
							lastName
							... @defer(label: "inner") {
								title
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Label: "outer", Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id title __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("u"),
									Value: &resolve.Object{
										Path:          []string{"u"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("lastName"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.String{Path: []string{"lastName"}},
											},
											{
												Name:  []byte("title"),
												Value: &resolve.String{Path: []string{"title"}},
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
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
									DataSource:                            &Source{},
									SetTemplateOutputToNullOnVariableNull: true,
									Variables: []resolve.Variable{
										&resolve.ResolvableObjectVariable{
											Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name:        []byte("__typename"),
														Value:       &resolve.String{Path: []string{"__typename"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"id"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "u", resolve.ObjectPath("u")),
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

	t.Run("[P6] one defer splits per field: unsafe fields go to the initial response, the entity-backed field stays deferred", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateThing {
					a: createThing {
						id
						... @defer {
							owner {
								id
								title
								lastName
							}
						}
					}
				}`,
			"CreateThing",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"a"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id owner {id title __typename __internal_id: id}}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("a"),
									Value: &resolve.Object{
										Path:          []string{"a"},
										PossibleTypes: map[string]struct{}{"Thing": {}},
										TypeName:      "Thing",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path:          []string{"owner"},
													PossibleTypes: map[string]struct{}{"User": {}},
													TypeName:      "User",
													Fields: []*resolve.Field{
														{
															Name:  []byte("id"),
															Value: &resolve.Scalar{Path: []string{"id"}},
														},
														{
															Name:  []byte("title"),
															Value: &resolve.String{Path: []string{"title"}},
														},
														{
															Name:  []byte("lastName"),
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.String{Path: []string{"lastName"}},
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
					DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
						DeferID: 1,
						Fetches: resolve.Sequence(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
									DataSource:                            &Source{},
									SetTemplateOutputToNullOnVariableNull: true,
									Variables: []resolve.Variable{
										&resolve.ResolvableObjectVariable{
											Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name:        []byte("__typename"),
														Value:       &resolve.String{Path: []string{"__typename"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"__internal_id"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "a.owner", resolve.ObjectPath("a"), resolve.ObjectPath("owner")),
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
	t.Run("[P7] defer around a mutation root field stays deferred without prevalidation", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateThings {
					a: createThing {
						id
					}
					... @defer {
						b: createThing {
							slow
						}
					}
				}`,
			"CreateThings",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {
							ID:       1,
							ParentID: 0,
							Path:     []string{},
						},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("a"),
									Value: &resolve.Object{
										Path:          []string{"a"},
										PossibleTypes: map[string]struct{}{"Thing": {}},
										TypeName:      "Thing",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
										},
									},
								},
								{
									Name:  []byte("b"),
									Defer: &resolve.DeferField{DeferID: 1},
									Value: &resolve.Object{
										Path:          []string{"b"},
										PossibleTypes: map[string]struct{}{"Thing": {}},
										TypeName:      "Thing",
										Fields: []*resolve.Field{
											{
												Name:  []byte("slow"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.String{Path: []string{"slow"}},
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
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{b: createThing {slow}}"}}`,
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

	t.Run("[P8] planner deletes an empty outer defer and the inner defer gets its parent", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer(label: "outer") {
							title
							... @defer(label: "inner") {
								lastName
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						2: {ID: 2, ParentID: 0, Label: "inner", Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id title __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("u"),
									Value: &resolve.Object{
										Path:          []string{"u"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("title"),
												Value: &resolve.String{Path: []string{"title"}},
											},
											{
												Name:  []byte("lastName"),
												Defer: &resolve.DeferField{DeferID: 2},
												Value: &resolve.String{Path: []string{"lastName"}},
											},
										},
									},
								},
							},
						},
					},
					DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
						DeferID: 2,
						Fetches: resolve.Sequence(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
									DeferID:           2,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
									DataSource:                            &Source{},
									SetTemplateOutputToNullOnVariableNull: true,
									Variables: []resolve.Variable{
										&resolve.ResolvableObjectVariable{
											Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name:        []byte("__typename"),
														Value:       &resolve.String{Path: []string{"__typename"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"id"}},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "u", resolve.ObjectPath("u")),
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
}
