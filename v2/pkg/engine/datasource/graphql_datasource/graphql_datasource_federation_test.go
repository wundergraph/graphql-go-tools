package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation_Typenames(t *testing.T) {
	t.Run("__typename on union", func(t *testing.T) {
		def := `
			schema {
				query: Query
			}
	
			type A {
				a: String
			}
	
			union U = A
	
			type Query {
				u: U
			}`

		t.Run("run", RunTest(
			def, `
			query TypenameOnUnion {
				u {
					__typename
				}
			}`,
			"TypenameOnUnion", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(resolve.Single(
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource:     &Source{},
								Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"{u {__typename}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
					)),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("u"),
								Value: &resolve.Object{
									Path:     []string{"u"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"A": {},
									},
									TypeName: "U",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
									},
								},
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
									FieldNames: []string{"u"},
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
			}, WithDefaultPostProcessor()))
	})

	t.Run("__typename on root query types", func(t *testing.T) {
		def := `
			scalar String

			type Query {
				q: String
			}
			type Mutation {
				m: String
			}
			type Subscription {
				s: String
			}`

		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"q"},
							},
							{
								TypeName:   "Mutation",
								FieldNames: []string{"m"},
							},
							{
								TypeName:   "Subscription",
								FieldNames: []string{"s"},
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
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"q"},
							},
							{
								TypeName:   "Mutation",
								FieldNames: []string{"m"},
							},
							{
								TypeName:   "Subscription",
								FieldNames: []string{"s"},
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
		}

		t.Run("on query", RunTest(
			def, `
			query TypenameOnQuery {
				__typename
				alias: __typename
				alias2: __typename
			}`,
			"TypenameOnQuery", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("__typename"),
								Value: &resolve.StaticString{
									Path:  []string{"__typename"},
									Value: "Query",
								},
							},
							{
								Name: []byte("alias"),
								Value: &resolve.StaticString{
									Path:  []string{"alias"},
									Value: "Query",
								},
							},
							{
								Name: []byte("alias2"),
								Value: &resolve.StaticString{
									Path:  []string{"alias2"},
									Value: "Query",
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))

		t.Run("on mutation", RunTest(
			def, `
			mutation TypenameOnMutation {
				__typename
			}`,
			"TypenameOnMutation", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("__typename"),
								Value: &resolve.StaticString{
									Path:  []string{"__typename"},
									Value: "Mutation",
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))
	})

	t.Run("typename should be selected on parent ds, not siblings", func(t *testing.T) {
		// in this test multiple subgraphs has the same root node Query.me
		// so parent won't be selected immediately, and we will select first available node

		def := `
			scalar String

			type Query {
				me: User
			}

			type User {
				id: String!
				name: String
				address: String
			}`

		ds1SDL := `
			scalar String
	
			type Query {
				me: User
			}
	
			type User @key(fields: "id") {
				id: String!
			}`
		ds1 := mustDataSourceConfiguration(
			t,
			"ds-id-1",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: []plan.FederationFieldConfiguration{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			},
			mustCustomConfiguration(t, ConfigurationInput{
				Fetch: &FetchConfiguration{
					URL: "https://example-1.com/graphql",
				},
				SchemaConfiguration: mustSchema(t, &FederationConfiguration{
					Enabled:    true,
					ServiceSDL: ds1SDL,
				}, ds1SDL),
			}),
		)

		ds2SDL := `
			scalar String
	
			type User @key(fields: "id") {
				id: String!
				name: String
			}`

		ds2 := mustDataSourceConfiguration(
			t,
			"ds-id-2",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "name"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: []plan.FederationFieldConfiguration{
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
						ServiceSDL: ds2SDL,
					}, ds2SDL),
			}),
		)

		ds4SDL := `
			scalar String

			type Query {
				me: User
			}

			type User @key(fields: "id") {
				id: String
				address: String
			}`
		ds4 := mustDataSourceConfiguration(
			t,
			"ds-id-4",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "address"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: []plan.FederationFieldConfiguration{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			},
			mustCustomConfiguration(t, ConfigurationInput{
				Fetch: &FetchConfiguration{
					URL: "https://example-4.com/graphql",
				},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{
						Enabled:    true,
						ServiceSDL: ds4SDL,
					}, ds4SDL),
			}),
		)

		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSource{
				ds2,
				ds1,
				ds4,
			},
			DisableResolveFieldPositions: true,
			Debug:                        plan.DebugConfiguration{},
		}

		t.Run("only __typename", RunTest(
			def, `
			query TypenameOnMe {
				me {
					__typename
				}
			}`,
			"TypenameOnMe", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource:     &Source{},
								Input:          `{"method":"POST","url":"https://example-1.com/graphql","body":{"query":"{me {__typename}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
									},
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))

		t.Run("__typename and field", RunTest(
			def, `
			query TypenameOnMe {
				me {
					__typename
					name
				}
			}`,
			"TypenameOnMe", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource:     &Source{},
								Input:          `{"method":"POST","url":"https://example-1.com/graphql","body":{"query":"{me {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:                                 `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
								PostProcessing:      SingleEntityPostProcessingConfiguration,
								RequiresEntityFetch: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "me", resolve.ObjectPath("me")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Nullable: true,
												Path:     []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))
	})
}

func TestGraphQLDataSourceFederation_Mutations(t *testing.T) {
	t.Run("serial mutations", func(t *testing.T) {
		def := `
			type Query {
				q: String
			}
			type Mutation {
				a: String!
				b: Object!
				c: String!
				d: Object!
			}

			type Object {
				id: ID!
				name: String!
				field: String!
			}`

		sub1 := `
			type Query {
				q: String!
			}
			type Mutation {
				b: Object!
				c: String!
				d: Object!
			}

			type Object @key(fields: "id") {
				id: ID!
				field: String!
			}`

		sub2 := `
			type Mutation {
				a: String!
			}

			type Object @key(fields: "id"){
				id: ID!
				name: String!
			}`

		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"q"},
							},
							{
								TypeName:   "Mutation",
								FieldNames: []string{"b", "c", "d"},
							},
							{
								TypeName:   "Object",
								FieldNames: []string{"id", "field"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Object",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://example.com/graphql",
						},
						SchemaConfiguration: mustSchema(t, &FederationConfiguration{
							Enabled:    true,
							ServiceSDL: sub1,
						}, sub1),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Object",
								FieldNames: []string{"id", "name"},
							},
							{
								TypeName:   "Mutation",
								FieldNames: []string{"a"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Object",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://example-2.com/graphql",
						},
						SchemaConfiguration: mustSchema(t, &FederationConfiguration{
							Enabled:    true,
							ServiceSDL: sub2,
						}, sub2),
					}),
				),
			},
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				// PrintOperationTransformations: true,
				// PrintOperationEnableASTRefs:   true,
				// PrintPlanningPaths:            true,
				// PrintQueryPlans:               true,
				// PrintNodeSuggestions:          true,
			},
		}

		t.Run("with entity call", RunTest(
			def, `
			mutation TypenameOnMutation {
				d {
					__typename
					id
					name
					field
				}
				c
				b {
					__typename
					id
					name
					field
				}
				a
			}`,
			"TypenameOnMutation", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"mutation{d {__typename id field}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						resolve.SingleWithPath(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:                                 `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Object {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
														OnTypeNames: [][]byte{[]byte("Object")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Object")},
													},
												},
											}),
										},
									},
									PostProcessing:      SingleEntityPostProcessingConfiguration,
									RequiresEntityFetch: true,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "d", resolve.ObjectPath("d")),
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"mutation{c}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           3,
									DependsOnFetchIDs: []int{0, 2},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"mutation{b {__typename id field}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						resolve.SingleWithPath(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           4,
									DependsOnFetchIDs: []int{3},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:                                 `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Object {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
														OnTypeNames: [][]byte{[]byte("Object")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Object")},
													},
												},
											}),
										},
									},
									PostProcessing:      SingleEntityPostProcessingConfiguration,
									RequiresEntityFetch: true,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}, "b", resolve.ObjectPath("b")),
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           5,
									DependsOnFetchIDs: []int{0, 2, 3},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"mutation{a}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("d"),
								Value: &resolve.Object{
									Path: []string{"d"},
									PossibleTypes: map[string]struct{}{
										"Object": {},
									},
									TypeName: "Object",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
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
											Name: []byte("field"),
											Value: &resolve.String{
												Path: []string{"field"},
											},
										},
									},
								},
							},
							{
								Name: []byte("c"),
								Value: &resolve.String{
									Path: []string{"c"},
								},
							},
							{
								Name: []byte("b"),
								Value: &resolve.Object{
									Path: []string{"b"},
									PossibleTypes: map[string]struct{}{
										"Object": {},
									},
									TypeName: "Object",
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
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
											Name: []byte("field"),
											Value: &resolve.String{
												Path: []string{"field"},
											},
										},
									},
								},
							},
							{
								Name: []byte("a"),
								Value: &resolve.String{
									Path: []string{"a"},
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))

		t.Run("with root mutations only", RunTest(
			def, `
			mutation TypenameOnMutation {
				c
				b {
					field
				}
				a
			}`,
			"TypenameOnMutation", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"mutation{c}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"mutation{b {field}}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						resolve.Single(
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{0, 1},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource:     &Source{},
									Input:          `{"method":"POST","url":"https://example-2.com/graphql","body":{"query":"mutation{a}"}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("c"),
								Value: &resolve.String{
									Path: []string{"c"},
								},
							},
							{
								Name: []byte("b"),
								Value: &resolve.Object{
									Path: []string{"b"},
									PossibleTypes: map[string]struct{}{
										"Object": {},
									},
									TypeName: "Object",
									Fields: []*resolve.Field{
										{
											Name: []byte("field"),
											Value: &resolve.String{
												Path: []string{"field"},
											},
										},
									},
								},
							},
							{
								Name: []byte("a"),
								Value: &resolve.String{
									Path: []string{"a"},
								},
							},
						},
					},
				},
			}, planConfiguration, WithDefaultPostProcessor()))
	})
}

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
		// TODO: add test for partially provided required fields
		// TODO: add test for provided fields which are not external e.g. "a externalA {A} notExternalB {B}"

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
						TypeName:           "Account",
						FieldNames:         []string{"id", "info", "address", "deliveryAddress"},
						ExternalFieldNames: []string{"name", "shippingInfo"},
					},
					{
						TypeName:   "Address",
						FieldNames: []string{"id", "line1", "line2"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:           "ShippingInfo",
						ExternalFieldNames: []string{"zip"},
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
			DisableIncludeInfo:           true,
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
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
									DataSource:     &Source{},
									PostProcessing: DefaultPostProcessingConfiguration,
								},
							}),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
																	Value: &resolve.Scalar{
																		Path: []string{"a"},
																	},
																},
																{
																	Name: []byte("b"),
																	Value: &resolve.Scalar{
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
							}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
									Value: &resolve.Object{
										Path:     []string{"user"},
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"User": {},
										},
										TypeName: "User",
										Fields: []*resolve.Field{
											{
												Name: []byte("account"),
												Value: &resolve.Object{
													Path:     []string{"account"},
													Nullable: true,
													PossibleTypes: map[string]struct{}{
														"Account": {},
													},
													TypeName: "Account",
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
																PossibleTypes: map[string]struct{}{
																	"ShippingInfo": {},
																},
																TypeName: "ShippingInfo",
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
				planConfiguration, WithDefaultPostProcessor())
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
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
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
									DataSourceID:   "user.service",
									DataSourceName: "user.service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "user",
										},
									},
								},
							}),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								Info: &resolve.FetchInfo{
									DataSourceID:   "account.service",
									DataSourceName: "account.service",
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
									Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
																	Value: &resolve.Scalar{
																		Path: []string{"a"},
																	},
																},
																{
																	Name: []byte("b"),
																	Value: &resolve.Scalar{
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
							}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
						),
						Info: &resolve.GraphQLResponseInfo{
							OperationType: ast.OperationTypeQuery,
						},
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("user"),
									Info: &resolve.FieldInfo{
										Name:            "user",
										ParentTypeNames: []string{"Query"},
										NamedType:       "User",
										Source: resolve.TypeFieldSource{
											IDs:   []string{"user.service"},
											Names: []string{"user.service"},
										},
										ExactParentTypeName: "Query",
									},
									Value: &resolve.Object{
										Path:     []string{"user"},
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"User": {},
										},
										TypeName:   "User",
										SourceName: "user.service",
										Fields: []*resolve.Field{
											{
												Name: []byte("account"),
												Info: &resolve.FieldInfo{
													Name:            "account",
													NamedType:       "Account",
													ParentTypeNames: []string{"User"},
													Source: resolve.TypeFieldSource{
														IDs:   []string{"user.service"},
														Names: []string{"user.service"},
													},
													ExactParentTypeName: "User",
												},
												Value: &resolve.Object{
													Path:     []string{"account"},
													Nullable: true,
													PossibleTypes: map[string]struct{}{
														"Account": {},
													},
													TypeName:   "Account",
													SourceName: "user.service",
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Info: &resolve.FieldInfo{
																Name:            "name",
																NamedType:       "String",
																ParentTypeNames: []string{"Account"},
																Source: resolve.TypeFieldSource{
																	IDs:   []string{"account.service"},
																	Names: []string{"account.service"},
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
																	IDs:   []string{"account.service"},
																	Names: []string{"account.service"},
																},
																ExactParentTypeName:  "Account",
																HasAuthorizationRule: true,
															},
															Value: &resolve.Object{
																Path:     []string{"shippingInfo"},
																Nullable: true,
																PossibleTypes: map[string]struct{}{
																	"ShippingInfo": {},
																},
																TypeName:   "ShippingInfo",
																SourceName: "account.service",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("zip"),
																		Info: &resolve.FieldInfo{
																			Name:            "zip",
																			NamedType:       "String",
																			ParentTypeNames: []string{"ShippingInfo"},
																			Source: resolve.TypeFieldSource{
																				IDs:   []string{"account.service"},
																				Names: []string{"account.service"},
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
												},
											},
										},
									},
								},
							},
						},
					},
				},
				planConfiguration, WithFieldInfo(), WithDefaultPostProcessor()))
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
				DisableIncludeInfo:           true,
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {__typename account {id} id} otherUser {__typename account {id} id}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "otherUser", resolve.ObjectPath("otherUser")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path: []string{"user"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
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
											Path: []string{"otherUser"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
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
					WithDefaultPostProcessor(),
				))
			})

			t.Run("operation name propagation", func(t *testing.T) {
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
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://service-a","body":{"query":"query CompositeKey__service_a {user {__typename account {id} id} otherUser {__typename account {id} id}}"}}`,
											DataSource:     &Source{},
											PostProcessing: DefaultPostProcessingConfiguration,
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://service-b","body":{"query":"query CompositeKey__service_b($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
									}, "user", resolve.ObjectPath("user")),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://service-b","body":{"query":"query CompositeKey__service_b($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
									}, "otherUser", resolve.ObjectPath("otherUser")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Path:     []string{"user"},
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
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Path:     []string{"otherUser"},
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
						plan.Configuration{
							Logger:                     nil,
							DefaultFlushIntervalMillis: 0,
							DataSources: []plan.DataSource{
								subgraphADatasourceConfiguration,
								subgraphBDatasourceConfiguration,
							},
							DisableResolveFieldPositions:   true,
							DisableIncludeInfo:             true,
							EnableOperationNamePropagation: true,
						},
						WithDefaultPostProcessor(),
					))
				})
				t.Run("anonymous", func(t *testing.T) {
					t.Run("run", RunTest(
						definition,
						`
						query  {
							user {
								foo
							}
							otherUser {
								foo
							}
						}`,
						"",
						&plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {__typename account {id} id} otherUser {__typename account {id} id}}"}}`,
											DataSource:     &Source{},
											PostProcessing: DefaultPostProcessingConfiguration,
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
									}, "user", resolve.ObjectPath("user")),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
									}, "otherUser", resolve.ObjectPath("otherUser")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Path:     []string{"user"},
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
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Path:     []string{"otherUser"},
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
						plan.Configuration{
							Logger:                     nil,
							DefaultFlushIntervalMillis: 0,
							DataSources: []plan.DataSource{
								subgraphADatasourceConfiguration,
								subgraphBDatasourceConfiguration,
							},
							DisableResolveFieldPositions:   true,
							DisableIncludeInfo:             true,
							EnableOperationNamePropagation: true,
						},
						WithDefaultPostProcessor(),
					))
				})
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {id name __typename account {id}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user", resolve.ObjectPath("user")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path: []string{"user"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
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
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"{user {id name account {type id} __typename}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user", resolve.ObjectPath("user")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path: []string{"user"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
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
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
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
					WithDefaultPostProcessor(),
				))
			})
		})

		t.Run("operation name propagation - composite keys subgraphs with special characters", func(t *testing.T) {
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
				"_s$rvic&+}{e-a___",
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
				"_service-b___",

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

			t.Run("expect query to be sanitized", func(t *testing.T) {
				t.Run("run", RunTest(
					definition,
					`
				query ___CompositeKey____ {
					user {
						foo
					}
					otherUser {
						foo
					}
				}`,
					"___CompositeKey____",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://service-a","body":{"query":"query CompositeKey__s_rvic_e_a {user {__typename account {id} id} otherUser {__typename account {id} id}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query CompositeKey__service_b($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://service-b","body":{"query":"query CompositeKey__service_b($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename foo}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "otherUser", resolve.ObjectPath("otherUser")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Path:     []string{"user"},
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
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Path:     []string{"otherUser"},
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
					plan.Configuration{
						Logger:                     nil,
						DefaultFlushIntervalMillis: 0,
						DataSources: []plan.DataSource{
							subgraphADatasourceConfiguration,
							subgraphBDatasourceConfiguration,
						},
						DisableResolveFieldPositions:   true,
						DisableIncludeInfo:             true,
						EnableOperationNamePropagation: true,
					},
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {line1 line2 __typename id}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename country city}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user.account.address", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("address")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0, 1},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!, $a: String!){_entities(representations: $representations){... on Address {__typename line3(test: $a) zip}}}","variables":{"a":"BOOM","representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user.account.address", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("address")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{0, 2},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename fullAddress}}}","variables":{"representations":[$$0$$]}}}`,
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
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
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
								}, "user.account.address", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("address")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("account"),
													Value: &resolve.Object{
														Path:     []string{"account"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
														Fields: []*resolve.Field{
															{
																Name: []byte("address"),
																Value: &resolve.Object{
																	Path:     []string{"address"},
																	Nullable: true,
																	PossibleTypes: map[string]struct{}{
																		"Address": {},
																	},
																	TypeName: "Address",
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("fullAddress"),
																			Value: &resolve.String{
																				Path: []string{"fullAddress"},
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
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename secretAddress {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
																		Value: &resolve.Scalar{
																			Path: []string{"a"},
																		},
																	},
																	{
																		Name: []byte("b"),
																		Value: &resolve.Scalar{
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
								}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{1},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename country city}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user.account.secretAddress", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("secretAddress")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{1, 2},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename zip}}}","variables":{"representations":[$$0$$]}}}`,
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
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
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
								}, "user.account.secretAddress", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("secretAddress")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           4,
										DependsOnFetchIDs: []int{1, 3},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename secretLine}}}","variables":{"representations":[$$0$$]}}}`,
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
															Name: []byte("zip"),
															Value: &resolve.String{
																Path: []string{"zip"},
															},
															OnTypeNames: [][]byte{[]byte("Address")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
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
								}, "user.account.secretAddress", resolve.ObjectPath("user"), resolve.ObjectPath("account"), resolve.ObjectPath("secretAddress")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("account"),
													Value: &resolve.Object{
														Path:     []string{"account"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
														Fields: []*resolve.Field{
															{
																Name: []byte("secretAddress"),
																Value: &resolve.Object{
																	Path:     []string{"secretAddress"},
																	Nullable: true,
																	PossibleTypes: map[string]struct{}{
																		"Address": {},
																	},
																	TypeName: "Address",
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("secretLine"),
																			Value: &resolve.String{
																				Path: []string{"secretLine"},
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
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
																		Value: &resolve.Scalar{
																			Path: []string{"a"},
																		},
																	},
																	{
																		Name: []byte("b"),
																		Value: &resolve.Scalar{
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
								}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0, 1},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:               `{"method":"POST","url":"http://user.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename deliveryAddress {line1}}}}","variables":{"representations":[$$0$$]}}}`,
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
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
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
																		Value: &resolve.Scalar{
																			Path: []string{"a"},
																		},
																	},
																	{
																		Name: []byte("b"),
																		Value: &resolve.Scalar{
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
								}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("account"),
													Value: &resolve.Object{
														Path:     []string{"account"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
														Fields: []*resolve.Field{
															{
																Name: []byte("deliveryAddress"),
																Value: &resolve.Object{
																	Path:     []string{"deliveryAddress"},
																	Nullable: true,
																	PossibleTypes: map[string]struct{}{
																		"Address": {},
																	},
																	TypeName: "Address",
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
						Debug: plan.DebugConfiguration{},
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
					WithDefaultPostProcessor(),
				)
			})

			t.Run("nested selection set - but requirements are provided", func(t *testing.T) {
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {oldAccount {deliveryAddress {line1} shippingInfo {zip} __typename id info {a b}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("oldAccount"),
													Value: &resolve.Object{
														Path:     []string{"oldAccount"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
														Fields: []*resolve.Field{
															{
																Name: []byte("deliveryAddress"),
																Value: &resolve.Object{
																	Path:     []string{"deliveryAddress"},
																	Nullable: true,
																	PossibleTypes: map[string]struct{}{
																		"Address": {},
																	},
																	TypeName: "Address",
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
						Debug: plan.DebugConfiguration{},
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
					WithDefaultPostProcessor(),
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
					Debug:                        plan.DebugConfiguration{},
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
										},
										FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
											DependsOnFetchIDs: []int{0, 1},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name: []byte("fullName"),
														Value: &resolve.String{
															Path: []string{"fullName"},
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
					)
				})

				t.Run("selected field with requires directive and required fields", func(t *testing.T) {

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
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
												DependsOnFetchIDs: []int{0, 1},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
												},
											},
										},
									},
								},
							},
							planConfiguration,
							WithDefaultPostProcessor(),
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
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
												DependsOnFetchIDs: []int{0, 1},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
												},
											},
										},
									},
								},
							},
							planConfiguration,
							WithDefaultPostProcessor(),
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
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename firstName lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
												DependsOnFetchIDs: []int{0, 1},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
												},
											},
										},
									},
								},
							},
							planConfiguration,
							WithDefaultPostProcessor(),
						)
					})
				})
			})

			t.Run("requires fields on entity with disabled entity resolver", func(t *testing.T) {
				// Usually, we could not plan a query to an entity with disabled entity resolver
				// But there is an exception when the entity has a requires directive
				// in this case we need to receive the keys and required field

				definition := `
					type Query {
						entities: [Entity!]!
					}
					
					type Entity {
						id: ID!
						name: String!
						property: String!
						otherID: ID!
					}
				`

				firstSubgraphSDL := `	
					type Query {
						entities: [Entity!]!
					}
					
					type Entity @key(fields: "id otherID", resolvable: false){
						id: ID!
						name: String! @external
						property: String! @requires(fields: "name")
						otherID: ID! @inaccessible
					}
				`

				firstDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"first-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"entities"},
							},
							{
								TypeName:   "Entity",
								FieldNames: []string{"id", "otherID", "property"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:              "Entity",
									SelectionSet:          "id otherID",
									DisableEntityResolver: true,
								},
							},
							Requires: plan.FederationFieldConfigurations{
								{
									TypeName:     "Entity",
									FieldName:    "property",
									SelectionSet: "name",
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
					type Entity @key(fields: "id otherID"){
						id: ID!
						otherID: ID! @inaccessible
						name: String @inaccessible
					}
				`

				secondDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"second-service",

					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Entity",
								FieldNames: []string{"id", "otherID", "name"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "Entity",
									SelectionSet: "id otherID",
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
						PrintQueryPlans: false,
					},
				}

				t.Run("select field with requires directive", func(t *testing.T) {
					RunWithPermutations(
						t,
						definition,
						`
						query Entities {
							entities {
								property
							}
						}`,
						"Entities",
						&plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entities {__typename id otherID}}"}}`,
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
											RequiresEntityBatchFetch:              true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("Entity")},
															},
															{
																Name: []byte("otherID"),
																Value: &resolve.Scalar{
																	Path: []string{"otherID"},
																},
																OnTypeNames: [][]byte{[]byte("Entity")},
															},
														},
													}),
												},
											},
											PostProcessing: EntitiesPostProcessingConfiguration,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}, "entities", resolve.ArrayPath("entities")),
									resolve.SingleWithPath(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{0, 1},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              true,
											Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename property}}}","variables":{"representations":[$$0$$]}}}`,
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
															{
																Name: []byte("id"),
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("Entity")},
															},
															{
																Name: []byte("otherID"),
																Value: &resolve.Scalar{
																	Path: []string{"otherID"},
																},
																OnTypeNames: [][]byte{[]byte("Entity")},
															},
														},
													}),
												},
											},
											PostProcessing: EntitiesPostProcessingConfiguration,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}, "entities", resolve.ArrayPath("entities")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("entities"),
											Value: &resolve.Array{
												Path: []string{"entities"},
												Item: &resolve.Object{
													Nullable: false,
													PossibleTypes: map[string]struct{}{
														"Entity": {},
													},
													TypeName: "Entity",
													Fields: []*resolve.Field{
														{
															Name: []byte("property"),
															Value: &resolve.String{
																Path: []string{"property"},
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
					)
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
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {firstName __typename id}}"}}`,
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
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
											DependsOnFetchIDs: []int{0, 1},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
														Name: []byte("fullName"),
														Value: &resolve.String{
															Path: []string{"fullName"},
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
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {firstName __typename id}}"}}`,
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
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[$$0$$]}}}`,
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
											DependsOnFetchIDs: []int{0, 1},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											RequiresEntityBatchFetch:              false,
											RequiresEntityFetch:                   true,
											Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName}}}","variables":{"representations":[$$0$$]}}}`,
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
											},
										},
									},
								},
							},
						},
						planConfiguration,
						WithDefaultPostProcessor(),
					)
				})
			})

			t.Run("requires fields from the root query subgraph nested under child node - jump over child node parent entity", func(t *testing.T) {
				definition := `
					type User {
						id: ID!
						firstName: String!
						lastName: String!
						fullName: FullName!
					}
	
					type FullName {
						id: ID!
						fullName: String!
					}

					type UserList {
						id: ID!
						name: String!
						users: ListOfUsers!
					}

					type ListOfUsers {
						users: [User!]!
					}

					type Query {
						list: UserList!
					}
				`

				firstSubgraphSDL := `	
					type User @key(fields: "id") {
						id: ID!
						fullName: FullName! @requires(fields: "firstName lastName")
						firstName: String! @external
						lastName: String! @external
					}
	
					type FullName @key(fields: "id") {
						id: ID!
						fullName: String!
					}

					type UserList @key(fields: "id") {
						id: ID!
						name: String!
					}

					type Query {
						list: UserList!
					}
				`

				firstDatasourceConfiguration := mustDataSourceConfiguration(
					t,
					"first-service",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"list"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "fullName"},
							},
							{
								TypeName:   "UserList",
								FieldNames: []string{"id", "name"},
							},
							{
								TypeName:   "FullName",
								FieldNames: []string{"id", "fullName"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "UserList",
									SelectionSet: "id",
								},
								{
									TypeName:     "FullName",
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

					type UserList @key(fields: "id") {
						id: ID!
						users: ListOfUsers!
					}

					type ListOfUsers {
						users: [User!]!
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
							{
								TypeName:   "UserList",
								FieldNames: []string{"id", "users"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "ListOfUsers",
								FieldNames: []string{"users"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "UserList",
									SelectionSet: "id",
								},
								{
									TypeName:     "FullName",
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
						PrintQueryPlans:               false,
						PrintNodeSuggestions:          false,
						PrintPlanningPaths:            false,
						PrintOperationTransformations: false,
					},
				}

				t.Run("selected only field with requires directive", func(t *testing.T) {
					t.Run("run", func(t *testing.T) {
						RunWithPermutations(
							t,
							definition,
							`
							query User {
								list {
									users {
										users {
											id
											fullName {
												id
												fullName
											}
										}
									}
								}
							}`,
							"User",
							&plan.SynchronousResponsePlan{
								Response: &resolve.GraphQLResponse{
									Fetches: resolve.Sequence(
										resolve.Single(&resolve.SingleFetch{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{list {__typename id}}"}}`,
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
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												PostProcessing:                        SingleEntityPostProcessingConfiguration,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on UserList {__typename users {users {id firstName lastName __typename}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																	OnTypeNames: [][]byte{[]byte("UserList")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.Scalar{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("UserList")},
																},
															},
														}),
													},
												},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}, "list", resolve.ObjectPath("list")),
										resolve.SingleWithPath(&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           2,
												DependsOnFetchIDs: []int{1},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              true,
												RequiresEntityFetch:                   false,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {id fullName}}}}","variables":{"representations":[$$0$$]}}}`,
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
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}, "list.users.users", resolve.ObjectPath("list"), resolve.ObjectPath("users"), resolve.ArrayPath("users")),
									),
									Data: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("list"),
												Value: &resolve.Object{
													Path: []string{"list"},
													PossibleTypes: map[string]struct{}{
														"UserList": {},
													},
													TypeName: "UserList",
													Fields: []*resolve.Field{
														{
															Name: []byte("users"),
															Value: &resolve.Object{
																Path: []string{"users"},
																PossibleTypes: map[string]struct{}{
																	"ListOfUsers": {},
																},
																TypeName: "ListOfUsers",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("users"),
																		Value: &resolve.Array{
																			Path: []string{"users"},
																			Item: &resolve.Object{
																				PossibleTypes: map[string]struct{}{
																					"User": {},
																				},
																				TypeName: "User",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("fullName"),
																						Value: &resolve.Object{
																							Path: []string{"fullName"},
																							PossibleTypes: map[string]struct{}{
																								"FullName": {},
																							},
																							TypeName: "FullName",
																							Fields: []*resolve.Field{
																								{
																									Name: []byte("id"),
																									Value: &resolve.Scalar{
																										Path: []string{"id"},
																									},
																								},
																								{
																									Name: []byte("fullName"),
																									Value: &resolve.String{
																										Path: []string{"fullName"},
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
							WithDefaultPostProcessor(),
						)
					})
				})

				t.Run("selected field with requires directive and required fields", func(t *testing.T) {
					t.Run("run", func(t *testing.T) {
						RunWithPermutations(
							t,
							definition,
							`
							query User {
								list {
									users {
										users {
											id
											fullName {
												id
												fullName
											}
											firstName
											lastName
										}
									}
								}
							}`,
							"User",
							&plan.SynchronousResponsePlan{
								Response: &resolve.GraphQLResponse{
									Fetches: resolve.Sequence(
										resolve.Single(&resolve.SingleFetch{
											FetchConfiguration: resolve.FetchConfiguration{
												Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{list {__typename id}}"}}`,
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
												RequiresEntityBatchFetch:              false,
												RequiresEntityFetch:                   true,
												PostProcessing:                        SingleEntityPostProcessingConfiguration,
												Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on UserList {__typename users {users {id firstName lastName __typename}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																	OnTypeNames: [][]byte{[]byte("UserList")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.Scalar{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("UserList")},
																},
															},
														}),
													},
												},
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}, "list", resolve.ObjectPath("list")),
										resolve.SingleWithPath(&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID:           2,
												DependsOnFetchIDs: []int{1},
											},
											FetchConfiguration: resolve.FetchConfiguration{
												RequiresEntityBatchFetch:              true,
												RequiresEntityFetch:                   false,
												PostProcessing:                        EntitiesPostProcessingConfiguration,
												Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {id fullName}}}}","variables":{"representations":[$$0$$]}}}`,
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
											},
											DataSourceIdentifier: []byte("graphql_datasource.Source"),
										}, "list.users.users", resolve.ObjectPath("list"), resolve.ObjectPath("users"), resolve.ArrayPath("users")),
									),
									Data: &resolve.Object{
										Fields: []*resolve.Field{
											{
												Name: []byte("list"),
												Value: &resolve.Object{
													Path: []string{"list"},
													PossibleTypes: map[string]struct{}{
														"UserList": {},
													},
													TypeName: "UserList",
													Fields: []*resolve.Field{
														{
															Name: []byte("users"),
															Value: &resolve.Object{
																Path: []string{"users"},
																PossibleTypes: map[string]struct{}{
																	"ListOfUsers": {},
																},
																TypeName: "ListOfUsers",
																Fields: []*resolve.Field{
																	{
																		Name: []byte("users"),
																		Value: &resolve.Array{
																			Path: []string{"users"},
																			Item: &resolve.Object{
																				PossibleTypes: map[string]struct{}{
																					"User": {},
																				},
																				TypeName: "User",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("fullName"),
																						Value: &resolve.Object{
																							Path: []string{"fullName"},
																							PossibleTypes: map[string]struct{}{
																								"FullName": {},
																							},
																							TypeName: "FullName",
																							Fields: []*resolve.Field{
																								{
																									Name: []byte("id"),
																									Value: &resolve.Scalar{
																										Path: []string{"id"},
																									},
																								},
																								{
																									Name: []byte("fullName"),
																									Value: &resolve.String{
																										Path: []string{"fullName"},
																									},
																								},
																							},
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
						)
					})
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {oldAccount {name shippingInfo {zip}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("oldAccount"),
													Value: &resolve.Object{
														Path:     []string{"oldAccount"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
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
																	PossibleTypes: map[string]struct{}{
																		"ShippingInfo": {},
																	},
																	TypeName: "ShippingInfo",
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
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}} oldAccount {name shippingInfo {zip}}}}"}}`,
										DataSource:     &Source{},
										PostProcessing: DefaultPostProcessingConfiguration,
									},
								}),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
									FetchConfiguration: resolve.FetchConfiguration{
										Input:                                 `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename name shippingInfo {zip}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
																		Value: &resolve.Scalar{
																			Path: []string{"a"},
																		},
																	},
																	{
																		Name: []byte("b"),
																		Value: &resolve.Scalar{
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
								}, "user.account", resolve.ObjectPath("user"), resolve.ObjectPath("account")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Value: &resolve.Object{
											Path:     []string{"user"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("account"),
													Value: &resolve.Object{
														Path:     []string{"account"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
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
																	PossibleTypes: map[string]struct{}{
																		"ShippingInfo": {},
																	},
																	TypeName: "ShippingInfo",
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
												{
													Name: []byte("oldAccount"),
													Value: &resolve.Object{
														Path:     []string{"oldAccount"},
														Nullable: true,
														PossibleTypes: map[string]struct{}{
															"Account": {},
														},
														TypeName: "Account",
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
																	PossibleTypes: map[string]struct{}{
																		"ShippingInfo": {},
																	},
																	TypeName: "ShippingInfo",
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
					WithDefaultPostProcessor(),
				)
			})

			t.Run("provided fields on entity jump", func(t *testing.T) {
				t.Run("external on a wrapping field", func(t *testing.T) {
					definition := `
						type User {
							id: ID!
							hostedImage: HostedImage!
						}
		
						type HostedImage {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}
		
						type Query {
							user: User!
						}`

					firstSubgraphSDL := `	
						type User @key(fields: "id") {
							id: ID!
						}
		
						type Query {
							user: User 
						}`

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
						type User @key(fields: "id") {
							id: ID!
							hostedImage: HostedImage! @provides(fields: "image {url}")
						}
		
						type HostedImage @key(field: "id") {
							id: ID!
							image: Image! @external
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}`

					secondDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"second-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "User",
									FieldNames: []string{"id", "hostedImage"},
								},
								{
									TypeName:           "HostedImage",
									FieldNames:         []string{"id"},
									ExternalFieldNames: []string{"image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										SelectionSet: "id",
									},
									{
										TypeName:     "HostedImage",
										SelectionSet: "id",
									},
								},
								Provides: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										FieldName:    "hostedImage",
										SelectionSet: "image {url}",
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
						type HostedImage @key(fields: "id") {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}`

					thirdDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"third-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "HostedImage",
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
							PrintQueryPlans: false,
						},
					}

					t.Run("query only provided fields", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												url
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {url}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("query provided fields + __typename", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {__typename url}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("when all fields queried - all non external fields are available under external parent if parent provided", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
												id
												width
												height
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {__typename url id width height}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("width"),
																						Value: &resolve.Integer{
																							Path: []string{"width"},
																						},
																					},
																					{
																						Name: []byte("height"),
																						Value: &resolve.Integer{
																							Path: []string{"height"},
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
							)
						})
					})

				})

				t.Run("external on each field of a type", func(t *testing.T) {
					definition := `
						type User {
							id: ID!
							hostedImage: HostedImage!
						}
		
						type HostedImage {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}
		
						type Query {
							user: User!
						}`

					firstSubgraphSDL := `	
						type User @key(fields: "id") {
							id: ID!
						}
		
						type Query {
							user: User 
						}`

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
						type User @key(fields: "id") {
							id: ID!
							hostedImage: HostedImage! @provides(fields: "image {url}")
						}
		
						type HostedImage @key(field: "id") {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID! @external
							url: String! @external
							width: Int! @external
							height: Int! @external
						}`

					secondDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"second-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "User",
									FieldNames: []string{"id", "hostedImage"},
								},
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:           "Image",
									ExternalFieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										SelectionSet: "id",
									},
									{
										TypeName:     "HostedImage",
										SelectionSet: "id",
									},
								},
								Provides: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										FieldName:    "hostedImage",
										SelectionSet: "image {url}",
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
						type HostedImage @key(fields: "id") {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}`

					thirdDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"third-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "HostedImage",
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
							PrintQueryPlans: false,
						},
					}

					t.Run("query only provided fields", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												url
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {url}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("query provided fields + __typename", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {__typename url}}}}}","variables":{"representations":[$$0$$]}}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("when all fields queried - fetch possible provided fields and get rest from the other subgraph", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
												id
												width
												height
											}
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
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {__typename url} __typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
													Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {id width height}}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																},
															}),
														},
													},
													PostProcessing: SingleEntityPostProcessingConfiguration,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("width"),
																						Value: &resolve.Integer{
																							Path: []string{"width"},
																						},
																					},
																					{
																						Name: []byte("height"),
																						Value: &resolve.Integer{
																							Path: []string{"height"},
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
							)
						})
					})

				})
			})

			t.Run("provided fields in a same subgraph", func(t *testing.T) {
				t.Run("external on a wrapping field", func(t *testing.T) {
					definition := `
						type User {
							id: ID!
							hostedImage: HostedImage!
						}
		
						type HostedImage {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}
		
						type Query {
							user: User!
						}`

					firstSubgraphSDL := `
						type Query {
							user: User!
						}
	
						type User @key(fields: "id") {
							id: ID!
							hostedImage: HostedImage! @provides(fields: "image {url}")
						}
		
						type HostedImage @key(field: "id") {
							id: ID!
							image: Image! @external
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
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
									FieldNames: []string{"id", "hostedImage"},
								},
								{
									TypeName:           "HostedImage",
									FieldNames:         []string{"id"},
									ExternalFieldNames: []string{"image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										SelectionSet: "id",
									},
									{
										TypeName:     "HostedImage",
										SelectionSet: "id",
									},
								},
								Provides: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										FieldName:    "hostedImage",
										SelectionSet: "image {url}",
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

					// subgraph is here to not have uniq nodes in a query
					secondSubgraphSDL := `		
						type HostedImage {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							width: Int!
							height: Int!
						}`

					secondDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"second-service",
						&plan.DataSourceMetadata{
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
								{
									TypeName:           "Image",
									ExternalFieldNames: []string{"id", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{},
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
						type HostedImage @key(fields: "id") {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}`

					thirdDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"third-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "HostedImage",
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
							PrintQueryPlans: false,
						},
					}

					t.Run("query only provided fields", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												url
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {url}}}}"}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("query provided fields + __typename", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {__typename url}}}}"}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("when all fields queried - all non external fields are available under external parent if parent provided", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
												id
												width
												height
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {__typename url id width height}}}}"}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("width"),
																						Value: &resolve.Integer{
																							Path: []string{"width"},
																						},
																					},
																					{
																						Name: []byte("height"),
																						Value: &resolve.Integer{
																							Path: []string{"height"},
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
							)
						})
					})

				})

				t.Run("external on a fields of a type", func(t *testing.T) {
					definition := `
						type User {
							id: ID!
							hostedImage: HostedImage!
						}
		
						type HostedImage {
							id: ID!
							image: Image!
							hosting: Hosting!
						}
		
						type Hosting{
							id: ID!
							category: String!
							name: String!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}
		
						type Query {
							user: User!
						}`

					firstSubgraphSDL := `
						type Query {
							user: User!
						}

						type User @key(fields: "id") {
							id: ID!
							hostedImage: HostedImage! @provides(fields: "image {url} hosting {id}")
						}
		
						type HostedImage @key(field: "id") {
							id: ID!
							image: Image!
							hosting: Hosting!
						}
		
						type Hosting @key(fields: "category") {
							id: ID! # NOTE: this field is provided but it is not external
							category: String!
						}

						type Image {
							id: ID! @external
							url: String! @external
							width: Int! @external
							height: Int! @external
						}`

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
									FieldNames: []string{"id", "hostedImage"},
								},
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image", "hosting"},
								},
								{
									TypeName:   "Hosting",
									FieldNames: []string{"id", "category"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:           "Image",
									ExternalFieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										SelectionSet: "id",
									},
									{
										TypeName:     "HostedImage",
										SelectionSet: "id",
									},
									{
										TypeName:     "Hosting",
										SelectionSet: "id category",
									},
								},
								Provides: plan.FederationFieldConfigurations{
									{
										TypeName:     "User",
										FieldName:    "hostedImage",
										SelectionSet: "image {url} hosting {id}",
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
						type Hosting @key(fields: "id category") {
							id: ID!
							category: String!
							name: String!
						}`

					secondDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"second-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Hosting",
									FieldNames: []string{"id", "category", "name"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "Hosting",
										SelectionSet: "id category",
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
						type HostedImage @key(fields: "id") {
							id: ID!
							image: Image!
						}
		
						type Image {
							id: ID!
							url: String!
							width: Int!
							height: Int!
						}`

					thirdDatasourceConfiguration := mustDataSourceConfiguration(
						t,
						"third-service",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "HostedImage",
									FieldNames: []string{"id", "image"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Image",
									FieldNames: []string{"id", "url", "width", "height"},
								},
							},
							FederationMetaData: plan.FederationMetaData{
								Keys: plan.FederationFieldConfigurations{
									{
										TypeName:     "HostedImage",
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
							PrintQueryPlans: false,
						},
					}

					t.Run("query only provided fields", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												url
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {url}}}}"}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("query provided fields + __typename", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {__typename url}}}}"}}`,
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
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
							)
						})
					})

					t.Run("query all image fields", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
												id
												width
												height
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {__typename url} __typename id}}}"}}`,
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
													Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {id width height}}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																},
															}),
														},
													},
													PostProcessing: SingleEntityPostProcessingConfiguration,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("width"),
																						Value: &resolve.Integer{
																							Path: []string{"width"},
																						},
																					},
																					{
																						Name: []byte("height"),
																						Value: &resolve.Integer{
																							Path: []string{"height"},
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
							)
						})
					})

					t.Run("query all image fields + hosting", func(t *testing.T) {
						t.Run("run", func(t *testing.T) {
							RunWithPermutations(
								t,
								definition,
								`
								query User {
									user {
										hostedImage {
											image {
												__typename
												url
												id
												width
												height
											}
											hosting {
												category
												name
											}
										}
									}
								}`,
								"User",
								&plan.SynchronousResponsePlan{
									Response: &resolve.GraphQLResponse{
										Fetches: resolve.Sequence(
											resolve.Single(&resolve.SingleFetch{
												FetchConfiguration: resolve.FetchConfiguration{
													Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {image {__typename url} hosting {category __typename id} __typename id}}}"}}`,
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
													Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {id width height}}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("HostedImage")},
																	},
																},
															}),
														},
													},
													PostProcessing: SingleEntityPostProcessingConfiguration,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
											resolve.SingleWithPath(&resolve.SingleFetch{
												FetchDependencies: resolve.FetchDependencies{
													FetchID:           2,
													DependsOnFetchIDs: []int{0},
												}, FetchConfiguration: resolve.FetchConfiguration{
													RequiresEntityBatchFetch:              false,
													RequiresEntityFetch:                   true,
													Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Hosting {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
																		OnTypeNames: [][]byte{[]byte("Hosting")},
																	},
																	{
																		Name: []byte("id"),
																		Value: &resolve.Scalar{
																			Path: []string{"id"},
																		},
																		OnTypeNames: [][]byte{[]byte("Hosting")},
																	},
																	{
																		Name: []byte("category"),
																		Value: &resolve.String{
																			Path: []string{"category"},
																		},
																		OnTypeNames: [][]byte{[]byte("Hosting")},
																	},
																},
															}),
														},
													},
													PostProcessing: SingleEntityPostProcessingConfiguration,
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
											}, "user.hostedImage.hosting", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage"), resolve.ObjectPath("hosting")),
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
																Name: []byte("hostedImage"),
																Value: &resolve.Object{
																	Path:     []string{"hostedImage"},
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
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path:       []string{"__typename"},
																							IsTypeName: true,
																						},
																					},
																					{
																						Name: []byte("url"),
																						Value: &resolve.String{
																							Path: []string{"url"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &resolve.Scalar{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("width"),
																						Value: &resolve.Integer{
																							Path: []string{"width"},
																						},
																					},
																					{
																						Name: []byte("height"),
																						Value: &resolve.Integer{
																							Path: []string{"height"},
																						},
																					},
																				},
																			},
																		},
																		{
																			Name: []byte("hosting"),
																			Value: &resolve.Object{
																				Path: []string{"hosting"},
																				PossibleTypes: map[string]struct{}{
																					"Hosting": {},
																				},
																				TypeName: "Hosting",
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("category"),
																						Value: &resolve.String{
																							Path: []string{"category"},
																						},
																					},
																					{
																						Name: []byte("name"),
																						Value: &resolve.String{
																							Path: []string{"name"},
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
							)
						})
					})

				})
			})

		})
	})

	t.Run("external composite keys", func(t *testing.T) {
		definition := `
				type User {
					details: Details!
				}
	
				type Details {
					forename: String!
					surname: String!
					middlename: String!
				}
	
				type Query {
					me: User
				}
			`

		firstSubgraphSDL := `
				type User @key(fields: "details {forename surname}") {
					details: Details! @external
				}
	
				type Details @key(fields: "forename surname"){
					forename: String! @external
					surname: String! @external
					middlename: String! @shareable
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
						TypeName:           "User",
						FieldNames:         []string{"details"},
						ExternalFieldNames: []string{"details"},
					},
					{
						TypeName:           "Details",
						FieldNames:         []string{"forename", "middlename", "surname"},
						ExternalFieldNames: []string{"forename", "surname"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "details {forename surname}",
						},
						{
							TypeName:     "Details",
							SelectionSet: "forename surname",
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
				type User @key(fields: "details {forename surname}") {
					details: Details!
				}
	
				type Details @key(fields: "forename surname"){
					forename: String!
					surname: String!
					middlename: String! @shareable
				}
			`

		secondDatasourceConfiguration := mustDataSourceConfiguration(
			t,
			"second-service",

			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"details"},
					},
					{
						TypeName:   "Details",
						FieldNames: []string{"forename", "middlename", "surname"},
					},
				},

				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "details {forename surname}",
						},
						{
							TypeName:     "Details",
							SelectionSet: "forename surname",
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

		t.Run("query __typename", func(t *testing.T) {
			query := `
					query Me {
						me {
							details {
								__typename
							}
						}
					}
				`

			expectedPlan := &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {__typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("details"),
											Value: &resolve.Object{
												Path: []string{"details"},
												PossibleTypes: map[string]struct{}{
													"Details": {},
												},
												TypeName: "Details",
												Fields: []*resolve.Field{
													{
														Name: []byte("__typename"),
														Value: &resolve.String{
															Path:       []string{"__typename"},
															IsTypeName: true,
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

			RunWithPermutations(
				t,
				definition,
				query,
				"Me",
				expectedPlan,
				planConfiguration,
				WithDefaultPostProcessor(),
			)
		})

		t.Run("query key fields", func(t *testing.T) {
			query := `
					query Me {
						me {
							details {
								forename
								surname
							}
						}
					}
				`

			expectedPlan := &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename surname}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("details"),
											Value: &resolve.Object{
												Path: []string{"details"},
												PossibleTypes: map[string]struct{}{
													"Details": {},
												},
												TypeName: "Details",
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
			}

			RunWithPermutations(
				t,
				definition,
				query,
				"Me",
				expectedPlan,
				planConfiguration,
				WithDefaultPostProcessor(),
			)
		})

		t.Run("query not external field on external path", func(t *testing.T) {
			query := `
					query Me {
						me {
							details {
								middlename
							}
						}
					}
				`

			expectedPlan := &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {middlename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("details"),
											Value: &resolve.Object{
												Path: []string{"details"},
												PossibleTypes: map[string]struct{}{
													"Details": {},
												},
												TypeName: "Details",
												Fields: []*resolve.Field{
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
								},
							},
						},
					},
				},
			}

			RunWithPermutations(
				t,
				definition,
				query,
				"Me",
				expectedPlan,
				planConfiguration,
				WithDefaultPostProcessor(),
			)
		})

		t.Run("query all fields", func(t *testing.T) {
			query := `
					query Me {
						me {
							details {
								__typename
								forename
								surname
								middlename
							}
						}
					}
				`

			expectedPlan := &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {__typename forename surname middlename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Path:     []string{"me"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"User": {},
									},
									TypeName: "User",
									Fields: []*resolve.Field{
										{
											Name: []byte("details"),
											Value: &resolve.Object{
												Path: []string{"details"},
												PossibleTypes: map[string]struct{}{
													"Details": {},
												},
												TypeName: "Details",
												Fields: []*resolve.Field{
													{
														Name: []byte("__typename"),
														Value: &resolve.String{
															Path:       []string{"__typename"},
															IsTypeName: true,
														},
													},
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
								},
							},
						},
					},
				},
			}

			RunWithPermutations(
				t,
				definition,
				query,
				"Me",
				expectedPlan,
				planConfiguration,
				WithDefaultPostProcessor(),
			)
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
				Debug:                        plan.DebugConfiguration{},
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          input,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("me"),
										Value: &resolve.Object{
											Path:     []string{"me"},
											Nullable: true,
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*resolve.Field{
												{
													Name: []byte("details"),
													Value: &resolve.Object{
														Path: []string{"details"},
														PossibleTypes: map[string]struct{}{
															"Details": {},
														},
														TypeName: "Details",
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
					WithDefaultPostProcessor(),
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
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname}}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									}),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															PossibleTypes: map[string]struct{}{
																"Details": {},
															},
															TypeName: "Details",
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
						WithDefaultPostProcessor(),
					)
				})

				t.Run("resolve from two subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											},
										},
									}, "me", resolve.ObjectPath("me")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															PossibleTypes: map[string]struct{}{
																"Details": {},
															},
															TypeName: "Details",
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
											},
										},
									},
								},
							},
						}
					}

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {surname}}}}","variables":{"representations":[$$0$$]}}}`)

					variant2 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {middlename}}}}","variables":{"representations":[$$0$$]}}}`)

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
						WithDefaultPostProcessor(),
					)
				})

				t.Run("resolve from two subgraphs - not shared field", func(t *testing.T) {
					expectedPlan := func(input1, input2 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											},
										},
									}, "me", resolve.ObjectPath("me")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															PossibleTypes: map[string]struct{}{
																"Details": {},
															},
															TypeName: "Details",
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
											},
										},
									},
								},
							},
						}
					}

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {__typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)
					variant2 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {__typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
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
						WithDefaultPostProcessor(),
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
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											},
										},
									}, "me", resolve.ObjectPath("me")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															PossibleTypes: map[string]struct{}{
																"Details": {},
															},
															TypeName: "Details",
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
							`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						),
						planConfiguration,
						WithDefaultPostProcessor(),
					)
				})

				t.Run("resolve from three subgraphs", func(t *testing.T) {
					expectedPlan := func(input1, input2, input3 string) *plan.SynchronousResponsePlan {
						return &plan.SynchronousResponsePlan{
							Response: &resolve.GraphQLResponse{
								Fetches: resolve.Sequence(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          input1,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
									}),
									resolve.SingleWithPath(&resolve.SingleFetch{
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											},
										},
									}, "me", resolve.ObjectPath("me")),
									resolve.SingleWithPath(&resolve.SingleFetch{
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
																Value: &resolve.Scalar{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											},
										},
									}, "me", resolve.ObjectPath("me")),
								),
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("me"),
											Value: &resolve.Object{
												Path:     []string{"me"},
												Nullable: true,
												PossibleTypes: map[string]struct{}{
													"User": {},
												},
												TypeName: "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("details"),
														Value: &resolve.Object{
															Path: []string{"details"},
															PossibleTypes: map[string]struct{}{
																"Details": {},
															},
															TypeName: "Details",
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
											},
										},
									},
								},
							},
						}
					}

					variant1 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {surname}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant2 := expectedPlan(
						`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {surname}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant3 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {middlename}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
					)

					variant4 := expectedPlan(
						`{"method":"POST","url":"http://second.service","body":{"query":"{me {details {forename surname} __typename id}}"}}`,
						`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {age}}}}","variables":{"representations":[$$0$$]}}}`,
						`{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename details {middlename}}}}","variables":{"representations":[$$0$$]}}}`,
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
						WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {__typename ... on User {__typename id} ... on Admin {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name} ... on Admin {__typename adminName}}}","variables":{"representations":[$$0$$]}}}`,
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
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Admin")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
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
								}, "account", resolve.ObjectPath("account")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Admin": {},
												"User":  {},
											},
											TypeName: "Node",
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {some {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "account.some", resolve.ObjectPath("account"), resolve.ObjectPath("some")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Admin": {},
												"User":  {},
											},
											TypeName: "Node",
											Fields: []*resolve.Field{
												{
													Name: []byte("some"),
													Value: &resolve.Object{
														Path: []string{"some"},
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {__typename ... on Admin {__typename id} ... on User {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "account", resolve.ObjectPath("account")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Admin": {},
												"User":  {},
											},
											TypeName: "Node",
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
				)
			})
		})

		t.Run("union + interface - union member is not implementing interface in one subgraph", func(t *testing.T) {
			definition := `
				type User implements Node {
					id: ID!
					name: String!
				}

				type Admin implements Node {
					id: ID!
					adminName: String!
				}

				interface Node {
					id: ID!
				}

				union Account = User | Admin

				type Query {
					account: Account!
				}
			`

			firstSubgraphSDL := `	
				type User implements Node @key(fields: "id") {
					id: ID!
				}

				type Admin @key(fields: "id") {
					id: ID!
				}

				interface Node {
					id: ID!
				}

				union Account = User | Admin

				type Query {
					account: Account
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
							FieldNames: []string{"id"},
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
				type Admin implements Node @key(fields: "id") {
					id: ID!
					adminName: String!
				}

				interface Node {
					id: ID!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Admin",
							FieldNames: []string{"id", "adminName"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Node",
							FieldNames: []string{"id"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
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

			t.Run("query with fragment on interface - need to expand", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
					query Accounts {
						account {
							... on Node {
								id
							}
						}
					}
				`,
					"Accounts",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{account {__typename ... on Admin {id} ... on User {id}}}"}}`,
										PostProcessing: DefaultPostProcessingConfiguration,
										DataSource:     &Source{},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Admin": {},
												"User":  {},
											},
											TypeName: "Account",
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Admin")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("User")},
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
					address: Address!
				}
	
				union Account = User | Admin | Moderator
				
				interface Node {
					title: String!
				}

				type Query {
					accounts: [Account!]
					nodes: [Node!]
				}

				type Address {
					id: ID!
					zip: String!
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

				type Address @key(fields: "id") {
					id: ID!
					zip: String!
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
						{
							TypeName:   "Address",
							FieldNames: []string{"id", "zip"},
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
							{
								TypeName:     "Address",
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
					address: Address!
				}

				type Address @key(fields: "id") {
					id: ID!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",

				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Moderator",
							FieldNames: []string{"moderatorID", "subject", "title", "address"},
						},
						{
							TypeName:   "Address",
							FieldNames: []string{"id"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Moderator",
								SelectionSet: "moderatorID",
							},
							{
								TypeName:     "Address",
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{accounts {__typename ... on User {__typename id} ... on Admin {__typename adminID} ... on Moderator {__typename moderatorID}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name} ... on Admin {__typename adminName}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
															Value: &resolve.Scalar{
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
								}, "accounts", resolve.ArrayPath("accounts")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Moderator {__typename subject}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "accounts", resolve.ArrayPath("accounts")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("accounts"),
										Value: &resolve.Array{
											Path:     []string{"accounts"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"Admin":     {},
													"User":      {},
													"Moderator": {},
												},
												TypeName: "Account",
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
											},
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
				)
			})

			t.Run("test nested union query with propagated operation name", func(t *testing.T) {
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
								address {
									zip
								}
							}
						}
					}
				`,
					"Accounts",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"query Accounts__first_service {accounts {__typename ... on User {__typename id} ... on Admin {__typename adminID} ... on Moderator {__typename moderatorID}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query Accounts__second_service($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name} ... on Admin {__typename adminName}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
															Value: &resolve.Scalar{
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
								}, "accounts", resolve.ArrayPath("accounts")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query Accounts__third_service($representations: [_Any!]!){_entities(representations: $representations){... on Moderator {__typename subject address {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "accounts", resolve.ArrayPath("accounts")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{2},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query Accounts__first_service($representations: [_Any!]!){_entities(representations: $representations){... on Address {__typename zip}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("Address")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Address")},
														},
													},
												}),
											},
										},
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "accounts.@.address", resolve.ArrayPath("accounts"), resolve.PathElementWithTypeNames(resolve.ObjectPath("address"), []string{"Moderator"})),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("accounts"),
										Value: &resolve.Array{
											Path:     []string{"accounts"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"Admin":     {},
													"Moderator": {},
													"User":      {},
												},
												TypeName: "Account",
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
													{
														Name: []byte("address"),
														Value: &resolve.Object{
															Path: []string{"address"},
															PossibleTypes: map[string]struct{}{
																"Address": {},
															},
															TypeName: "Address",
															Fields: []*resolve.Field{
																{
																	Name: []byte("zip"),
																	Value: &resolve.String{
																		Path: []string{"zip"},
																	},
																},
															},
														},
														OnTypeNames: [][]byte{[]byte("Moderator")},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					plan.Configuration{
						DataSources: []plan.DataSource{
							firstDatasourceConfiguration,
							secondDatasourceConfiguration,
							thirdDatasourceConfiguration,
						},
						DisableResolveFieldPositions:   true,
						EnableOperationNamePropagation: true,
					},
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{nodes {__typename ... on Admin {title} ... on Moderator {__typename moderatorID} ... on User {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Moderator {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "nodes", resolve.ArrayPath("nodes")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									}, FetchConfiguration: resolve.FetchConfiguration{
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "nodes", resolve.ArrayPath("nodes")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("nodes"),
										Value: &resolve.Array{
											Path:     []string{"nodes"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: false,
												PossibleTypes: map[string]struct{}{
													"Admin":     {},
													"User":      {},
													"Moderator": {},
												},
												TypeName: "Node",
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
											},
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name uuid}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename address {country}}}}","variables":{"representations":[$$0$$]}}}`,
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
									FetchID:           3,
									DependsOnFetchIDs: []int{1},
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
													PossibleTypes: map[string]struct{}{
														"Address": {},
													},
													TypeName: "Address",
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
								},
							},
						},
					},
				},
				planConfiguration,
				WithDefaultPostProcessor(),
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
				Debug:                        plan.DebugConfiguration{},
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {field1 __typename key1}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename field2 key2}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{1},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename field3 key3}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{2},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename field4}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
				)
			})

			t.Run("field from the last subgraph in a chain", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
				query User {
					user {
						field4
					}
				}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {__typename key1}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename key2}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{1},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename key3}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{2},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename field4}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
													Name: []byte("field4"),
													Value: &resolve.String{
														Path: []string{"field4"},
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {key1 field1 __typename}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename key2 field2}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{1},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename key3 field3}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "user", resolve.ObjectPath("user")),
								resolve.SingleWithPath(&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           3,
										DependsOnFetchIDs: []int{2},
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename field4}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
									}, FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityBatchFetch:              false,
										RequiresEntityFetch:                   true,
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{userWithName {name __typename id}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "userWithName", resolve.ObjectPath("userWithName")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("userWithName"),
										Value: &resolve.Object{
											Path:     []string{"userWithName"},
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
					planConfiguration,
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entity {id name __typename}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename age}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "entity", resolve.ObjectPath("entity")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("entity"),
										Value: &resolve.Object{
											Path:     []string{"entity"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Entity": {},
											},
											TypeName: "Entity",
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entity {id name isEntity __typename}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename age}}}","variables":{"representations":[$$0$$]}}}`,
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
															Value: &resolve.Scalar{
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
								}, "entity", resolve.ObjectPath("entity")),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("entity"),
										Value: &resolve.Object{
											Path:     []string{"entity"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Entity": {},
											},
											TypeName: "Entity",
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
										},
									},
								},
							},
						},
					},
					planConfiguration,
					WithDefaultPostProcessor(),
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
				
				type Entity @key(fields: "id") {
					id: ID!
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
							FieldNames: []string{"id", "isEntity"},
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

				type Entity @key(fields: "id") @key(fields: "name", resolvable: false) @key(fields: "uuid") {
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
								TypeName:     "Entity",
								SelectionSet: "id",
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
				
				type Entity @key(fields: "name") @key(fields: "uuid", resolvable: false) {
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
						FieldNames: []string{"uuid", "name", "age", "isImportant"},
					},
				},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
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
				Debug: plan.DebugConfiguration{
					PrintQueryPlans: true,
				},
			}

			t.Run("query all possible fields", func(t *testing.T) {
				entityOneNestedFetch2Second := func(fetchID int, variantOne bool) resolve.Fetch {
					var entitySelectionSet string
					if variantOne {
						entitySelectionSet = "uuid name age rating"
					} else {
						entitySelectionSet = "name rating"
					}

					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{0},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename ` + entitySelectionSet + `}}}","variables":{"representations":[$$0$$]}}}`,
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
												Value: &resolve.Scalar{
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
				entityOneNestedFetch2Third := func(fetchID int, variantOne bool) resolve.Fetch {
					var entitySelectionSet string
					if variantOne {
						entitySelectionSet = "isImportant"
					} else {
						entitySelectionSet = "uuid age isImportant"
					}

					return &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{
							FetchID:           fetchID,
							DependsOnFetchIDs: []int{1},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename ` + entitySelectionSet + `}}}","variables":{"representations":[$$0$$]}}}`,
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
							Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename isEntity}}}","variables":{"representations":[$$0$$]}}}`,
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
												Value: &resolve.Scalar{
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
							Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename isImportant}}}","variables":{"representations":[$$0$$]}}}`,
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
							DependsOnFetchIDs: []int{7},
						}, FetchConfiguration: resolve.FetchConfiguration{
							RequiresEntityBatchFetch:              false,
							RequiresEntityFetch:                   true,
							Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename isEntity}}}","variables":{"representations":[$$0$$]}}}`,
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
												Value: &resolve.Scalar{
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
							Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename id rating}}}","variables":{"representations":[$$0$$]}}}`,
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
												Value: &resolve.Scalar{
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
							Fetches: resolve.Sequence(
								resolve.Parallel(
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 0,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entityOne {id isEntity __typename}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 3,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://second.service","body":{"query":"{entityTwo {id uuid name age rating __typename}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
									resolve.Single(&resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 6,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input:          `{"method":"POST","url":"http://third.service","body":{"query":"{entityThree {uuid name age isImportant __typename}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
											DataSource:     &Source{},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									}),
								),
								resolve.Parallel(
									resolve.SingleWithPath(entityOneFetchOne, "entityOne", resolve.ObjectPath("entityOne")),
									resolve.SingleWithPath(entityTwoFetchOne, "entityTwo", resolve.ObjectPath("entityTwo")),
									resolve.SingleWithPath(entityTwoFetchTwo, "entityTwo", resolve.ObjectPath("entityTwo")),
									resolve.SingleWithPath(entityThreeFetchOne, "entityThree", resolve.ObjectPath("entityThree")),
								),
								resolve.Parallel(
									resolve.SingleWithPath(entityOneFetchTwo, "entityOne", resolve.ObjectPath("entityOne")),
									resolve.SingleWithPath(entityThreeFetchTwo, "entityThree", resolve.ObjectPath("entityThree")),
								),
							),
							Data: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("entityOne"),
										Value: &resolve.Object{
											Path:     []string{"entityOne"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Entity": {},
											},
											TypeName: "Entity",
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("uuid"),
													Value: &resolve.Scalar{
														Path: []string{"uuid"},
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
										},
									},
									{
										Name: []byte("entityTwo"),
										Value: &resolve.Object{
											Path:     []string{"entityTwo"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Entity": {},
											},
											TypeName: "Entity",
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("uuid"),
													Value: &resolve.Scalar{
														Path: []string{"uuid"},
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
										},
									},
									{
										Name: []byte("entityThree"),
										Value: &resolve.Object{
											Path:     []string{"entityThree"},
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Entity": {},
											},
											TypeName: "Entity",
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("uuid"),
													Value: &resolve.Scalar{
														Path: []string{"uuid"},
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
										},
									},
								},
							},
						},
					}
				}

				variant1 := expectedPlan(
					entityOneNestedFetch2Second(1, true), entityOneNestedFetch2Third(2, true),
					entityTwoNestedFetch2First(4), entityTwoNestedFetch2Third(5),
					entityThreeNestedFetch2Second(7), entityThreeNestedFetch2First(8),
				)

				variant2 := expectedPlan(
					entityOneNestedFetch2Second(1, false), entityOneNestedFetch2Third(2, false),
					entityTwoNestedFetch2First(4), entityTwoNestedFetch2Third(5),
					entityThreeNestedFetch2Second(7), entityThreeNestedFetch2First(8),
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
								uuid
								name
								age
								isEntity
								isImportant
								rating
							}
							entityTwo {
								id
								uuid
								name
								age
								isEntity
								isImportant
								rating
							}
							entityThree {
								id
								uuid
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
					WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableOrderSequenceByDependencies(), postprocess.DisableMergeFields()),
				)
			})

			t.Run("query last field in a chain first-second-third", func(t *testing.T) {
				expectedPlan := &plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{entityOne {__typename id}}"}}`,
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
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
							}, "entityOne", resolve.ObjectPath("entityOne")),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{1},
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename isImportant}}}","variables":{"representations":[$$0$$]}}}`,
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
							}, "entityOne", resolve.ObjectPath("entityOne")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("entityOne"),
									Value: &resolve.Object{
										Path:     []string{"entityOne"},
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"Entity": {},
										},
										TypeName: "Entity",
										Fields: []*resolve.Field{
											{
												Name: []byte("isImportant"),
												Value: &resolve.Boolean{
													Path: []string{"isImportant"},
												},
											},
										},
									},
								},
							},
						},
					},
				}

				RunWithPermutations(
					t,
					definition,
					`
						query Query {
							entityOne {
								isImportant
							}
						}
					`,
					"Query",
					expectedPlan,
					planConfiguration,
					WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableOrderSequenceByDependencies(), postprocess.DisableMergeFields()),
				)
			})

			t.Run("query last field in a chain third-second-first", func(t *testing.T) {
				expectedPlan := &plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://third.service","body":{"query":"{entityThree {__typename uuid}}"}}`,
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
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename id}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
							}, "entityThree", resolve.ObjectPath("entityThree")),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{1},
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://first.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Entity {__typename isEntity}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
							}, "entityThree", resolve.ObjectPath("entityThree")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("entityThree"),
									Value: &resolve.Object{
										Path:     []string{"entityThree"},
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"Entity": {},
										},
										TypeName: "Entity",
										Fields: []*resolve.Field{
											{
												Name: []byte("isEntity"),
												Value: &resolve.Boolean{
													Path: []string{"isEntity"},
												},
											},
										},
									},
								},
							},
						},
					},
				}

				RunWithPermutations(
					t,
					definition,
					`
						query Query {
							entityThree {
								isEntity
							}
						}
					`,
					"Query",
					expectedPlan,
					planConfiguration,
					WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableOrderSequenceByDependencies(), postprocess.DisableMergeFields()),
				)
			})
		})
	})

	t.Run("field alias", func(t *testing.T) {
		definition := `
				type User {
					id: ID!
					userID: ID!
					title: String!
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
				type User @key(fields: "id") {
					id: ID!
					title: String!
					userID: ID!
				}
			`

		secondDatasourceConfiguration := mustDataSourceConfiguration(
			t,
			"second-service",

			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "title", "userID"},
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
			Debug:                        plan.DebugConfiguration{},
		}

		t.Run("properly select userID aliased as ID", func(t *testing.T) {
			RunWithPermutations(
				t,
				definition,
				`
						query User {
							user {
								id: userID
								title
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
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id: userID title}}}","variables":{"representations":[$$0$$]}}}`,
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
			)
		})

		t.Run("aliased root fields", func(t *testing.T) {
			RunWithPermutations(
				t,
				definition,
				`
						query User {
							userA: user {
								id
								title
							}
							userB: user {
								id
								title
							}
						}`,
				"User",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{userA: user {id __typename} userB: user {id __typename}}"}}`,
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
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
							}, "userA", resolve.ObjectPath("userA")),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           2,
									DependsOnFetchIDs: []int{0},
								}, FetchConfiguration: resolve.FetchConfiguration{
									RequiresEntityBatchFetch:              false,
									RequiresEntityFetch:                   true,
									Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
							}, "userB", resolve.ObjectPath("userB")),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("userA"),
									Value: &resolve.Object{
										Path:     []string{"userA"},
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"User": {},
										},
										TypeName: "User",
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
											},
										},
									},
								},
								{
									Name: []byte("userB"),
									Value: &resolve.Object{
										Path:     []string{"userB"},
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"User": {},
										},
										TypeName: "User",
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
			)
		})
	})

	t.Run("external edge cases", func(t *testing.T) {
		t.Run("conditional keys - provides on entity", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					name: String!
					title: String!
					hostedImage: HostedImage!
					hostedImageWithProvides: HostedImage!
				}

				type HostedImage {
					id: ID!
					host: String!
					image: Image!
				}

				type Image {
					id: ID!
					url: String!
					cdnUrl: String!
				}

				type Query {
					user: User!
				}
			`

			firstSubgraphSDL := `	
				type User @key(fields: "id") {
					id: ID!
					title: String!
					hostedImage: HostedImage!
				}

				type HostedImage @key(fields: "id") {
					id: ID!
					host: String!
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
							FieldNames: []string{"id", "title", "hostedImage"},
						},
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id", "host"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:     "HostedImage",
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
					hostedImageWithProvides: HostedImage! @provides(fields: "image {id url}")
				}

				type HostedImage @key(fields: "id") {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID! @external
					url: String! @external
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "hostedImageWithProvides"},
						},
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id", "image"},
						},
						{
							TypeName: "Image",
							// image fields listed in both fields and external fields
							// because they could be used as a key when they provided, so they become a root node
							// but this root node is conditional and determined by conditional key
							FieldNames:         []string{"id", "url"},
							ExternalFieldNames: []string{"id", "url"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:     "Image",
								SelectionSet: "id",
								Conditions: []plan.KeyCondition{
									{
										Coordinates: []plan.KeyConditionCoordinate{
											{
												TypeName:  "User",
												FieldName: "hostedImageWithProvides",
											},
											{
												TypeName:  "HostedImage",
												FieldName: "image",
											},
											{
												TypeName:  "Image",
												FieldName: "id",
											},
										},
										FieldPath: []string{"hostedImageWithProvides", "image", "id"},
									},
								},
								DisableEntityResolver: true,
							},
							{
								TypeName:     "HostedImage",
								SelectionSet: "id",
							},
						},
						Provides: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								FieldName:    "hostedImageWithProvides",
								SelectionSet: "image {id url}",
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
				type HostedImage @key(fields: "id") {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID!
					url: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id", "image"},
						},
						{
							TypeName:   "Image",
							FieldNames: []string{"id", "url"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "HostedImage",
								SelectionSet: "id",
							},
							{
								TypeName:              "Image",
								SelectionSet:          "id",
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

			fourthSubgraphSDL := `
				type Image @key(fields: "id") {
					id: ID!
					cdnUrl: String!
				}
			`

			fourthDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"fourth-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Image",
							FieldNames: []string{"id", "cdnUrl"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Image",
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
				Debug: plan.DebugConfiguration{
					PrintQueryPlans: false,
				},
			}

			t.Run("query provided external fields and use them as a conditional implicit key", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								hostedImageWithProvides {
									image {
										cdnUrl
									}
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
				)
			})

			t.Run("do not query external conditional fields - Image.id key field is present in a query", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										id
										cdnUrl
									}
								}
							}
						}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {id __typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
								}, "user.hostedImage.image", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage"), resolve.ObjectPath("image")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
																			Name: []byte("id"),
																			Value: &resolve.Scalar{
																				Path: []string{"id"},
																			},
																		},
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
				)
			})

			t.Run("do not query external conditional fields - No Image.id key in a query", func(t *testing.T) {
				/*
					Tricky edge case

					from the first subgraph we should get user with hostedImage.id
					this should allow us to jump to third subgraph to get Image.id - third subgraph is the only place from where we could get it

					the problem here - at the first iterations we don't know yet that we should select image from some of the subgraphs, as it doesn't have selectable fields
					because cdnUrl coming from the different subgraph
				*/

				// TODO: implement same kind of test but with HostedImage type as union and interface
				// TODO: add test when parent nodes are shareable and should be selected basic on keys to child

				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										cdnUrl
									}
								}
							}
						}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
								}, "user.hostedImage.image", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage"), resolve.ObjectPath("image")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
				)
			})

			t.Run("it is allowed to query a typename even if other fields are external", func(t *testing.T) {
				expectedPlan := func(service string) plan.Plan {
					return &plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {hostedImage {__typename id}}}"}}`,
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
										Input:                                 `{"method":"POST","url":"` + service + `","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {__typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path:       []string{"__typename"},
																				IsTypeName: true,
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

				variant1 := expectedPlan("http://third.service")
				variant2 := expectedPlan("http://second.service")

				RunWithPermutationsVariants(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										__typename
									}
								}
							}
						}`,
					"User",
					[]plan.Plan{
						variant2,
						variant2,
						variant1,
						variant1,
						variant2,
						variant1,
						variant2,
						variant2,
						variant2,
						variant2,
						variant2,
						variant2,
						variant1,
						variant1,
						variant1,
						variant1,
						variant1,
						variant1,
						variant2,
						variant1,
						variant2,
						variant2,
						variant1,
						variant1,
					},
					planConfiguration,
					WithDefaultPostProcessor(),
				)
			})

		})

		t.Run("conditional keys variant - provides on local type", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					hostedImage: HostedImage!
				}

				type Host {
					image: HostedImage!
				}

				type HostedImage {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID!
					url: String!
				}

				type Query {
					user: User!
					host: Host!
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
				type User @key(fields: "id") {
					id: ID!
				}

				type Host {
					image: HostedImage! @provides(fields: "image {id url}")
				}

				type HostedImage @key(fields: "id") {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID! @external
					url: String! @external
				}

				type Query {
					host: Host!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id"},
						},
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id", "image"},
						},
						{
							TypeName: "Image",
							// image fields listed in both fields and external fields
							// because they could be used as a key when they provided, so they become a root node
							// but this root node is conditional and determined by conditional key
							FieldNames:         []string{"id", "url"},
							ExternalFieldNames: []string{"id", "url"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Host",
							FieldNames: []string{"image"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:     "Image",
								SelectionSet: "id",
								Conditions: []plan.KeyCondition{
									{
										Coordinates: []plan.KeyConditionCoordinate{
											{
												TypeName:  "Host",
												FieldName: "image",
											},
											{
												TypeName:  "HostedImage",
												FieldName: "image",
											},
											{
												TypeName:  "Image",
												FieldName: "id",
											},
										},
										FieldPath: []string{"image", "image", "id"},
									},
								},
							},
							{
								TypeName:     "HostedImage",
								SelectionSet: "id",
							},
						},
						Provides: plan.FederationFieldConfigurations{
							{
								TypeName:     "Host",
								FieldName:    "image",
								SelectionSet: "image {id url}",
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
				type HostedImage @key(fields: "id") {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID!
					url: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id", "image"},
						},
						{
							TypeName:   "Image",
							FieldNames: []string{"id", "url"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "HostedImage",
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

			fourthSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					hostedImage: HostedImage!
				}

				type HostedImage @key(fields: "id") {
					id: ID!
				}
			`

			fourthDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"fourth-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "hostedImage"},
						},
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"id"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:     "HostedImage",
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
				Debug: plan.DebugConfiguration{
					PrintQueryPlans: false,
				},
			}

			t.Run("do not query external conditional fields", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										id
										url
									}
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
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {id url}}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
																			Name: []byte("id"),
																			Value: &resolve.Scalar{
																				Path: []string{"id"},
																			},
																		},
																		{
																			Name: []byte("url"),
																			Value: &resolve.String{
																				Path: []string{"url"},
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
				)
			})

			t.Run("it is allowed to query a typename even if other fields are external", func(t *testing.T) {
				expectedPlan := func(service string) *plan.SynchronousResponsePlan {
					return &plan.SynchronousResponsePlan{
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
										Input:                                 `{"method":"POST","url":"http://fourth.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {__typename id}}}}","variables":{"representations":[$$0$$]}}}`,
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
										Input:                                 `{"method":"POST","url":"` + service + `","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on HostedImage {__typename image {__typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("HostedImage")},
														},
													},
												}),
											},
										},
										PostProcessing: SingleEntityPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								}, "user.hostedImage", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path:       []string{"__typename"},
																				IsTypeName: true,
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

				variant1 := expectedPlan("http://third.service")
				variant2 := expectedPlan("http://second.service")

				RunWithPermutationsVariants(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										__typename
									}
								}
							}
						}`,
					"User",
					[]plan.Plan{
						variant2,
						variant2,
						variant1,
						variant1,
						variant2,
						variant1,
						variant2,
						variant2,
						variant2,
						variant2,
						variant2,
						variant2,
						variant1,
						variant1,
						variant1,
						variant1,
						variant1,
						variant1,
						variant2,
						variant1,
						variant2,
						variant2,
						variant1,
						variant1,
					},
					planConfiguration,
					WithDefaultPostProcessor(),
				)
			})

		})

		t.Run("external key fields are not really external", func(t *testing.T) {
			definition := `
			type User {
				id: ID!
				name: String!
			}

			type Query {
				user: User!
			}`

			firstSubgraphSDL := `
			type User @key(fields: "id") {
				id: ID! @external
			}

			type Query {
				user: User
			}`

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
							TypeName:           "User",
							FieldNames:         []string{"id"},
							ExternalFieldNames: []string{"id"},
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
				))

			secondSubgraphSDL := `
			type User @key(fields: "id") {
				id: ID!
				name: String!
			}`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",
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
				))

			planConfiguration := plan.Configuration{
				DataSources: []plan.DataSource{
					firstDatasourceConfiguration,
					secondDatasourceConfiguration,
				},
				DisableResolveFieldPositions: true,
				Debug:                        plan.DebugConfiguration{},
			}

			t.Run("run", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								id
							}
						}`,
					"User",
					&plan.SynchronousResponsePlan{
						Response: &resolve.GraphQLResponse{
							Fetches: resolve.Sequence(
								resolve.Single(&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										Input:          `{"method":"POST","url":"http://first.service","body":{"query":"{user {id}}"}}`,
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
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
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
				)
			})
		})
	})

	t.Run("parent based selection when no child nodes selected", func(t *testing.T) {
		t.Run("image should be selected based on parent not child selection", func(t *testing.T) {
			definition := `
				type User {
					id: ID!
					hostedImage: HostedImage!
				}

				type HostedImage {
					id: ID!
					image: Image!
				}

				type Image {
					id: ID!
					url: String!
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
				type User @key(fields: "id") {
					id: ID!
					hostedImage: HostedImage!
				}

				type HostedImage {
					image: Image!
				}

				type Image @key(fields: "id", resolvable: false) {
					id: ID!
				}
			`

			secondDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"second-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "hostedImage"},
						},
						{
							TypeName:   "Image",
							FieldNames: []string{"id"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"image"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
							{
								TypeName:              "Image",
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
				type HostedImage {
					image: Image!
				}

				type Image @key(fields: "id") {
					id: ID!
					url: String!
				}
			`

			thirdDatasourceConfiguration := mustDataSourceConfiguration(
				t,
				"third-service",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Image",
							FieldNames: []string{"id", "url"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "HostedImage",
							FieldNames: []string{"image"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Image",
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
					PrintQueryPlans: false,
				},
			}

			t.Run("run", func(t *testing.T) {
				RunWithPermutations(
					t,
					definition,
					`
						query User {
							user {
								hostedImage {
									image {
										url
									}
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
										Input:                                 `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename hostedImage {image {__typename id}}}}}","variables":{"representations":[$$0$$]}}}`,
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
										Input:                                 `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Image {__typename url}}}","variables":{"representations":[$$0$$]}}}`,
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
								}, "user.hostedImage.image", resolve.ObjectPath("user"), resolve.ObjectPath("hostedImage"), resolve.ObjectPath("image")),
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
													Name: []byte("hostedImage"),
													Value: &resolve.Object{
														Path:     []string{"hostedImage"},
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
																			Name: []byte("url"),
																			Value: &resolve.String{
																				Path: []string{"url"},
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
				)
			})
		})
	})
}
