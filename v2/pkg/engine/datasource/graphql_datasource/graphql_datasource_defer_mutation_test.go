package graphql_datasource

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

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
			info: Info!
		}

		type Info {
			a: String!
			b: String!
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
			info: Info! @shareable
		}

		type Info {
			a: String!
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
			info: Info! @shareable
		}

		type Info {
			b: String!
		}
	`

	mutationDataSource := func(thingIsEntity bool) plan.DataSource {
		sdl := mutationSubgraphSDL
		metadata := &plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"thing"}},
				{TypeName: "Mutation", FieldNames: []string{"createThing", "createUser"}},
				{TypeName: "User", FieldNames: []string{"id", "title", "info"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Thing", FieldNames: []string{"id", "slow", "owner"}},
				{TypeName: "Info", FieldNames: []string{"a"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		}
		if thingIsEntity {
			sdl += `
				extend type Thing
					@key(fields: "id")`
			metadata.RootNodes = append(metadata.RootNodes, metadata.ChildNodes[0])
			metadata.ChildNodes = metadata.ChildNodes[1:]
			metadata.Keys = append(metadata.Keys, plan.FederationFieldConfiguration{
				TypeName:     "Thing",
				SelectionSet: "id",
			})
		}
		return mustDataSourceConfiguration(
			t,
			"mutation-service",
			metadata,
			mustCustomConfiguration(t, ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://mutation.service"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: sdl},
					sdl,
				),
			}),
		)
	}

	usersDataSource := mustDataSourceConfiguration(
		t,
		"users-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "lastName", "info"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Info", FieldNames: []string{"b"}},
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
		DataSources:                  []plan.DataSource{mutationDataSource(false), usersDataSource},
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

	t.Run("[P4] entity key on the mutation subgraph keeps the defer with an entity fetch on the same subgraph", func(t *testing.T) {
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
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
												Defer: &resolve.DeferField{DeferID: 1},
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
									Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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

	t.Run("[P5] inner defer on the mutation subgraph and outer entity defer both stay deferred", func(t *testing.T) {
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
						2: {ID: 2, ParentID: 1, Label: "inner", Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
												Defer: &resolve.DeferField{DeferID: 2},
												Value: &resolve.String{Path: []string{"title"}},
											},
										},
									},
								},
							},
						},
					},
					DeferTree: resolve.DeferSequence(resolve.DeferSingle(&resolve.DeferFetchGroup{
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
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
										DeferID:           2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
					),
				},
			},
			planConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P6] one defer splits per field: the non-entity parent goes to the initial response, its entity fields stay deferred", func(t *testing.T) {
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
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id owner {__typename __internal_id: id}}}"}}`,
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
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.Scalar{Path: []string{"id"}},
														},
														{
															Name:  []byte("title"),
															Defer: &resolve.DeferField{DeferID: 1},
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
									Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id title}}}","variables":{"representations":[$$0$$]}}}`,
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
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
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
	t.Run("[P21] refilter preserves the deferred entity key without an ancestor key", func(t *testing.T) {
		entityPlanConfiguration := planConfiguration
		entityPlanConfiguration.DataSources = []plan.DataSource{
			mutationDataSource(true),
			usersDataSource,
		}
		var mutationInput, entityInput bytes.Buffer
		require.NoError(t, json.Compact(&mutationInput, []byte(`
			{
				"method": "POST",
				"url": "http://mutation.service",
				"body": {
					"query": "mutation{a: createThing {owner {__internal_typename: __typename __typename id}}}"
				}
			}`)))
		require.NoError(t, json.Compact(&entityInput, []byte(`
			{
				"method": "POST",
				"url": "http://mutation.service",
				"body": {
					"query": "query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}",
					"variables": {
						"representations": ["$$0$$"]
					}
				}
			}`)))
		RunWithPermutations(t, definition, `
			mutation M {
				a: createThing {
					owner {
						... @defer {
							title
						}
					}
				}
			}`, "M", &plan.DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				DeferDescriptors: map[int]resolve.DeferDescriptor{
					1: {
						ID:   1,
						Path: []string{"a", "owner"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          mutationInput.String(),
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
											Name: []byte("owner"),
											Value: &resolve.Object{
												Path:          []string{"owner"},
												PossibleTypes: map[string]struct{}{"User": {}},
												TypeName:      "User",
												Fields: []*resolve.Field{
													{
														Name:  []byte("title"),
														Defer: &resolve.DeferField{DeferID: 1},
														Value: &resolve.String{Path: []string{"title"}},
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
								Input:                                 string(bytes.ReplaceAll(entityInput.Bytes(), []byte(`"$$0$$"`), []byte("$$0$$"))),
								DataSource:                            &Source{},
								RequiresEntityFetch:                   true,
								SetTemplateOutputToNullOnVariableNull: true,
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
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
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "a.owner", resolve.ObjectPath("a"), resolve.ObjectPath("owner")),
					),
				}),
			},
		}, entityPlanConfiguration, WithDefaultPostProcessor(), WithDefer(), WithCalculateFieldDependencies())
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

	t.Run("[P8] outer defer on the mutation subgraph and inner entity defer both stay deferred", func(t *testing.T) {
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
						1: {ID: 1, ParentID: 0, Label: "outer", Path: []string{"u"}},
						2: {ID: 2, ParentID: 1, Label: "inner", Path: []string{"u"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
												Defer: &resolve.DeferField{DeferID: 1},
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
					DeferTree: resolve.DeferSequence(
						resolve.DeferSingle(&resolve.DeferFetchGroup{
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
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
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
					),
				},
			},
			planConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	requiresDefinition := `
		type User {
			id: ID!
			lastName: String!
			fullName: FullName!
		}

		type FullName {
			title: String!
			slow: String!
		}

		type Query {
			user: User!
		}

		type Mutation {
			createUser: User!
		}
	`

	firstSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String! @external
			fullName: FullName! @requires(fields: "lastName")
		}

		type FullName {
			title: String!
			slow: String!
		}

		type Query {
			user: User!
		}

		type Mutation {
			createUser: User!
		}
	`

	secondSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String!
		}
	`

	requiresPlanConfiguration := plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"first-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"user"}},
						{TypeName: "Mutation", FieldNames: []string{"createUser"}},
						{TypeName: "User", FieldNames: []string{"id", "fullName"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "FullName", FieldNames: []string{"title", "slow"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "User", SelectionSet: "id"},
						},
						Requires: plan.FederationFieldConfigurations{
							{TypeName: "User", FieldName: "fullName", SelectionSet: "lastName"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://first.service"},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{Enabled: true, ServiceSDL: firstSubgraphSDL},
						firstSubgraphSDL,
					),
				}),
			),
			mustDataSourceConfiguration(
				t,
				"second-service",
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
					Fetch: &FetchConfiguration{URL: "http://second.service"},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{Enabled: true, ServiceSDL: secondSubgraphSDL},
						secondSubgraphSDL,
					),
				}),
			),
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("[P9] nested defer below a deferred @requires field keeps its own entity fetch", func(t *testing.T) {
		RunWithPermutations(
			t,
			requiresDefinition,
			`
				mutation CreateUser {
					createUser {
						id
						... @defer {
							fullName {
								title
								... @defer {
									slow
								}
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"createUser"}},
						2: {ID: 2, ParentID: 1, Path: []string{"createUser", "fullName"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"mutation{createUser {id __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("createUser"),
									Value: &resolve.Object{
										Path:          []string{"createUser"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("fullName"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.Object{
													Path:          []string{"fullName"},
													PossibleTypes: map[string]struct{}{"FullName": {}},
													TypeName:      "FullName",
													Fields: []*resolve.Field{
														{
															Name:  []byte("title"),
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.String{Path: []string{"title"}},
														},
														{
															Name:  []byte("slow"),
															Defer: &resolve.DeferField{DeferID: 2},
															Value: &resolve.String{Path: []string{"slow"}},
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
					DeferTree: resolve.DeferSequence(
						resolve.DeferSingle(&resolve.DeferFetchGroup{
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_lastName: lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "createUser", resolve.ObjectPath("createUser")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{0, 1},
										DeferID:           1,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Name:        []byte("lastName"),
															Value:       &resolve.String{Path: []string{"__internal_lastName"}},
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
								}, "createUser", resolve.ObjectPath("createUser")),
							),
						}),
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0, 1},
										DeferID:           2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Name:        []byte("lastName"),
															Value:       &resolve.String{Path: []string{"__internal_lastName"}},
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
								}, "createUser", resolve.ObjectPath("createUser")),
							),
						}),
					),
				},
			},
			requiresPlanConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P10] defer below a @requires field in the initial response keeps its own entity fetch", func(t *testing.T) {
		RunWithPermutations(
			t,
			requiresDefinition,
			`
				mutation CreateUser {
					createUser {
						id
						fullName {
							title
							... @defer {
								slow
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"createUser", "fullName"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"mutation{createUser {id __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
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
							}, "createUser", resolve.ObjectPath("createUser")),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           3,
									DependsOnFetchIDs: []int{0, 1},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name:        []byte("lastName"),
														Value:       &resolve.String{Path: []string{"lastName"}},
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
							}, "createUser", resolve.ObjectPath("createUser")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("createUser"),
									Value: &resolve.Object{
										Path:          []string{"createUser"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name: []byte("fullName"),
												Value: &resolve.Object{
													Path:          []string{"fullName"},
													PossibleTypes: map[string]struct{}{"FullName": {}},
													TypeName:      "FullName",
													Fields: []*resolve.Field{
														{
															Name:  []byte("title"),
															Value: &resolve.String{Path: []string{"title"}},
														},
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
							},
						},
					},
					DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
						DeferID: 1,
						Fetches: resolve.Sequence(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{0, 1},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name:        []byte("lastName"),
														Value:       &resolve.String{Path: []string{"lastName"}},
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
							}, "createUser", resolve.ObjectPath("createUser")),
						),
					}),
				},
			},
			requiresPlanConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P11] shareable field below a mutation root field stays deferred on both subgraphs", func(t *testing.T) {
		// The fetch IDs follow the order of the data sources, so each permutation has its own plan.
		RunWithPermutationsVariants(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer {
							info {
								a
								b
							}
						}
					}
				}`,
			"CreateUser",
			[]plan.Plan{
				&plan.DeferResponsePlan{
					Response: &resolve.GraphQLDeferResponse{
						DeferDescriptors: map[int]resolve.DeferDescriptor{
							1: {ID: 1, ParentID: 0, Path: []string{"u"}},
						},
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
													Name:  []byte("info"),
													Defer: &resolve.DeferField{DeferID: 1},
													Value: &resolve.Object{
														Path:          []string{"info"},
														PossibleTypes: map[string]struct{}{"Info": {}},
														TypeName:      "Info",
														Fields: []*resolve.Field{
															{
																Name:  []byte("a"),
																Defer: &resolve.DeferField{DeferID: 1},
																Value: &resolve.String{Path: []string{"a"}},
															},
															{
																Name:  []byte("b"),
																Defer: &resolve.DeferField{DeferID: 1},
																Value: &resolve.String{Path: []string{"b"}},
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
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[$$0$$]}}}`,
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
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
										DeferID:           1,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[$$0$$]}}}`,
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
				&plan.DeferResponsePlan{
					Response: &resolve.GraphQLDeferResponse{
						DeferDescriptors: map[int]resolve.DeferDescriptor{
							1: {ID: 1, ParentID: 0, Path: []string{"u"}},
						},
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
													Name:  []byte("info"),
													Defer: &resolve.DeferField{DeferID: 1},
													Value: &resolve.Object{
														Path:          []string{"info"},
														PossibleTypes: map[string]struct{}{"Info": {}},
														TypeName:      "Info",
														Fields: []*resolve.Field{
															{
																Name:  []byte("a"),
																Defer: &resolve.DeferField{DeferID: 1},
																Value: &resolve.String{Path: []string{"a"}},
															},
															{
																Name:  []byte("b"),
																Defer: &resolve.DeferField{DeferID: 1},
																Value: &resolve.String{Path: []string{"b"}},
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
										Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[$$0$$]}}}`,
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
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
										DeferID:           1,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[$$0$$]}}}`,
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
			},
			planConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P12] planner deletes an empty outer defer and the inner defer gets its parent", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateThing {
					a: createThing {
						id
						... @defer(label: "outer") {
							slow
							... @defer(label: "inner") {
								owner {
									lastName
								}
							}
						}
					}
				}`,
			"CreateThing",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						2: {ID: 2, ParentID: 0, Label: "inner", Path: []string{"a"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{a: createThing {id slow owner {__typename id}}}"}}`,
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
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path:          []string{"owner"},
													PossibleTypes: map[string]struct{}{"User": {}},
													TypeName:      "User",
													Fields: []*resolve.Field{
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

	t.Run("[P13] deferred field of a value type branches out at the nearest entity above it", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						info {
							... @defer {
								a
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"u", "info"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id info {__internal_typename: __typename} __typename}}"}}`,
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
												Name: []byte("info"),
												Value: &resolve.Object{
													Path:          []string{"info"},
													PossibleTypes: map[string]struct{}{"Info": {}},
													TypeName:      "Info",
													Fields: []*resolve.Field{
														{
															Name:  []byte("a"),
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.String{Path: []string{"a"}},
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
									Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[$$0$$]}}}`,
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

	t.Run("[P14] shareable field without a key jump on the mutation subgraph stays in the initial response", func(t *testing.T) {
		unresolvableMutationSubgraphSDL := `
			type User @key(fields: "id", resolvable: false) {
				id: ID!
				title: String!
				info: Info! @shareable
			}

			type Info {
				a: String!
			}

			type Mutation {
				createUser: User!
			}
		`

		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer {
							info {
								a
								b
							}
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
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id info {a} __typename}}"}}`,
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
												Name: []byte("info"),
												Value: &resolve.Object{
													Path:          []string{"info"},
													PossibleTypes: map[string]struct{}{"Info": {}},
													TypeName:      "Info",
													Fields: []*resolve.Field{
														{
															Name:  []byte("a"),
															Value: &resolve.String{Path: []string{"a"}},
														},
														{
															Name:  []byte("b"),
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.String{Path: []string{"b"}},
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
									Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[$$0$$]}}}`,
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
			plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"mutation-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Mutation", FieldNames: []string{"createUser"}},
								{TypeName: "User", FieldNames: []string{"id", "title", "info"}},
							},
							ChildNodes: []plan.TypeField{
								{TypeName: "Info", FieldNames: []string{"a"}},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{TypeName: "User", SelectionSet: "id", DisableEntityResolver: true},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{URL: "http://mutation.service"},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{Enabled: true, ServiceSDL: unresolvableMutationSubgraphSDL},
								unresolvableMutationSubgraphSDL,
							),
						}),
					),
					usersDataSource,
				},
				DisableResolveFieldPositions: true,
			},
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P15] inner defer below a value type in a jumped outer defer gets its own entity fetch", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer(label: "outer") {
							title
							info {
								... @defer(label: "inner") {
									a
								}
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Label: "outer", Path: []string{"u"}},
						2: {ID: 2, ParentID: 1, Label: "inner", Path: []string{"u", "info"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.String{Path: []string{"title"}},
											},
											{
												Name:  []byte("info"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.Object{
													Path:          []string{"info"},
													PossibleTypes: map[string]struct{}{"Info": {}},
													TypeName:      "Info",
													Fields: []*resolve.Field{
														{
															Name:  []byte("a"),
															Defer: &resolve.DeferField{DeferID: 2},
															Value: &resolve.String{Path: []string{"a"}},
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
					DeferTree: resolve.DeferSequence(
						resolve.DeferSingle(&resolve.DeferFetchGroup{
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
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title info {__internal_typename: __typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
										DeferID:           2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[$$0$$]}}}`,
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
					),
				},
			},
			planConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	// The planner puts the shareable info field on the users subgraph because its only field b lives there.
	// The plan without the inner defer has the same info fetch on the users subgraph.
	t.Run("[P16] inner defer from the other subgraph below a value type in a jumped outer defer", func(t *testing.T) {
		RunWithPermutations(
			t,
			definition,
			`
				mutation CreateUser {
					u: createUser {
						id
						... @defer(label: "outer") {
							title
							info {
								... @defer(label: "inner") {
									b
								}
							}
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Label: "outer", Path: []string{"u"}},
						2: {ID: 2, ParentID: 1, Label: "inner", Path: []string{"u", "info"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{u: createUser {id __typename}}"}}`,
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
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.String{Path: []string{"title"}},
											},
											{
												Name:  []byte("info"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.Object{
													Path:          []string{"info"},
													PossibleTypes: map[string]struct{}{"Info": {}},
													TypeName:      "Info",
													Fields: []*resolve.Field{
														{
															Name:  []byte("b"),
															Defer: &resolve.DeferField{DeferID: 2},
															Value: &resolve.String{Path: []string{"b"}},
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
					DeferTree: resolve.DeferSequence(
						resolve.DeferSingle(&resolve.DeferFetchGroup{
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
										Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{0},
										DeferID:           1,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {__internal_typename: __typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
										DeferID:           2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://users.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[$$0$$]}}}`,
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
					),
				},
			},
			planConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P17] defer below a mutation root field of an interface object jumps back into the interface object", func(t *testing.T) {
		interfaceObjectDefinition := `
			interface Account {
				id: ID!
				title: String!
			}

			type User implements Account {
				id: ID!
				title: String!
			}

			type Query {
				account: Account
			}

			type Mutation {
				createAccount: Account!
			}
		`

		interfaceObjectSubgraphSDL := `
			type Account @key(fields: "id") @interfaceObject {
				id: ID!
				title: String!
			}

			type Mutation {
				createAccount: Account!
			}
		`

		entityInterfaceSubgraphSDL := `
			interface Account @key(fields: "id") {
				id: ID!
			}

			type User implements Account @key(fields: "id") {
				id: ID!
			}

			type Query {
				account: Account
			}
		`

		RunWithPermutations(
			t,
			interfaceObjectDefinition,
			`
				mutation CreateAccount {
					createAccount {
						id
						... @defer {
							title
						}
					}
				}`,
			"CreateAccount",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"createAccount"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://interface-object.service","body":{"query":"mutation{createAccount {__typename id}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("createAccount"),
									Value: &resolve.Object{
										Path:          []string{"createAccount"},
										PossibleTypes: map[string]struct{}{"Account": {}, "User": {}},
										TypeName:      "Account",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("title"),
												Defer: &resolve.DeferField{DeferID: 1},
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
									Input:                                 `{"method":"POST","url":"http://interface-object.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
														OnTypeNames: [][]byte{[]byte("Account")},
													},
													{
														Name:        []byte("id"),
														Value:       &resolve.Scalar{Path: []string{"id"}},
														OnTypeNames: [][]byte{[]byte("Account")},
													},
												},
											}),
										},
									},
									PostProcessing: SingleEntityPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "createAccount", resolve.ObjectPath("createAccount")),
						),
					}),
				},
			},
			plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"interface-object-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Mutation", FieldNames: []string{"createAccount"}},
								{TypeName: "Account", FieldNames: []string{"id", "title"}},
								{TypeName: "User", FieldNames: []string{"id", "title"}},
							},
							FederationMetaData: plan.FederationMetaData{
								InterfaceObjects: []plan.EntityInterfaceConfiguration{
									{InterfaceTypeName: "Account", ConcreteTypeNames: []string{"User"}},
								},
								Keys: plan.FederationFieldConfigurations{
									{TypeName: "Account", SelectionSet: "id"},
									{TypeName: "User", SelectionSet: "id"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{URL: "http://interface-object.service"},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{Enabled: true, ServiceSDL: interfaceObjectSubgraphSDL},
								interfaceObjectSubgraphSDL,
							),
						}),
					),
					mustDataSourceConfiguration(
						t,
						"entity-interface-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"account"}},
								{TypeName: "Account", FieldNames: []string{"id"}},
								{TypeName: "User", FieldNames: []string{"id"}},
							},
							FederationMetaData: plan.FederationMetaData{
								EntityInterfaces: []plan.EntityInterfaceConfiguration{
									{InterfaceTypeName: "Account", ConcreteTypeNames: []string{"User"}},
								},
								Keys: plan.FederationFieldConfigurations{
									{TypeName: "Account", SelectionSet: "id"},
									{TypeName: "User", SelectionSet: "id"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{URL: "http://entity-interface.service"},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{Enabled: true, ServiceSDL: entityInterfaceSubgraphSDL},
								entityInterfaceSubgraphSDL,
							),
						}),
					),
				},
				DisableResolveFieldPositions: true,
			},
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P18] defer key jump skips a key with an external field on the mutation subgraph", func(t *testing.T) {
		externalKeyDefinition := `
			type User {
				id: ID!
				sku: ID!
				title: String!
			}

			type Query {
				user: User
			}

			type Mutation {
				createUser: User!
			}
		`

		externalKeyMutationSubgraphSDL := `
			type User @key(fields: "sku") @key(fields: "id") {
				sku: ID! @external
				id: ID!
				title: String!
			}

			type Mutation {
				createUser: User!
			}
		`

		skuSubgraphSDL := `
			type User @key(fields: "id") {
				id: ID!
				sku: ID!
			}

			type Query {
				user: User
			}
		`

		RunWithPermutations(
			t,
			externalKeyDefinition,
			`
				mutation CreateUser {
					createUser {
						id
						... @defer {
							title
						}
					}
				}`,
			"CreateUser",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"createUser"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://mutation.service","body":{"query":"mutation{createUser {id __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("createUser"),
									Value: &resolve.Object{
										Path:          []string{"createUser"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("title"),
												Defer: &resolve.DeferField{DeferID: 1},
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
									Input:                                 `{"method":"POST","url":"http://mutation.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
							}, "createUser", resolve.ObjectPath("createUser")),
						),
					}),
				},
			},
			plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"mutation-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Mutation", FieldNames: []string{"createUser"}},
								{TypeName: "User", FieldNames: []string{"id", "title"}, ExternalFieldNames: []string{"sku"}},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{TypeName: "User", SelectionSet: "sku"},
									{TypeName: "User", SelectionSet: "id"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{URL: "http://mutation.service"},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{Enabled: true, ServiceSDL: externalKeyMutationSubgraphSDL},
								externalKeyMutationSubgraphSDL,
							),
						}),
					),
					mustDataSourceConfiguration(
						t,
						"sku-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"user"}},
								{TypeName: "User", FieldNames: []string{"id", "sku"}},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{TypeName: "User", SelectionSet: "id"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{URL: "http://sku.service"},
							SchemaConfiguration: mustSchema(t,
								&FederationConfiguration{Enabled: true, ServiceSDL: skuSubgraphSDL},
								skuSubgraphSDL,
							),
						}),
					),
				},
				DisableResolveFieldPositions: true,
			},
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P19] query defer below a @requires field gets an entity fetch with the required field", func(t *testing.T) {
		RunWithPermutations(
			t,
			requiresDefinition,
			`
				query User {
					user {
						id
						fullName {
							title
							... @defer {
								slow
							}
						}
					}
				}`,
			"User",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"user", "fullName"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {id __typename}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
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
							}, "user", resolve.ObjectPath("user")),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           3,
									DependsOnFetchIDs: []int{0, 1},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name:        []byte("lastName"),
														Value:       &resolve.String{Path: []string{"lastName"}},
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
							}, "user", resolve.ObjectPath("user")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
									Value: &resolve.Object{
										Path:          []string{"user"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name: []byte("fullName"),
												Value: &resolve.Object{
													Path:          []string{"fullName"},
													PossibleTypes: map[string]struct{}{"FullName": {}},
													TypeName:      "FullName",
													Fields: []*resolve.Field{
														{
															Name:  []byte("title"),
															Value: &resolve.String{Path: []string{"title"}},
														},
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
							},
						},
					},
					DeferTree: resolve.DeferSingle(&resolve.DeferFetchGroup{
						DeferID: 1,
						Fetches: resolve.Sequence(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{0, 1},
									DeferID:           1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name:        []byte("lastName"),
														Value:       &resolve.String{Path: []string{"lastName"}},
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
							}, "user", resolve.ObjectPath("user")),
						),
					}),
				},
			},
			requiresPlanConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})

	t.Run("[P20] query nested defer below a deferred @requires field gets an entity fetch with the required field", func(t *testing.T) {
		RunWithPermutations(
			t,
			requiresDefinition,
			`
				query User {
					user {
						id
						... @defer {
							fullName {
								title
								... @defer {
									slow
								}
							}
						}
					}
				}`,
			"User",
			&plan.DeferResponsePlan{
				Response: &resolve.GraphQLDeferResponse{
					DeferDescriptors: map[int]resolve.DeferDescriptor{
						1: {ID: 1, ParentID: 0, Path: []string{"user"}},
						2: {ID: 2, ParentID: 1, Path: []string{"user", "fullName"}},
					},
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {id __typename}}"}}`,
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
										Path:          []string{"user"},
										PossibleTypes: map[string]struct{}{"User": {}},
										TypeName:      "User",
										Fields: []*resolve.Field{
											{
												Name:  []byte("id"),
												Value: &resolve.Scalar{Path: []string{"id"}},
											},
											{
												Name:  []byte("fullName"),
												Defer: &resolve.DeferField{DeferID: 1},
												Value: &resolve.Object{
													Path:          []string{"fullName"},
													PossibleTypes: map[string]struct{}{"FullName": {}},
													TypeName:      "FullName",
													Fields: []*resolve.Field{
														{
															Name:  []byte("title"),
															Defer: &resolve.DeferField{DeferID: 1},
															Value: &resolve.String{Path: []string{"title"}},
														},
														{
															Name:  []byte("slow"),
															Defer: &resolve.DeferField{DeferID: 2},
															Value: &resolve.String{Path: []string{"slow"}},
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
					DeferTree: resolve.DeferSequence(
						resolve.DeferSingle(&resolve.DeferFetchGroup{
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_lastName: lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{0, 1},
										DeferID:           1,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Name:        []byte("lastName"),
															Value:       &resolve.String{Path: []string{"__internal_lastName"}},
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
								}, "user", resolve.ObjectPath("user")),
							),
						}),
						resolve.DeferSingle(&resolve.DeferFetchGroup{
							DeferID: 2,
							Fetches: resolve.Sequence(
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0, 1},
										DeferID:           2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Name:        []byte("lastName"),
															Value:       &resolve.String{Path: []string{"__internal_lastName"}},
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
								}, "user", resolve.ObjectPath("user")),
							),
						}),
					),
				},
			},
			requiresPlanConfiguration,
			WithDefaultPostProcessor(),
			WithDefer(),
			WithCalculateFieldDependencies(),
		)
	})
}
