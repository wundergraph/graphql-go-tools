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
												OnTypeNames: [][]byte{[]byte("Admin"), []byte("Moderator"), []byte("User")},
											},
										},
										Fetch: &resolve.SingleFetch{
											SerialID: 1,
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
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator")},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator")},
																},
																{
																	Name: []byte("__typename"),
																	Value: &resolve.StaticString{
																		Path:  []string{"__typename"},
																		Value: "Account",
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
