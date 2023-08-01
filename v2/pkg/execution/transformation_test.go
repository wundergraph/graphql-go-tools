package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jensneuse/pipeline/pkg/pipe"
	"github.com/jensneuse/pipeline/pkg/step"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
)

func TestExecution_With_Transformation(t *testing.T) {

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "foo",
						Source: &DataSourceInvocation{
							DataSource: &FakeDataSource{
								data: []byte(`{"foo":"bar"}`),
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("foo"),
							HasResolvedData: true,
							Value: &Value{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "foo",
									},
									Transformation: &PipelineTransformation{
										pipeline: pipe.Pipeline{
											Steps: []pipe.Step{
												func() pipe.Step {
													s, _ := step.NewJSON("{{ upper . }}") // simple example using the sprig function upper
													return s
												}(),
											},
										},
									},
								},
								ValueType: StringValueType,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"foo": "BAR",
		},
	}

	wantResult, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	want := string(wantResult)
	got := prettyJSON(out)

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
		return
	}
}

func TestPlanner_WithTransformation(t *testing.T) {
	t.Run("pipeline transformation string dataSourceConfig", run(transformationSchema, `
		query TransformationQuery {
			foo
		}
	`, func(base *datasource.BasePlanner) {
		base.Config = datasource.PlannerConfiguration{
			TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
				{
					TypeName:  "query",
					FieldName: "foo",
					Mapping: &datasource.MappingConfiguration{
						Disabled: true,
					},
					DataSource: datasource.SourceConfig{
						Name: "StaticDataSource",
						Config: toJSON(datasource.StaticDataSourceConfig{
							Data: "{\"bar\":\"baz\"}",
						}),
					},
				},
			},
		}
		panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", datasource.StaticDataSourcePlannerFactoryFactory{}))
	},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &datasource.StaticDataSource{
									Data: []byte("{\"bar\":\"baz\"}"),
								},
							},
							BufferName: "foo",
						},
						Fields: []Field{
							{
								Name:            []byte("foo"),
								HasResolvedData: true,
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										Transformation: &PipelineTransformation{
											pipeline: pipe.Pipeline{
												Steps: []pipe.Step{
													step.NoOpStep{},
												},
											},
										},
									},
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		}))
	t.Run("pipeline transformation file dataSourceConfig", run(transformationSchema, `
		query TransformationQuery {
			bar
		}
	`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "bar",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(datasource.StaticDataSourceConfig{
								Data: "{\"bar\":\"baz\"}",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", datasource.StaticDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &datasource.StaticDataSource{
									Data: []byte("{\"bar\":\"baz\"}"),
								},
							},
							BufferName: "bar",
						},
						Fields: []Field{
							{
								Name:            []byte("bar"),
								HasResolvedData: true,
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										Transformation: &PipelineTransformation{
											pipeline: pipe.Pipeline{
												Steps: []pipe.Step{
													step.NoOpStep{},
												},
											},
										},
									},
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		}))
}

const transformationSchema = `
schema {
    query: Query
}

directive @transformation(
	mode: TRANSFORMATION_MODE = PIPELINE
	pipelineConfigFile: String
	pipelineConfigString: String
) on FIELD_DEFINITION

enum TRANSFORMATION_MODE {
	PIPELINE
}

type Query {
	foo: String!
        @StaticDataSource(
            data: "{\"bar\":\"baz\"}"
        )
		@transformation(
			mode: PIPELINE
			pipelineConfigString: """
			{
				"steps": [
					{
						"kind": "NOOP"
					}
				]
			}
			"""
		)
	bar: String!
        @StaticDataSource(
            data: "{\"bar\":\"baz\"}"
        )
		@transformation(
			mode: PIPELINE
			pipelineConfigFile: "./testdata/simple_pipeline.json"
		)
}`
