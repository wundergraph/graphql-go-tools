package execute

import (
	"bytes"
	"context"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource"
	statement "github.com/jensneuse/graphql-go-tools/pkg/engine/statementv3"
)

func TestExecutor_ExecuteSingleStatement(t *testing.T) {
	stmt := statement.SingleStatement{
		DataSourceDefinitions: []statement.DataSourceDefinition{
			statement.ResolveOne{
				Input: "bar",
				Resolvers: []statement.ResolverDefinition{
					{
						Name: "static",
					},
				},
			},
		},
		Template: statement.Object{
			FieldSets: []statement.FieldSet{
				{
					Fields: []statement.Field{
						{
							Name: "data",
							Value: statement.Object{
								ResultSetSelector: []int{0},
								FieldSets: []statement.FieldSet{
									{
										Fields: []statement.Field{
											{
												Name:  "foo",
												Value: statement.ResultValue("foo"),
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

	ex := New()
	ex.RegisterResolver("static",datasource.StaticDataSource{})

	id, err := ex.PrepareSingleStatement(stmt)
	if err != nil {
		t.Fatal(err)
	}

	ctx := Context{
		Context: context.Background(),
	}

	out := bytes.Buffer{}
	_, err = ex.ExecutePreparedSingleStatement(ctx, id, &out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"foo":"bar"}}`
	got := out.String()

	if want != got {
		t.Fatalf("want:\n%s\ngot:\n%s\n", want, got)
	}
}

func TestResolveData(t *testing.T){
	
}