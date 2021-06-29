package subscription

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
)

type handlerRoutine func(ctx context.Context) func() bool

func TestHandler_Handle(t *testing.T) {
	starwars.SetRelativePathToStarWarsPackage("../starwars")

	t.Run("engine v1", func(t *testing.T) {
		executorPool := NewExecutorV1Pool(starwars.NewExecutionHandler(t))

		t.Run("connection_init", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should send connection error message when error on read occurrs", func(t *testing.T) {
				client.prepareConnectionInitMessage().withError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				expectedMessage := Message{
					Type:    MessageTypeConnectionError,
					Payload: jsonizePayload(t, "could not read message from client"),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			t.Run("should successfully init connection and respond with ack", func(t *testing.T) {
				client.reconnect().and().prepareConnectionInitMessage().withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				expectedMessage := Message{
					Type: MessageTypeConnectionAck,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})
		})

		t.Run("connection_keep_alive", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should successfully send keep alive messages after connection_init", func(t *testing.T) {
				keepAliveInterval, err := time.ParseDuration("5ms")
				require.NoError(t, err)

				subscriptionHandler.ChangeKeepAliveInterval(keepAliveInterval)

				client.prepareConnectionInitMessage().withoutError().and().send()
				ctx, cancelFunc := context.WithCancel(context.Background())

				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				expectedMessage := Message{
					Type: MessageTypeConnectionKeepAlive,
				}

				messagesFromServer := client.readFromServer()
				waitForKeepAliveMessage := func() bool {
					for len(messagesFromServer) < 2 {
						messagesFromServer = client.readFromServer()
					}
					return true
				}

				assert.Eventually(t, waitForKeepAliveMessage, 1*time.Second, 5*time.Millisecond)
				assert.Contains(t, messagesFromServer, expectedMessage)

				cancelFunc()
			})
		})

		t.Run("erroneous operation(s)", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
			ctx, cancelFunc := context.WithCancel(context.Background())
			handlerRoutineFunc := handlerRoutine(ctx)
			go handlerRoutineFunc()

			t.Run("should send error when query contains syntax errors", func(t *testing.T) {
				payload := []byte(`{"operationName": "Broken", "query Broken {": "", "variables": null}`)
				client.prepareStartMessage("1", payload).withoutError().send()

				waitForClientHavingAMessage := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingAMessage, 5*time.Second, 5*time.Millisecond)

				jsonErrorMsg, err := json.Marshal("document doesn't contain any executable operation, locations: [], path: []")
				require.NoError(t, err)

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrorMsg,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			cancelFunc()
		})

		t.Run("non-subscription query", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should process query and return error when query is not valid", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileInvalidQuery, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingAMessage := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingAMessage, 1*time.Second, 5*time.Millisecond)

				jsonErrMessage, err := json.Marshal("field: invalid not defined on type: Character, locations: [], path: [query,hero,invalid]")
				require.NoError(t, err)
				expectedErrorMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrMessage,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedErrorMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should process and send result for a query", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingTwoMessages := func() bool {
					return client.hasMoreMessagesThan(1)
				}
				require.Eventually(t, waitForClientHavingTwoMessages, 60*time.Second, 5*time.Millisecond)

				expectedDataMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":null}`),
				}

				expectedCompleteMessage := Message{
					Id:      "1",
					Type:    MessageTypeComplete,
					Payload: nil,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedDataMessage)
				assert.Contains(t, messagesFromServer, expectedCompleteMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})
		})

		t.Run("subscription query", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should start subscription on start", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileRemainingJedisSubscription, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				time.Sleep(10 * time.Millisecond)
				cancelFunc()

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":null}`),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
				assert.Equal(t, 1, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should stop subscription on stop and send complete message to client", func(t *testing.T) {
				client.reconnect().prepareStopMessage("1").withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForCanceledSubscription := func() bool {
					for subscriptionHandler.ActiveSubscriptions() > 0 {
					}
					return true
				}

				assert.Eventually(t, waitForCanceledSubscription, 1*time.Second, 5*time.Millisecond)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeComplete,
					Payload: nil,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)

				cancelFunc()
			})
		})

		t.Run("connection_terminate", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should successfully disconnect from client", func(t *testing.T) {
				client.prepareConnectionTerminateMessage().withoutError().and().send()
				require.True(t, client.connected)

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				assert.False(t, client.connected)
			})
		})

		t.Run("client is disconnected", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("server should not read from client and stop handler", func(t *testing.T) {
				err := client.Disconnect()
				require.NoError(t, err)
				require.False(t, client.connected)

				client.prepareConnectionInitMessage().withoutError()
				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				assert.False(t, client.serverHasRead)
			})
		})
	})

	t.Run("engine v2", func(t *testing.T) {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		starWarsSchema, err := graphql.NewSchemaFromString(string(starwars.Schema(t)))
		require.NoError(t, err)

		engineConf := graphql.NewEngineV2Configuration(starWarsSchema)
		engineConf.SetDataSources([]plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{TypeName: "Subscription", FieldNames: []string{"remainingJedis"}},
				},
				Factory: &graphql_datasource.Factory{},
				Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Subscription: graphql_datasource.SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
			},
		})
		engine, err := graphql.NewExecutionEngineV2(ctx, abstractlogger.NoopLogger, engineConf)
		require.NoError(t, err)

		streamStub := subscription.NewStreamStub([]byte("graphql_websocket_subscription"), ctx.Done())

		websocketManager := subscription.NewManager(streamStub)
		websocketManager.Run(ctx.Done())

		executorPool := NewExecutorV2Pool(engine)
		t.Run("connection_init", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should send connection error message when error on read occurrs", func(t *testing.T) {
				client.prepareConnectionInitMessage().withError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				expectedMessage := Message{
					Type:    MessageTypeConnectionError,
					Payload: jsonizePayload(t, "could not read message from client"),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			t.Run("should successfully init connection and respond with ack", func(t *testing.T) {
				client.reconnect().and().prepareConnectionInitMessage().withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				expectedMessage := Message{
					Type: MessageTypeConnectionAck,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})
		})

		t.Run("connection_keep_alive", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should successfully send keep alive messages after connection_init", func(t *testing.T) {
				keepAliveInterval, err := time.ParseDuration("5ms")
				require.NoError(t, err)

				subscriptionHandler.ChangeKeepAliveInterval(keepAliveInterval)

				client.prepareConnectionInitMessage().withoutError().and().send()
				ctx, cancelFunc := context.WithCancel(context.Background())

				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				expectedMessage := Message{
					Type: MessageTypeConnectionKeepAlive,
				}

				messagesFromServer := client.readFromServer()
				waitForKeepAliveMessage := func() bool {
					for len(messagesFromServer) < 2 {
						messagesFromServer = client.readFromServer()
					}
					return true
				}

				assert.Eventually(t, waitForKeepAliveMessage, 1*time.Second, 5*time.Millisecond)
				assert.Contains(t, messagesFromServer, expectedMessage)

				cancelFunc()
			})
		})

		t.Run("erroneous operation(s)", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
			ctx, cancelFunc := context.WithCancel(context.Background())
			handlerRoutineFunc := handlerRoutine(ctx)
			go handlerRoutineFunc()

			t.Run("should send error when query contains syntax errors", func(t *testing.T) {
				payload := []byte(`{"operationName": "Broken", "query Broken {": "", "variables": null}`)
				client.prepareStartMessage("1", payload).withoutError().send()

				waitForClientHavingAMessage := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingAMessage, 5*time.Second, 5*time.Millisecond)

				jsonErrorMsg, err := json.Marshal("external: cannot find an operation with name: Broken, locations: [], path: []")
				require.NoError(t, err)

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrorMsg,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			cancelFunc()
		})

		t.Run("non-subscription query", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should process query and return error when query is not valid", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileInvalidQuery, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingAMessage := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingAMessage, 1*time.Second, 5*time.Millisecond)

				jsonErrMessage, err := json.Marshal("field: invalid not defined on type: Character, locations: [], path: [query,hero,invalid]")
				require.NoError(t, err)
				expectedErrorMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrMessage,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedErrorMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should process and send result for a query", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingTwoMessages := func() bool {
					return client.hasMoreMessagesThan(1)
				}
				require.Eventually(t, waitForClientHavingTwoMessages, 60*time.Second, 5*time.Millisecond)

				expectedDataMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":{"hero":null}}`),
				}

				expectedCompleteMessage := Message{
					Id:      "1",
					Type:    MessageTypeComplete,
					Payload: nil,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedDataMessage)
				assert.Contains(t, messagesFromServer, expectedCompleteMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})
		})

		t.Run("subscription query", func(t *testing.T) {
			subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should start subscription on start", func(t *testing.T) {
				payload := starwars.LoadQuery(t, starwars.FileRemainingJedisSubscription, nil)
				client.prepareStartMessage("1", payload).withoutError().and().send()
				go streamStub.SendMessage(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`, []byte(`{"remainingJedis":1}`))

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				time.Sleep(10 * time.Millisecond)
				cancelFunc()

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":{"remainingJedis":1}}`),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
				assert.Equal(t, 1, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should stop subscription on stop and send complete message to client", func(t *testing.T) {
				client.reconnect().prepareStopMessage("1").withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForCanceledSubscription := func() bool {
					for subscriptionHandler.ActiveSubscriptions() > 0 {
					}
					return true
				}

				assert.Eventually(t, waitForCanceledSubscription, 1*time.Second, 5*time.Millisecond)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeComplete,
					Payload: nil,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)

				cancelFunc()
			})
		})

		t.Run("connection_terminate", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("should successfully disconnect from client", func(t *testing.T) {
				client.prepareConnectionTerminateMessage().withoutError().and().send()
				require.True(t, client.connected)

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				assert.False(t, client.connected)
			})
		})

		t.Run("client is disconnected", func(t *testing.T) {
			_, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

			t.Run("server should not read from client and stop handler", func(t *testing.T) {
				err := client.Disconnect()
				require.NoError(t, err)
				require.False(t, client.connected)

				client.prepareConnectionInitMessage().withoutError()
				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				assert.False(t, client.serverHasRead)
			})
		})
	})

}

func setupSubscriptionHandlerTest(t *testing.T, executorPool ExecutorPool) (subscriptionHandler *Handler, client *mockClient, routine handlerRoutine) {
	client = newMockClient()

	var err error
	subscriptionHandler, err = NewHandler(abstractlogger.NoopLogger, client, executorPool)
	require.NoError(t, err)

	routine = func(ctx context.Context) func() bool {
		return func() bool {
			subscriptionHandler.Handle(ctx)
			return true
		}
	}

	return subscriptionHandler, client, routine
}

func jsonizePayload(t *testing.T, payload interface{}) json.RawMessage {
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	return jsonBytes
}
