package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/pkg/starwars"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/subscriptiontesting"
)

type handlerRoutine func(ctx context.Context) func() bool

type websocketHook struct {
	called bool
	reqCtx context.Context
	hook   func(reqCtx context.Context, operation *graphql.Request) error
}

func (w *websocketHook) OnBeforeStart(reqCtx context.Context, operation *graphql.Request) error {
	w.called = true
	if w.hook != nil {
		return w.hook(reqCtx, operation)
	}
	return nil
}

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

				messagesFromServer := client.readFromServer()
				assert.Len(t, messagesFromServer, 1)
				assert.Equal(t, "1", messagesFromServer[0].Id)
				assert.Equal(t, MessageTypeError, messagesFromServer[0].Type)
				assert.Equal(t, `[{"message":"document doesn't contain any executable operation"}]`, string(messagesFromServer[0].Payload))
			})

			cancelFunc()
		})

		t.Run("non-subscription query", func(t *testing.T) {

			t.Run("should process query and return error when query is not valid", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
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

				messagesFromServer := client.readFromServer()
				assert.Len(t, messagesFromServer, 1)
				assert.Equal(t, "1", messagesFromServer[0].Id)
				assert.Equal(t, MessageTypeError, messagesFromServer[0].Type)
				assert.Equal(t, `[{"message":"field: invalid not defined on type: Character","path":["query","hero","invalid"]}]`, string(messagesFromServer[0].Payload))
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should process and send result for a query", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
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

		chatServer := httptest.NewServer(subscriptiontesting.ChatGraphQLEndpointHandler())
		defer chatServer.Close()

		t.Run("connection_init", func(t *testing.T) {
			var initPayloadAuthorization string

			executorPool, _ := setupEngineV2(t, ctx, chatServer.URL)
			_, client, handlerRoutine := setupSubscriptionHandlerWithInitFuncTest(t, executorPool, func(ctx context.Context, initPayload InitPayload) (context.Context, error) {
				if initPayloadAuthorization == "" {
					return ctx, nil
				}

				if initPayloadAuthorization != initPayload.Authorization() {
					return nil, fmt.Errorf("unknown user: %s", initPayload.Authorization())
				}

				return ctx, nil
			})

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

			t.Run("should send connection error message when error on check initial payload occurrs", func(t *testing.T) {
				initPayloadAuthorization = "123"
				defer func() { initPayloadAuthorization = "" }()

				client.reconnect().and().prepareConnectionInitMessageWithPayload([]byte(`{"Authorization": "111"}`)).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())

				cancelFunc()
				require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

				expectedMessage := Message{
					Type:    MessageTypeConnectionTerminate,
					Payload: jsonizePayload(t, "failed to accept the websocket connection"),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			t.Run("should successfully init connection and respond with ack when initial payload successfully occurred ", func(t *testing.T) {
				initPayloadAuthorization = "123"
				defer func() { initPayloadAuthorization = "" }()

				client.reconnect().and().prepareConnectionInitMessageWithPayload([]byte(`{"Authorization": "123"}`)).withoutError().and().send()

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
			executorPool, _ := setupEngineV2(t, ctx, chatServer.URL)
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
			executorPool, _ := setupEngineV2(t, ctx, chatServer.URL)
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

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: []byte(`[{"message":"document doesn't contain any executable operation"}]`),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
			})

			cancelFunc()
		})

		t.Run("non-subscription query", func(t *testing.T) {
			executorPool, hookHolder := setupEngineV2(t, ctx, chatServer.URL)

			t.Run("should process query and return error when query is not valid", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.InvalidOperation)
				require.NoError(t, err)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingAMessage := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingAMessage, 1*time.Second, 5*time.Millisecond)

				expectedErrorMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: []byte(`[{"message":"field: serverName not defined on type: Query","path":["query","serverName"]}]`),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedErrorMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should process and send result for a query", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.MutationSendMessage)
				require.NoError(t, err)

				hookHolder.hook = func(ctx context.Context, operation *graphql.Request) error {
					assert.Equal(t, hookHolder.reqCtx, ctx)
					assert.Contains(t, operation.Query, "mutation SendMessage")
					return nil
				}
				defer func() {
					hookHolder.hook = nil
				}()

				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				defer cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingTwoMessages := func() bool {
					return client.hasMoreMessagesThan(1)
				}
				require.Eventually(t, waitForClientHavingTwoMessages, 60*time.Second, 5*time.Millisecond)

				expectedDataMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":{"post":{"text":"Hello World!","createdBy":"myuser"}}}`),
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
				assert.True(t, hookHolder.called)
			})

			t.Run("should process and send error message from hook for a query", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.MutationSendMessage)
				require.NoError(t, err)

				errMsg := "error_on_operation"
				hookHolder.hook = func(ctx context.Context, operation *graphql.Request) error {
					return errors.New(errMsg)
				}
				defer func() {
					hookHolder.hook = nil
				}()

				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				waitForClientHavingTwoMessages := func() bool {
					return client.hasMoreMessagesThan(0)
				}
				require.Eventually(t, waitForClientHavingTwoMessages, 5*time.Second, 5*time.Millisecond)

				jsonErrMessage, err := json.Marshal(graphql.RequestErrors{
					{Message: errMsg},
				})
				require.NoError(t, err)
				expectedErrMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrMessage,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedErrMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
				assert.True(t, hookHolder.called)
			})

		})

		t.Run("subscription query", func(t *testing.T) {
			executorPool, hookHolder := setupEngineV2(t, ctx, chatServer.URL)

			t.Run("should start subscription on start", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.SubscriptionLiveMessages)
				require.NoError(t, err)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				time.Sleep(10 * time.Millisecond)
				defer cancelFunc()

				go sendChatMutation(t, chatServer.URL)

				require.Eventually(t, func() bool {
					return client.hasMoreMessagesThan(0)
				}, 1*time.Second, 10*time.Millisecond)

				expectedMessage := Message{
					Id:      "1",
					Type:    MessageTypeData,
					Payload: []byte(`{"data":{"messageAdded":{"text":"Hello World!","createdBy":"myuser"}}}`),
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedMessage)
				assert.Equal(t, 1, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should fail with validation error for invalid Subscription", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.InvalidSubscriptionLiveMessages)
				require.NoError(t, err)
				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				time.Sleep(10 * time.Millisecond)
				cancelFunc()

				go sendChatMutation(t, chatServer.URL)

				require.Eventually(t, func() bool {
					return client.hasMoreMessagesThan(0)
				}, 1*time.Second, 10*time.Millisecond)

				messagesFromServer := client.readFromServer()
				assert.Len(t, messagesFromServer, 1)
				assert.Equal(t, "1", messagesFromServer[0].Id)
				assert.Equal(t, MessageTypeError, messagesFromServer[0].Type)
				assert.Equal(t, `[{"message":"differing fields for objectName 'a' on (potentially) same type","path":["subscription","messageAdded"]}]`, string(messagesFromServer[0].Payload))
				assert.Equal(t, 1, subscriptionHandler.ActiveSubscriptions())
			})

			t.Run("should stop subscription on stop and send complete message to client", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)
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

			t.Run("should interrupt subscription on start and return error message from hook", func(t *testing.T) {
				subscriptionHandler, client, handlerRoutine := setupSubscriptionHandlerTest(t, executorPool)

				payload, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.SubscriptionLiveMessages)
				require.NoError(t, err)

				errMsg := "sub_interrupted"
				hookHolder.hook = func(ctx context.Context, operation *graphql.Request) error {
					return errors.New(errMsg)
				}

				client.prepareStartMessage("1", payload).withoutError().and().send()

				ctx, cancelFunc := context.WithCancel(context.Background())
				handlerRoutineFunc := handlerRoutine(ctx)
				go handlerRoutineFunc()

				time.Sleep(10 * time.Millisecond)
				cancelFunc()

				go sendChatMutation(t, chatServer.URL)

				require.Eventually(t, func() bool {
					return client.hasMoreMessagesThan(0)
				}, 1*time.Second, 10*time.Millisecond)

				jsonErrMessage, err := json.Marshal(graphql.RequestErrors{
					{Message: errMsg},
				})
				require.NoError(t, err)
				expectedErrMessage := Message{
					Id:      "1",
					Type:    MessageTypeError,
					Payload: jsonErrMessage,
				}

				messagesFromServer := client.readFromServer()
				assert.Contains(t, messagesFromServer, expectedErrMessage)
				assert.Equal(t, 0, subscriptionHandler.ActiveSubscriptions())
				assert.True(t, hookHolder.called)
			})
		})

		t.Run("connection_terminate", func(t *testing.T) {
			executorPool, _ := setupEngineV2(t, ctx, chatServer.URL)
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
			executorPool, _ := setupEngineV2(t, ctx, chatServer.URL)
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

func setupEngineV2(t *testing.T, ctx context.Context, chatServerURL string) (*ExecutorV2Pool, *websocketHook) {
	chatSchemaBytes, err := subscriptiontesting.LoadSchemaFromExamplesDirectoryWithinPkg()
	require.NoError(t, err)

	chatSchema, err := graphql.NewSchemaFromReader(bytes.NewBuffer(chatSchemaBytes))
	require.NoError(t, err)

	engineConf := graphql.NewEngineV2Configuration(chatSchema)
	engineConf.SetDataSources([]plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{TypeName: "Mutation", FieldNames: []string{"post"}},
				{TypeName: "Subscription", FieldNames: []string{"messageAdded"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Message", FieldNames: []string{"text", "createdBy"}},
			},
			Factory: &graphql_datasource.Factory{
				HTTPClient: httpclient.DefaultNetHttpClient,
			},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    chatServerURL,
					Method: http.MethodPost,
					Header: nil,
				},
				Subscription: graphql_datasource.SubscriptionConfiguration{
					URL: chatServerURL,
				},
			}),
		},
	})
	engineConf.SetFieldConfigurations([]plan.FieldConfiguration{
		{
			TypeName:  "Mutation",
			FieldName: "post",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "roomName",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "username",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "text",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Subscription",
			FieldName: "messageAdded",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "roomName",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	})

	hookHolder := &websocketHook{
		reqCtx: context.Background(),
	}
	engineConf.SetWebsocketBeforeStartHook(hookHolder)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:8080", nil)
	require.NoError(t, err)

	req.Header.Set("X-Other-Key", "x-other-value")

	initCtx := NewInitialHttpRequestContext(req)

	engine, err := graphql.NewExecutionEngineV2(initCtx, abstractlogger.NoopLogger, engineConf)
	require.NoError(t, err)

	executorPool := NewExecutorV2Pool(engine, hookHolder.reqCtx)

	return executorPool, hookHolder
}

func setupSubscriptionHandlerTest(t *testing.T, executorPool ExecutorPool) (subscriptionHandler *Handler, client *mockClient, routine handlerRoutine) {
	return setupSubscriptionHandlerWithInitFuncTest(t, executorPool, nil)
}

func setupSubscriptionHandlerWithInitFuncTest(
	t *testing.T,
	executorPool ExecutorPool,
	initFunc WebsocketInitFunc,
) (subscriptionHandler *Handler, client *mockClient, routine handlerRoutine) {
	client = newMockClient()

	var err error
	subscriptionHandler, err = NewHandlerWithInitFunc(abstractlogger.NoopLogger, client, executorPool, initFunc)
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

func sendChatMutation(t *testing.T, url string) {
	reqBody, err := subscriptiontesting.GraphQLRequestForOperation(subscriptiontesting.MutationSendMessage)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	httpClient := http.Client{}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
