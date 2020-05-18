package staticdatasource

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	definition = `type Query { hello: String }`
	operation  = `{ hello }`
)

func TestStaticDataSourcePlanning(t *testing.T) {
	t.Run("simple", datasourcetesting.RunTest(definition, operation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name:  []byte("hello"),
									Value: &resolve.String{},
								},
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      []byte("world"),
						DataSource: Source{},
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"hello"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "data",
							Value: []byte(`world`),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Query",
					FieldName:             "hello",
					DisableDefaultMapping: true,
				},
			},
		},
	))
}
