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

func (t *testPubsub) Subscribe(_ context.Context, _ NatsSubscriptionEventConfiguration, _ resolve.SubscriptionUpdater) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Publish(_ context.Context, _ NatsPublishAndRequestEventConfiguration) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Request(_ context.Context, _ NatsPublishAndRequestEventConfiguration, _ io.Writer) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Shutdown(_ context.Context) error {
	return errors.New("not implemented")
}

func TestPubSub(t *testing.T) {
	factory := &Factory[Configuration]{
		natsPubSubBySourceName: map[string]NatsPubSub{"default": &testPubsub{}},
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
				Metadata: &EventMetadata{
					ProviderID: "default",
					FieldName:  "helloQuery",
					Type:       EventTypeRequest,
					TypeName:   "Query",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"helloQuery.{{ args.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					ProviderID: "default",
					FieldName:  "helloMutation",
					Type:       EventTypePublish,
					TypeName:   "Mutation",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"helloMutation.{{ args.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					ProviderID: "default",
					FieldName:  "helloSubscription",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"helloSubscription.{{ args.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					ProviderID: "default",
					FieldName:  "subscriptionWithMultipleSubjects",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"firstSubscription.{{ args.firstId }}", "secondSubscription.{{ args.secondId }}"},
				},
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
							Input: `{"subject":"helloQuery.$$0$$", "data": {"id":$$0$$}, "providerId":"default"}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
								},
							},
							DataSource: &NatsRequestDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloQuery"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.NatsRequestDataSource"),
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
							Input: `{"subject":"helloMutation.$$0$$", "data": {"id":$$0$$,"input":$$1$$}, "providerId":"default"}`,
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
							DataSource: &NatsPublishDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloMutation"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.NatsPublishDataSource"),
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
					Input: []byte(`{"providerId":"default","subjects":["helloSubscription.$$0$$"]}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string"]}`),
						},
					},
					Source: &NatsSubscriptionSource{
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
					Input: []byte(`{"providerId":"default","subjects":["firstSubscription.$$0$$","secondSubscription.$$1$$"]}`),
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
					Source: &NatsSubscriptionSource{
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
