package kafka_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDefinition = `
schema {
    subscription: Subscription
}

type Subscription {
    remainingJedis: Int!
}
`

type runTestOnTestDefinitionOptions func(planConfig *plan.Configuration, extraChecks *[]datasourcetesting.CheckFunc)

func runTestOnTestDefinition(operation, operationName string, expectedPlan plan.Plan, options ...runTestOnTestDefinitionOptions) func(t *testing.T) {
	extraChecks := make([]datasourcetesting.CheckFunc, 0)
	config := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"remainingJedis"},
					},
				},
				Custom: ConfigJSON(Configuration{
					Subscription: SubscriptionConfiguration{
						BrokerAddr: "localhost:9092",
						Topic:      "test.topic",
						GroupID:    "test.consumer.group",
						ClientID:   "test.client.id",
					},
				}),
				Factory: &Factory{},
			},
		},
	}

	for _, opt := range options {
		opt(&config, &extraChecks)
	}

	return datasourcetesting.RunTest(testDefinition, operation, operationName, expectedPlan, config, extraChecks...)
}

func testWithFactory(factory *Factory) runTestOnTestDefinitionOptions {
	return func(planConfig *plan.Configuration, extraChecks *[]datasourcetesting.CheckFunc) {
		for _, ds := range planConfig.DataSources {
			ds.Factory = factory
		}
	}
}

func TestKafkaDataSource(t *testing.T) {
	factory := &Factory{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("subscription", runTestOnTestDefinition(`
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"broker_addr":"localhost:9092","topic":"test.topic","group_id":"test.consumer.group","client_id":"test.client.id"}`),
				Source: &SubscriptionSource{
					client: NewKafkaConsumerGroupBridge(ctx, logger()),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("remainingJedis"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
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

	t.Run("subscription with variables", datasourcetesting.RunTest(`
		type Subscription {
			foo(bar: String): Int!
 		}
`, `
		subscription SubscriptionWithVariables($bar: String) {
			foo(bar: $bar)
		}
	`, "SubscriptionWithVariables", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"broker_addr":"localhost:9092","topic":"test.topic.$$0$$","group_id":"test.consumer.group","client_id":"test.client.id"}`),
				Variables: resolve.NewVariables(
					&resolve.ContextVariable{
						Path:     []string{"bar"},
						Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
					},
				),
				Source: &SubscriptionSource{
					client: NewKafkaConsumerGroupBridge(ctx, logger()),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"foo"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"foo"},
					},
				},
				Custom: ConfigJSON(Configuration{
					Subscription: SubscriptionConfiguration{
						BrokerAddr: "localhost:9092",
						Topic:      "test.topic.{{.arguments.bar}}",
						GroupID:    "test.consumer.group",
						ClientID:   "test.client.id",
					},
				}),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Subscription",
				FieldName: "foo",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "bar",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}))
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f FailingSubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	return errSubscriptionClientFail
}

func TestKafkaDataSource_Subscription_Start(t *testing.T) {

	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		subscriptionSource := SubscriptionSource{client: NewKafkaConsumerGroupBridge(ctx, abstractlogger.NoopLogger)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"broker_addr":"",topic":"","group_id":""}`), nil)
		assert.Error(t, err)
	})

	t.Run("should send and receive a message, then cancel subscription", func(t *testing.T) {
		next := make(chan []byte)
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		topic := "graphql-go-tools.test.topic"
		groupID := "graphql-go-tools.test.groupid"
		source := newSubscriptionSource(resolverLifecycle)

		fr := &sarama.FetchResponse{Version: 11}
		mockBroker := newMockKafkaBroker(t, topic, groupID, fr)
		defer mockBroker.Close()

		options := GraphQLSubscriptionOptions{
			BrokerAddr: mockBroker.Addr(),
			Topic:      topic,
			GroupID:    groupID,
			ClientID:   "graphql-go-tools.test.groupid",
		}
		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)
		err = source.Start(subscriptionLifecycle, optionsBytes, next)
		require.NoError(t, err)

		testMessageKey := sarama.StringEncoder("test.message.key")
		testMessageValue := sarama.StringEncoder(`{"stock":[{"name":"Trilby","price":293,"inStock":2}]}`)

		// Add a message to the topic. The consumer group will fetch that message and trigger ConsumeClaim method.
		fr.AddMessage(topic, defaultPartition, testMessageKey, testMessageValue, 0)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"stock":[{"name":"Trilby","price":293,"inStock":2}]}}`, string(nextBytes))

		cancelSubscription()
		_, ok := <-next
		assert.False(t, ok)
	})
}
