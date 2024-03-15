package pubsub_datasource

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

type testPubsub struct {
}

func (t *testPubsub) ID() string {
	return "test"
}

func (t *testPubsub) Subscribe(ctx context.Context, topic string, updater resolve.SubscriptionUpdater) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Publish(ctx context.Context, topic string, data []byte) error {
	return errors.New("not implemented")
}

func (t *testPubsub) Request(ctx context.Context, topic string, data []byte, w io.Writer) error {
	return errors.New("not implemented")
}

type testConnector struct {
}

func (c *testConnector) New(ctx context.Context) PubSub {
	return &testPubsub{}
}

func TestPubSub(t *testing.T) {
	factory := &Factory{
		Connector: &testConnector{},
	}

	const schema = `
	type Query {
		helloQuery(id: String!): String! @eventsRequest(topic: "helloQuery.{{ args.id }}")
	}

	type Mutation {
		helloMutation(id: String!, input: String!): String! @eventsPublish(topic: "helloMutation.{{ args.id }}")
	}

	type Subscription {
		helloSubscription(id: String!): String! @eventsSubscribe(topic: "helloSubscription.{{ args.id }}")
	}`

	dataSourceConfig := Configuration{
		Events: []EventConfiguration{
			{
				Type:      EventTypeRequest,
				TypeName:  "Query",
				FieldName: "helloQuery",
				Topic:     "helloQuery.{{ args.id }}",
			},
			{
				Type:      EventTypePublish,
				TypeName:  "Mutation",
				FieldName: "helloMutation",
				Topic:     "helloMutation.{{ args.id }}",
			},
			{
				Type:      EventTypeSubscribe,
				TypeName:  "Subscription",
				FieldName: "helloSubscription",
				Topic:     "helloSubscription.{{ args.id }}",
			},
		},
	}

	planConfig := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"helloQuery"},
					},
					{
						TypeName:   "Mutation",
						FieldNames: []string{"helloMutation"},
					},
					{
						TypeName:   "Subscription",
						FieldNames: []string{"helloSubscription"},
					},
				},
				Custom:  ConfigJson(dataSourceConfig),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "helloQuery",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Mutation",
				FieldName: "helloMutation",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "input",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Subscription",
				FieldName: "helloSubscription",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("query", func(t *testing.T) {
		const operation = "query HelloQuery { helloQuery(id:42) }"
		const operationName = `HelloQuery`
		expect := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("helloQuery"),
							Value: &resolve.String{
								Path: []string{"helloQuery"},
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input: `{"topic":"helloQuery.$$0$$", "data": {"id":$$0$$}}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation("{}"),
								},
							},
							DataSource: &RequestDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloQuery"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.RequestDataSource"),
					},
				},
			},
		}
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("mutation", func(t *testing.T) {
		const operation = "mutation HelloMutation { helloMutation(id: 42, input:\"world\") }"
		const operationName = `HelloMutation`
		expect := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("helloMutation"),
							Value: &resolve.String{
								Path: []string{"helloMutation"},
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input: `{"topic":"helloMutation.$$0$$", "data": {"id":$$0$$,"input":$$1$$}}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation("{}"),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewJSONVariableRenderer(),
								},
							},
							DataSource: &PublishDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloMutation"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.PublishDataSource"),
					},
				},
			},
		}
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("subscription", func(t *testing.T) {
		const operation = "subscription HelloSubscription { helloSubscription(id: 42) }"
		const operationName = `HelloSubscription`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"topic":"helloSubscription.$$0$$"}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a"},
							Renderer: resolve.NewPlainVariableRendererWithValidation("{}"),
						},
					},
					Source: &SubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"helloSubscription"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("helloSubscription"),
								Value: &resolve.String{
									Path: []string{"helloSubscription"},
								},
							},
						},
					},
				},
			},
		}
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})
}
