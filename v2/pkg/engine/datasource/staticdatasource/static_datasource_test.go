package staticdatasource

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const (
	definition = `type Query { hello: String }`
	operation  = `{ hello }`
)

func TestStaticDataSourcePlanning(t *testing.T) {
	t.Run("simple", datasourcetesting.RunTest(definition, operation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("hello"),
							Value: &resolve.String{
								Nullable: true,
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						Input:                "world",
						DataSource:           Source{},
						DataSourceIdentifier: []byte("staticdatasource.Source"),
						DisallowSingleFlight: true,
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"hello"},
						},
					},
					Custom: ConfigJSON(Configuration{
						Data: "world",
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "hello",
					DisableDefaultMapping: true,
				},
			},
			DisableResolveFieldPositions: true,
		},
	))
}
