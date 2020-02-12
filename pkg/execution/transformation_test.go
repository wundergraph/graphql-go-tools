package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"github.com/jensneuse/pipeline/pkg/step"
	"testing"
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
									PathSelector: PathSelector{
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
								ValueType:StringValueType,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
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
	t.Run("pipeline transformation string config", run(withBaseSchema(transformationSchema), `
		query TransformationQuery {
			foo
		}
	`, ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: []byte("foo"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &StaticDataSourcePlanner{}
			},
		},
	}, &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Value: []byte("{\"bar\":\"baz\"}"),
								},
							},
							DataSource: &StaticDataSource{},
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
								ValueType:StringValueType,
							},
						},
					},
				},
			},
		},
	}))
	t.Run("pipeline transformation file config", run(withBaseSchema(transformationSchema), `
		query TransformationQuery {
			bar
		}
	`, ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: []byte("bar"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &StaticDataSourcePlanner{}
			},
		},
	}, &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Value: []byte("{\"bar\":\"baz\"}"),
								},
							},
							DataSource: &StaticDataSource{},
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
								ValueType:StringValueType,
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
		@mapping(mode: NONE)
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
		@mapping(mode: NONE)
		@transformation(
			mode: PIPELINE
			pipelineConfigFile: "./testdata/simple_pipeline.json"
		)
}`
