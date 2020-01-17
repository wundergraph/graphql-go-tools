package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
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
								DataResolvingConfig:DataResolvingConfig{
									PathSelector:PathSelector{
										Path: "foo",
									},
									Transformation: &PipelineTransformation{
										pipeline: pipe.Pipeline{
											Steps: []pipe.Step{
												func() pipe.Step {
													s,_ := step.NewJSON("{{ upper . }}")
													return s
												}(),
											},
										},
									},
								},
								QuoteValue: true,
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
			"foo": "bar",
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
