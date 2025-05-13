package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
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
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
													},
												},
												OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){user(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								},
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
													Value: &resolve.Scalar{
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
						}, "user", resolve.ObjectPath("user")),
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){user(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								},
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
													Value: &resolve.Scalar{
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
						}, "user", resolve.ObjectPath("user")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
						}, "user", resolve.ObjectPath("user")),
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
													PossibleTypes: map[string]struct{}{
														"Location": {},
													},
													TypeName: "Location",
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
			},
			planConfiguration,
			WithDefaultPostProcessor(),
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){admin(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								},
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
													Value: &resolve.Scalar{
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
						}, "admin", resolve.ObjectPath("admin")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("admin"),
								Value: &resolve.Object{
									Path:     []string{"admin"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Admin": {},
									},
									TypeName: "Admin",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){admin(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
								Variables: []resolve.Variable{
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRenderer(),
									},
								},
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
													Value: &resolve.Scalar{
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
						}, "admin", resolve.ObjectPath("admin")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
						}, "admin", resolve.ObjectPath("admin")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("admin"),
								Value: &resolve.Object{
									Path:     []string{"admin"},
									Nullable: true,
									PossibleTypes: map[string]struct{}{
										"Admin": {},
									},
									TypeName: "Admin",
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
													PossibleTypes: map[string]struct{}{
														"Location": {},
													},
													TypeName: "Location",
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
			},
			planConfiguration,
			WithDefaultPostProcessor(),
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
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
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsUnion {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "allAccountsUnion", resolve.ArrayPath("allAccountsUnion")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsUnion", resolve.ArrayPath("allAccountsUnion")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsUnion"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsUnion"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
										},
										TypeName: "Accounts",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsUnion {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "allAccountsUnion", resolve.ArrayPath("allAccountsUnion")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsUnion", resolve.ArrayPath("allAccountsUnion")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsUnion", resolve.ArrayPath("allAccountsUnion")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsUnion"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsUnion"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
										},
										TypeName: "Accounts",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           3,
								DependsOnFetchIDs: []int{0, 2},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id __typename} ... on User {id __typename}}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
												Name: []byte("locations"),
												Value: &resolve.Array{
													Path:     []string{"locations"},
													Nullable: true,
													Item: &resolve.Object{
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
						resolve.SingleWithPath(&resolve.SingleFetch{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename} ... on User {__typename}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename locations {country} id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename ... on User {title __typename}}}}","variables":{"representations":[$$0$$]}}}`,
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
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path: []string{"accountLocations"},
									Item: &resolve.Object{
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
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
														PossibleTypes: map[string]struct{}{
															"Location": {},
														},
														TypeName: "Location",
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

	t.Run("query 16 - Interface to interface object which has requires", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _16_InterfaceToInterfaceObjectWithRequires {
					allAccountsInterface {
						id
						age
						fullTitle
					}
				}`,
			"_16_InterfaceToInterfaceObjectWithRequires",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{allAccountsInterface {__typename ... on Admin {id __typename} ... on Moderator {id title __typename} ... on User {id title __typename}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0, 2},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age fullTitle}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
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
						}, "allAccountsInterface", resolve.ArrayPath("allAccountsInterface")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("allAccountsInterface"),
								Value: &resolve.Array{
									Path:     []string{"allAccountsInterface"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
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
			WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
		))
	})

	t.Run("query 17 Interface object to Interface object with required field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _17_InterfaceObjectToInterfaceObject {
					accountLocations {
						age
						fullTitle
					}
				}`,
			"_17_InterfaceObjectToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           3,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1, 3},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4004/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {age fullTitle}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
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
													Value: &resolve.Scalar{
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
													OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
												},
												{
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path:     []string{"accountLocations"},
									Nullable: false,
									Item: &resolve.Object{
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
										Fields: []*resolve.Field{
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("fullTitle"),
												Value: &resolve.String{
													Path: []string{"fullTitle"},
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
			WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
		))
	})

	t.Run("query 17.1 Interface object to Interface object with required field", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query  _17_1_InterfaceObjectToInterfaceObject {
					accountLocations {
						age
						uniqueTitle
					}
				}`,
			"_17_1_InterfaceObjectToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.Parallel(
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename} ... on Moderator {__typename title} ... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
														Value: &resolve.Scalar{
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
														Value: &resolve.Scalar{
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
														Value: &resolve.Scalar{
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
							}, "accountLocations", resolve.ArrayPath("accountLocations")),
							resolve.SingleWithPath(&resolve.SingleFetch{
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
														Value: &resolve.Scalar{
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
														Value: &resolve.Scalar{
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
														Value: &resolve.Scalar{
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
							}, "accountLocations", resolve.ArrayPath("accountLocations")),
						),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           3,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4003/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Admin {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           4,
								DependsOnFetchIDs: []int{0, 1, 3},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {uniqueTitle}}}","variables":{"representations":[$$0$$]}}}`,
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
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Admin"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
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
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("Moderator"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
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
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path:     []string{"accountLocations"},
									Nullable: false,
									Item: &resolve.Object{
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
										},
										TypeName: "Account",
										Fields: []*resolve.Field{
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Admin")},
											},
											{
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
											},
											{
												Name: []byte("age"),
												Value: &resolve.Integer{
													Path: []string{"age"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
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
			WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
			// WithPrintPlan(),
		))
	})

	t.Run("query 18 Interface object with required field and its requirements", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query _18_InterfaceObjectWithRequiredFieldAndItsRequirements {
					accountLocations {
						... on User {
							title
							uniqueTitle
						}
						id
					}
				}`,
			"_18_InterfaceObjectWithRequiredFieldAndItsRequirements",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"{accountLocations {__typename id}}"}}`,
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
								Input: `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[$$0$$]}}}`,
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
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0, 1},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {uniqueTitle}}}","variables":{"representations":[$$0$$]}}}`,
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
													Name: []byte("title"),
													Value: &resolve.String{
														Path: []string{"title"},
													},
													OnTypeNames: [][]byte{[]byte("User"), []byte("Account")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
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
						}, "accountLocations", resolve.ArrayPath("accountLocations")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("accountLocations"),
								Value: &resolve.Array{
									Path:     []string{"accountLocations"},
									Nullable: false,
									Item: &resolve.Object{
										Nullable: false,
										PossibleTypes: map[string]struct{}{
											"Admin":     {},
											"Moderator": {},
											"User":      {},
											"Account":   {},
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
												OnTypeNames: [][]byte{[]byte("Account")},
											},
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
												OnTypeNames: [][]byte{[]byte("Moderator")},
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
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
												},
												OnTypeNames: [][]byte{[]byte("User")},
											},
											{
												Name: []byte("uniqueTitle"),
												Value: &resolve.String{
													Path: []string{"uniqueTitle"},
												},
												OnTypeNames: [][]byte{[]byte("Account")},
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
			WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
			// WithPrintPlan(),
		))
	})

	t.Run("query 19 Interface object with field having argument", func(t *testing.T) {
		t.Run("on interface object", func(t *testing.T) {
			t.Run("run", RunTest(
				definition,
				`
				query _18_InterfaceObjectWithRequiredFieldAndItsRequirements {
					accountLocations {
						id
						fieldWithArg(arg: "value")
					}
				}`,
				"_18_InterfaceObjectWithRequiredFieldAndItsRequirements",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($a: String!){accountLocations {__typename id fieldWithArg(arg: $a)}}","variables":{"a":$$0$$}}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
									Variables: []resolve.Variable{
										&resolve.ContextVariable{
											Path:     []string{"a"},
											Renderer: resolve.NewJSONVariableRenderer(),
										},
									},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
						),
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("accountLocations"),
									Value: &resolve.Array{
										Path:     []string{"accountLocations"},
										Nullable: false,
										Item: &resolve.Object{
											Nullable: false,
											PossibleTypes: map[string]struct{}{
												"Admin":     {},
												"Moderator": {},
												"User":      {},
												"Account":   {},
											},
											TypeName: "Account",
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("fieldWithArg"),
													Value: &resolve.String{
														Path: []string{"fieldWithArg"},
													},
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
				WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
				// WithPrintPlan(),
			))
		})

		t.Run("on concrete type", func(t *testing.T) {
			t.Run("run", RunTest(
				definition,
				`
				query _18_InterfaceObjectWithRequiredFieldAndItsRequirements {
					user(id: "1") {
						id
						fieldWithArg(arg: "value")
					}
				}`,
				"_18_InterfaceObjectWithRequiredFieldAndItsRequirements",
				&plan.SynchronousResponsePlan{
					Response: &resolve.GraphQLResponse{
						Fetches: resolve.Sequence(
							resolve.Single(&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"query($a: ID!){user(id: $a){id __typename}}","variables":{"a":$$0$$}}}`,
									PostProcessing: DefaultPostProcessingConfiguration,
									DataSource:     &Source{},
									Variables: []resolve.Variable{
										&resolve.ContextVariable{
											Path:     []string{"a"},
											Renderer: resolve.NewJSONVariableRenderer(),
										},
									},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							}),
							resolve.SingleWithPath(&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input: `{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!, $b: String!){_entities(representations: $representations){... on Account {fieldWithArg(arg: $b)}}}","variables":{"representations":[$$1$$],"b":$$0$$}}}`,
									Variables: []resolve.Variable{
										&resolve.ContextVariable{
											Path:     []string{"b"},
											Renderer: resolve.NewJSONVariableRenderer(),
										},
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
														Value: &resolve.Scalar{
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
							}, "user", resolve.ObjectPath("user")),
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
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("fieldWithArg"),
												Value: &resolve.String{
													Path: []string{"fieldWithArg"},
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
				WithDefaultCustomPostProcessor(postprocess.DisableResolveInputTemplates()),
				// WithPrintPlan(),
			))
		})

	})
}

func BenchmarkPlanner(b *testing.B) {

	federationFactory := &Factory[Configuration]{}
	definition := EntityInterfacesDefinition
	planConfiguration := *EntityInterfacesPlanConfigurationBench(b, federationFactory)

	operation := `
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
				}`

	operationName := "_13_InterfaceToInterfaceObjects_InterfaceFragment_ExternalField"

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)

	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		b.Fatal(err)
	}
	norm := astnormalization.NewWithOpts(astnormalization.WithExtractVariables(), astnormalization.WithInlineFragmentSpreads(), astnormalization.WithRemoveFragmentDefinitions(), astnormalization.WithRemoveUnusedVariables())
	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)

	normalized := unsafeprinter.PrettyPrint(&op)
	_ = normalized

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, &report)

	p, err := plan.NewPlanner(planConfiguration)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		actualPlan := p.Plan(&op, &def, operationName, &report)
		_ = actualPlan
		if report.HasErrors() {
			b.Fatal(report.Error())
		}
	}
}
