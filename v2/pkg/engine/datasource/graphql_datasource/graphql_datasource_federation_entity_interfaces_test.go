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
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":""}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{},
					},
				},
			},
			planConfiguration,
		))

	})

}
