package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

func TestGraphQLWSMessageReader_Read(t *testing.T) {
	data := []byte(`{ "id": "1", "type": "connection_init", "payload": { "headers": { "key": "value" } } }`)
	expectedMessage := &GraphQLWSMessage{
		Id:      "1",
		Type:    "connection_init",
		Payload: json.RawMessage(`{ "headers": { "key": "value" } }`),
	}

	reader := GraphQLWSMessageReader{
		logger: abstractlogger.Noop{},
	}
	message, err := reader.Read(data)
	assert.NoError(t, err)
	assert.Equal(t, expectedMessage, message)
}

func TestGraphQLWSMessageWriter_WriteData(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteData("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write message data to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"data","payload":{"data":{"hello":"world"}}}`)
		err := writer.WriteData("1", []byte(`{"data":{"hello":"world"}}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteComplete(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteComplete("1")
		assert.Error(t, err)
	})
	t.Run("should successfully write complete message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"complete"}`)
		err := writer.WriteComplete("1")
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteKeepAlive(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteKeepAlive()
		assert.Error(t, err)
	})
	t.Run("should successfully write keep-alive (ka) message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"ka"}`)
		err := writer.WriteKeepAlive()
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteTerminate(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteTerminate(`failed to accept the websocket connection`)
		assert.Error(t, err)
	})
	t.Run("should successfully write terminate message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_terminate","payload":"failed to accept the websocket connection"}`)
		err := writer.WriteTerminate(`failed to accept the websocket connection`)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteConnectionError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteConnectionError(`could not read message from client`)
		assert.Error(t, err)
	})
	t.Run("should successfully write connection error message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_error","payload":"could not read message from client"}`)
		err := writer.WriteConnectionError(`could not read message from client`)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		requestErrors := graphql.RequestErrorsFromError(errors.New("request error"))
		err := writer.WriteError("1", requestErrors)
		assert.Error(t, err)
	})
	t.Run("should successfully write error message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"request error"}]}`)
		requestErrors := graphql.RequestErrorsFromError(errors.New("request error"))
		err := writer.WriteError("1", requestErrors)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSMessageWriter_WriteAck(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteAck()
		assert.Error(t, err)
	})
	t.Run("should successfully write ack message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		err := writer.WriteAck()
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLWSWriteEventHandler_Emit(t *testing.T) {
	t.Run("should write on completed", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.Emit(subscription.EventTypeOnSubscriptionCompleted, "1", nil, nil)
		expectedMessage := []byte(`{"id":"1","type":"complete"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on data", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.Emit(subscription.EventTypeOnSubscriptionData, "1", []byte(`{ "data": { "hello": "world" } }`), nil)
		expectedMessage := []byte(`{"id":"1","type":"data","payload":{"data":{"hello":"world"}}}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on error", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.Emit(subscription.EventTypeOnError, "1", nil, errors.New("error occurred"))
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"error occurred"}]}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on duplicated subscriber id", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.Emit(subscription.EventTypeOnDuplicatedSubscriberID, "1", nil, subscription.ErrSubscriberIDAlreadyExists)
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"subscriber id already exists"}]}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on connection_error", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.Emit(subscription.EventTypeOnConnectionError, "", nil, errors.New("connection error occurred"))
		expectedMessage := []byte(`{"type":"connection_error","payload":"connection error occurred"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on non-subscription execution result", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		go func() {
			writeEventHandler.Emit(subscription.EventTypeOnNonSubscriptionExecutionResult, "1", []byte(`{ "data": { "hello": "world" } }`), nil)
		}()

		assert.Eventually(t, func() bool {
			expectedDataMessage := []byte(`{"id":"1","type":"data","payload":{"data":{"hello":"world"}}}`)
			actualDataMessage := testClient.readMessageToClient()
			assert.Equal(t, expectedDataMessage, actualDataMessage)
			expectedCompleteMessage := []byte(`{"id":"1","type":"complete"}`)
			actualCompleteMessage := testClient.readMessageToClient()
			assert.Equal(t, expectedCompleteMessage, actualCompleteMessage)
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})
}

func TestGraphQLWSWriteEventHandler_HandleWriteEvent(t *testing.T) {
	t.Run("should write keep_alive", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionKeepAlive, "", nil, nil)
		expectedMessage := []byte(`{"type":"ka"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write ack", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLWSWriteEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionAck, "", nil, nil)
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestProtocolGraphQLWSHandler_Handle(t *testing.T) {
	t.Run("should return connection_error when an unexpected message type is used", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		expectedMessage := []byte(`{"type":"connection_error","payload":"unexpected message type: something"}`)
		err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"something"}`))
		assert.NoError(t, err)
		assert.Equal(t, testClient.readMessageToClient(), expectedMessage)
	})

	t.Run("should terminate connections on connection_terminate from client", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)
		mockEngine.EXPECT().TerminateAllSubscriptions(gomock.Eq(protocol.EventHandler()))

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_terminate"}`))
		assert.NoError(t, err)
	})

	t.Run("should init connection and respond with ack and ka", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)
		protocol.keepAliveInterval = 5 * time.Millisecond

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		ctx, cancelFunc := context.WithCancel(context.Background())

		assert.Eventually(t, func() bool {
			expectedMessageAck := []byte(`{"type":"connection_ack"}`)
			expectedMessageKeepAlive := []byte(`{"type":"ka"}`)
			err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_init"}`))
			assert.NoError(t, err)
			assert.Equal(t, expectedMessageAck, testClient.readMessageToClient())

			time.Sleep(8 * time.Millisecond)
			assert.Equal(t, expectedMessageKeepAlive, testClient.readMessageToClient())
			cancelFunc()

			return true
		}, 1*time.Second, 5*time.Millisecond)

	})

	t.Run("should start an operation on start from client", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)
		mockEngine.EXPECT().StartOperation(gomock.Eq(ctx), "1", []byte(`{"query":"{ hello }"}`), gomock.Eq(protocol.EventHandler()))

		err := protocol.Handle(ctx, mockEngine, []byte(`{"id":"1","type":"start","payload":{"query":"{ hello }"}}`))
		assert.NoError(t, err)
	})

	t.Run("should stop a subscription on stop from client", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)
		mockEngine.EXPECT().StopSubscription("1", gomock.Eq(protocol.EventHandler()))

		err := protocol.Handle(ctx, mockEngine, []byte(`{"id":"1","type":"stop"}`))
		assert.NoError(t, err)
	})

	t.Run("should not panic on broken input", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_init","payload":{something}}`))
		assert.NoError(t, err)

		expectedMessage := []byte(`{"type":"error","payload":[{"message":"json syntax error"}]}`)
		actualMessage := testClient.readMessageToClient()
		assert.Equal(t, expectedMessage, actualMessage)
	})
}

func NewTestGraphQLWSWriteEventHandler(testClient subscription.TransportClient) GraphQLWSWriteEventHandler {
	return GraphQLWSWriteEventHandler{
		logger: abstractlogger.Noop{},
		Writer: GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			mu:     &sync.Mutex{},
			Client: testClient,
		},
	}
}

func NewTestProtocolGraphQLWSHandler(testClient subscription.TransportClient) *ProtocolGraphQLWSHandler {
	return &ProtocolGraphQLWSHandler{
		logger: abstractlogger.Noop{},
		reader: GraphQLWSMessageReader{
			logger: abstractlogger.Noop{},
		},
		writeEventHandler: NewTestGraphQLWSWriteEventHandler(testClient),
		keepAliveInterval: 30,
	}
}
