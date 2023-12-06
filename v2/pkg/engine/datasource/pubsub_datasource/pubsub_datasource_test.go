package pubsub_datasource

import (
	"net/http"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type CheckFunc func(t *testing.T, op ast.Document, actualPlan plan.Plan)

type runTestOnTestDefinitionOptions func(planConfig *plan.Configuration, extraChecks *[]CheckFunc)

func runTestOnTestDefinition(operation, operationName string, expectedPlan plan.Plan, options ...runTestOnTestDefinitionOptions) func(t *testing.T) {
	extraChecks := make([]CheckFunc, 0)
	config := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"hero", "heroByBirthdate", "droid", "droids", "search", "stringList", "nestedStringList"},
					},
					{
						TypeName:   "Mutation",
						FieldNames: []string{"createReview"},
					},
					{
						TypeName:   "Subscription",
						FieldNames: []string{"remainingJedis"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Review",
						FieldNames: []string{"id", "stars", "commentary"},
					},
					{
						TypeName:   "Character",
						FieldNames: []string{"name", "friends"},
					},
					{
						TypeName:   "Human",
						FieldNames: []string{"name", "height", "friends"},
					},
					{
						TypeName:   "Droid",
						FieldNames: []string{"name", "primaryFunction", "friends"},
					},
					{
						TypeName:   "Starship",
						FieldNames: []string{"name", "length"},
					},
				},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL:    "https://swapi.com/graphql",
						Method: "POST",
					},
					Subscription: SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
				Factory: &Factory{},
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "heroByBirthdate",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "birthdate",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droids",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "ids",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}

	for _, opt := range options {
		opt(&config, &extraChecks)
	}

	return RunTest(testDefinition, operation, operationName, expectedPlan, config, extraChecks...)
}

func TestPubSubDataSource(t *testing.T) {
	t.Run("Subscription", runTestOnTestDefinition(`
	subscription RemainingJedis {
		remainingJedis
	}
`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`),
				Source: &SubscriptionSource{
					NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, ctx),
				},
				PostProcessing: DefaultPostProcessingConfiguration,
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("remainingJedis"),
							Value: &resolve.Integer{
								Path:     []string{"remainingJedis"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, testWithFactory(factory)))
}
