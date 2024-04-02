package pubsub_datasource

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type testPubsub struct {
}

func (t *testPubsub) ID() string {
	return "test"
}

func (t *testPubsub) Subscribe(_ context.Context, _ []string, _ resolve.SubscriptionUpdater, streamConfiguration *StreamConfiguration) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Publish(_ context.Context, _ string, _ []byte) error {
	return errors.New("not implemented")
}

func (t *testPubsub) Request(_ context.Context, _ string, _ []byte, _ io.Writer) error {
	return errors.New("not implemented")
}

func TestPubSub(t *testing.T) {
	factory := &Factory[Configuration]{
		PubSubBySourceName: map[string]PubSub{"default": &testPubsub{}},
	}

	const schema = `
	type Query {
		helloQuery(id: String!): String! @eventsRequest(subject: "helloQuery.{{ args.id }}")
	}

	type Mutation {
		helloMutation(id: String!, input: String!): String! @eventsPublish(subject: "helloMutation.{{ args.id }}")
	}

	type Subscription {
		helloSubscription(id: String!): String! @eventsSubscribe(subjects: ["helloSubscription.{{ args.id }}"])
		subscriptionWithMultipleSubjects(firstId: String!, secondId: String!): String! @eventsSubscribe(subjects: ["firstSubscription.{{ args.firstId }}", "secondSubscription.{{ args.secondId }}"])
	}`

	dataSourceCustomConfig := Configuration{
		Events: []EventConfiguration{
			{
				FieldName:  "helloQuery",
				SourceName: "default",
				Subjects:   []string{"helloQuery.{{ args.id }}"},
				Type:       EventTypeRequest,
				TypeName:   "Query",
			},
			{
				FieldName:  "helloMutation",
				SourceName: "default",
				Subjects:   []string{"helloMutation.{{ args.id }}"},
				Type:       EventTypePublish,
				TypeName:   "Mutation",
			},
			{
				FieldName:  "helloSubscription",
				SourceName: "default",
				Subjects:   []string{"helloSubscription.{{ args.id }}"},
				Type:       EventTypeSubscribe,
				TypeName:   "Subscription",
			},
			{
				FieldName:  "subscriptionWithMultipleSubjects",
				SourceName: "default",
				Subjects:   []string{"firstSubscription.{{ args.firstId }}", "secondSubscription.{{ args.secondId }}"},
				Type:       EventTypeSubscribe,
				TypeName:   "Subscription",
			},
		},
	}

	dataSourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"test",
		factory,
		&plan.DataSourceMetadata{
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
				{
					TypeName:   "Subscription",
					FieldNames: []string{"subscriptionWithMultipleSubjects"},
				},
			},
		},
		dataSourceCustomConfig,
	)
	require.NoError(t, err)

	planConfig := plan.Configuration{
		DataSources: []plan.DataSource{
			dataSourceConfiguration,
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
			{
				TypeName:  "Subscription",
				FieldName: "subscriptionWithMultipleSubjects",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "firstId",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "secondId",
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
							Input: `{"subject":"helloQuery.$$0$$", "data": {"id":$$0$$}, "sourceName":"default"}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
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
							Input: `{"subject":"helloMutation.$$0$$", "data": {"id":$$0$$,"input":$$1$$}, "sourceName":"default"}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
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
					Input: []byte(`{"subjects":["helloSubscription.$$0$$"], "sourceName":"default"}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
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

	t.Run("subscription with multiple subjects", func(t *testing.T) {
		const operation = "subscription SubscriptionWithMultipleSubjects { subscriptionWithMultipleSubjects(firstId: 11, secondId: 23) }"
		const operationName = `SubscriptionWithMultipleSubjects`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"subjects":["firstSubscription.$$0$$","secondSubscription.$$1$$"], "sourceName":"default"}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"b"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
						},
					},
					Source: &SubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"subscriptionWithMultipleSubjects"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("subscriptionWithMultipleSubjects"),
								Value: &resolve.String{
									Path: []string{"subscriptionWithMultipleSubjects"},
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
