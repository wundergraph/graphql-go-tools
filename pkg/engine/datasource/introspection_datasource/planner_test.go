package introspection_datasource

import (
	"encoding/json"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	schema = `
		type Query {
			friend: String
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
	dataSourceIdentifier := []byte("introspection_datasource.Source")

	planConfiguration := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"__schema", "__type"},
					},
					{
						TypeName:   "__Type",
						FieldNames: []string{"fields", "enumValues"},
					},
				},
				Factory: &Factory{},
				Custom:  json.RawMessage{},
			},
		},
		Fields: plan.FieldConfigurations{
			{
				TypeName:              "Query",
				FieldName:             "__schema",
				DisableDefaultMapping: true,
			},
			{
				TypeName:              "Query",
				FieldName:             "__type",
				DisableDefaultMapping: true,
			},
			{
				TypeName:              "__Type",
				FieldName:             "fields",
				DisableDefaultMapping: true,
			},
			{
				TypeName:              "__Type",
				FieldName:             "enumValues",
				DisableDefaultMapping: true,
			},
		},
	}

	t.Run("type introspection request", datasourcetesting.RunTest(schema, typeIntrospection, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:             0,
						Input:                `{"request_type":2,"type_name":"$$0$$"}`,
						DataSource:           &Source{},
						DataSourceIdentifier: dataSourceIdentifier,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:               []string{"a"},
								Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"string"}`),
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("__type"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
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
		planConfiguration,
	))

	t.Run("schema introspection request", datasourcetesting.RunTest(schema, schemaIntrospection, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:             0,
						Input:                `{"request_type":1}`,
						DataSource:           &Source{},
						DataSourceIdentifier: dataSourceIdentifier,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("__schema"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
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
		planConfiguration,
	))

	t.Run("type introspection request with fields args", datasourcetesting.RunTest(schema, typeIntrospectionWithArgs, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:             0,
						Input:                `{"request_type":2,"type_name":"$$0$$"}`,
						DataSource:           &Source{},
						DataSourceIdentifier: dataSourceIdentifier,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:               []string{"a"},
								Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"string"}`),
							},
						),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("__type"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Fetch: &resolve.ParallelFetch{
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{
											BufferId:   1,
											Input:      `{"request_type":3,"on_type_name":"$$0$$","include_deprecated":$$1$$}`,
											DataSource: &Source{},
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path:               []string{"name"},
													Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"string"}`),
												},
												&resolve.ContextVariable{
													Path:               []string{"b"},
													Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"boolean"}`),
												},
											),
											DataSourceIdentifier: dataSourceIdentifier,
										},
										&resolve.SingleFetch{
											BufferId:   2,
											Input:      `{"request_type":4,"on_type_name":"$$0$$","include_deprecated":$$1$$}`,
											DataSource: &Source{},
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path:               []string{"name"},
													Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"string"}`),
												},
												&resolve.ContextVariable{
													Path:               []string{"c"},
													Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":"boolean"}`),
												},
											),
											DataSourceIdentifier: dataSourceIdentifier,
										},
									},
								},
								Fields: []*resolve.Field{
									{
										BufferID:  1,
										HasBuffer: true,
										Name:      []byte("fields"),
										Value: &resolve.Array{
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
										BufferID:  2,
										HasBuffer: true,
										Name:      []byte("enumValues"),
										Value: &resolve.Array{
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
		planConfiguration,
	))

}
