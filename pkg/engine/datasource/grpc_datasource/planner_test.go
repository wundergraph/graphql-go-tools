package grpc_datasource

import (
	"net/http"
	"testing"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"

	. "github.com/wundergraph/graphql-go-tools/pkg/engine/datasourcetesting"
)

func TestGrpcDataSourcePlanning(t *testing.T) {
	t.Run("inline object value with arguments", RunTest(
		starwarsGeneratedSchema, `
			query GetHero($episode: starwars_Episode) {
			  starwars_StarwarsService_GetHero(input: {episode: $episode}){
				id
				name
			  }
			}`,
		"GetHero",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"header":{"auth":["abc"]},"body":{"episode":$$0$$}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"episode"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
							},
						),
						DisableDataLoader:    true,
						DataSourceIdentifier: []byte("grpc_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("starwars_StarwarsService_GetHero"),
							Position: resolve.Position{
								Line:   3,
								Column: 6,
							},
							Value: &resolve.Object{
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path:     []string{"id"},
											Nullable: true,
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: true,
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
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"starwars_StarwarsService_GetHero"},
						},
					},
					Custom: ConfigJson(Configuration{
						Request: RequestConfiguration{
							Header: http.Header{"auth": []string{"abc"}},
							Body:   "{{ .arguments.input }}",
						},
					}),
					Factory: &Factory{},
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Query",
					FieldName:             "starwars_StarwarsService_GetHero",
					DisableDefaultMapping: true,
				},
			},
		},
	))

}

const starwarsGeneratedSchema = `
type Query {
  starwars_StarwarsService_GetHero(input: starwars_GetHeroRequest_Input): starwars_Character
  starwars_StarwarsService_GetHuman(input: starwars_GetHumanRequest_Input): starwars_Character
  starwars_StarwarsService_GetDroid(input: starwars_GetDroidRequest_Input): starwars_Character
  starwars_StarwarsService_ListHumans(input: starwars_ListEmptyRequest_Input): starwars_ListHumansResponse
  starwars_StarwarsService_ListDroids(input: starwars_ListEmptyRequest_Input): starwars_ListDroidsResponse
  starwars_StarwarsService_connectivityState(tryToConnect: Boolean): ConnectivityState
}

type starwars_Character {
  id: String
  name: String
  friends: [starwars_Character]
  appears_in: [starwars_Episode]
  home_planet: String
  primary_function: String
  type: starwars_Type
}

enum starwars_Episode {
  _
  NEWHOPE
  EMPIRE
  JEDI
}

enum starwars_Type {
  HUMAN
  DROID
}

input starwars_GetHeroRequest_Input {
  episode: starwars_Episode
}

input starwars_GetHumanRequest_Input {
  id: String
}

input starwars_GetDroidRequest_Input {
  id: String
}

type starwars_ListHumansResponse {
  humans: [starwars_Character]
}

scalar starwars_ListEmptyRequest_Input @specifiedBy(url: "http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf")

type starwars_ListDroidsResponse {
  droids: [starwars_Character]
}

enum ConnectivityState {
  IDLE
  CONNECTING
  READY
  TRANSIENT_FAILURE
  SHUTDOWN
}
`
