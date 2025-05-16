package staticdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"

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
				RawFetches: []*resolve.FetchItem{
					{
						Fetch: &resolve.SingleFetch{
							DataSourceIdentifier: []byte("staticdatasource.Source"),
							FetchConfiguration: resolve.FetchConfiguration{
								Input:      "world",
								DataSource: Source{},
							},
						},
					},
				},
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("hello"),
							Value: &resolve.String{
								Nullable: true,
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				func(t *testing.T) plan.DataSource {
					cfg, err := plan.NewDataSourceConfiguration[Configuration](
						"staticdatasource.Source",
						&Factory[Configuration]{},
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"hello"},
								},
							},
						},
						Configuration{
							Data: "world",
						},
					)
					require.NoError(t, err)
					return cfg
				}(t),
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
