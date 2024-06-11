package introspection_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const (
	schema = `
		type Query {
			friend: String
		}
	`

	schemaWithCustomRootOperationTypes = `
		schema {
			query: CustomQuery
			mutation: CustomMutation
			subscription: CustomSubscription
		}

		type CustomQuery {
			friend: String
		}

		type CustomMutation {
			addFriend: Boolean
		}

		type CustomSubscription {
			lastAddedFriend: String
		}
	`

	typeIntrospection = `
		query typeIntrospection {
			__type(name: "Query") {
				name
				kind
			}
		}
	`

	schemaIntrospection = `
		query typeIntrospection {
			__schema {
				queryType {
					name
				}
			}
		}
	`

	schemaIntrospectionForAllRootOperationTypeNames = `
		query typeIntrospection {
			__schema {
				queryType {
					name
				}
				mutationType {
					name
				}
				subscriptionType {
					name
				}
			}
		}
	`

	typeIntrospectionWithArgs = `
		query typeIntrospection {
			__type(name: "Query") {
				fields(includeDeprecated: true) {
					name
				}
				enumValues(includeDeprecated: true) {
					name
				}
			}
		}
	`
)

func TestIntrospectionDataSourcePlanning(t *testing.T) {
	runTest := func(schema string, introspectionQuery string, expectedPlan plan.Plan) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			def := unsafeparser.ParseGraphqlDocumentString(schema)
			err := asttransform.MergeDefinitionWithBaseSchema(&def)
			require.NoError(t, err)

			var (
				introspectionData introspection.Data
				report            operationreport.Report
			)

			gen := introspection.NewGenerator()
			gen.Generate(&def, &report, &introspectionData)
			require.False(t, report.HasErrors())

			cfgFactory := IntrospectionConfigFactory{introspectionData: &introspectionData}
			introspectionDataSources := cfgFactory.BuildDataSourceConfigurations()

			planConfiguration := plan.Configuration{
				DataSources: introspectionDataSources,
				Fields:      cfgFactory.BuildFieldConfigurations(),
			}

			datasourcetesting.RunTest(schema, introspectionQuery, "", expectedPlan, planConfiguration, datasourcetesting.WithMultiFetchPostProcessor())(t)
		}
	}

	dataSourceIdentifier := []byte("introspection_datasource.Source")

	t.Run("type introspection request", runTest(schema, typeIntrospection,
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSourceIdentifier: dataSourceIdentifier,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"request_type":2,"type_name":"$$0$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"__type"},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__type"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Path:     []string{"__type"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name: []byte("kind"),
										Value: &resolve.String{
											Path: []string{"kind"},
										},
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("schema introspection request", runTest(schema, schemaIntrospection,
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSourceIdentifier: dataSourceIdentifier,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"request_type":1}`,
							DataSource: &Source{},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"__schema"},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__schema"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Path: []string{"__schema"},
								Fields: []*resolve.Field{
									{
										Name: []byte("queryType"),
										Value: &resolve.Object{
											Path: []string{"queryType"},
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   5,
														Column: 6,
													},
												},
											},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("schema introspection request with custom root operation types", runTest(schemaWithCustomRootOperationTypes, schemaIntrospectionForAllRootOperationTypeNames,
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSourceIdentifier: dataSourceIdentifier,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"request_type":1}`,
							DataSource: &Source{},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"__schema"},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__schema"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Path: []string{"__schema"},
								Fields: []*resolve.Field{
									{
										Name: []byte("queryType"),
										Value: &resolve.Object{
											Path: []string{"queryType"},
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   5,
														Column: 6,
													},
												},
											},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name: []byte("mutationType"),
										Value: &resolve.Object{
											Path:     []string{"mutationType"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   8,
														Column: 6,
													},
												},
											},
										},
										Position: resolve.Position{
											Line:   7,
											Column: 5,
										},
									},
									{
										Name: []byte("subscriptionType"),
										Value: &resolve.Object{
											Path:     []string{"subscriptionType"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   11,
														Column: 6,
													},
												},
											},
										},
										Position: resolve.Position{
											Line:   10,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("type introspection request with fields args", runTest(schema, typeIntrospectionWithArgs,
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						DataSourceIdentifier: dataSourceIdentifier,
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"request_type":2,"type_name":"$$0$$"}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"__type"},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__type"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Path:     []string{"__type"},
								Nullable: true,
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 1,
											},
											DataSourceIdentifier: dataSourceIdentifier,
											FetchConfiguration: resolve.FetchConfiguration{
												Input:      `{"request_type":3,"on_type_name":"$$0$$","include_deprecated":$$1$$}`,
												DataSource: &Source{},
												Variables: resolve.NewVariables(
													&resolve.ObjectVariable{
														Path:     []string{"name"},
														Renderer: resolve.NewPlainVariableRenderer(),
													},
													&resolve.ContextVariable{
														Path:     []string{"b"},
														Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["boolean","null"]}`),
													},
												),
												PostProcessing: resolve.PostProcessingConfiguration{
													MergePath: []string{"fields"},
												},
											},
										},
										&resolve.SingleFetch{
											FetchDependencies: resolve.FetchDependencies{
												FetchID: 2,
											},
											DataSourceIdentifier: dataSourceIdentifier,
											FetchConfiguration: resolve.FetchConfiguration{
												Input:      `{"request_type":4,"on_type_name":"$$0$$","include_deprecated":$$1$$}`,
												DataSource: &Source{},
												Variables: resolve.NewVariables(
													&resolve.ObjectVariable{
														Path:     []string{"name"},
														Renderer: resolve.NewPlainVariableRenderer(),
													},
													&resolve.ContextVariable{
														Path:     []string{"b"},
														Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["boolean","null"]}`),
													},
												),
												PostProcessing: resolve.PostProcessingConfiguration{
													MergePath: []string{"enumValues"},
												},
											},
										},
									},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("fields"),
										Value: &resolve.Array{
											Path:     []string{"fields"},
											Nullable: true,
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														Position: resolve.Position{
															Line:   5,
															Column: 6,
														},
													},
												},
											},
										}, Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name: []byte("enumValues"),
										Value: &resolve.Array{
											Path:     []string{"enumValues"},
											Nullable: true,
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														Position: resolve.Position{
															Line:   8,
															Column: 6,
														},
													},
												},
											},
										}, Position: resolve.Position{
											Line:   7,
											Column: 5,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
}
