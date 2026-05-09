package graphql_datasource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestQueryOrderBaseline(t *testing.T) {
	definition := `
		type Query {
			me: User
			organisations(ids: [ID!]!): [Organisation!]!
		}

		type User {
			id: ID!
			firstName: String!
			lastName: String!
			currentPractice: Practice
		}

		type Organisation {
			id: ID!
			name: String!
			shortCode: String!
		}

		type Practice {
			id: ID!
		}
	`

	operation := `
		query Baseline($a: [ID!]!) {
			me {
				firstName
				lastName
				currentPractice {
					id
				}
			}
			organisations(ids: $a) {
				name
				shortCode
				id
			}
		}
	`

	userSubgraphSDL := `
		type Query {
			me: User
		}

		type User @key(fields: "id") {
			id: ID!
			firstName: String!
			lastName: String!
		}
	`

	organisationSubgraphSDL := `
		type Query {
			organisations(ids: [ID!]!): [Organisation!]!
		}

		type Organisation {
			id: ID!
			name: String!
			shortCode: String!
		}
	`

	practiceSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID! @external
			currentPractice: Practice
		}

		type Practice {
			id: ID!
		}
	`

	config := plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"user-subgraph",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "firstName", "lastName"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://user-subgraph",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: userSubgraphSDL,
						},
						userSubgraphSDL,
					),
				}),
			),
			mustDataSourceConfiguration(
				t,
				"organisation-subgraph",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"organisations"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Organisation",
							FieldNames: []string{"id", "name", "shortCode"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://organisation-subgraph",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: organisationSubgraphSDL,
						},
						organisationSubgraphSDL,
					),
				}),
			),
			mustDataSourceConfiguration(
				t,
				"practice-subgraph",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "currentPractice"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Practice",
							FieldNames: []string{"id"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://practice-subgraph",
					},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{
							Enabled:    true,
							ServiceSDL: practiceSubgraphSDL,
						},
						practiceSubgraphSDL,
					),
				}),
			),
		},
		DisableResolveFieldPositions: true,
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Query",
				FieldName: "organisations",
				Arguments: plan.ArgumentsConfigurations{
					{
						Name:       "ids",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		Debug: plan.DebugConfiguration{},
	}

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)

	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	require.NoError(t, err)

	norm := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)

	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	p, err := plan.NewPlanner(config)
	require.NoError(t, err)

	actualPlan := p.Plan(&op, &def, "Baseline", &report, plan.IncludeQueryPlanInResponse())
	require.False(t, report.HasErrors(), report.Error())
	postprocess.NewProcessor().Process(actualPlan)

	responsePlan, ok := actualPlan.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	require.NotNil(t, responsePlan.Response)
	require.NotNil(t, responsePlan.Response.Fetches)

	queryPlan := responsePlan.Response.Fetches.QueryPlan()
	require.NotNil(t, queryPlan)

	assertQueryOrderBaselineShape(t, queryPlan)

	actual := queryPlan.PrettyPrint()
	require.Equal(t, strings.TrimSpace(expectedQueryOrderBaselinePlan), strings.TrimSpace(actual))
}

func assertQueryOrderBaselineShape(t *testing.T, queryPlan *resolve.FetchTreeQueryPlanNode) {
	t.Helper()

	require.Equal(t, resolve.FetchTreeNodeKindSequence, queryPlan.Kind)
	require.Len(t, queryPlan.Children, 2)

	parallel := queryPlan.Children[0]
	require.Equal(t, resolve.FetchTreeNodeKindParallel, parallel.Kind)
	require.Len(t, parallel.Children, 2)

	fetchByService := make(map[string]*resolve.FetchTreeQueryPlan, len(parallel.Children))
	for _, child := range parallel.Children {
		require.Equal(t, resolve.FetchTreeNodeKindSingle, child.Kind)
		require.NotNil(t, child.Fetch)
		require.Equal(t, "Single", child.Fetch.Kind)
		fetchByService[child.Fetch.SubgraphName] = child.Fetch
	}
	require.Contains(t, fetchByService, "user-subgraph")
	require.Contains(t, fetchByService, "organisation-subgraph")

	practice := queryPlan.Children[1]
	require.Equal(t, resolve.FetchTreeNodeKindSingle, practice.Kind)
	require.NotNil(t, practice.Fetch)
	require.Equal(t, "Entity", practice.Fetch.Kind)
	require.Equal(t, "practice-subgraph", practice.Fetch.SubgraphName)

	require.Equal(t,
		[]int{fetchByService["user-subgraph"].FetchID},
		practice.Fetch.DependsOnFetchIDs,
		"practice entity fetch must depend only on the user-subgraph fetch",
	)

	require.Len(t, practice.Fetch.Representations, 1)
	rep := practice.Fetch.Representations[0]
	require.Equal(t, resolve.RepresentationKindKey, rep.Kind)
	require.Equal(t, "User", rep.TypeName)
	require.Contains(t, rep.Fragment, "__typename")
	require.Contains(t, rep.Fragment, "id")
}

const expectedQueryOrderBaselinePlan = `
QueryPlan {
  Sequence {
    Parallel {
      Fetch(service: "user-subgraph") {
        {
            me {
                firstName
                lastName
                __typename
                id
            }
        }
      }
      Fetch(service: "organisation-subgraph") {
        {
            organisations(ids: $a){
                name
                shortCode
                id
            }
        }
      }
    }
    Fetch(service: "practice-subgraph") {
      {
        fragment Key on User {
            __typename
            id
        }
      } =>
      {
          _entities(representations: $representations){
              ... on User {
                  __typename
                  currentPractice {
                      id
                  }
              }
          }
      }
    }
  }
}
`
